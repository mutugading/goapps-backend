package oracle

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	"github.com/mutugading/goapps-backend/services/payroll/internal/infrastructure/etl"
)

// num renders NUMBER as locale-independent text ('.' decimal, no exponent).
func num(col string) string {
	return "TO_CHAR(" + col + ", 'TM9', 'NLS_NUMERIC_CHARACTERS=''.,''')"
}

func rekapSQL(table, prefix string, cols []string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "SELECT %sYYMM, %sNIK, %sNAMA, %sDIVISION", prefix, prefix, prefix, prefix)
	for _, c := range cols {
		b.WriteString(", " + num(prefix+strings.ToUpper(c)))
	}
	fmt.Fprintf(&b, " FROM %s WHERE %sYYMM = :1 AND %sDIVISION = :2 ORDER BY %sNIK", table, prefix, prefix, prefix)
	return b.String()
}

var (
	// S1: VIEW_ATT_REKAP_HM (MPAR_*).
	sqlRekap = rekapSQL("mgthris.view_att_rekap_hm", "MPAR_", etl.RekapNumCols)
	// S3: MGT_PAYROLL_ATT_REKAP_ADD (MPARA_*; only day_work..shift).
	sqlRekapAdd = rekapSQL("mgtdat.mgt_payroll_att_rekap_add", "MPARA_", etl.RekapAddNumCols)
	// S4: MGT_PAYROLL_SCH_REKAP.
	sqlSchRekap = "SELECT MPSR_YYMM, MPSR_NIK, MPSR_SCHEDULE, MPSR_DIVISION, " + num("MPSR_NO_SCH") +
		" FROM mgtdat.mgt_payroll_sch_rekap WHERE MPSR_YYMM = :1 AND MPSR_DIVISION = :2 ORDER BY MPSR_NIK, MPSR_SCHEDULE"
	// S2: 05§2 verbatim, except work_hours as text and an ORDER BY for stable batches.
	sqlDaily = `SELECT a.hata_date AS work_date,
       e.hmemd_sys_id AS hris_sys_id,
       e.hmemd_nik AS nik,
       dv.hmdiv_div_name AS division,
       a.hata_hmschd_schedule AS schedule_code,
       a.hata_actual_in AS check_in,
       a.hata_actual_out AS check_out,
       ` + num("a.hata_actual_no_of_hour") + ` AS work_hours
FROM   mgthris.hm_att_actual a
JOIN   mgthris.hm_emp_data   e  ON e.hmemd_sys_id  = a.hata_hmemd_sys_id
JOIN   mgthris.hm_division   dv ON dv.hmdiv_div_id = e.hmemd_hmdiv_div_id
WHERE  a.hata_date BETWEEN :1 AND :2
AND    dv.hmdiv_div_name = :3
ORDER  BY e.hmemd_nik, a.hata_date`
)

// AttSource implements etl.AttReader over Oracle.
type AttSource struct{ c *Client }

var _ etl.AttReader = (*AttSource)(nil)

// NewAttSource returns the reader for client c.
func NewAttSource(c *Client) *AttSource { return &AttSource{c: c} }

func (s *AttSource) query(ctx context.Context, q string, args ...any) (*sql.Rows, context.CancelFunc, error) {
	ctx, cancel := context.WithTimeout(ctx, QueryTimeout)
	rows, err := s.c.db.QueryContext(ctx, q, args...)
	if err != nil {
		cancel()
		return nil, nil, fmt.Errorf("oracle query: %w", err)
	}
	return rows, cancel, nil
}

// Rekap reads S1 for one period/division.
func (s *AttSource) Rekap(ctx context.Context, yymm int, division string) ([]etl.RekapRow, error) {
	return s.rekap(ctx, sqlRekap, etl.RekapNumCols, yymm, division)
}

// RekapAdd reads S3 (phase 0–2 only).
func (s *AttSource) RekapAdd(ctx context.Context, yymm int, division string) ([]etl.RekapRow, error) {
	return s.rekap(ctx, sqlRekapAdd, etl.RekapAddNumCols, yymm, division)
}

