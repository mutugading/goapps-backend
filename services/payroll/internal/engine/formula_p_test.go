package engine

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func pc(id int, amt, sts string) EmpComponent {
	return EmpComponent{Key: CompKey{CompP, id}, Amount: d(amt), Sts: sts}
}

// keyLine runs Calculate and returns the line of k.
func keyLine(t *testing.T, in Input, k CompKey) Line {
	t.Helper()
	res, err := Calculate(in)
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range res.Lines {
		if l.Key == k {
			return l
		}
	}
	t.Fatalf("no line %s", k)
	return Line{}
}

// Golden cases computed by hand from FNC_MGT_PAYROLL_COMPNT_CALC (D-27); 09§3 golden from DETAIL follows in B-GOLD.
func TestPFormulas(t *testing.T) {
	att := AttRekap{"pay_tot": d("26"), "alpa": d("1"), "ijin": d("1"), "dirumahkan": d("2")}
	addP := func(id int, amt string) map[CompKey][]Addition {
		return map[CompKey][]Addition{{CompP, id}: {{Amount: d(amt)}}}
	}
	jkt := func(in *Input) { in.Division = "JAKARTA"; in.Workdays = intp(22) }
	cases := []struct {
		name string
		mod  func(*Input)
		comp EmpComponent
		want string
	}{
		{"P2 ADDITION F", func(in *Input) { in.Additions = addP(2, "150000") }, pc(2, "0", "F"), "150000"},
		{"P9 ADDITION D", func(in *Input) { in.Additions = addP(9, "300000") }, pc(9, "0", "D"), "260000"},
		{"P71 ADDITION none → NULL", nil, pc(71, "0", "F"), ""},
		{"P19 ADDITION", func(in *Input) { in.Additions = addP(19, "12345") }, pc(19, "0", "F"), "12345"},
		{"P4 F → A", nil, pc(4, "75000", "F"), "75000"},
		// G6/D12: SOLO 3000000/30 × 2, JAKARTA 3000000/22 × 2 = 272727.27…
		{"P4 SOLO /DM", nil, pc(4, "0", "D"), "200000"},
		{"P4 JAKARTA /WD (G6)", jkt, pc(4, "0", "D"), "272727"},
		{"P4 SOLO STS(T1) D → NULL", func(in *Input) { in.Components[0].Sts = "D" }, pc(4, "0", "D"), ""},
		{"P4 JAKARTA STS(T1) D → 0", func(in *Input) { jkt(in); in.Components[0].Sts = "D" }, pc(4, "0", "D"), "0"},
		{"P4 SOLO no rekap → NULL", func(in *Input) { in.Att = nil }, pc(4, "0", "D"), ""},
		{"P4 JAKARTA no rekap → 0", func(in *Input) { jkt(in); in.Att = nil }, pc(4, "0", "D"), "0"},
		{"P4 JKT code, lower case", func(in *Input) { jkt(in); in.Division = "jkt" }, pc(4, "0", "D"), "272727"},
		{"P4 solo lower case", func(in *Input) { in.Division = "solo" }, pc(4, "0", "D"), "200000"},
		{"P4 other division → NULL", func(in *Input) { in.Division = "SURABAYA" }, pc(4, "0", "D"), ""},
		{"P4 other STS → NULL", nil, pc(4, "75000", "X"), ""},
		// BPJS (D-25 shared with T): TETAP 3000000 ≥ NAKMIN → × rate
		{"P13 NAKER F", nil, pc(13, "0.02", "F"), "60000"},
		{"P14 NAKER below min", func(in *Input) { in.Components[0].Amount = d("2000000") }, pc(14, "0.02", "F"), "51843"}, // 2592154×0.02
		// TETAP_D = 3000000/30×26 + 2×3000000/30/2 = 2700000
		{"P11 NAKER D", nil, pc(11, "0.0024", "D"), "6480"},
		{"P16 KES clamp max", func(in *Input) { in.Components[0].Amount = d("15000000") }, pc(16, "0.01", "F"), "120000"},
		// G7: January uses JAN_BASE (T901) instead of TETAP; 1800000 < KESMIN → KESMIN × 1%
		{"P15 KES January JAN_BASE (G7)", func(in *Input) {
			in.Period.YYMM = 202601
			in.Components = append(in.Components, tc(901, "1800000", "F"))
		}, pc(15, "0.04", "F"), "103686"}, // 2592154×0.04 = 103686.16
		{"P18 PENSIUN D", nil, pc(18, "0.01", "D"), "27000"},
		{"P17 PENSIUN clamp max", func(in *Input) { in.Components[0].Amount = d("20000000") }, pc(17, "0.02", "F"), "221726"},
		{"P24 PRORATE D", nil, pc(24, "12345", "D"), "10699"}, // 12345/30×26 = 10699
		{"P73 FIXED F", nil, pc(73, "50000", "F"), "50000"},
		{"P8 no legacy branch → NULL", nil, pc(8, "50000", "F"), ""},
		{"P10 no legacy branch → NULL", nil, pc(10, "50000", "F"), ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			in := tInput(att, c.comp)
			if c.mod != nil {
				c.mod(&in)
			}
			checkAmt(t, c.name, keyLine(t, in, c.comp.Key), c.want)
		})
	}
}

