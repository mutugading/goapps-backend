package etl

import (
	"testing"
	"time"
)

// 06:00 WIB harus tersimpan sebagai instan 23:00Z hari sebelumnya (T021d1).
func TestMapDaily_CheckInInstantIsUTCOfWIBClock(t *testing.T) {
	wib := time.FixedZone("Asia/Jakarta", 7*3600)
	in := time.Date(2026, 10, 3, 6, 0, 0, 0, wib)
	out := time.Date(2026, 10, 3, 14, 0, 0, 0, wib)
	d := time.Date(2026, 10, 3, 0, 0, 0, 0, time.UTC)
	p := mapDaily([]DailyRow{{WorkDate: d, NIK: "1", Division: "SOLO", CheckIn: &in, CheckOut: &out}}, d, d, "SOLO")
	if len(p.rows) != 1 {
		t.Fatalf("rows=%d", len(p.rows))
	}
	r := p.rows[0]
	if r[4] != "2026-10-02T23:00:00Z" || r[5] != "2026-10-03T07:00:00Z" {
		t.Fatalf("check_in/out = %q / %q", r[4], r[5])
	}
}
