package recon

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/shopspring/decimal"
)

func d(s string) decimal.Decimal { return decimal.RequireFromString(s) }

func TestBuild_SumsByHand(t *testing.T) {
	r := raw{
		divisions: []string{"JAKARTA", "SOLO"},
		detail: []detailRow{
			{202601, "SOLO", "T", d("1000.10"), 2}, {202601, "SOLO", "P", d("200.05"), 1},
			{202601, "JAKARTA", "T", d("500.00"), 1}, {202602, "SOLO", "T", d("0.20"), 1},
		},
		head: []headRow{
			{202601, "SOLO", d("1000.10"), d("200.05"), d("800.05"), 1},
			{202601, "JAKARTA", d("500.00"), d("0"), d("500.00"), 1},
			{202602, "SOLO", d("0.20"), d("0"), d("0.20"), 1},
		},
		levels: []LevelCount{{"SOLO", "Staff", 3}},
	}
	rep := build(Query{CompanyID: 1, YYMMFrom: 202601, YYMMTo: 202602}, r)
	if !rep.OK() || len(rep.Failures) != 0 {
		t.Fatalf("failures %v", rep.Failures)
	}
	if len(rep.Periods) != 3 || rep.Periods[0].Division != "JAKARTA" || rep.Periods[1].Division != "SOLO" || rep.Periods[2].YYMM != 202602 {
		t.Fatalf("periods %+v", rep.Periods)
	}
	p := rep.Periods[1]
	if !p.DetailT.Equal(d("1000.10")) || !p.DetailP.Equal(d("200.05")) || p.DetailLines != 3 || !p.HeadNet.Equal(d("800.05")) {
		t.Fatalf("period %+v", p)
	}
	// 0.1 + 0.2 style sums must be exact: 1000.10 + 500.00 + 0.20 = 1500.30
	if !rep.Totals.DetailT.Equal(d("1500.30")) || !rep.Totals.DetailP.Equal(d("200.05")) ||
		!rep.Totals.HeadTunjangan.Equal(d("1500.30")) || !rep.Totals.HeadPotongan.Equal(d("200.05")) || !rep.Totals.HeadNet.Equal(d("1300.25")) {
		t.Fatalf("totals %+v", rep.Totals)
	}
}

func TestBuild_UnmappedAndPlaintextFail(t *testing.T) {
	r := raw{
		unmapped: []UnmappedComponent{{"T", 99, "SOLO", 4, false}, {"P", 7, "JAKARTA", 1, false}},
		pii:      []PlainPIIColumn{{"payroll", "pyr_employee", "pe_ktp_no", "character varying"}},
	}
	rep := build(Query{CompanyID: 1, YYMMFrom: 202601, YYMMTo: 202601}, r)
	if rep.OK() || rep.UnmappedLegacyLines != 5 || len(rep.Failures) != 2 {
		t.Fatalf("ok=%v unmapped=%d failures=%v", rep.OK(), rep.UnmappedLegacyLines, rep.Failures)
	}
}

func TestReport_NoPerPersonFields(t *testing.T) {
	b, err := json.Marshal(build(Query{CompanyID: 1, YYMMFrom: 1, YYMMTo: 1}, raw{}))
	if err != nil {
		t.Fatal(err)
	}
	for _, banned := range []string{"nik", "nama", "name"} {
		if strings.Contains(strings.ToLower(string(b)), `"`+banned+`"`) {
			t.Errorf("report has %q field: %s", banned, b)
		}
	}
	if !strings.Contains(string(b), `"failures":[]`) || !strings.Contains(string(b), `"periods":[]`) {
		t.Errorf("empty lists must be [] not null: %s", b)
	}
}

func TestValidate_BadQuery(t *testing.T) {
	for _, q := range []Query{{}, {CompanyID: 1, YYMMFrom: 202602, YYMMTo: 202601}} {
		if _, err := Validate(nil, nil, q); err == nil { //nolint:staticcheck // rejected before any use of ctx/db
			t.Errorf("%+v: want error", q)
		}
	}
}