func TestPAllMapped(t *testing.T) {
	for id := 0; id <= 999; id++ {
		if _, ok := specFor(CompKey{CompP, id}); !ok {
			t.Fatalf("P%03d unmapped", id)
		}
	}
}

func TestP4JakartaNoWorkdays(t *testing.T) {
	in := tInput(AttRekap{"alpa": d("1")}, pc(4, "0", "D"))
	in.Division = "JAKARTA"
	if _, err := Calculate(in); !errors.Is(err, ErrNoWorkdays) {
		t.Fatalf("err = %v, want ErrNoWorkdays", err)
	}
}

func TestLoanDue(t *testing.T) {
	loans := []LoanDue{
		{LoanID: 7, Seq: 3, Principal: d("500000"), Interest: d("50000"), Remark: "Cicilan Pinjaman LN/SOLO/2026/0001 Ke 3 Dari 10"},
		{LoanID: 9, Seq: 1, Principal: d("200000"), Interest: d("0")},
	}
	// No P32/P33 component rows: LOAN_DUE still produces both lines, summed over loans.
	in := tInput(AttRekap{"pay_tot": d("26")})
	in.Loans = loans
	l32 := keyLine(t, in, CompKey{CompP, 32})
	checkAmt(t, "P32", l32, "700000")
	checkAmt(t, "P33", keyLine(t, in, CompKey{CompP, 33}), "50000")
	if !strings.Contains(l32.Remark, "LN/SOLO/2026/0001") || !strings.Contains(l32.Remark, "Pinjaman 9 ke-1") {
		t.Errorf("remark = %q", l32.Remark)
	}
	// Manual addition (Q-L2) is added to LOAN_DUE; a D component prorates only the addition part.
	in.Components = append(in.Components, pc(32, "0", "D"))
	in.Additions = map[CompKey][]Addition{{CompP, 32}: {{Amount: d("300000"), Source: "MANUAL"}}}
	checkAmt(t, "P32 manual+due", keyLine(t, in, CompKey{CompP, 32}), "960000") // 300000/30×26 + 700000
	// P33 with STS D: the LOAN_DUE interest is not prorated.
	in.Components = append(in.Components, pc(33, "0", "D"))
	checkAmt(t, "P33 D due", keyLine(t, in, CompKey{CompP, 33}), "50000")
	in.Components = in.Components[:len(in.Components)-1]
	// Migrated LOAN addition together with LOAN_DUE is rejected.
	in.Additions = map[CompKey][]Addition{{CompP, 33}: {{Amount: d("1"), Source: "loan"}}}
	if _, err := Calculate(in); !errors.Is(err, ErrLoanDouble) {
		t.Fatalf("err = %v, want ErrLoanDouble", err)
	}
	// Without LOAN_DUE a LOAN addition is the legacy v1 path (needs the component row, like legacy).
	in.Loans = nil
	in.Components = append(in.Components, pc(33, "0", "F"))
	checkAmt(t, "P33 legacy addition", keyLine(t, in, CompKey{CompP, 33}), "1")
}

func TestLoanNetNegativeWarning(t *testing.T) {
	in := tInput(AttRekap{"pay_tot": d("26")})
	in.Loans = []LoanDue{{LoanID: 1, Seq: 1, Principal: d("3000040")}} // net −40 rounds to 0 but still warns
	res, err := Calculate(in)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Warnings) != 1 || !strings.HasPrefix(res.Warnings[0], WarnLoanNetNegative) {
		t.Errorf("warnings = %v", res.Warnings)
	}
}

