package etl

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// TableGlSetup is the legacy GL account setup (11§4.1, 1.338 rows, no PK).
const TableGlSetup = "MGT_PAYROLL_SETUP"

// glValidFrom is the start of history for migrated GL setup rows (11§4.5 step 6).
const glValidFrom = 201712

// Legacy MPS_JENIS values and derived line kinds.
const (
	glJenisBalance = "EMPLOYEE BALANCE"
	glKindComp     = "COMPONENT"
	glKindPayable  = "PAYABLE"
)

// glPayableJenis are the legacy MPS_JENIS of the per-group payable line (D-63 butir 1): SALARY001 sums
// PM_TOT_TUNJANGAN - PM_TOT_POTONGAN - PM_ROUNDING per Level_Status into it. THR uses "THR Payables".
var glPayableJenis = map[string]bool{"SALARY PAYABLES": true, "THR PAYABLES": true}

// GlSetupOptions controls the GL setup load. ExpectRows = 0 means unchecked (production: 1338).
type GlSetupOptions struct {
	CompanyID  int16
	ExpectRows int
}

type glKey struct {
	division, journal, group, cost string
	sysid                          int64
	kind                           string
}

type glRow struct {
	key             glKey
	main, sub, drcr string
	desc            string
	subFromEmp      bool
	balance         bool // legacy Employee Balance row: a flag on component accounts, not a journal line (D-63 butir 2)
	legacy          map[string]string
	srcLine         int
}

// glBalanceKey is one "main account with employee sub-account" marker: journal type + main account. The legacy
// SALARY001 subquery does not filter the Employee Balance row on division, so the marker applies to both divisions.
type glBalanceKey struct{ journal, main string }

var glCostGroups = map[string]string{costGroupMGT: costGroupMGT, "MB": "MB", costGroupContract: costGroupContract}

// glJournalType maps MPS_TYPE (GAJI/THR/Bonus) to pgs_journal_type; "" = unknown.
func glJournalType(s string) string {
	if strings.EqualFold(s, "GAJI") {
		return "GAJI"
	}
	return thrKind(s) // THR / BONUS
}

// GlAccount normalizes MPS_AKUN / MPS_MAIN / MPS_SUB (11§4.5 step 1): "main-sub" in MPS_AKUN is split,
// otherwise MPS_MAIN/MPS_SUB are used (MPS_AKUN alone counts as main). ok=false when no main account remains.
func GlAccount(akun, main, sub string) (m, s string, ok bool) {
	akun, main, sub = strings.TrimSpace(akun), strings.TrimSpace(main), strings.TrimSpace(sub)
	if i := strings.Index(akun, "-"); i >= 0 {
		m, s = strings.TrimSpace(akun[:i]), strings.TrimSpace(akun[i+1:])
		return m, s, m != ""
	}
	if main == "" {
		main = akun
	}
	return main, sub, main != ""
}

// parseGlRow validates one source row; the string is the exception text ("" = ok).
func parseGlRow(r []string, ix map[string]int, sysids map[sysKey]int64) (glRow, string) {
	c := func(n string) string { return r[ix[n]] }
	var g glRow
	g.key.division = strings.ToUpper(c("MPS_DIVISION"))
	if g.key.division != DivSolo && g.key.division != DivJakarta {
		return g, "MPS_DIVISION tidak dikenal: " + fmt.Sprintf("%q", c("MPS_DIVISION"))
	}
	if g.key.journal = glJournalType(c("MPS_TYPE")); g.key.journal == "" {
		return g, fmt.Sprintf("MPS_TYPE %q bukan GAJI/THR/Bonus", c("MPS_TYPE"))
	}
	var ok bool
	if g.main, g.sub, ok = GlAccount(c("MPS_AKUN"), c("MPS_MAIN"), c("MPS_SUB")); !ok {
		return g, "akun tidak terurai (MPS_AKUN/MPS_MAIN kosong)"
	}
	g.desc = c("MPS_DESCRIPTION")
	jenis := strings.ToUpper(c("MPS_JENIS"))
	if jenis == glJenisBalance {
		// D-63 butir 2: legacy rows have no jabatan/no/drcr; they only mark the main account as "sub = employee".
		g.balance = true
		return g, ""
	}
	if exc := parseGlLine(&g, c); exc != "" {
		return g, exc
	}
	if glPayableJenis[jenis] {
		g.key.kind = glKindPayable
		return g, ""
	}
	g.key.kind = glKindComp
	typ := map[string]string{"TUNJANGAN": typeTunjangan, "POTONGAN": typePotongan}[jenis]
	if typ == "" {
		return g, fmt.Sprintf("MPS_JENIS %q bukan Tunjangan/Potongan/Salary Payables/THR Payables/Employee Balance", c("MPS_JENIS"))
	}
	var no int
	if _, err := fmt.Sscanf(c("MPS_NO"), "%d", &no); err != nil {
		return g, fmt.Sprintf("MPS_NO %q bukan angka", c("MPS_NO"))
	}
	sys, found := sysids[sysKey{typ, no}]
	if !found {
		return g, fmt.Sprintf("komponen %s %d (%s) tanpa peta sysid di pyr_component_legacy_map", jenis, no, g.key.division)
	}
	g.key.sysid = sys
	return g, ""
}

