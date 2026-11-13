package oracle

import (
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/mutugading/goapps-backend/services/payroll/internal/infrastructure/config"
	"github.com/mutugading/goapps-backend/services/payroll/internal/infrastructure/etl"
)

type fakeScanner struct{ vals []any }

func (f fakeScanner) Scan(dest ...any) error {
	if len(dest) != len(f.vals) {
		return errors.New("column count mismatch")
	}
	for i, d := range dest {
		switch p := d.(type) {
		case *sql.NullInt64:
			*p = f.vals[i].(sql.NullInt64)
		case *sql.NullString:
			*p = f.vals[i].(sql.NullString)
		case *sql.NullTime:
			*p = f.vals[i].(sql.NullTime)
		default:
			return errors.New("unexpected dest")
		}
	}
	return nil
}

func ns(s string) sql.NullString { return sql.NullString{String: s, Valid: true} }

func TestParseNum(t *testing.T) {
	tests := []struct {
		in    sql.NullString
		valid bool
		want  string
		err   bool
	}{
		{ns("25"), true, "25", false},
		{ns(".5"), true, "0.5", false},
		{ns("-.25"), true, "-0.25", false},
		{ns("3.75"), true, "3.75", false},
		{ns("1234567.89"), true, "1234567.89", false},
		{sql.NullString{}, false, "", false},
		{ns("  "), false, "", false},
		{ns("1,5"), false, "", true},
		{ns("abc"), false, "", true},
	}
	for _, tt := range tests {
		got, err := parseNum(tt.in)
		if (err != nil) != tt.err || got.Valid != tt.valid {
			t.Fatalf("parseNum(%q) = %v, %v", tt.in.String, got, err)
		}
		if tt.valid && got.Decimal.String() != tt.want {
			t.Fatalf("parseNum(%q) = %s, want %s", tt.in.String, got.Decimal, tt.want)
		}
	}
}

func TestScanRekap(t *testing.T) {
	cols := []string{"day_work", "lembur_1", "shift"}
	f := fakeScanner{vals: []any{sql.NullInt64{Int64: 202610, Valid: true}, ns("1234"), ns("Budi"), ns("SOLO"),
		ns("25"), ns(".5"), sql.NullString{}}}
	r, err := scanRekap(f, cols)
	if err != nil {
		t.Fatal(err)
	}
	if r.Yymm != 202610 || r.NIK != "1234" || r.Division != "SOLO" {
		t.Fatalf("bad row %+v", r)
	}
	if r.Nums["lembur_1"].Decimal.String() != "0.5" || r.Nums["shift"].Valid {
		t.Fatalf("bad nums %+v", r.Nums)
	}
	f.vals[4] = ns("x")
	if _, err := scanRekap(f, cols); err == nil {
		t.Fatal("want error for non-numeric")
	}
}

func TestScanDailyDateAndNull(t *testing.T) {
	d := time.Date(2026, 10, 3, 0, 0, 0, 0, time.UTC)
	in := time.Date(2026, 10, 3, 7, 1, 0, 0, time.UTC)
	f := fakeScanner{vals: []any{sql.NullTime{Time: d, Valid: true}, ns("99"), ns("1234"), ns("SOLO"), ns("P"),
		sql.NullTime{Time: in, Valid: true}, sql.NullTime{}, ns("7.5"), ns("Shift"), sql.NullString{}}}
	r, err := scanDaily(f)
	if err != nil {
		t.Fatal(err)
	}
	if !r.WorkDate.Equal(d) || r.CheckIn == nil || !r.CheckIn.Equal(time.Date(2026, 10, 3, 7, 1, 0, 0, wibZone)) || r.CheckOut != nil || r.WorkHours.Decimal.String() != "7.5" ||
		r.SchType != "Shift" || r.OtInwork != "" {
		t.Fatalf("bad daily %+v", r)
	}
}

func TestScanSch(t *testing.T) {
	f := fakeScanner{vals: []any{sql.NullInt64{Int64: 202610, Valid: true}, ns("1234"), ns("NW"), ns("SOLO"), ns("4")}}
	r, err := scanSch(f)
	if err != nil || r.Schedule != "NW" || r.NoSch.Decimal.String() != "4" {
		t.Fatalf("bad sch %+v %v", r, err)
	}
}

