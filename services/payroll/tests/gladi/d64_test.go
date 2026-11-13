// Package gladi holds rehearsal tests that run against a COPY of the dev database (D-64), never against dev or production.
package gladi

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"sort"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib" // driver
	"github.com/shopspring/decimal"

	"github.com/mutugading/goapps-backend/services/payroll/internal/application/run"
	"github.com/mutugading/goapps-backend/services/payroll/internal/domain/scope"
	"github.com/mutugading/goapps-backend/services/payroll/internal/infrastructure/audit"
	"github.com/mutugading/goapps-backend/services/payroll/internal/infrastructure/etl"
	"github.com/mutugading/goapps-backend/services/payroll/internal/infrastructure/postgres"
)

// T029b (D-64): 202611 (skipped when real) → 202612 → 202701 per division in payroll_d64, rule set 1 v1, then invariants:
//   - non-PPh lines of 202611/202612 = engine 202610 per NIK (synthetic attendance = 202610);
//   - P1 202612 = 06§6.2/D-29 recomputed from an independent SQL over Jan–Nov;
//   - P1 202701 = TER(gross) without YTD;
//   - T15/P15 202701 not NULL and based on JAN_BASE (T901 dummy = TETAP of 202612).
//
// Env: GLADI_D64_DSN (payroll_migrator/owner), GLADI_D64_APP_DSN (payroll_app), GLADI_D64_ETL_DSN (payroll_etl), all on
// database payroll_d64. Optional GLADI_D64_OUT (report folder, default /tmp/t029b).

const (
	dbName    = "payroll_d64"
	company   = int16(3)
	srcYYMM   = 202610
	ruleSetID = 1
)

var (
	divisions = []string{"SOLO", "JAKARTA"}
	targets   = []int{202611, 202612, 202701}
)

type env struct {
	own, app, etl *sql.DB
	out           string
}

func openEnv(t *testing.T) env {
	t.Helper()
	dsn := os.Getenv("GLADI_D64_DSN")
	if dsn == "" {
		t.Skip("GLADI_D64_DSN kosong: gladi D-64 dilewati")
	}
	e := env{out: os.Getenv("GLADI_D64_OUT")}
	if e.out == "" {
		e.out = "/tmp/t029b"
	}
	for _, c := range []struct {
		db  **sql.DB
		dsn string
		env string
	}{{&e.own, dsn, "GLADI_D64_DSN"}, {&e.app, os.Getenv("GLADI_D64_APP_DSN"), "GLADI_D64_APP_DSN"}, {&e.etl, os.Getenv("GLADI_D64_ETL_DSN"), "GLADI_D64_ETL_DSN"}} {
		if c.dsn == "" {
			t.Fatalf("%s wajib diisi", c.env)
		}
		db, err := sql.Open("pgx", c.dsn)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = db.Close() })
		var name string
		if err := db.QueryRow(`SELECT current_database()`).Scan(&name); err != nil {
			t.Fatalf("%s: %v", c.env, err)
		}
		if name != dbName { // D-64 (5): never dev or production
			t.Fatalf("%s menunjuk DB %q, wajib %q", c.env, name, dbName)
		}
		var local bool // the copy lives in the local dev container only (inet_server_addr NULL = unix socket)
		if err := db.QueryRow(`SELECT COALESCE(host(inet_server_addr()) IN ('127.0.0.1','::1') OR inet_server_addr() << '172.16.0.0/12'::inet, true)`).Scan(&local); err != nil {
			t.Fatalf("%s: %v", c.env, err)
		}
		if !local {
			t.Fatalf("%s bukan server lokal (container dev): gladi D-64 ditolak", c.env)
		}
		*c.db = db
	}
	return e
}

func scoped() context.Context {
	var ss []scope.Scope
	for _, d := range divisions {
		s, err := scope.New(company, d)
		if err != nil {
			panic(err)
		}
		ss = append(ss, s)
	}
	return scope.WithIdentity(context.Background(), scope.Identity{UserID: "gladi-d64", Scopes: ss})
}

func rules(ctx context.Context, tx *sql.Tx, q run.Query, id int) (run.Rules, error) {
	rr, err := postgres.NewRuleSetLoader().Load(ctx, tx, postgres.RuleQuery{RuleSetID: id, CompanyID: q.CompanyID, Division: q.Division, YYMM: q.YYMM})
	if postgres.IsRuleError(err) {
		return run.Rules{}, fmt.Errorf("%w: %w", run.ErrRuleIncomplete, err)
	}
	if err != nil {
		return run.Rules{}, err
	}
	return run.Rules{Set: rr.Rules, Tax: rr.Tax, Subsidy: rr.Subsidy}, nil
}

