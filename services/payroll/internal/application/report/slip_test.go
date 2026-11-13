package report

import (
	"testing"

	"github.com/shopspring/decimal"
)

func TestBuildOvertime(t *testing.T) {
	d := decimal.RequireFromString
	jam := map[string]decimal.Decimal{"lembur1": d("2.5"), "nas1": d("1"), "nas2": d("2"), "nas3": d("0.5")}
	rate := d("10001")
	m := func(level string) map[string]decimal.Decimal {
		out := map[string]decimal.Decimal{}
		for _, l := range BuildOvertime(jam, rate, level) {
			out[l.Name] = l.Amount
		}
		return out
	}
	ex := m("Executive")
	if !ex["Lembur NAS (Executive)"].Equal(d("70007")) { // (1+2+0.5)*2*10001
		t.Fatalf("executive NAS: %v", ex["Lembur NAS (Executive)"])
	}
	if _, ok := ex["Lembur NAS 1"]; ok {
		t.Fatal("executive tidak boleh punya baris NAS per kolom")
	}
	if !ex["Lembur 1"].Equal(d("25002.5")) { // round(2.5*10001, 2)
		t.Fatalf("lembur1: %v", ex["Lembur 1"])
	}
	n := m("Staff")
	if !n["Lembur NAS 2"].Equal(d("20002")) || !n["Lembur NAS 3"].Equal(d("5000.5")) {
		t.Fatalf("non-executive: %+v", n)
	}
}

func TestRoundMoneyHalfUp(t *testing.T) {
	if got := roundMoney(decimal.RequireFromString("0.125")); !got.Equal(decimal.RequireFromString("0.13")) {
		t.Fatalf("half-up: %v", got)
	}
}