func TestQueriesUseBindsAndColumnCounts(t *testing.T) {
	for name, q := range map[string]string{"rekap": sqlRekap, "add": sqlRekapAdd, "sch": sqlSchRekap, "daily": sqlDaily} {
		if !strings.Contains(q, ":1") || !strings.Contains(q, ":2") {
			t.Errorf("%s: missing bind variables", name)
		}
		if strings.Contains(strings.ToUpper(q), "FLOAT") {
			t.Errorf("%s: float", name)
		}
	}
	if !strings.Contains(sqlDaily, "LEFT   JOIN mgthris.hm_schedule s ON s.hmschd_schedule = a.hata_hmschd_schedule") ||
		!strings.Contains(sqlDaily, "s.hmschd_ot_inwork") {
		t.Error("daily schedule join (05§5)")
	}
	if !strings.Contains(sqlDaily, ":3") || !strings.Contains(sqlDaily, "hata_date BETWEEN :1 AND :2") {
		t.Error("daily binds")
	}
	if !strings.Contains(sqlRekapAdd, "MPARA_SHIFT") || strings.Contains(sqlRekapAdd, "MPARA_DAYOFF") {
		t.Error("add columns")
	}
	if len(etl.RekapAddNumCols) != 20 || etl.RekapAddNumCols[19] != "shift" {
		t.Error("RekapAddNumCols must end at shift")
	}
}

func TestNewClientRequiresConfig(t *testing.T) {
	if _, err := NewClient(config.OracleSyncConfig{}); err == nil {
		t.Fatal("want error when not configured")
	}
}

func TestWibClock_EdgeCases(t *testing.T) {
	jkt := time.FixedZone("x", 7*3600)
	cases := []struct {
		name string
		in   sql.NullTime
		want *time.Time
	}{
		{"null", sql.NullTime{}, nil},
		{"utc tag", sql.NullTime{Time: time.Date(2026, 10, 3, 6, 0, 0, 0, time.UTC), Valid: true}, ptrTime(time.Date(2026, 10, 3, 6, 0, 0, 0, jkt))},
		{"local tag", sql.NullTime{Time: time.Date(2026, 10, 3, 6, 0, 0, 0, time.Local), Valid: true}, ptrTime(time.Date(2026, 10, 3, 6, 0, 0, 0, jkt))},
		{"other zone tag", sql.NullTime{Time: time.Date(2026, 10, 3, 6, 0, 0, 0, time.FixedZone("y", -5*3600)), Valid: true}, ptrTime(time.Date(2026, 10, 3, 6, 0, 0, 0, jkt))},
		{"midnight", sql.NullTime{Time: time.Date(2026, 10, 3, 0, 30, 0, 0, time.UTC), Valid: true}, ptrTime(time.Date(2026, 10, 3, 0, 30, 0, 0, jkt))},
		{"checkout past midnight", sql.NullTime{Time: time.Date(2026, 10, 4, 1, 5, 7, 123, time.UTC), Valid: true}, ptrTime(time.Date(2026, 10, 4, 1, 5, 7, 123, jkt))},
	}
	for _, c := range cases {
		got := wibClock(c.in)
		if c.want == nil {
			if got != nil {
				t.Errorf("%s: want nil, got %v", c.name, got)
			}
			continue
		}
		if got == nil || !got.Equal(*c.want) {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}

func TestScanDaily_CheckOutValidIsWIB(t *testing.T) {
	d := time.Date(2026, 10, 3, 0, 0, 0, 0, time.UTC)
	out := time.Date(2026, 10, 4, 1, 5, 0, 0, time.UTC)
	f := fakeScanner{vals: []any{sql.NullTime{Time: d, Valid: true}, ns("99"), ns("1234"), ns("SOLO"), ns("N"),
		sql.NullTime{}, sql.NullTime{Time: out, Valid: true}, ns("8"), ns("Shift"), sql.NullString{}}}
	r, err := scanDaily(f)
	if err != nil {
		t.Fatal(err)
	}
	if r.CheckOut == nil || !r.CheckOut.Equal(time.Date(2026, 10, 4, 1, 5, 0, 0, wibZone)) || r.CheckIn != nil {
		t.Fatalf("bad checkout %+v", r)
	}
}

func ptrTime(t time.Time) *time.Time { return &t }