func (s *AttSource) rekap(ctx context.Context, q string, cols []string, yymm int, division string) ([]etl.RekapRow, error) {
	rows, cancel, err := s.query(ctx, q, yymm, division)
	if err != nil {
		return nil, err
	}
	defer cancel()
	defer rows.Close() //nolint:errcheck // read-only cursor
	var out []etl.RekapRow
	for rows.Next() {
		r, err := scanRekap(rows, cols)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// SchRekap reads S4 (phase 0–2 only).
func (s *AttSource) SchRekap(ctx context.Context, yymm int, division string) ([]etl.SchRekapRow, error) {
	rows, cancel, err := s.query(ctx, sqlSchRekap, yymm, division)
	if err != nil {
		return nil, err
	}
	defer cancel()
	defer rows.Close() //nolint:errcheck // read-only cursor
	var out []etl.SchRekapRow
	for rows.Next() {
		r, err := scanSch(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// Daily reads S2 for work dates start..end (inclusive) of one division.
func (s *AttSource) Daily(ctx context.Context, start, end time.Time, division string) ([]etl.DailyRow, error) {
	rows, cancel, err := s.query(ctx, sqlDaily, start, end, division)
	if err != nil {
		return nil, err
	}
	defer cancel()
	defer rows.Close() //nolint:errcheck // read-only cursor
	var out []etl.DailyRow
	for rows.Next() {
		r, err := scanDaily(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// scanner is the part of *sql.Rows the row mappers need (fakeable in tests).
type scanner interface{ Scan(dest ...any) error }

func scanRekap(sc scanner, cols []string) (etl.RekapRow, error) {
	var (
		yymm          sql.NullInt64
		nik, nama, dv sql.NullString
	)
	nums := make([]sql.NullString, len(cols))
	dest := []any{&yymm, &nik, &nama, &dv}
	for i := range nums {
		dest = append(dest, &nums[i])
	}
	if err := sc.Scan(dest...); err != nil {
		return etl.RekapRow{}, fmt.Errorf("scan rekap: %w", err)
	}
	r := etl.RekapRow{Yymm: int(yymm.Int64), NIK: nik.String, Nama: nama.String, Division: dv.String,
		Nums: make(map[string]decimal.NullDecimal, len(cols))}
	for i, c := range cols {
		d, err := parseNum(nums[i])
		if err != nil {
			return etl.RekapRow{}, fmt.Errorf("rekap %s nik %s: %w", c, r.NIK, err)
		}
		r.Nums[c] = d
	}
	return r, nil
}

func scanSch(sc scanner) (etl.SchRekapRow, error) {
	var (
		yymm              sql.NullInt64
		nik, sch, dv, num sql.NullString
	)
	if err := sc.Scan(&yymm, &nik, &sch, &dv, &num); err != nil {
		return etl.SchRekapRow{}, fmt.Errorf("scan sch rekap: %w", err)
	}
	d, err := parseNum(num)
	if err != nil {
		return etl.SchRekapRow{}, fmt.Errorf("sch rekap nik %s: %w", nik.String, err)
	}
	return etl.SchRekapRow{Yymm: int(yymm.Int64), NIK: nik.String, Schedule: sch.String, Division: dv.String, NoSch: d}, nil
}

func scanDaily(sc scanner) (etl.DailyRow, error) {
	var (
		date, in, out            sql.NullTime
		sys, nik, dv, sch, hours sql.NullString
	)
	if err := sc.Scan(&date, &sys, &nik, &dv, &sch, &in, &out, &hours); err != nil {
		return etl.DailyRow{}, fmt.Errorf("scan daily: %w", err)
	}
	h, err := parseNum(hours)
	if err != nil {
		return etl.DailyRow{}, fmt.Errorf("daily nik %s: %w", nik.String, err)
	}
	return etl.DailyRow{WorkDate: date.Time, HRISSysID: sys.String, NIK: nik.String, Division: dv.String,
		ScheduleCode: sch.String, CheckIn: nullTime(in), CheckOut: nullTime(out), WorkHours: h}, nil
}

func nullTime(t sql.NullTime) *time.Time {
	if !t.Valid {
		return nil
	}
	v := t.Time
	return &v
}

// parseNum maps Oracle NUMBER text to decimal; NULL/blank stays invalid (not zero).
func parseNum(s sql.NullString) (decimal.NullDecimal, error) {
	t := strings.TrimSpace(s.String)
	if !s.Valid || t == "" {
		return decimal.NullDecimal{}, nil
	}
	if strings.HasPrefix(t, ".") { // TM9 renders 0.5 as ".5"
		t = "0" + t
	} else if strings.HasPrefix(t, "-.") {
		t = "-0" + t[1:]
	}
	d, err := decimal.NewFromString(t)
	if err != nil {
		return decimal.NullDecimal{}, fmt.Errorf("invalid NUMBER %q: %w", s.String, err)
	}
	return decimal.NullDecimal{Decimal: d, Valid: true}, nil
}
