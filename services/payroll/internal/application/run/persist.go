package run

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	"github.com/mutugading/goapps-backend/services/payroll/internal/engine"
)

// errPeriodMoved: the period left OPEN/DRAFT after the service check (cannot happen under FOR UPDATE).
var errPeriodMoved = errors.New("run: period status changed during commit")

// persistBatch bounds the jsonb parameter of one INSERT (≈ 2 MB of detail rows).
const persistBatch = 5000

// legacy psd_tax values (MPCE_TAX): 2 = taxable, 0 = not.
const (
	taxYes = 2
	taxNo  = 0
)

// persistRun writes one calculated run (T017). Rows go through INSERT .. SELECT FROM jsonb_to_recordset:
// COPY is refused on RLS tables, and the policies and fn_guard_closed must still see every row.
type persistRun struct {
	q          Query
	ruleSetID  int
	snapshotID int64
	payDate    time.Time
	actor      string
	res        DryRunResult
	inputs     []engine.Input
}

type runIDs struct{ run, superseded int64 }

func (p persistRun) save(ctx context.Context, tx *sql.Tx) (runIDs, error) {
	var ids runIDs
	if err := checkTotals(p.res.Results); err != nil {
		return ids, err
	}
	// The period predicates repeat the service check: fn_guard_closed only covers head/detail/addition.
	err := tx.QueryRowContext(ctx, `UPDATE payroll.pyr_run SET pr_status = 'SUPERSEDED'
WHERE pr_company_id = $1 AND pr_yymm = $2 AND pr_division = $3 AND pr_status = 'ACTIVE' AND pr_parent_run_id IS NULL
  AND EXISTS (SELECT 1 FROM payroll.pyr_period WHERE pp_company_id = $1 AND pp_yymm = $2 AND pp_division = $3
              AND pp_status IN ('OPEN', 'DRAFT'))
RETURNING pr_id`, p.q.CompanyID, p.q.YYMM, p.q.Division).Scan(&ids.superseded)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return ids, fmt.Errorf("supersede run: %w", err)
	}
	warnings, err := json.Marshal(nonNil(p.res.Warnings))
	if err != nil {
		return ids, fmt.Errorf("marshal warnings: %w", err)
	}
	s := p.res.Summary
	if err := tx.QueryRowContext(ctx, `INSERT INTO payroll.pyr_run (pr_company_id, pr_yymm, pr_division, pr_pay_date, pr_rule_set_id,
  pr_snapshot_id, pr_status, pr_kind, pr_input_hash, pr_emp_count, pr_line_count, pr_tot_tunjangan, pr_tot_potongan,
  pr_warnings, pr_started_by, pr_finished_at)
VALUES ($1,$2,$3,$4,$5,$6,'ACTIVE','ENGINE',$7,$8,$9,$10,$11,$12::jsonb,$13,now()) RETURNING pr_id`,
		p.q.CompanyID, p.q.YYMM, p.q.Division, p.payDate, p.ruleSetID, p.snapshotID, p.res.InputHash,
		s.EmpCount, s.LineCount, s.TotalT.StringFixed(2), s.TotalP.StringFixed(2), string(warnings), p.actor).Scan(&ids.run); err != nil {
		return ids, fmt.Errorf("insert run: %w", err)
	}
	if err := p.insertHeads(ctx, tx, ids.run); err != nil {
		return ids, err
	}
	if err := p.insertDetails(ctx, tx, ids.run); err != nil {
		return ids, err
	}
	upd, err := tx.ExecContext(ctx, `UPDATE payroll.pyr_period SET pp_active_run_id = $4, pp_status = 'DRAFT'
WHERE pp_company_id = $1 AND pp_yymm = $2 AND pp_division = $3 AND pp_status IN ('OPEN', 'DRAFT')`,
		p.q.CompanyID, p.q.YYMM, p.q.Division, ids.run)
	if err != nil {
		return ids, fmt.Errorf("update period: %w", err)
	}
	n, err := upd.RowsAffected()
	if err != nil {
		return ids, fmt.Errorf("update period: %w", err)
	}
	if n != 1 {
		return ids, fmt.Errorf("%w: period %d %s not OPEN/DRAFT", errPeriodMoved, p.q.YYMM, p.q.Division)
	}
	return ids, nil
}

// errTotalsMismatch: a header total differs from Σ of its lines (engine bug; nothing is persisted).
var errTotalsMismatch = errors.New("run: header total != sum of lines")

// checkTotals asserts per NIK that the header totals equal Σ line (T/P) before anything is written.
func checkTotals(results []engine.Result) error {
	for _, r := range results {
		if r.Header == nil {
			continue
		}
		var t, p decimal.Decimal
		for _, l := range r.Lines {
			if l.Key.Type == engine.CompT {
				t = t.Add(l.AmountOrZero())
			} else {
				p = p.Add(l.AmountOrZero())
			}
		}
		if !t.Equal(r.Header.TotTunjangan) || !p.Equal(r.Header.TotPotongan) {
			return fmt.Errorf("%w: nik %s T %s/%s P %s/%s", errTotalsMismatch, r.NIK, t, r.Header.TotTunjangan, p, r.Header.TotPotongan)
		}
	}
	return nil
}

