package pdf

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"
)

func d(s string) decimal.Decimal { return decimal.RequireFromString(s) }

func syntheticLogo(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	for x := 0; x < 8; x++ {
		for y := 0; y < 8; y++ {
			img.Set(x, y, color.RGBA{R: 200, G: 30, B: 30, A: 255})
		}
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func sample() Slip {
	return Slip{Company: "PT Contoh Sintetis", PeriodName: "Januari 2027", Period: "Jan - 27", NIK: "Z0001", Nama: "Budi",
		Dept: "D01 Weaving", Grade: "G5", Jabatan: "Operator", TaxCode: "K1", NPWP: "99.888.777.6-555.000", BankAccount: "7654321098",
		PayDate: "26-JAN-27",
		Tunjangan: []Line{{Name: "Gaji Pokok", Amount: d("1000000"), Basis: "1000000", Remarks: "pokok"}, {Name: "NolTunj", Amount: d("0")},
			{Name: "Rate Kecil", Amount: d("500"), Basis: "0.0089"}},
		Potongan: []Line{{Name: "BPJS", Amount: d("50000"), Basis: "5000000", Remarks: "1% x basis"}, {Name: "NolPot", Amount: d("0")},
			{Name: "Subsidi PPh", Amount: d("-2500")}},
		TotTunjangan: d("1000500"), TotPotongan: d("47500"), NetPay: d("953000"),
		Info:       Attendance{Valid: true, Working: d("21"), Off: d("4"), Leave: d("1"), Alpa: d("0"), Ijin: d("2")},
		Perhatian:  []string{"Rahasia."},
		ContactIT:  "it@example.test",
		ApprovedAt: time.Date(2027, 1, 20, 8, 0, 0, 0, time.UTC)}
}

func TestRenderDeterministicAndZeroLines(t *testing.T) {
	s := sample()
	s.Logo = syntheticLogo(t)
	a, err := Render(s)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := Render(s)
	if ContentHash(a) != ContentHash(b) {
		t.Fatal("hash isi tidak stabil antar render")
	}
	if !bytes.Contains(a, []byte("Gaji Pokok")) || bytes.Contains(a, []byte("NolTunj")) || bytes.Contains(a, []byte("NolPot")) {
		t.Fatal("baris 0 tercetak atau baris nonzero hilang")
	}
	if !bytes.Contains(a, []byte("Budi / Z0001")) {
		t.Fatal("watermark nama+NIK hilang")
	}
	if !bytes.Contains(a, []byte("-2.500,00")) {
		t.Fatal("subsidi PPh negatif tidak tercetak")
	}
	if !bytes.HasPrefix(a, []byte("%PDF")) || !bytes.Contains(a, []byte("/MediaBox [0 0 841.89 595.28]")) {
		t.Fatal("bukan A4 landscape")
	}
	if n := bytes.Count(a, []byte("/Type /Page\n")); n != 1 {
		t.Fatalf("halaman = %d, harus 1", n)
	}
}

func TestRenderF3Fields(t *testing.T) {
	a, err := Render(sample())
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"99.888.777.6-555.000", "7654321098", "Jan - 27", "26-JAN-27", "D01 Weaving", "K1", "G5",
		"0.0089", "1.000.000", "PENGHASILAN", "POTONGAN", "Total Penghasilan", "Total Potongan", "Rounding", "Net Paid",
		"Perhatian", "Informasi", "Working", "Alpa", "Ijin", "it@example.test", "1% x basis"} {
		if !bytes.Contains(a, []byte(want)) {
			t.Errorf("tidak ada di PDF: %q", want)
		}
	}
}

func TestRenderNoLogoTextOnly(t *testing.T) {
	a, err := Render(sample()) // Logo kosong
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(a, []byte("/Subtype /Image")) {
		t.Fatal("tanpa logo tidak boleh ada gambar")
	}
	s := sample()
	s.Logo = syntheticLogo(t)
	b, err := Render(s)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(b, []byte("/Subtype /Image")) {
		t.Fatal("logo tidak masuk PDF")
	}
}

func TestRenderLongRemarksOnePageKeepsAmount(t *testing.T) {
	s := sample()
	s.Tunjangan = nil
	long := strings.Repeat("keterangan panjang sekali ", 20)
	for i := 0; i < 16; i++ {
		s.Tunjangan = append(s.Tunjangan, Line{Name: "Komponen sintetis", Amount: d("12345.67"),
			Basis: "1000000", Remarks: long})
	}
	a, err := Render(s)
	if err != nil {
		t.Fatal(err)
	}
	if n := bytes.Count(a, []byte("/Type /Page\n")); n != 1 {
		t.Fatalf("halaman = %d, harus 1", n)
	}
	if got := bytes.Count(a, []byte("12.345,67")); got != 16 {
		t.Fatalf("amount tercetak %d kali, harus 16 (tidak terpotong)", got)
	}
}

func TestRenderRemarksMultiline(t *testing.T) {
	s := sample()
	s.Tunjangan = []Line{{Name: "Gaji Pokok", Amount: d("1000"), Remarks: "baris satu\nbaris dua"}}
	a, err := Render(s)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(a, []byte("baris satu")) || !bytes.Contains(a, []byte("baris dua")) {
		t.Fatal("remarks multi-baris hilang")
	}
}

func TestBasisText(t *testing.T) {
	for in, want := range map[string]string{"": "", "0.0089": "0.0089", "1234567.00": "1.234.567", "0": "0", "-5000": "-5.000", "x": ""} {
		if got := BasisText(in); got != want {
			t.Errorf("%q: %q != %q", in, got, want)
		}
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
	if bytes.Contains(enc, []byte("Gaji Pokok")) || bytes.Contains(enc, []byte("7654321098")) || !bytes.Contains(enc, []byte("/Encrypt")) {
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
}

func TestRenderTooManyLinesFails(t *testing.T) {
	s := sample()
	s.Tunjangan, s.Potongan = nil, nil
	for i := 0; i < 40; i++ {
		s.Tunjangan = append(s.Tunjangan, Line{Name: "Komponen sintetis tunjangan", Amount: d("1000"), Remarks: "x"})
		s.Potongan = append(s.Potongan, Line{Name: "Komponen sintetis potongan", Amount: d("1000"), Remarks: "x"})
	}
	_, err := Render(s)
	if !errors.Is(err, ErrTooManyLines) {
		t.Fatalf("err = %v, harus ErrTooManyLines", err)
	}
	if strings.Contains(err.Error(), "Komponen") || strings.Contains(err.Error(), "Z0001") {
		t.Fatal("pesan error memuat data")
	}
}

func TestRenderLongNPWPNeverCut(t *testing.T) {
	s := sample()
	s.NPWP = strings.Repeat("9", 80)
	if _, err := Render(s); err == nil {
		t.Fatal("NPWP yang tidak muat harus gagal, bukan terpotong")
	}
	s = sample()
	s.Jabatan = strings.Repeat("Jabatan panjang ", 10) // non-strict cells end with an ellipsis
	a, err := Render(s)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(a, []byte("...")) {
		t.Fatal("teks terpotong tanpa tanda")
	}
}