// Header of 06§7 / legacy PRC_SALARY_MGT for a SOLO staff with T and P lines.
func TestHeaderMain(t *testing.T) {
	in := tInput(AttRekap{"pay_tot": d("26"), "day_work": d("24"), "alpa": d("1"), "ijin": d("1"),
		"lembur_1": d("4"), "lembur_nas_2": d("2")},
		tc(2, "500000", "F"), tc(7, "250000", "F"), tc(55, "10000", "F"),
		pc(4, "0", "D"), pc(13, "0.02", "F"), pc(24, "12345", "D"),
	)
	in.Additions = map[CompKey][]Addition{{CompP, 9}: {{Amount: d("123456")}}}
	in.Components = append(in.Components, pc(9, "0", "F"))
	res, err := Calculate(in)
	if err != nil {
		t.Fatal(err)
	}
	h := res.Header
	if h == nil {
		t.Fatal("header nil")
	}
	// T: 3000000 + 500000 + 250000 + 10000 = 3760000
	// P: P4 200000 + P9 123456 + P13 (TETAP 3760000×0.02) 75200 + P24 10699 = 409355
	// net 3350645 → take_home 3350600, rounding 45
	for name, p := range map[string][2]string{
		"totT": {h.TotTunjangan.String(), "3760000"}, "totP": {h.TotPotongan.String(), "409355"},
		"takeHome": {h.TakeHome.String(), "3350600"}, "rounding": {h.Rounding.String(), "45"},
		"basic": {h.BasicPay.String(), "3000000"}, "jabatan": {h.TunjJabatan.String(), "500000"},
		"kesejahteraan": {h.TunjKesejahteraan.String(), "250000"},
		// (3000000 + 250000 + 10000)/173, unrounded
		"rate": {h.RatePerJam.StringFixed(6), "18843.930636"},
		"l1":   {h.JamLembur["lembur_1"].String(), "6"}, "nas2": {h.JamLembur["lembur_nas_2"].String(), "6"},
	} {
		if p[0] != p[1] {
			t.Errorf("%s = %s, want %s", name, p[0], p[1])
		}
	}
	if h.TunjRumah != nil || h.TunjTransport != nil {
		t.Errorf("absent T3/T4 must be NULL: %v %v", h.TunjRumah, h.TunjTransport)
	}
	if _, ok := h.JamLembur["lembur_off_1"]; ok {
		t.Error("NULL recap hours must stay absent")
	}
	if !strings.HasPrefix(h.Remarks, "Working : 24 Days, Off : 0 Days, Leave : 0 Days, Alpa : 1 Days, Ijin : 1 Days, Total : 28 Days") {
		t.Errorf("remarks = %q", h.Remarks)
	}
}

func TestHeaderExecutiveAndNullLevel(t *testing.T) {
	in := tInput(AttRekap{"lembur_nas_2": d("2"), "lembur_nas_3": d("1")}, tc(2, "460000", "F"), tc(7, "999", "F"))
	in.Emp.Level = "Executive"
	h, _ := Calculate(in)
	if h.Header.RatePerJam.StringFixed(0) != "20000" || !h.Header.JamLembur["lembur_nas_2"].Equal(d("4")) ||
		!h.Header.JamLembur["lembur_nas_3"].Equal(d("2")) {
		t.Errorf("executive: rate=%s jam=%v", h.Header.RatePerJam, h.Header.JamLembur)
	}
	in.Emp.Level = ""
	if h, _ = Calculate(in); h.Header.RatePerJam != nil {
		t.Errorf("NULL level: rate = %s, want NULL", h.Header.RatePerJam)
	}
}

// Legacy MAIN joins DETAIL, which keeps only non-zero lines and P0: all-zero → no header.
func TestHeaderNeedsDetailRow(t *testing.T) {
	in := tInput(AttRekap{"pay_tot": d("0")})
	in.Components = []EmpComponent{tc(1, "3000000", "D")}
	if res, _ := Calculate(in); res.Header != nil {
		t.Fatal("legacy: all-zero lines must not get a header")
	}
	withP0 := in
	withP0.Components = append([]EmpComponent{}, in.Components...)
	withP0.Components = append(withP0.Components, pComp(0, "0", "F", false))
	res, err := Calculate(withP0)
	if err != nil || res.Header == nil {
		t.Fatalf("a P0 line keeps the header even when every line is zero: %v", err)
	}
	in.Components = append(in.Components, tc(2, "100", "F"))
	if res, _ := Calculate(in); res.Header == nil {
		t.Fatal("a non-zero line keeps the header")
	}
}