// parseGlLine validates the columns every journal line (component or payable) needs: jabatan, dept and drcr.
func parseGlLine(g *glRow, c func(string) string) string {
	if g.key.group = c("MPS_JABATAN"); g.key.group == "" {
		return "MPS_JABATAN kosong"
	}
	if d := strings.ToUpper(c("MPS_DEPT")); d != "" {
		cg, ok := glCostGroups[d]
		if !ok {
			return fmt.Sprintf("MPS_DEPT %q bukan MGT/MB/Contract", c("MPS_DEPT"))
		}
		g.key.cost = cg
	}
	if g.drcr = strings.ToUpper(c("MPS_DRCR")); g.drcr != "D" && g.drcr != "C" {
		return fmt.Sprintf("MPS_DRCR %q bukan D/C", c("MPS_DRCR"))
	}
	return ""
}

// glDeptGroup is a department-specific gl_group override (pggm_dept_code; wins over the level-only row).
type glDeptGroup struct{ division, level, dept, group string }

// glDeptGroups: legacy CASE "PM_EMP_TYPE = 'Driver' AND PM_DEPT_CODE = 'HRMC' → 'Driver Contract'" (SOLO, D-63 butir 5).
var glDeptGroups = []glDeptGroup{{DivSolo, "Driver", "HRMC", "Driver Contract"}}

// glGroupMap is the legacy Level_Status CASE (SALARY001) as level → gl_group rows per division. The department
// dependent case (Driver in HRMC) is seeded separately from glDeptGroups.
func glGroupMap() map[string]map[string]string {
	solo := map[string]string{levelExecutive: levelExecutive}
	jkt := map[string]string{
		levelStaffHO: levelStaffHO, "Office Boy": levelStaffHO, "Driver HO": "Driver HO",
		levelStaffBandung: levelStaffBandung, "OB Bandung": levelStaffBandung, "Driver Bandung": levelStaffBandung,
		"Solo Asmen": "Solo Asmen",
	}
	for _, r := range LevelRules() {
		if _, isJkt := jkt[r.Level]; !isJkt && r.Level != levelExecutive {
			solo[r.Level] = "Operator"
		}
	}
	return map[string]map[string]string{DivSolo: solo, DivJakarta: jkt}
}

