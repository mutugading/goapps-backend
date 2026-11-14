package report

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strconv"

	"github.com/shopspring/decimal"
	"github.com/xuri/excelize/v2"

	"github.com/mutugading/goapps-backend/services/payroll/internal/application/run"
	"github.com/mutugading/goapps-backend/services/payroll/internal/domain/scope"
)

// Sheet names of the GL workbook: F-2 `upload` (SALARY001 layout, 11§4.4) and the control sheet.
const (
	SheetUpload  = "upload"
	SheetGLCheck = "Check"
)

// ErrNoBookRate is returned when the period has no IDR→USD BOOK rate, or more than one (SALARY001 returns 0 for
// NO_DATA_FOUND / TOO_MANY_ROWS; GoApps rejects both, 11§4.4).
var ErrNoBookRate = errors.New("report: kurs BOOK IDR→USD periode wajib tepat satu di pyr_fx_rate")

// ErrGLFinalExists is returned when a FINAL export already exists for the run and journal: a FINAL file is uploaded
// to Orion once; void the earlier one first (PREVIEW exports may repeat).
var ErrGLFinalExists = errors.New("report: ekspor GL FINAL untuk run ini sudah ada")

// ErrNoActor is returned when the context carries no actor for pge_generated_by / audit.
var ErrNoActor = errors.New("report: aktor wajib untuk ekspor GL")

const (
	journalGaji  = "GAJI"
	statusClosed = "CLOSED"
)