func TestBirthdayLine(t *testing.T) {
	bd := func(y, m, day int) *time.Time { v := time.Date(y, time.Month(m), day, 0, 0, 0, 0, time.UTC); return &v }
	cases := []struct {
		yymm int
		bd   *time.Time
		want string
	}{
		{202610, bd(1990, 10, 26), "Selamat Ulang Tahun Ke 36 Tanggal 26 Oktober 2026"},
		{202610, bd(1990, 11, 25), "Selamat Ulang Tahun Ke 36 Tanggal 25 November 2026"},
		{202610, bd(1990, 10, 25), ""},
		{202612, bd(1990, 1, 10), "Selamat Ulang Tahun Ke 37 Tanggal 10 Januari 2027"},
		{202702, bd(2000, 2, 29), "Selamat Ulang Tahun Ke 27 Tanggal 29 Februari 2027"},
		{202610, nil, ""},
		{202612, bd(1990, 1, 26), ""}, // December window ends 25/01 of the next year
	}
	for _, c := range cases {
		if got := birthdayLine(Period{YYMM: c.yymm}, c.bd); got != c.want {
			t.Errorf("%d %v: %q, want %q", c.yymm, c.bd, got, c.want)
		}
	}
}

// Header across the year boundary: the birthday window is read from YYMM (D-27), so a December
// run mentions a January birthday of the next year and a January run uses JAN period/year.
func TestHeaderDecemberJanuary(t *testing.T) {
	bd := func(y, m, day int) *time.Time { v := time.Date(y, time.Month(m), day, 0, 0, 0, 0, time.UTC); return &v }
	cases := []struct {
		name string
		yymm int
		bd   *time.Time
		want string // empty: no birthday line
	}{
		{"december, january birthday", 202612, bd(1990, 1, 10), "Selamat Ulang Tahun Ke 37 Tanggal 10 Januari 2027"},
		{"december, own month in window", 202612, bd(1990, 12, 30), "Selamat Ulang Tahun Ke 36 Tanggal 30 Desember 2026"},
		{"december, before window", 202612, bd(1990, 12, 25), ""},
		{"january, january birthday after 25", 202701, bd(1990, 1, 26), "Selamat Ulang Tahun Ke 37 Tanggal 26 Januari 2027"},
		{"january, january birthday before 26", 202701, bd(1990, 1, 10), ""},
		{"january, february birthday", 202701, bd(1990, 2, 20), "Selamat Ulang Tahun Ke 37 Tanggal 20 Februari 2027"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			in := tInput(AttRekap{"pay_tot": d("26")})
			in.Period.YYMM = c.yymm
			in.Emp.BirthDate = c.bd
			res, err := Calculate(in)
			if err != nil {
				t.Fatal(err)
			}
			if res.Header == nil {
				t.Fatal("header nil")
			}
			got := ""
			if i := strings.Index(res.Header.Remarks, "Selamat"); i >= 0 {
				got = res.Header.Remarks[i:]
			}
			if got != c.want {
				t.Errorf("birthday = %q, want %q", got, c.want)
			}
		})
	}
}

// P32 with an addition on a D component plus LOAN_DUE feeds tot_potongan and the header
// (only the addition part is prorated, D-27 (c)).
func TestHeaderLoanDueWithDAddition(t *testing.T) {
	in := tInput(AttRekap{"pay_tot": d("26")}, pc(32, "0", "D"))
	in.Additions = map[CompKey][]Addition{{CompP, 32}: {{Amount: d("300000"), Source: "MANUAL"}}}
	in.Loans = []LoanDue{{LoanID: 1, Seq: 1, Principal: d("500000"), Interest: d("50000")}}
	res, err := Calculate(in)
	if err != nil {
		t.Fatal(err)
	}
	// P32 = 300000/30×26 + 500000 = 760000; P33 = 50000 (line without component row)
	checkAmt(t, "P32", keyLine(t, in, CompKey{CompP, 32}), "760000")
	checkAmt(t, "P33", keyLine(t, in, CompKey{CompP, 33}), "50000")
	if res.Header == nil || res.Header.TotPotongan.String() != "810000" {
		t.Fatalf("header = %+v", res.Header)
	}
	// 3000000 − 810000 = 2190000
	if res.Header.TakeHome.String() != "2190000" {
		t.Errorf("take_home = %s", res.Header.TakeHome)
	}
}
