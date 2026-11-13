package report

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/shopspring/decimal"
	"github.com/xuri/excelize/v2"

	"github.com/mutugading/goapps-backend/services/payroll/internal/application/run"
)

// Sheet names of the reco workbook (F-2, GOLIVE-FORMATS). Ringkasan (T028b) is kept as a third sheet.
const (
	SheetRecoDetail = "detail"
	SheetPreCheck   = "pra-cek"
)

// ErrNoPIIKey is returned when the reco export is asked without the PII key (bank accounts cannot be decrypted).
var ErrNoPIIKey = errors.New("report: kunci PII wajib untuk kolom PM_BANK_ACNT")

// DetailRow is one NIK × component line of the F-2 `detail` sheet. HasSetup=false means no GL setup row matched
// (the MPS_* columns stay empty and the line is listed in the pre-check sheet).
type DetailRow struct {
	NIK, Nama, DeptCode, Section, Grade, Status, LevelStatus string
	DateJoin, BirthDate                                      *time.Time
	MGTMB, BankAcnt                                          string
	No                                                       int
	Type, Description                                        string // Type = "Tunjangan" | "Potongan"
	Amount                                                   decimal.Decimal
	Basis                                                    *decimal.Decimal
	Remarks, Division                                        string
	HasSetup                                                 bool
	Akun, Main, Sub, DrCr, SetupDesc                         string
}

// RecoData is the run (heads/components for Ringkasan) plus the F-2 detail rows.
type RecoData struct {
	RunData
	Rows []DetailRow
	YYMM int
	// Transfer is the F-2 `transfer` sheet data (Mandiri payable rows); bank issues are listed in pra-cek.
	Transfer BankData
	// F-2 additional / reg / OT payroll sheets.
	Additions []AdditionRow
	Reg       []RegRow
	Overtime  []OTRow
}

// EarnDeduc is the signed amount: Tunjangan +, Potongan −.
func (r DetailRow) EarnDeduc() decimal.Decimal {
	if r.Type == typePotongan {
		return r.Amount.Neg()
	}
	return r.Amount
}

// MPSDescription is the setup description plus " Mon YYYY" (11§4.1); empty when there is no setup row.
func (r DetailRow) MPSDescription(yymm int) string {
	if !r.HasSetup {
		return ""
	}
	return r.SetupDesc + " " + monthYear(yymm)
}

func monthYear(yymm int) string {
	m := yymm % 100
	if m < 1 || m > 12 {
		return fmt.Sprint(yymm)
	}
	return time.Month(m).String()[:3] + fmt.Sprintf(" %d", yymm/100)
}

// PreCheck is a component without a GL setup row, with the number of lines affected.
type PreCheck struct {
	Type, Name string
	No, Lines  int
}

// PreChecks lists lines without a GL setup row grouped by component (type, no).
func PreChecks(rows []DetailRow) []PreCheck {
	idx := map[string]int{}
	var out []PreCheck
	for _, r := range rows {
		if r.HasSetup {
			continue
		}
		k := fmt.Sprintf("%s/%d", r.Type, r.No)
		i, ok := idx[k]
		if !ok {
			i = len(out)
			idx[k] = i
			out = append(out, PreCheck{Type: r.Type, No: r.No, Name: r.Description})
		}
		out[i].Lines++
	}
	return out
}

// ExportReco is Export for the reco workbook; key is the PII key used to decrypt the bank account.
func (s *Service) ExportReco(ctx context.Context, q Query, key string, build func(RecoData) (rows int, err error)) error {
	if key == "" {
		return ErrNoPIIKey
	}
	return s.DB.ScopedTxOpts(ctx, run.SnapshotTx, func(tx *sql.Tx) error {
		d, err := Load(ctx, tx, q)
		if err != nil {
			return err
		}
		rd := RecoData{RunData: d, YYMM: q.YYMM}
		if rd.Rows, err = loadRecoRows(ctx, tx, q, d.RunID, key); err != nil {
			return err
		}
		all, err := loadBankRows(ctx, tx, q, d.RunID, key)
		if err != nil {
			return err
		}
		rd.Transfer = SplitBank(q.YYMM, all, MandiriBank)
		if rd.Additions, err = loadAdditions(ctx, tx, q); err != nil {
			return err
		}
		if rd.Reg, err = loadReg(ctx, tx, d.RunID); err != nil {
			return err
		}
		if rd.Overtime, err = loadOvertime(ctx, tx, q, d.RunID); err != nil {
			return err
		}
		rows, err := build(rd)
		if err != nil {
			return err
		}
		return s.writeAudit(ctx, tx, q, "reco", rows, map[string]any{"pra_cek": len(PreChecks(rd.Rows)), "pra_cek_transfer": len(rd.Transfer.Issues)})
	})
}

