package report

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/shopspring/decimal"

	"github.com/mutugading/goapps-backend/services/payroll/internal/infrastructure/pdf"
)

func TestRoundMoneyHalfUp(t *testing.T) {
	if got := roundMoney(decimal.RequireFromString("0.125")); !got.Equal(decimal.RequireFromString("0.13")) {
		t.Fatalf("half-up: %v", got)
	}
}

func TestPeriodHeaderAndPayDate(t *testing.T) {
	// F-3: "Mon - YY" and "DD-MON-YY" use English abbreviations (Oracle default NLS)
	if got := periodHeader(202612); got != "Dec - 26" {
		t.Fatalf("period Des: %q", got)
	}
	dec := time.Date(2026, 12, 26, 0, 0, 0, 0, time.UTC)
	if got := payDateText(&dec); got != "26-DEC-26" {
		t.Fatalf("paydate Des: %q", got)
	}
	if got := periodHeader(202701); got != "Jan - 27" {
		t.Fatalf("period: %q", got)
	}
	tm := time.Date(2027, 1, 26, 0, 0, 0, 0, time.UTC)
	if got := payDateText(&tm); got != "26-JAN-27" {
		t.Fatalf("paydate: %q", got)
	}
	if payDateText(nil) != "" {
		t.Fatal("nil harus kosong")
	}
}

func TestBuildSlipOrderBasisRemarksZero(t *testing.T) {
	d := RunData{Comps: []Comp{{Type: "P", ID: 2, Name: "BPJS"}, {Type: "T", ID: 1, Name: "Gaji"}, {Type: "T", ID: 3, Name: "Nol"}}}
	m := decimal.RequireFromString
	e := Emp{NIK: "Z1", Nama: "Budi", Amounts: map[string]decimal.Decimal{"P2": m("-5"), "T1": m("100"), "T3": m("0")}}
	det := map[string]slipDetail{"T1": {Basis: "0.0089", Remarks: "rem"}}
	s := BuildSlip(d, e, slipExtra{Dept: "D01 X", Currency: "IDR", NPWP: "1", Info: pdfAtt(true)}, det, SlipConfig{}, "PT", 202701, "Januari 2027", time.Unix(0, 0))
	if len(s.Tunjangan) != 1 || s.Tunjangan[0].Basis != "0.0089" || s.Tunjangan[0].Remarks != "rem" {
		t.Fatalf("tunjangan %+v", s.Tunjangan)
	}
	if len(s.Potongan) != 1 || !s.Potongan[0].Amount.Equal(m("-5")) {
		t.Fatalf("potongan (subsidi negatif tetap dicetak) %+v", s.Potongan)
	}
	if len(s.Perhatian) == 0 || s.Period != "Jan - 27" || s.NPWP != "1" {
		t.Fatalf("slip %+v", s)
	}
}

func TestSanitizeErrHidesKey(t *testing.T) {
	got := sanitizeErr(errors.New("boom secret-key-123 end"), "secret-key-123")
	if strings.Contains(got, "secret-key-123") {
		t.Fatalf("kunci bocor: %s", got)
	}
}

func TestSanitizeErrShortKey(t *testing.T) {
	// a short key would also mangle unrelated text, so the whole message is withheld instead of substituted
	got := sanitizeErr(errors.New("column ab missing: abab"), "ab")
	if strings.Contains(got, "ab") && !strings.Contains(got, "disembunyikan") {
		t.Fatalf("kunci pendek bocor atau pesan rusak: %s", got)
	}
	if got := sanitizeErr(errors.New("no secret here"), "ab"); got != "no secret here" {
		t.Fatalf("pesan tanpa kunci harus utuh: %s", got)
	}
	if got := sanitizeErr(errors.New("x"), ""); got != "x" {
		t.Fatal("tanpa kunci pesan harus utuh")
	}
}

func TestSlipConfigFromEnvTrimsKey(t *testing.T) {
	c, err := SlipConfigFromEnv("  key-with-newline\r\n")
	if err != nil || c.PIIKey != "key-with-newline" {
		t.Fatalf("kunci tidak di-trim: %q %v", c.PIIKey, err)
	}
	if _, err := SlipConfigFromEnv(" \n"); !errors.Is(err, ErrNoPIIKey) {
		t.Fatalf("kunci kosong harus ErrNoPIIKey: %v", err)
	}
}

func pdfAtt(valid bool) pdf.Attendance { return pdf.Attendance{Valid: valid} }

func TestRedactKeepsChainAndHidesKey(t *testing.T) {
	orig := fmt.Errorf("query: %w", &pgconn.PgError{Code: "40001", Message: "could not serialize secret-key-123"})
	err := redact("read slip extra", orig, "secret-key-123")
	var pe *pgconn.PgError
	if !errors.As(err, &pe) || pe.Code != "40001" {
		t.Fatalf("errors.As ke PgError hilang: %v", err)
	}
	if strings.Contains(err.Error(), "secret-key-123") {
		t.Fatalf("kunci bocor: %s", err)
	}
	if !errors.Is(redact("x", context.Canceled, "k"), context.Canceled) {
		t.Fatal("errors.Is context.Canceled hilang")
	}
}
