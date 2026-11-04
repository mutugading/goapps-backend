// Package golden tests the golden-case extractor (T018a) with a synthetic source: no database, no real salaries.
package golden

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/mutugading/goapps-backend/services/payroll/internal/application/recon"
	"github.com/mutugading/goapps-backend/services/payroll/internal/engine"
)

type fakeSource struct{ byDiv map[string][]recon.Candidate }

func (f fakeSource) Candidates(_ context.Context, q recon.SourceQuery) ([]recon.Candidate, error) {
	return f.byDiv[q.Division], nil
}

func d(s string) decimal.Decimal { return decimal.RequireFromString(s) }

func cand(div, nik, level, sts string, dirumahkan string, premiFail int, join time.Time, formulas ...string) recon.Candidate {
	period := engine.Period{YYMM: 202608, Start: time.Date(2026, 7, 26, 0, 0, 0, 0, time.UTC), End: time.Date(2026, 8, 25, 0, 0, 0, 0, time.UTC)}
	birth := time.Date(1990, 1, 2, 0, 0, 0, 0, time.UTC)
	days := 26
	in := engine.Input{
		Period: period, Division: div, Workdays: &days,
		Emp:        engine.Employee{NIK: nik, Name: "Nama Rahasia " + nik, Level: level, GapokSts: sts, BirthDate: &birth, JoinDate: &join},
		Components: []engine.EmpComponent{{Key: engine.CompKey{Type: engine.CompT, ID: 1}, Amount: d("3000000"), Sts: sts}},
		Att:        engine.AttRekap{"day_work": d("24"), "dirumahkan": d(dirumahkan)},
		Additions: map[engine.CompKey][]engine.Addition{
			{Type: engine.CompP, ID: 32}: {{Amount: d("250000.75"), Remark: "cicil rahasia", Source: "LOAN"}}},
		Daily: engine.DailyFails{PremiFail: premiFail},
	}
	amt := "3000000"
	c := recon.Candidate{Division: div, Input: in, Formulas: map[string]bool{},
		Expected: recon.Expected{Lines: []recon.ExpectedLine{{Type: "T", ID: 1, Amount: &amt}, {Type: "T", ID: 40}},
			Head: &recon.ExpectedHead{TotTunjangan: "3000000", TotPotongan: "0", Rounding: "0"}}}
	for _, f := range formulas {
		c.Formulas[f] = true
	}
	return c
}

func extract(t *testing.T, src recon.Source, per int) (string, recon.Report) {
	t.Helper()
	out := t.TempDir()
	rep, err := recon.ExtractGolden(context.Background(), src, recon.Options{CompanyID: 1, Divisions: []string{"SOLO", "JAKARTA"},
		YYMM: 202608, RuleSetID: 1, PerFormula: per, Out: out})
	if err != nil {
		t.Fatal(err)
	}
	return out, rep
}

func fixture() fakeSource {
	old := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	newer := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	return fakeSource{byDiv: map[string][]recon.Candidate{
		"SOLO": {
			cand("SOLO", "1001", "Operator Shift I,II,III", "F", "0", 0, old, "PRORATE_30", "OT"),
			cand("SOLO", "1002", "Driver", "D", "2", 3, newer, "PRORATE_30"),
			cand("SOLO", "1003", "Driver", "D", "0", 0, old, "PRORATE_30"),
		},
		"JAKARTA": {
			cand("JAKARTA", "2001", "Executive", "F", "0", 0, old, "PRORATE_30", "OT"),
			cand("JAKARTA", "2002", "Staff HO", "D", "1", 1, old, "OT"),
		},
	}}
}