const recoSQL = `SELECT d.psd_nik, COALESCE(h.psh_nama,''), COALESCE(h.psh_dept_code,''), COALESCE(h.psh_section,''),
  COALESCE(h.psh_grade,''), COALESCE(h.psh_emp_type, e.pe_employment_status, ''), COALESCE(g.pggm_gl_group,''),
  h.psh_date_join, e.pe_birth_date, COALESCE(e.pe_cost_group,''),
  CASE WHEN e.pe_bank_acnt_enc IS NULL THEN '' ELSE payroll.pgp_sym_decrypt(e.pe_bank_acnt_enc, $3::text) END,
  d.psd_id, d.psd_type::text, COALESCE(m.pcm_name, d.psd_description, ''), COALESCE(d.psd_amount::text,'0'),
  d.psd_basis_amount::text, COALESCE(d.psd_remarks,''), h.psh_division::text,
  s.pgs_id IS NOT NULL, COALESCE(s.pgs_account_main,''), COALESCE(s.pgs_account_sub,''), COALESCE(s.pgs_drcr::text,''),
  COALESCE(s.pgs_description,'')
FROM payroll.pyr_salary_detail d
JOIN payroll.pyr_salary_head h ON h.psh_run_id = d.psd_run_id AND h.psh_nik = d.psd_nik
LEFT JOIN payroll.pyr_employee e ON e.pe_company_id = $2 AND e.pe_nik = h.psh_nik AND e.pe_division = h.psh_division
LEFT JOIN payroll.pyr_gl_group_map g ON g.pggm_company_id = $2 AND g.pggm_division = h.psh_division::text AND g.pggm_level = e.pe_level
LEFT JOIN payroll.pyr_component_master m ON m.pcm_type = d.psd_type AND m.pcm_id = d.psd_id
LEFT JOIN payroll.pyr_component_legacy_map lm ON lm.pclm_company_id = $2 AND lm.pclm_legacy_type = d.psd_type
  AND lm.pclm_legacy_id = d.psd_id AND lm.pclm_legacy_division = h.psh_division::text
LEFT JOIN LATERAL (SELECT x.* FROM payroll.pyr_gl_setup x
  WHERE x.pgs_company_id = $2 AND x.pgs_division = h.psh_division::text AND x.pgs_journal_type = 'GAJI'
    AND x.pgs_gl_group = g.pggm_gl_group AND x.pgs_line_kind = 'COMPONENT' AND x.pgs_component_sysid = lm.pclm_sysid
    AND (x.pgs_cost_group IS NULL OR x.pgs_cost_group = e.pe_cost_group)
    AND x.pgs_valid_from <= h.psh_yymm AND (x.pgs_valid_to IS NULL OR x.pgs_valid_to >= h.psh_yymm)
  ORDER BY (x.pgs_cost_group IS NULL), x.pgs_valid_from DESC LIMIT 1) s ON true
WHERE d.psd_run_id = $1
ORDER BY d.psd_nik, COALESCE(m.pcm_serial, 1073741824), d.psd_type, d.psd_id`