// lines is amount per NIK per component code (T015, P001…); nil = NULL amount.
type lines map[string]map[string]*decimal.Decimal

func TestD64(t *testing.T) {
	e := openEnv(t)
	ctx := context.Background()
	rep := newReport()

	// D-64 (4) amendment (user, T029b): the copy carries legacy T901 rows (non-dummy) that block dummy-janbase; only here they go.
	res, err := e.own.Exec(`DELETE FROM payroll.pyr_employee_component WHERE pec_company_id=$1 AND updated_by IS DISTINCT FROM $2
  AND pec_component_sysid IN (SELECT pc_sysid FROM payroll.pyr_component WHERE pc_company_id=$1 AND pc_code='T901')`, company, etl.DummyMarker)
	if err != nil {
		t.Fatal(err)
	}
	if n, _ := res.RowsAffected(); n > 0 { //nolint:errcheck // count only
		rep.addf("T901 legacy dihapus di %s: %d baris", dbName, n)
		if _, err := e.own.Exec(`INSERT INTO payroll.pyr_audit_log (pal_actor, pal_action, pal_entity, pal_entity_key, pal_company_id, pal_after)
VALUES ('gladi-d64','GLADI_D64_T901_PURGE','dummy_d64','T901',$1,jsonb_build_object('rows',$2::int))`, company, n); err != nil {
			t.Fatal(err)
		}
	}

	base := map[string]lines{}
	for _, div := range divisions {
		base[div] = dryLines(t, e, div, srcYYMM)
		rep.addf("baseline engine %d %s: %d NIK", srcYYMM, div, len(base[div]))
	}

	runs := map[int]map[string]lines{}
	for _, yymm := range targets {
		if yymm == 202701 {
			jb, err := etl.RunDummyJanBase(ctx, e.etl, company, yymm)
			if err != nil {
				t.Fatalf("dummy-janbase: %v", err)
			}
			rep.addf("dummy-janbase %d: T901 ditulis %d (sumber %d)", yymm, jb.Written, jb.Source)
		}
		runs[yymm] = map[string]lines{}
		for _, div := range divisions {
			prepare(t, e, rep, div, yymm)
			cr, err := run.NewService(postgres.NewDBFromSQL(e.app), rules, audit.NewWriter()).Commit(scoped(),
				run.CommitCommand{CompanyID: company, Division: div, YYMM: yymm, RuleSetID: ruleSetID})
			if err != nil {
				t.Fatalf("commit %d %s: %v", yymm, div, err)
			}
			rep.addf("commit %d %s: run %d, %d NIK, %d baris, T %s, P %s, %d warning", yymm, div, cr.RunID, cr.Summary.EmpCount,
				cr.Summary.LineCount, cr.Summary.TotalT.StringFixed(0), cr.Summary.TotalP.StringFixed(0), len(cr.Warnings))
			rep.warnings(yymm, div, cr.Warnings)
			runs[yymm][div] = runLines(t, e, cr.RunID)
		}
	}

	for _, div := range divisions {
		checkSame(t, rep, div, 202611, base[div], runs[202611][div])
		checkSame(t, rep, div, 202612, base[div], runs[202612][div])
		checkDecember(t, e, rep, div, runs[202612][div])
		checkJanuary(t, e, rep, div, runs[202612][div], runs[202701][div])
	}
	rep.write(t, e.out)
}

// prepare clones 202610 into yymm unless the period already has a real snapshot (D-64 (1)).
func prepare(t *testing.T, e env, rep *report, div string, yymm int) {
	t.Helper()
	var real int
	if err := e.own.QueryRow(`SELECT count(*) FROM payroll.pyr_att_snapshot WHERE pas_company_id=$1 AND pas_yymm=$2 AND pas_division=$3
  AND created_by IS DISTINCT FROM $4`, company, yymm, div, etl.DummyMarker).Scan(&real); err != nil {
		t.Fatal(err)
	}
	if real > 0 {
		rep.addf("%d %s: snapshot riil ada, clone dilewati", yymm, div)
		return
	}
	d, err := etl.RunDummyPeriod(context.Background(), e.etl, etl.DummyPeriodQuery{Company: company, From: srcYYMM, To: yymm, Division: div})
	if err != nil {
		t.Fatalf("dummy-period %d %s: %v", yymm, div, err)
	}
	rep.addf("dummy-period %d %s: snapshot %d, baris %d, addition %d, attd_days %d, sudah ada %v", yymm, div, d.SnapshotID, d.Rows,
		d.Additions, d.AttdDays, d.Unchanged)
}