func TestExtractGolden_FilesAndNoPII(t *testing.T) {
	out, rep := extract(t, fixture(), 20)
	if rep.Candidates != 5 || rep.PerFormula["PRORATE_30"] != 4 || rep.PerFormula["OT"] != 3 {
		t.Fatalf("report %+v", rep)
	}
	files, _ := filepath.Glob(filepath.Join(out, "*", "*.json"))
	if len(files) != 7 {
		t.Fatalf("berkas %d, want 7", len(files))
	}
	for _, f := range files {
		b, err := os.ReadFile(f) //nolint:gosec // test reads its temp dir
		if err != nil {
			t.Fatal(err)
		}
		s := string(b)
		for _, bad := range []string{"Nama Rahasia", "cicil rahasia", "1990-01-02", `"1001"`, `"2001"`, `"2002"`, `"1002"`, `"1003"`} {
			if strings.Contains(s, bad) {
				t.Fatalf("%s memuat PII %q", f, bad)
			}
		}
		if base := filepath.Base(f); strings.HasPrefix(base, "1") || strings.HasPrefix(base, "2") {
			t.Fatalf("nama berkas memuat NIK: %s", base)
		}
		var c recon.Case
		if err := json.Unmarshal(b, &c); err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(c.Input.Emp.NIK, "E") || c.Input.Emp.Name != "" || c.Input.Emp.BirthDate != nil {
			t.Fatalf("%s: identitas tidak diganti: %+v", f, c.Input.Emp)
		}
		if want := fmt.Sprintf("%s_%d.json", c.Input.Emp.NIK, 202608); filepath.Base(f) != want {
			t.Fatalf("nama berkas %s, want %s", filepath.Base(f), want)
		}
		if len(c.Expected.Lines) != 2 || c.Expected.Lines[1].Amount != nil {
			t.Fatalf("expected lines %+v (NULL legacy harus tetap null)", c.Expected.Lines)
		}
	}
}

func TestExtractGolden_RoundTripToInput(t *testing.T) {
	out, _ := extract(t, fixture(), 1)
	files, _ := filepath.Glob(filepath.Join(out, "OT", "*.json"))
	if len(files) != 1 {
		t.Fatalf("berkas OT %d", len(files))
	}
	b, _ := os.ReadFile(files[0]) //nolint:gosec // temp dir
	var c recon.Case
	if err := json.Unmarshal(b, &c); err != nil {
		t.Fatal(err)
	}
	in, err := c.Input.ToInput(nil, engine.TaxTables{})
	if err != nil {
		t.Fatal(err)
	}
	adds := in.Additions[engine.CompKey{Type: engine.CompP, ID: 32}]
	if len(adds) != 1 || !adds[0].Amount.Equal(d("250000.75")) || adds[0].Remark != "" {
		t.Fatalf("additions %+v", adds)
	}
	if in.Workdays == nil || *in.Workdays != 26 || !in.Att.Get("day_work").Equal(d("24")) {
		t.Fatalf("input %+v", in)
	}
}

func TestExtractGolden_SelectCoversVariation(t *testing.T) {
	// per-formula 2 of PRORATE_30 must pick the most different employees, not the first two.
	_, rep := extract(t, fixture(), 2)
	if rep.PerFormula["PRORATE_30"] != 2 || rep.PerFormula["OT"] != 2 {
		t.Fatalf("report %+v", rep.PerFormula)
	}
	picked := recon.Select(flatten(fixture()), "PRORATE_30", 2)
	seen := map[string]bool{}
	for _, f := range recon.Features(picked[0]) {
		seen[f] = true
	}
	gain := 0
	for _, f := range recon.Features(picked[1]) {
		if !seen[f] {
			gain++
		}
	}
	if gain < 3 {
		t.Fatalf("kasus kedua hanya menambah %d kelas variasi: %s, %s", gain, picked[0].NIK(), picked[1].NIK())
	}
}

func TestExtractGolden_BadOptions(t *testing.T) {
	_, err := recon.ExtractGolden(context.Background(), fixture(), recon.Options{YYMM: 202613, PerFormula: 1, Out: t.TempDir(), Divisions: []string{"SOLO"}})
	if err == nil {
		t.Fatal("yymm tidak valid harus ditolak")
	}
}

func flatten(s fakeSource) []recon.Candidate {
	return append(append([]recon.Candidate(nil), s.byDiv["SOLO"]...), s.byDiv["JAKARTA"]...)
}