// RunGlSetup loads MGT_PAYROLL_SETUP into pyr_gl_setup (+ seeds pyr_gl_group_map) in ONE transaction (11§4.5, D-63).
// Tunjangan/Potongan rows become COMPONENT lines, Salary/THR Payables rows PAYABLE lines (per group, no component).
// Employee Balance rows are not lines: they set pgs_sub_from_employee on COMPONENT lines of the same journal type and
// main account (legacy SALARY001 sub_nik; no NIK fallback, 11§4.2) and count as loaded. Rows that cannot be loaded (unknown division/type/dept/drcr, unparsable account, component without sysid map) and
// duplicate keys with differing account/drcr (the legacy table has no PK) are reported as Exceptions and not loaded;
// identical duplicates collapse into one row (Warnings). Idempotent: upsert on the unique index, pgs_legacy_row = source row.
//
//nolint:gocognit,gocyclo // linear read -> classify -> upsert pipeline, same shape as RunThrLegacy
func RunGlSetup(ctx context.Context, db *sql.DB, src Source, o GlSetupOptions) (TableReport, error) {
	t, err := src.Read(TableGlSetup)
	if err != nil {
		return TableReport{}, err
	}
	rep := TableReport{Table: t.Name, SrcRows: len(t.Rows), Checksum: t.Checksum()}
	if o.ExpectRows > 0 && rep.SrcRows != o.ExpectRows {
		return rep, fmt.Errorf("%s: %d baris, diharapkan %d", TableGlSetup, rep.SrcRows, o.ExpectRows)
	}
	ix, err := bindCols(t, []string{"MPS_JABATAN", "MPS_NO", "MPS_JENIS", "MPS_DEPT", "MPS_AKUN", "MPS_MAIN", "MPS_SUB", "MPS_DRCR", "MPS_DESCRIPTION", "MPS_TYPE", "MPS_DIVISION"})
	if err != nil {
		return rep, err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return rep, err
	}
	defer func() { _ = tx.Rollback() }() //nolint:errcheck // no-op after Commit
	sysids := map[string]map[sysKey]int64{}
	groups := make([]glRow, 0, len(t.Rows))
	byKey := map[glKey][]glRow{}
	balances := map[glBalanceKey]int{}
	for i, r := range t.Rows {
		div := strings.ToUpper(r[ix["MPS_DIVISION"]])
		if _, ok := sysids[div]; !ok && (div == DivSolo || div == DivJakarta) {
			if sysids[div], err = glSysids(ctx, tx, o.CompanyID, div); err != nil {
				return rep, err
			}
		}
		g, exc := parseGlRow(r, ix, sysids[div])
		if exc != "" {
			rep.Exceptions = append(rep.Exceptions, fmt.Sprintf("%s baris %d: %s", t.Name, i+2, exc))
			continue
		}
		if g.balance {
			balances[glBalanceKey{g.key.journal, g.main}]++
			continue
		}
		g.srcLine = i + 2
		g.legacy = map[string]string{}
		for j, c := range t.Cols {
			g.legacy[c] = r[j]
		}
		byKey[g.key] = append(byKey[g.key], g)
		groups = append(groups, g)
	}
	loaded := applyBalances(groups, byKey, balances, &rep)
	seen := map[glKey]bool{}
	for _, g := range groups {
		if seen[g.key] {
			continue
		}
		seen[g.key] = true
		dups := byKey[g.key]
		if conflict := glConflict(dups); conflict != "" {
			for _, d := range dups {
				rep.Exceptions = append(rep.Exceptions, fmt.Sprintf("%s baris %d: duplikat kunci %s dengan akun/drcr berbeda (%s)", t.Name, d.srcLine, glKeyText(d.key), conflict))
			}
			continue
		}
		if len(dups) > 1 {
			rep.Warnings = append(rep.Warnings, fmt.Sprintf("%d baris identik digabung: %s", len(dups), glKeyText(g.key)))
		}
		if err := upsertGl(ctx, tx, o.CompanyID, dups[0]); err != nil {
			return rep, fmt.Errorf("baris %d: %w", g.srcLine, err)
		}
		loaded += len(dups)
	}
	rep.DstRows = loaded
	if err := loadGlGroupMap(ctx, tx, o.CompanyID, &rep); err != nil {
		return rep, err
	}
	if err := tx.Commit(); err != nil {
		return rep, err
	}
	return rep, nil
}

// applyBalances marks COMPONENT lines whose (journal, main) has an Employee Balance row, warns about markers that match
// no component, and returns the number of Employee Balance source rows consumed.
func applyBalances(groups []glRow, byKey map[glKey][]glRow, balances map[glBalanceKey]int, rep *TableReport) int {
	used := map[glBalanceKey]bool{}
	for _, g := range groups {
		bk := glBalanceKey{g.key.journal, g.main}
		if g.key.kind != glKindComp || balances[bk] == 0 {
			continue
		}
		used[bk] = true
		for i := range byKey[g.key] {
			byKey[g.key][i].subFromEmp = true
		}
	}
	n := 0
	keys := make([]glBalanceKey, 0, len(balances))
	for k, c := range balances {
		n += c
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i].journal+keys[i].main < keys[j].journal+keys[j].main })
	for _, k := range keys {
		if !used[k] {
			rep.Warnings = append(rep.Warnings, fmt.Sprintf("Employee Balance %s akun %s tanpa baris komponen ber-akun sama", k.journal, k.main))
		}
	}
	return n
}

func glKeyText(k glKey) string {
	return fmt.Sprintf("%s/%s/%s/cost=%q/sysid=%d/%s", k.division, k.journal, k.group, k.cost, k.sysid, k.kind)
}

// glConflict reports the first account/drcr difference among rows sharing one key.
func glConflict(rows []glRow) string {
	for _, r := range rows[1:] {
		if r.main != rows[0].main || r.sub != rows[0].sub || r.drcr != rows[0].drcr {
			return fmt.Sprintf("%s-%s %s vs %s-%s %s", rows[0].main, rows[0].sub, rows[0].drcr, r.main, r.sub, r.drcr)
		}
	}
	return ""
}

