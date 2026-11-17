package recon

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/shopspring/decimal"

	"github.com/mutugading/goapps-backend/services/payroll/internal/infrastructure/etl"
)

func dd(s string) decimal.Decimal { return decimal.RequireFromString(s) }

func TestCompare_MatchAndDiffs(t *testing.T) {
	rep := Report{YYMMFrom: 202601, YYMMTo: 202602, Periods: []PeriodAgg{
		{YYMM: 202601, Division: "SOLO", DetailT: dd("10.00"), DetailP: dd("2"), DetailLines: 3, HeadTunjangan: dd("10"), HeadPotongan: dd("2"), HeadNet: dd("8"), HeadRows: 1},
		{YYMM: 202601, Division: "JAKARTA", DetailT: dd("5"), DetailLines: 1, HeadTunjangan: dd("5"), HeadNet: dd("5"), HeadRows: 1},
		{YYMM: 202602, Division: "SOLO", DetailT: dd("1"), DetailLines: 1, HeadTunjangan: dd("1"), HeadNet: dd("1"), HeadRows: 1},
	}, Failures: []string{}}
	dump := etl.LegacyDump{Periods: []etl.LegacyAgg{
		{YYMM: 202601, Division: "SOLO", DetailT: dd("10"), DetailP: dd("2"), DetailLines: 3, HeadTunj: dd("10"), HeadPot: dd("2"), HeadNet: dd("8"), HeadRows: 1, Synthetic: 1},
		{YYMM: 202601, Division: "JAKARTA", DetailT: dd("5.01"), DetailLines: 1, HeadTunj: dd("5"), HeadNet: dd("5"), HeadRows: 1},
		{YYMM: 202603, Division: "SOLO", DetailT: dd("3"), DetailLines: 1},
	}, Skipped: []etl.LegacySkip{{YYMM: 202512, Division: "JAKARTA", MainRows: 2, DetailRows: 4, DetailT: dd("9")}}}
	c := Compare(rep, dump)
	if c.MatchedPeriods != 1 || len(c.SkippedD351) != 1 || len(c.SyntheticD352) != 1 {
		t.Fatalf("%+v", c)
	}
	var jkt, dbOnly, dumpOnly int
	for _, df := range c.Diffs {
		switch {
		case df.YYMM == 202601 && df.Division == "JAKARTA" && df.Field == FieldDetailT && df.DB == "5.00" && df.Dump == "5.01" && df.Delta == "0.01":
			jkt++
		case df.Where == "db_only" && df.YYMM == 202602:
			dbOnly++
		case df.Where == "dump_only" && df.YYMM == 202603:
			dumpOnly++
		default:
			t.Errorf("unexpected diff %+v", df)
		}
	}
	// db_only 202602/SOLO: detail_t, detail_lines, head_tunj, head_net, head_rows = 5; dump_only 202603: detail_t, detail_lines = 2
	if jkt != 1 || dbOnly != 5 || dumpOnly != 2 || c.OK() {
		t.Errorf("jkt %d dbOnly %d dumpOnly %d: %+v", jkt, dbOnly, dumpOnly, c.Diffs)
	}
}

func TestCompare_Clean(t *testing.T) {
	rep := Report{Periods: []PeriodAgg{{YYMM: 202601, Division: "SOLO", DetailT: dd("1"), DetailLines: 1, HeadTunjangan: dd("1"), HeadNet: dd("1"), HeadRows: 1}}}
	c := Compare(rep, etl.LegacyDump{Periods: []etl.LegacyAgg{{YYMM: 202601, Division: "SOLO", DetailT: dd("1.00"), DetailLines: 1, HeadTunj: dd("1"), HeadNet: dd("1"), HeadRows: 1}}})
	if !c.OK() || c.MatchedPeriods != 1 || c.Diffs == nil {
		t.Fatalf("%+v", c)
	}
}

func TestComparison_NoPerPersonFields(t *testing.T) {
	r := Report{Failures: []string{}}
	r.DumpCompare = &Comparison{Diffs: []Diff{}, SkippedD351: []SkippedPeriod{}, SyntheticD352: []SyntheticPeriod{}}
	b, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	for _, banned := range []string{`"nik"`, `"nama"`, `"name"`} {
		if strings.Contains(strings.ToLower(string(b)), banned) {
			t.Errorf("has %s: %s", banned, b)
		}
	}
	if !strings.Contains(string(b), `"diffs":[]`) {
		t.Errorf("empty diffs must be []: %s", b)
	}
}