func loadRecoRows(ctx context.Context, tx *sql.Tx, q Query, runID int64, key string) ([]DetailRow, error) {
	rows, err := tx.QueryContext(ctx, recoSQL, runID, q.CompanyID, key)
	if err != nil {
		return nil, fmt.Errorf("read reco detail: %w", err)
	}
	defer closeRows(rows)
	var out []DetailRow
	for rows.Next() {
		var r DetailRow
		var join, birth sql.NullTime
		var amt string
		var basis sql.NullString
		if err := rows.Scan(&r.NIK, &r.Nama, &r.DeptCode, &r.Section, &r.Grade, &r.Status, &r.LevelStatus, &join, &birth,
			&r.MGTMB, &r.BankAcnt, &r.No, &r.Type, &r.Description, &amt, &basis, &r.Remarks, &r.Division,
			&r.HasSetup, &r.Main, &r.Sub, &r.DrCr, &r.SetupDesc); err != nil {
			return nil, fmt.Errorf("scan reco detail: %w", err)
		}
		if r.Amount, err = decimal.NewFromString(amt); err != nil {
			return nil, fmt.Errorf("reco amount: %w", err)
		}
		if basis.Valid {
			b, err := decimal.NewFromString(basis.String)
			if err != nil {
				return nil, fmt.Errorf("reco basis: %w", err)
			}
			r.Basis = &b
		}
		if join.Valid {
			r.DateJoin = &join.Time
		}
		if birth.Valid {
			r.BirthDate = &birth.Time
		}
		switch {
		case r.Sub != "":
			r.Akun = r.Main + "-" + r.Sub
		default:
			r.Akun = r.Main
		}
		if r.Type == typePotongan && r.Description == "" {
			r.Description = fmt.Sprint(r.No)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// recoHeader is the F-2 detail header, in order.
var recoHeader = []any{"PM_YYMM", "PM_NIK", "PM_NAMA", "PM_DEPT_CODE", "PM_SECTION", "PM_GRADE", "PM_STATUS", "LEVEL_STATUS",
	"PE_DATE_JOIN", "PE_BIRTH_DATE", "MGTMBKONTRAK", "PM_BANK_ACNT", "PD_NO", "PD_TYPE", "PD_DESCRIPTION", "PD_AMOUNT",
	"MPS_AKUN", "MPS_MAIN", "MPS_SUB", "MPS_DRCR", "MPS_DESCRIPTION", "EARNDEDUC", "PD_BASIS_AMOUNT", "PD_REMARKS", "PD_DIVISION"}

func optStr(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func optDate(t *time.Time) any {
	if t == nil {
		return nil
	}
	return *t
}

// WriteReco writes the workbook (detail, pra-cek, Ringkasan) and returns the detail row count (NIK × component).
// The workbook holds real NIKs, salaries, and bank accounts: never commit it.
func WriteReco(d RecoData, w io.Writer) (int, error) {
	f := excelize.NewFile()
	defer f.Close() //nolint:errcheck // in-memory workbook
	if err := f.SetSheetName("Sheet1", SheetRecoDetail); err != nil {
		return 0, fmt.Errorf("rename sheet: %w", err)
	}
	for _, n := range []string{SheetTransfer, SheetAdditional, SheetReg, SheetOvertime, SheetPreCheck, SheetSummary} {
		if _, err := f.NewSheet(n); err != nil {
			return 0, fmt.Errorf("new sheet: %w", err)
		}
	}
	sw := &sheetWriter{f: f}
	sw.recoDetail(d)
	sw.transfer(d)
	sw.additional(d)
	sw.reg(d)
	sw.overtime(d)
	sw.preCheck(d)
	sw.summary(d.RunData)
	if sw.err != nil {
		return 0, sw.err
	}
	if err := sw.dateColumns(SheetRecoDetail, "I", "J"); err != nil {
		return 0, err
	}
	if err := f.Write(w); err != nil {
		return 0, err
	}
	return len(d.Rows), nil
}

func (s *sheetWriter) dateColumns(sheet string, cols ...string) error {
	layout := "dd-mmm-yy"
	st, err := s.f.NewStyle(&excelize.Style{CustomNumFmt: &layout})
	if err != nil {
		return err
	}
	for _, c := range cols {
		if err := s.f.SetColStyle(sheet, c, st); err != nil {
			return err
		}
	}
	return nil
}

func (s *sheetWriter) recoDetail(d RecoData) {
	s.start(SheetRecoDetail)
	s.put(recoHeader...)
	for _, r := range d.Rows {
		var basis any
		if r.Basis != nil {
			basis = *r.Basis
		}
		s.put(d.YYMM, r.NIK, r.Nama, optStr(r.DeptCode), optStr(r.Section), optStr(r.Grade), optStr(r.Status), optStr(r.LevelStatus),
			optDate(r.DateJoin), optDate(r.BirthDate), optStr(r.MGTMB), optStr(r.BankAcnt), r.No, r.Type, r.Description, r.Amount,
			optStr(r.Akun), optStr(r.Main), optStr(r.Sub), optStr(r.DrCr), optStr(r.MPSDescription(d.YYMM)), r.EarnDeduc(),
			basis, optStr(r.Remarks), r.Division)
	}
}

func (s *sheetWriter) preCheck(d RecoData) {
	s.start(SheetPreCheck)
	s.put("PD_TYPE", "PD_NO", "PD_DESCRIPTION", "Baris tanpa setup GL")
	for _, p := range PreChecks(d.Rows) {
		s.put(p.Type, p.No, p.Name, p.Lines)
	}
	s.transferPreCheck(d)
}

const typePotongan = "Potongan"
