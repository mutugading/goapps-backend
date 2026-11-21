package etl

import (
	"fmt"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/mutugading/goapps-backend/services/payroll/internal/infrastructure/etl"
)

// T021d1: jam WIB dari Oracle harus tersimpan sebagai instan yang benar di
// payroll_stg.pyr_stg_att_daily.psad_check_in/out (timestamptz).
func TestAttSync_DailyCheckInOutStoredAsWIB(t *testing.T) {
	const ym = 203808
	wib := time.FixedZone("Asia/Jakarta", 7*3600)
	in := time.Date(2038, 8, 10, 6, 0, 0, 0, wib)  // 06:00 WIB = 2038-08-09 23:00Z
	out := time.Date(2038, 8, 11, 1, 5, 0, 0, wib) // shift malam, lewat tengah malam
	hours := decimal.NullDecimal{Decimal: decimal.RequireFromString("8"), Valid: true}
	f := &fakeAtt{daily: []etl.DailyRow{
		{WorkDate: time.Date(2038, 8, 10, 0, 0, 0, 0, time.UTC), NIK: "TZ1", Division: "SOLO", ScheduleCode: "N", CheckIn: &in, CheckOut: &out, WorkHours: hours},
		{WorkDate: time.Date(2038, 8, 12, 0, 0, 0, 0, time.UTC), NIK: "TZ1", Division: "SOLO", ScheduleCode: "N", WorkHours: hours},
	}}
	q := attQ(ym, "SOLO")
	q.Source = etl.AttSrcDaily
	run1(t, f, q)

	q1 := func(col, date string) string {
		return scalar(t, fmt.Sprintf(`SELECT coalesce(%s, 'null') FROM payroll_stg.pyr_stg_att_daily WHERE psad_nik='TZ1' AND psad_work_date='%s'`, col, date))
	}
	if got := q1(`to_char(psad_check_in AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"')`, "2038-08-10"); got != "2038-08-09T23:00:00Z" {
		t.Fatalf("check_in instant = %s", got)
	}
	if got := q1(`to_char(psad_check_in AT TIME ZONE 'Asia/Jakarta', 'YYYY-MM-DD HH24:MI')`, "2038-08-10"); got != "2038-08-10 06:00" {
		t.Fatalf("check_in WIB = %s", got)
	}
	if got := q1(`to_char(psad_check_out AT TIME ZONE 'Asia/Jakarta', 'YYYY-MM-DD HH24:MI')`, "2038-08-10"); got != "2038-08-11 01:05" {
		t.Fatalf("check_out WIB = %s", got)
	}
	if got := q1(`psad_check_in::text`, "2038-08-12"); got != "null" {
		t.Fatalf("null check_in = %s", got)
	}
}
