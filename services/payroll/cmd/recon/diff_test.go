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
	if !ok || back["division"] != "SOLO" || sum["parity"] != true {
		t.Fatalf("json %s", out.String())
	}
	var s bytes.Buffer
	printSummary(&s, rep)
	if !strings.Contains(s.String(), "ONLY_LEGACY   1") || !strings.Contains(s.String(), "parity=true") {
		t.Fatalf("summary %s", s.String())
	}
}
