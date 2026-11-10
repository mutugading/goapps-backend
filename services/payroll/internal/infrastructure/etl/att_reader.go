package etl

import (
	"context"
	"time"

	"github.com/shopspring/decimal"
)

// Division values accepted by the attendance sync (D-58 (6)).
const (
	DivSolo    = "SOLO"
	DivJakarta = "JAKARTA"
)

// Numeric MPAR_* column names (lower case, without prefix) of VIEW_ATT_REKAP_HM, in
// pyr_stg_att_rekap_hm order. MGT_PAYROLL_ATT_REKAP_ADD uses the subset up to "shift".
var (
	RekapNumCols = []string{
		"day_work", "lembur_1", "lembur_2",
		"lembur_nas_1", "lembur_nas_2", "lembur_nas_3", "lembur_nas_tot",
		"lembur_off_1", "lembur_off_2", "lembur_off_3", "lembur_off_tot",
		"lembur_nasp_1", "lembur_nasp_2", "lembur_nasp_3", "lembur_nasp_tot",
		"cuti", "alpa", "ijin", "pay_tot", "shift", "dayoff", "dirumahkan",
		"sakit_75", "sakit_50", "sakit_25", "otiw_decr",
	}
	RekapAddNumCols = RekapNumCols[:20] // day_work .. shift (MPARA_*)
)

// RekapRow is one S1/S3 row. Nums holds decimals keyed by RekapNumCols; a NULL column has
// Valid=false (legacy NVL semantics are applied later, not here).
type RekapRow struct {
	Yymm     int
	NIK      string
	Nama     string
	Division string
	Nums     map[string]decimal.NullDecimal
}

// DailyRow is one S2 row (05§2): one day of one employee.
type DailyRow struct {
	WorkDate     time.Time
	HRISSysID    string
	NIK          string
	Division     string
	ScheduleCode string
	CheckIn      *time.Time
	CheckOut     *time.Time
	WorkHours    decimal.NullDecimal
}

// SchRekapRow is one S4 row: number of schedules of one type (NW/NWS) per NIK.
type SchRekapRow struct {
	Yymm     int
	NIK      string
	Schedule string
	Division string
	NoSch    decimal.NullDecimal
}

// AttReader reads the HRIS attendance sources S1–S4 (05§2). Read-only; implemented by
// infrastructure/oracle, faked in tests of the sync use case.
type AttReader interface {
	Rekap(ctx context.Context, yymm int, division string) ([]RekapRow, error)
	Daily(ctx context.Context, start, end time.Time, division string) ([]DailyRow, error)
	RekapAdd(ctx context.Context, yymm int, division string) ([]RekapRow, error)
	SchRekap(ctx context.Context, yymm int, division string) ([]SchRekapRow, error)
}