// ExportGL builds the GAJI journal of the run, lets write render it, records pyr_gl_export (PREVIEW, or FINAL when the
// period is CLOSED) with the sha256 of the bytes and the totals, and writes the REPORT_EXPORT audit row, all in one
// transaction. Nothing is recorded when the journal is invalid (GLError) or write fails.
func (s *Service) ExportGL(ctx context.Context, q Query, write func(GLJournal) ([]byte, error)) (GLJournal, error) {
	var j GLJournal
	id, ok := scope.FromContext(ctx)
	if !ok || id.UserID == "" {
		return j, ErrNoActor
	}
	err := s.DB.ScopedTxOpts(ctx, run.SnapshotTx, func(tx *sql.Tx) error {
		st, runID, err := glRun(ctx, tx, q)
		if err != nil {
			return err
		}
		in, err := loadGLInput(ctx, tx, q, runID)
		if err != nil {
			return err
		}
		if j, err = BuildGL(in); err != nil {
			return err
		}
		status := "PREVIEW"
		if st == statusClosed {
			status = "FINAL"
			if err := checkNoFinal(ctx, tx, runID); err != nil {
				return err
			}
		}
		b, err := write(j)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(b)
		if _, err := tx.ExecContext(ctx, `INSERT INTO payroll.pyr_gl_export (pge_run_id, pge_journal_type, pge_status, pge_row_count,
  pge_total_debit, pge_total_credit, pge_file_sha256, pge_generated_by) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
			runID, journalGaji, status, len(j.Rows), j.Debit.String(), j.Credit.String(), hex.EncodeToString(sum[:]), id.UserID); err != nil {
			return fmt.Errorf("write gl export: %w", err)
		}
		return s.writeAudit(ctx, tx, q, "gl", len(j.Rows), map[string]any{"status": status, "nik": j.NIKCount})
	})
	return j, err
}

// checkNoFinal serializes FINAL exports of a run (transaction advisory lock) and rejects a second FINAL.
func checkNoFinal(ctx context.Context, tx *sql.Tx, runID int64) error {
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtext('payroll.gl_export'), hashtext($1))`, strconv.FormatInt(runID, 10)); err != nil {
		return fmt.Errorf("gl export lock: %w", err)
	}
	var n int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM payroll.pyr_gl_export
WHERE pge_run_id = $1 AND pge_journal_type = $2 AND pge_status = 'FINAL'`, runID, journalGaji).Scan(&n); err != nil {
		return fmt.Errorf("read gl export: %w", err)
	}
	if n > 0 {
		return fmt.Errorf("%w (run %d %s)", ErrGLFinalExists, runID, journalGaji)
	}
	return nil
}

// glRun checks the period like Load (APPROVED/CLOSED unless AllowOpen) and returns its status and ACTIVE run.
func glRun(ctx context.Context, tx *sql.Tx, q Query) (string, int64, error) {
	var st string
	err := tx.QueryRowContext(ctx, `SELECT pp_status::text FROM payroll.pyr_period
WHERE pp_company_id = $1 AND pp_yymm = $2 AND pp_division = $3`, q.CompanyID, q.YYMM, q.Division).Scan(&st)
	if errors.Is(err, sql.ErrNoRows) {
		return "", 0, fmt.Errorf("%w: periode %d %s tidak ada", ErrNoRun, q.YYMM, q.Division)
	}
	if err != nil {
		return "", 0, fmt.Errorf("read period: %w", err)
	}
	if st != "APPROVED" && st != statusClosed && !q.AllowOpen {
		return "", 0, fmt.Errorf("%w (status %s, periode %d %s)", ErrPeriodNotApproved, st, q.YYMM, q.Division)
	}
	// The ACTIVE run of the period: ENGINE after go-live, LEGACY for the parity check (T028l on 202610).
	var id int64
	err = tx.QueryRowContext(ctx, `SELECT pr_id FROM payroll.pyr_run
WHERE pr_company_id = $1 AND pr_yymm = $2 AND pr_division = $3 AND pr_status = 'ACTIVE' AND pr_parent_run_id IS NULL
ORDER BY (pr_kind = 'ENGINE') DESC LIMIT 1`, q.CompanyID, q.YYMM, q.Division).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return "", 0, fmt.Errorf("%w: %d %s", ErrNoRun, q.YYMM, q.Division)
	}
	if err != nil {
		return "", 0, fmt.Errorf("find run: %w", err)
	}
	return st, id, nil
}

// glGroupSQL picks the gl_group of a head: level frozen in the run (psh_emp_type, = PM_EMP_TYPE / pe_level at run
// time), dept-specific mapping first (D-63 butir 5: SOLO Driver + HRMC → Driver Contract).
const glGroupSQL = `LEFT JOIN LATERAL (SELECT x.pggm_gl_group FROM payroll.pyr_gl_group_map x
  WHERE x.pggm_company_id = h.psh_company_id AND x.pggm_division = h.psh_division::text
    AND x.pggm_level = COALESCE(NULLIF(h.psh_emp_type,''), e.pe_level)
    AND (x.pggm_dept_code IS NULL OR x.pggm_dept_code = h.psh_dept_code)
  ORDER BY (x.pggm_dept_code IS NULL) LIMIT 1) g ON true`

const glHeadSQL = `SELECT h.psh_nik, COALESCE(h.psh_dept_code,''), COALESCE(g.pggm_gl_group,''), COALESCE(e.pe_cost_group,''),
  COALESCE(e.pe_orion_sub_acnt,''), h.psh_tot_tunjangan::text, h.psh_tot_potongan::text, h.psh_rounding::text,
  h.psh_net_pay::text
FROM payroll.pyr_salary_head h
LEFT JOIN payroll.pyr_employee e ON e.pe_company_id = h.psh_company_id AND e.pe_nik = h.psh_nik AND e.pe_division = h.psh_division
` + glGroupSQL + `
WHERE h.psh_run_id = $1 ORDER BY h.psh_nik`

const glLineSQL = `SELECT d.psd_nik, d.psd_type::text, d.psd_id, d.psd_amount::text, s.pgs_id IS NOT NULL,
  COALESCE(s.pgs_account_main,''), COALESCE(s.pgs_account_sub,''), COALESCE(s.pgs_drcr::text,''), COALESCE(s.pgs_description,''),
  COALESCE(s.pgs_sub_from_employee, false)
FROM payroll.pyr_salary_detail d
JOIN payroll.pyr_salary_head h ON h.psh_run_id = d.psd_run_id AND h.psh_nik = d.psd_nik
LEFT JOIN payroll.pyr_employee e ON e.pe_company_id = h.psh_company_id AND e.pe_nik = h.psh_nik AND e.pe_division = h.psh_division
` + glGroupSQL + `
LEFT JOIN payroll.pyr_component_legacy_map lm ON lm.pclm_company_id = h.psh_company_id AND lm.pclm_legacy_type = d.psd_type
  AND lm.pclm_legacy_id = d.psd_id AND lm.pclm_legacy_division = h.psh_division::text
LEFT JOIN LATERAL (SELECT x.* FROM payroll.pyr_gl_setup x
  WHERE x.pgs_company_id = h.psh_company_id AND x.pgs_division = h.psh_division::text AND x.pgs_journal_type = 'GAJI'
    AND x.pgs_gl_group = g.pggm_gl_group AND x.pgs_line_kind = 'COMPONENT' AND x.pgs_component_sysid = lm.pclm_sysid
    AND (x.pgs_cost_group IS NULL OR x.pgs_cost_group = e.pe_cost_group)
    AND x.pgs_valid_from <= h.psh_yymm AND (x.pgs_valid_to IS NULL OR x.pgs_valid_to >= h.psh_yymm)
  ORDER BY (x.pgs_cost_group IS NULL), x.pgs_valid_from DESC LIMIT 1) s ON true
WHERE d.psd_run_id = $1 AND d.psd_amount <> 0
ORDER BY d.psd_nik, d.psd_type, d.psd_id`

const glPayableSQL = `SELECT pgs_gl_group, pgs_account_main, COALESCE(pgs_account_sub,''), pgs_drcr::text, COALESCE(pgs_description,'')
FROM payroll.pyr_gl_setup
WHERE pgs_company_id = $1 AND pgs_division = $2 AND pgs_journal_type = 'GAJI' AND pgs_line_kind = 'PAYABLE'
  AND pgs_valid_from <= $3 AND (pgs_valid_to IS NULL OR pgs_valid_to >= $3)
ORDER BY pgs_gl_group, pgs_valid_from DESC`

// glRateSQL mirrors SALARY001: TO_CHAR(CER_EFF_FRM_DT,'RRRRMM') = :P_YYMM, IDR→USD type 'B' (loaded as BOOK).
const glRateSQL = `SELECT pfr_rate::text FROM payroll.pyr_fx_rate
WHERE to_char(pfr_rate_date, 'YYYYMM') = $1 AND pfr_from_ccy = 'IDR' AND pfr_to_ccy = 'USD' AND pfr_rate_type = 'BOOK'`

func loadGLInput(ctx context.Context, tx *sql.Tx, q Query, runID int64) (GLInput, error) {
	in := GLInput{YYMM: q.YYMM, Payables: map[string]GLAccount{}}
	var err error
	if in.Rate, err = loadBookRate(ctx, tx, q.YYMM); err != nil {
		return in, err
	}
	if in.Heads, err = loadGLHeads(ctx, tx, runID); err != nil {
		return in, err
	}
	if in.Lines, err = loadGLLines(ctx, tx, runID); err != nil {
		return in, err
	}
	rows, err := tx.QueryContext(ctx, glPayableSQL, q.CompanyID, q.Division, q.YYMM)
	if err != nil {
		return in, fmt.Errorf("read payable setup: %w", err)
	}
	defer closeRows(rows)
	for rows.Next() {
		var g string
		var a GLAccount
		if err := rows.Scan(&g, &a.Main, &a.Sub, &a.DrCr, &a.Desc); err != nil {
			return in, fmt.Errorf("scan payable setup: %w", err)
		}
		if _, ok := in.Payables[g]; !ok { // latest valid_from wins
			in.Payables[g] = a
		}
	}
	return in, rows.Err()
}

func loadBookRate(ctx context.Context, tx *sql.Tx, yymm int) (decimal.Decimal, error) {
	rows, err := tx.QueryContext(ctx, glRateSQL, strconv.Itoa(yymm))
	if err != nil {
		return decimal.Zero, fmt.Errorf("read fx rate: %w", err)
	}
	defer closeRows(rows)
	var rates []string
	for rows.Next() {
		var r string
		if err := rows.Scan(&r); err != nil {
			return decimal.Zero, fmt.Errorf("scan fx rate: %w", err)
		}
		rates = append(rates, r)
	}
	if err := rows.Err(); err != nil {
		return decimal.Zero, err
	}
	if len(rates) != 1 {
		return decimal.Zero, fmt.Errorf("%w (periode %d: %d baris)", ErrNoBookRate, yymm, len(rates))
	}
	return decimal.NewFromString(rates[0])
}

func loadGLHeads(ctx context.Context, tx *sql.Tx, runID int64) ([]GLHead, error) {
	rows, err := tx.QueryContext(ctx, glHeadSQL, runID)
	if err != nil {
		return nil, fmt.Errorf("read gl heads: %w", err)
	}
	defer closeRows(rows)
	var out []GLHead
	for rows.Next() {
		var h GLHead
		var t, p, r string
		var n sql.NullString
		if err := rows.Scan(&h.NIK, &h.Dept, &h.Group, &h.CostGroup, &h.SubAcnt, &t, &p, &r, &n); err != nil {
			return nil, fmt.Errorf("scan gl head: %w", err)
		}
		for _, f := range []struct {
			dst *decimal.Decimal
			s   string
		}{{&h.Tunj, t}, {&h.Pot, p}, {&h.Rounding, r}} {
			if *f.dst, err = decimal.NewFromString(f.s); err != nil {
				return nil, fmt.Errorf("gl head: %w", err)
			}
		}
		if n.Valid {
			v, err := decimal.NewFromString(n.String)
			if err != nil {
				return nil, fmt.Errorf("gl head net: %w", err)
			}
			h.NetPay = &v
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

func loadGLLines(ctx context.Context, tx *sql.Tx, runID int64) ([]GLLine, error) {
	rows, err := tx.QueryContext(ctx, glLineSQL, runID)
	if err != nil {
		return nil, fmt.Errorf("read gl lines: %w", err)
	}
	defer closeRows(rows)
	var out []GLLine
	for rows.Next() {
		var l GLLine
		var amt string
		if err := rows.Scan(&l.NIK, &l.Type, &l.No, &amt, &l.HasSetup, &l.Main, &l.Sub, &l.DrCr, &l.Desc, &l.SubFromEmployee); err != nil {
			return nil, fmt.Errorf("scan gl line: %w", err)
		}
		if l.Amount, err = decimal.NewFromString(amt); err != nil {
			return nil, fmt.Errorf("gl amount: %w", err)
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// WriteGL writes the GL workbook: `upload` (3 title rows, header on row 4, 13 columns of SALARY001) and `Check`
// (Σ per account and D/C, Σ D, Σ C, difference, NIK count, Σ net_pay).
func WriteGL(j GLJournal, w io.Writer) error {
	f := excelize.NewFile()
	defer f.Close() //nolint:errcheck // in-memory workbook
	if err := f.SetSheetName("Sheet1", SheetUpload); err != nil {
		return fmt.Errorf("rename sheet: %w", err)
	}
	if _, err := f.NewSheet(SheetGLCheck); err != nil {
		return fmt.Errorf("new sheet: %w", err)
	}
	sw := &sheetWriter{f: f}
	sw.upload(j)
	sw.glCheck(j)
	if sw.err != nil {
		return sw.err
	}
	return f.Write(w)
}

func (s *sheetWriter) upload(j GLJournal) {
	s.start(SheetUpload)
	s.put("SALARY SETUP Gaji")
	s.put()
	s.put("Processing Month " + strconv.Itoa(j.YYMM))
	s.put("No.", "Processing Month", "Dept", "Status", "Level Status", "MGT MB Contract", "Main Account", "Sub Account",
		"Dr / Cr", "Description", "FC Amount", "Exchange Rate", "LC Amount")
	for i, r := range j.Rows {
		s.put(i+1, j.YYMM, optStr(r.Dept), r.Group, r.Group, r.MGTMB(), r.Main, optStr(r.Sub), r.DrCr, r.Desc, r.FC, j.Rate, r.LC)
	}
}

func (s *sheetWriter) glCheck(j GLJournal) {
	s.start(SheetGLCheck)
	s.put("Main Account", "Sub Account", "Dr / Cr", "FC Amount", "Baris")
	for _, a := range j.AccountTotals() {
		s.put(a.Main, optStr(a.Sub), a.DrCr, a.FC, a.Rows)
	}
	s.put()
	s.put("Σ Debit", j.Debit)
	s.put("Σ Credit", j.Credit)
	s.put("Selisih", j.Debit.Sub(j.Credit))
	s.put("Σ LC Debit", j.LCDebit)
	s.put("Σ LC Credit", j.LCCredit)
	s.put("Selisih LC (pembulatan per baris)", j.LCDebit.Sub(j.LCCredit))
	s.put("Jumlah NIK", j.NIKCount)
	s.put("Σ psh_net_pay", j.NetPay)
	s.put("Σ (T − P − R) = Σ Salary Payables", j.Payables)
	s.put("Selisih net_pay", j.NetPay.Sub(j.Payables))
}