// headRow is one pyr_salary_head row; the employee attributes are joined from pyr_employee.
type headRow struct {
	NIK         string          `json:"nik"`
	BasicPay    *string         `json:"basic_pay"`
	TunjJabatan *string         `json:"tunj_jabatan"`
	TunjRumah   *string         `json:"tunj_rumah"`
	TunjKesejah *string         `json:"tunj_kesejahteraan"`
	TunjTrans   *string         `json:"tunj_transport"`
	RatePerJam  *string         `json:"rate_perjam"`
	TotT        string          `json:"tot_t"`
	TotP        string          `json:"tot_p"`
	Rounding    string          `json:"rounding"`
	JamLembur   json.RawMessage `json:"jam_lembur"`
	Remarks     string          `json:"remarks"`
}

func (p persistRun) insertHeads(ctx context.Context, tx *sql.Tx, runID int64) error {
	rows := make([]headRow, 0, len(p.res.Results))
	for _, r := range p.res.Results {
		if r.Header == nil { // legacy compat without recap: lines only (D2)
			continue
		}
		h := r.Header
		jam, err := jamLemburJSON(h.JamLembur)
		if err != nil {
			return fmt.Errorf("jam lembur %s: %w", r.NIK, err)
		}
		var rate *string
		if h.RatePerJam != nil {
			v := h.RatePerJam.Round(2).StringFixed(2)
			rate = &v
		}
		rows = append(rows, headRow{NIK: r.NIK, BasicPay: money(h.BasicPay), TunjJabatan: money(h.TunjJabatan),
			TunjRumah: money(h.TunjRumah), TunjKesejah: money(h.TunjKesejahteraan), TunjTrans: money(h.TunjTransport),
			RatePerJam: rate, TotT: h.TotTunjangan.StringFixed(2), TotP: h.TotPotongan.StringFixed(2),
			Rounding: h.Rounding.StringFixed(2), JamLembur: jam, Remarks: h.Remarks})
	}
	return insertJSON(ctx, tx, rows, `INSERT INTO payroll.pyr_salary_head (psh_run_id, psh_company_id, psh_nik, psh_yymm, psh_division,
  psh_nama, psh_dept, psh_dept_code, psh_section, psh_grade, psh_jabatan, psh_emp_type, psh_pembayaran, psh_date_join, psh_pay_date,
  psh_tax_code, psh_basic_pay, psh_tunj_jabatan, psh_tunj_rumah, psh_tunj_kesejahteraan, psh_tunj_transport, psh_rate_perjam,
  psh_tot_tunjangan, psh_tot_potongan, psh_rounding, psh_day_pay, psh_jam_lembur, psh_remarks)
SELECT $2, $3, x.nik, $4, $5, e.pe_nama, e.pe_dept, e.pe_dept_id, e.pe_section, e.pe_grade, e.pe_designation, e.pe_level, 'Gaji',
  e.pe_date_join, $6, e.pe_tax_id, x.basic_pay, x.tunj_jabatan, x.tunj_rumah, x.tunj_kesejahteraan, x.tunj_transport, x.rate_perjam,
  x.tot_t, x.tot_p, x.rounding, NULL, x.jam_lembur, NULLIF(x.remarks, '')
FROM jsonb_to_recordset($1::jsonb) AS x(nik text, basic_pay numeric, tunj_jabatan numeric, tunj_rumah numeric,
  tunj_kesejahteraan numeric, tunj_transport numeric, rate_perjam numeric, tot_t numeric, tot_p numeric, rounding numeric,
  jam_lembur jsonb, remarks text)
LEFT JOIN payroll.pyr_employee e ON e.pe_company_id = $3 AND e.pe_division = $5 AND e.pe_nik = x.nik`,
		runID, p.q.CompanyID, p.q.YYMM, p.q.Division, p.payDate)
}

// detailRow is one pyr_salary_detail row; description and sysid are joined from pyr_component.
type detailRow struct {
	NIK     string          `json:"nik"`
	Type    string          `json:"typ"`
	ID      int             `json:"id"`
	Code    string          `json:"code"` // pc_code, e.g. T001 (engine.CompKey.String)
	Amount  *string         `json:"amount"`
	Sts     string          `json:"sts"`
	Tax     int             `json:"tax"`
	TPC     string          `json:"tpc"`
	Base    *string         `json:"base"` // nil = line without basis → psd_basis_amount NULL
	Qty     string          `json:"qty"`
	Rate    string          `json:"rate"`
	Formula string          `json:"formula"`
	Remark  string          `json:"remark"`
	Meta    json.RawMessage `json:"meta"`
}