// dryLines calculates yymm in memory (nothing persisted) with the same loader and runner as Commit.
func dryLines(t *testing.T, e env, div string, yymm int) lines {
	t.Helper()
	out := lines{}
	q := run.Query{CompanyID: company, Division: div, YYMM: yymm}
	ctx := scoped()
	err := postgres.NewDBFromSQL(e.app).ScopedTxOpts(ctx, run.SnapshotTx, func(tx *sql.Tx) error {
		r, err := rules(ctx, tx, q, ruleSetID)
		if err != nil {
			return err
		}
		loaded, err := run.NewLoader().Load(ctx, tx, q, r)
		if err != nil {
			return err
		}
		res, err := run.NewRunner().DryRun(ctx, loaded)
		if err != nil {
			return err
		}
		for _, r := range res.Results {
			m := map[string]*decimal.Decimal{}
			for _, l := range r.Lines {
				m[l.Key.String()] = l.Amount
			}
			out[r.NIK] = m
		}
		return nil
	})
	if err != nil {
		t.Fatalf("dry %d %s: %v", yymm, div, err)
	}
	return out
}

func runLines(t *testing.T, e env, runID int64) lines {
	t.Helper()
	rows, err := e.own.Query(`SELECT psd_nik, CASE psd_type WHEN 'Tunjangan' THEN 'T' ELSE 'P' END || lpad(psd_id::text, 3, '0'), psd_amount::text
FROM payroll.pyr_salary_detail WHERE psd_run_id = $1`, runID)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	out := lines{}
	for rows.Next() {
		var nik, code string
		var amt sql.NullString
		if err := rows.Scan(&nik, &code, &amt); err != nil {
			t.Fatal(err)
		}
		if out[nik] == nil {
			out[nik] = map[string]*decimal.Decimal{}
		}
		var p *decimal.Decimal
		if amt.Valid {
			d := decimal.RequireFromString(amt.String)
			p = &d
		}
		out[nik][code] = p
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

// pphCodes are recomputed every month (P1 TER/annual, P0 subsidy on P1, P5 company PPh): excluded from the sameness check.
var pphCodes = map[string]bool{"P000": true, "P001": true, "P005": true}

// checkSame: synthetic attendance = 202610, so every non-PPh line equals the engine 202610 line per NIK.
func checkSame(t *testing.T, rep *report, div string, yymm int, base, got lines) {
	t.Helper()
	var diffNIK, onlyBase, onlyRun, p1Same, p1Diff int
	for _, nik := range sortedKeys(got) {
		b, ok := base[nik]
		if !ok {
			onlyRun++
			continue
		}
		if d := diffLines(b, got[nik]); len(d) > 0 {
			diffNIK++
			rep.detailf("SAME %d %s %s: %v", yymm, div, nik, d)
		}
		if eqAmt(b["P001"], got[nik]["P001"]) {
			p1Same++
		} else {
			p1Diff++
		}
	}
	for nik := range base {
		if _, ok := got[nik]; !ok {
			onlyBase++
		}
	}
	rep.addf("invarian non-PPh %d %s = engine %d: NIK beda %d, hanya baseline %d, hanya run %d; P1 sama %d / beda %d", yymm, div, srcYYMM,
		diffNIK, onlyBase, onlyRun, p1Same, p1Diff)
	if diffNIK > 0 || onlyBase > 0 || onlyRun > 0 {
		t.Errorf("%d %s: non-PPh tidak sama dengan %d (NIK beda %d, hanya baseline %d, hanya run %d); lihat laporan", yymm, div, srcYYMM,
			diffNIK, onlyBase, onlyRun)
	}
	if yymm == 202611 && p1Diff > 0 {
		t.Errorf("%d %s: %d NIK P1 TER beda dari %d padahal gross sama", yymm, div, p1Diff, srcYYMM)
	}
}

func diffLines(a, b map[string]*decimal.Decimal) []string {
	var out []string
	for _, k := range unionKeys(a, b) {
		if pphCodes[k] {
			continue
		}
		x, okA := a[k]
		y, okB := b[k]
		if okA != okB || !eqAmt(x, y) {
			out = append(out, fmt.Sprintf("%s %s→%s", k, fmtAmt(x, okA), fmtAmt(y, okB)))
		}
	}
	return out
}

func eqAmt(a, b *decimal.Decimal) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return a.Equal(*b)
}

func fmtAmt(a *decimal.Decimal, ok bool) string {
	switch {
	case !ok:
		return "-"
	case a == nil:
		return "NULL"
	default:
		return a.String()
	}
}

func unionKeys(a, b map[string]*decimal.Decimal) []string {
	m := map[string]bool{}
	for k := range a {
		m[k] = true
	}
	for k := range b {
		m[k] = true
	}
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
