package pdf

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"
)

func sample() Slip {
	d := decimal.RequireFromString
	return Slip{Company: "MGT", PeriodName: "Januari 2027", Division: "SOLO", NIK: "Z0001", Nama: "Budi", Dept: "PPC",
		Tunjangan:    []Line{{"Gaji Pokok", d("1000000")}, {"NolTunj", d("0")}},
		Potongan:     []Line{{"BPJS", d("50000")}, {"NolPot", d("0")}},
		Overtime:     []Line{{"Lembur 1", d("0")}},
		TotTunjangan: d("1000000"), TotPotongan: d("50000"), NetPay: d("950000"),
		ApprovedAt: time.Date(2027, 1, 20, 8, 0, 0, 0, time.UTC)}
}

func TestRenderDeterministicAndZeroLines(t *testing.T) {
	a, err := Render(sample())
	if err != nil {
		t.Fatal(err)
	}
	b, _ := Render(sample())
	if ContentHash(a) != ContentHash(b) {
		t.Fatal("hash isi tidak stabil antar render")
	}
	if !bytes.Contains(a, []byte("Gaji Pokok")) || bytes.Contains(a, []byte("NolTunj")) || bytes.Contains(a, []byte("NolPot")) || bytes.Contains(a, []byte("Lembur 1")) {
		t.Fatal("baris 0 tercetak atau baris nonzero hilang")
	}
	if !bytes.Contains(a, []byte("Budi / Z0001")) {
		t.Fatal("watermark nama+NIK hilang")
	}
}

func TestEncryptPassword(t *testing.T) {
	plain, _ := Render(sample())
	dob := time.Date(1990, 3, 7, 0, 0, 0, 0, time.UTC)
	pw, err := Password(PolicyDOB, &dob)
	if err != nil || pw != "07031990" {
		t.Fatalf("pw=%q err=%v", pw, err)
	}
	enc, err := Encrypt(plain, pw)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(enc, []byte("Gaji Pokok")) || !bytes.Contains(enc, []byte("/Encrypt")) {
		t.Fatal("PDF tidak terenkripsi")
	}
	if err := Validate(enc, pw); err != nil {
		t.Fatalf("password benar gagal: %v", err)
	}
	if err := Validate(enc, "01011990"); err == nil {
		t.Fatal("password salah lolos")
	}
}

func TestPasswordNoDOB(t *testing.T) {
	if _, err := Password(PolicyDOB, nil); err == nil {
		t.Fatal("harus gagal")
	}
	if _, err := Password("NIK_DOB", nil); err == nil {
		t.Fatal("kebijakan lain harus ditolak")
	}
}

func TestMoney(t *testing.T) {
	for in, want := range map[string]string{"1234567.5": "1.234.567,50", "0": "0,00", "-1000": "-1.000,00", "999.999": "1.000,00"} {
		if got := Money(decimal.RequireFromString(in)); got != want {
			t.Errorf("%s: %s != %s", in, got, want)
		}
	}
	_ = strings.TrimSpace
}