func (p persistRun) insertDetails(ctx context.Context, tx *sql.Tx, runID int64) error {
	sts := componentSts(p.inputs)
	rows := make([]detailRow, 0, p.res.Summary.LineCount)
	for _, r := range p.res.Results {
		for _, l := range r.Lines {
			d, err := newDetailRow(r.NIK, l, sts[r.NIK][l.Key])
			if err != nil {
				return err
			}
			rows = append(rows, d)
		}
	}
	return insertJSON(ctx, tx, rows, `INSERT INTO payroll.pyr_salary_detail (psd_run_id, psd_nik, psd_type, psd_id, psd_description,
  psd_amount, psd_sts, psd_tax, psd_tax_paid_company, psd_basis_amount, psd_qty, psd_rate, psd_formula, psd_remarks,
  psd_component_sysid, psd_amount_base, psd_meta)
SELECT $2, x.nik, x.typ::payroll.comp_type, x.id, c.pc_name, x.amount, NULLIF(x.sts, ''), x.tax, x.tpc, x.base, x.qty, x.rate,
  NULLIF(x.formula, ''), NULLIF(x.remark, ''), c.pc_sysid, x.amount, x.meta
FROM jsonb_to_recordset($1::jsonb) AS x(nik text, typ text, id int, amount numeric, sts text, tax smallint, tpc text,
  base numeric, qty numeric, rate numeric, formula text, remark text, meta jsonb, code text)
LEFT JOIN payroll.pyr_component c ON c.pc_company_id = $3 AND c.pc_code = x.code`,
		runID, p.q.CompanyID)
}

func newDetailRow(nik string, l engine.Line, sts string) (detailRow, error) {
	typ := "Tunjangan"
	if l.Key.Type == engine.CompP {
		typ = "Potongan"
	}
	d := detailRow{NIK: nik, Type: typ, ID: l.Key.ID, Code: l.Key.String(), Amount: money(l.Amount), Sts: sts, Tax: taxNo,
		TPC: "N", Base: basisOf(l.Base), Qty: l.Qty.Round(4).String(), Rate: l.Rate.Round(6).String(),
		Formula: l.Formula, Remark: l.Remark}
	if l.Tax {
		d.Tax = taxYes
	}
	if l.TaxPaidCompany {
		d.TPC = "Y"
	}
	if len(l.Meta) > 0 {
		b, err := json.Marshal(l.Meta)
		if err != nil {
			return d, fmt.Errorf("meta %s %s: %w", nik, l.Key, err)
		}
		d.Meta = b
	}
	return d, nil
}

// componentSts is the F/D status of each employee component (psd_sts), which Line does not carry.
func componentSts(inputs []engine.Input) map[string]map[engine.CompKey]string {
	out := make(map[string]map[engine.CompKey]string, len(inputs))
	for _, in := range inputs {
		m := make(map[engine.CompKey]string, len(in.Components))
		for _, c := range in.Components {
			m[c.Key] = c.Sts
		}
		out[in.Emp.NIK] = m
	}
	return out
}

// insertJSON runs stmt ($1 = jsonb array, then args) in batches of persistBatch rows.
func insertJSON[T any](ctx context.Context, tx *sql.Tx, rows []T, stmt string, args ...any) error {
	for i := 0; i < len(rows); i += persistBatch {
		b, err := json.Marshal(rows[i:min(i+persistBatch, len(rows))])
		if err != nil {
			return fmt.Errorf("marshal rows: %w", err)
		}
		if _, err := tx.ExecContext(ctx, stmt, append([]any{string(b)}, args...)...); err != nil {
			return fmt.Errorf("insert %s: %w", strings.Fields(stmt)[2], err)
		}
	}
	return nil
}

// jamLemburKeys maps the engine keys to the psh_jam_lembur keys written by the legacy ETL (T009e).
var jamLemburKeys = map[string]string{
	"lembur_1": "lembur1", "lembur_2": "lembur2",
	"lembur_nas_1": "nas1", "lembur_nas_2": "nas2", "lembur_nas_3": "nas3",
	"lembur_off_1": "off1", "lembur_off_2": "off2", "lembur_off_3": "off3",
	"lembur_nasp_1": "nasp1", "lembur_nasp_2": "nasp2", "lembur_nasp_3": "nasp3",
}

func jamLemburJSON(m map[string]decimal.Decimal) (json.RawMessage, error) {
	if len(m) == 0 {
		return nil, nil
	}
	out := make(map[string]json.Number, len(m))
	for k, v := range m {
		key, ok := jamLemburKeys[k]
		if !ok {
			key = k
		}
		out[key] = json.Number(v.String())
	}
	return json.Marshal(out)
}

// basisOf: a zero Base means the component has no calculation basis (engine.Line carries no pointer), so
// psd_basis_amount stays NULL instead of a misleading 0.00.
func basisOf(d decimal.Decimal) *string {
	if d.IsZero() {
		return nil
	}
	s := d.StringFixed(2)
	return &s
}

func money(d *decimal.Decimal) *string {
	if d == nil {
		return nil
	}
	s := d.StringFixed(2)
	return &s
}

func nonNil(w []Issue) []Issue {
	if w == nil {
		return []Issue{}
	}
	return w
}