func glSysids(ctx context.Context, tx *sql.Tx, company int16, division string) (map[sysKey]int64, error) {
	rows, err := tx.QueryContext(ctx, `SELECT lower(pclm_legacy_type::text), pclm_legacy_id, pclm_sysid FROM payroll.pyr_component_legacy_map
WHERE pclm_company_id=$1 AND pclm_legacy_division=$2`, company, division)
	if err != nil {
		return nil, err
	}
	defer closeQuiet(rows)
	m := map[sysKey]int64{}
	for rows.Next() {
		var k sysKey
		var s int64
		if err := rows.Scan(&k.typ, &k.id, &s); err != nil {
			return nil, err
		}
		m[k] = s
	}
	return m, rows.Err()
}

func upsertGl(ctx context.Context, tx *sql.Tx, company int16, g glRow) error {
	legacy, err := json.Marshal(g.legacy)
	if err != nil {
		return err
	}
	var cost, sysid, sub any
	if g.key.cost != "" {
		cost = g.key.cost
	}
	if g.key.sysid != 0 {
		sysid = g.key.sysid
	}
	if g.sub != "" {
		sub = g.sub
	}
	_, err = tx.ExecContext(ctx, `
INSERT INTO payroll.pyr_gl_setup (pgs_company_id, pgs_division, pgs_journal_type, pgs_gl_group, pgs_cost_group, pgs_component_sysid,
  pgs_line_kind, pgs_account_main, pgs_account_sub, pgs_sub_from_employee, pgs_drcr, pgs_description, pgs_valid_from, pgs_legacy_row)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14::jsonb)
ON CONFLICT (pgs_company_id, pgs_division, pgs_journal_type, pgs_gl_group, coalesce(pgs_cost_group,'*'), coalesce(pgs_component_sysid,0), pgs_line_kind, pgs_valid_from)
DO UPDATE SET pgs_account_main=EXCLUDED.pgs_account_main, pgs_account_sub=EXCLUDED.pgs_account_sub,
  pgs_sub_from_employee=EXCLUDED.pgs_sub_from_employee, pgs_drcr=EXCLUDED.pgs_drcr,
  pgs_description=EXCLUDED.pgs_description, pgs_legacy_row=EXCLUDED.pgs_legacy_row`,
		company, g.key.division, g.key.journal, g.key.group, cost, sysid, g.key.kind, g.main, sub, g.subFromEmp, g.drcr, g.desc, glValidFrom, string(legacy))
	return err
}

// loadGlGroupMap seeds pyr_gl_group_map and warns when a mapped gl_group has no setup row for its division.
func loadGlGroupMap(ctx context.Context, tx *sql.Tx, company int16, rep *TableReport) error {
	rows := make([]glDeptGroup, 0, 32)
	for div, m := range glGroupMap() {
		for level, grp := range m {
			rows = append(rows, glDeptGroup{div, level, "", grp})
		}
	}
	for _, r := range append(rows, glDeptGroups...) {
		var dept any
		if r.dept != "" {
			dept = r.dept
		}
		if _, err := tx.ExecContext(ctx, `
INSERT INTO payroll.pyr_gl_group_map (pggm_company_id, pggm_division, pggm_level, pggm_dept_code, pggm_gl_group) VALUES ($1,$2,$3,$4,$5)
ON CONFLICT (pggm_company_id, pggm_division, pggm_level, coalesce(pggm_dept_code,'*')) DO UPDATE SET pggm_gl_group=EXCLUDED.pggm_gl_group`,
			company, r.division, r.level, dept, r.group); err != nil {
			return fmt.Errorf("gl_group_map %s/%s/%s: %w", r.division, r.level, r.dept, err)
		}
	}
	q, err := tx.QueryContext(ctx, `SELECT DISTINCT m.pggm_division, m.pggm_gl_group FROM payroll.pyr_gl_group_map m
WHERE m.pggm_company_id=$1 AND NOT EXISTS (SELECT 1 FROM payroll.pyr_gl_setup s WHERE s.pgs_company_id=m.pggm_company_id
  AND s.pgs_division=m.pggm_division AND s.pgs_gl_group=m.pggm_gl_group) ORDER BY 1,2`, company)
	if err != nil {
		return err
	}
	defer closeQuiet(q)
	for q.Next() {
		var d, g string
		if err := q.Scan(&d, &g); err != nil {
			return err
		}
		rep.Warnings = append(rep.Warnings, fmt.Sprintf("gl_group %q (%s) ada di group_map tetapi tanpa baris setup GL", g, d))
	}
	return q.Err()
}
