package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/shopspring/decimal"

	"github.com/mutugading/goapps-backend/services/payroll/internal/application/recon"
	"github.com/mutugading/goapps-backend/services/payroll/internal/engine"
)

func TestDiffCmd_UsageErrors(t *testing.T) {
	t.Setenv("PAYROLL_RO_DSN", "")
	for _, args := range [][]string{
		{},
		{"-yymm", "202608", "-division", "SOLO"},
		{"-yymm", "202608", "-division", "SOLO", "-dsn", "x", "-target", "run:abc"},
		{"-bogus"},
	} {
		var out, errb bytes.Buffer
		if code := diffCmd(args, &out, &errb); code != exitError || out.Len() != 0 {
			t.Errorf("%v: exit %d out %q", args, code, out.String())
		}
	}
}

func TestWriteReportAndSummary(t *testing.T) {
	amt := decimal.NewFromInt(10)
	rep := recon.Diff(recon.DiffInput{
		Meta:   recon.DiffMeta{Division: "SOLO", YYMM: 202608, Target: recon.TargetDry},
		Legacy: map[string]recon.DiffEmp{"N": {Lines: []recon.DiffLine{{Key: engine.CompKey{Type: engine.CompT, ID: 1}, Amount: &amt}}}},
		Target: map[string]recon.DiffEmp{"N": {}},
	})
	var out bytes.Buffer
	if err := writeReport(rep, "", &out); err != nil {
		t.Fatal(err)
	}
	var back map[string]any
	if err := json.Unmarshal(out.Bytes(), &back); err != nil {
		t.Fatal(err)
	}
	sum, ok := back["summary"].(map[string]any)
	if !ok || back["division"] != "SOLO" || sum["parity"] != false {
		t.Fatalf("json %s", out.String())
	}
	var s bytes.Buffer
	printSummary(&s, rep)
	if !strings.Contains(s.String(), "ONLY_LEGACY   1") || !strings.Contains(s.String(), "parity=false") || !strings.Contains(s.String(), "gagal paritas: ONLY_LEGACY=1") {
		t.Fatalf("summary %s", s.String())
	}
}

// D-49: the CLI summary names the ONLY_* failure only when it happens, and ONLY_* alone fails parity.
func TestPrintSummary_OnlyRowsFailure(t *testing.T) {
	line := func(id int, amt string) recon.DiffLine {
		a := decimal.RequireFromString(amt)
		return recon.DiffLine{Key: engine.CompKey{Type: engine.CompT, ID: id}, Amount: &a}
	}
	run := func(target []recon.DiffLine) (recon.DiffReport, string) {
		rep := recon.Diff(recon.DiffInput{
			Meta:   recon.DiffMeta{Division: "SOLO", YYMM: 202610, Target: recon.TargetDry},
			Legacy: map[string]recon.DiffEmp{"N": {Lines: []recon.DiffLine{line(1, "100")}}},
			Target: map[string]recon.DiffEmp{"N": {Lines: target}},
		})
		var b bytes.Buffer
		printSummary(&b, rep)
		return rep, b.String()
	}
	rep, out := run([]recon.DiffLine{line(1, "100"), line(2, "5")})
	if rep.Summary.Parity || !strings.Contains(out, "gagal paritas: ONLY_LEGACY=0 ONLY_TARGET=1") {
		t.Errorf("only-target must fail and be named: %s", out)
	}
	rep, out = run([]recon.DiffLine{line(1, "100")})
	if !rep.Summary.Parity || strings.Contains(out, "gagal paritas") {
		t.Errorf("clean run must not name a failure: %s", out)
	}
}
