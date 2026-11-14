package run

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/mutugading/goapps-backend/services/payroll/internal/engine"
)

type stubRules struct{}

func (stubRules) ID() int                                 { return 1 }
func (stubRules) LegacyCompat() bool                      { return true }
func (stubRules) Constant(string) (decimal.Decimal, bool) { return decimal.Decimal{}, false }

func syntheticInputs(n int) []engine.Input {
	ins := make([]engine.Input, n)
	for i := range ins {
		// descending NIK so the runner must reorder
		nik := fmt.Sprintf("N%05d", n-i)
		ins[i] = engine.Input{
			Period: engine.Period{YYMM: 202603}, Division: "SOLO", Rules: stubRules{},
			Emp:        engine.Employee{NIK: nik},
			Components: []engine.EmpComponent{{Key: engine.CompKey{Type: engine.CompT, ID: 1}, Amount: decimal.NewFromInt(int64(i + 1))}},
		}
	}
	return ins
}

// fakeCalc: T line = 1000, P line = 100; errors and warnings chosen by NIK.
func fakeCalc(in engine.Input) (engine.Result, error) {
	switch in.Emp.NIK {
	case "N00002":
		return engine.Result{}, fmt.Errorf("p4: %w", engine.ErrNoWorkdays)
	case "N00003":
		return engine.Result{}, engine.ErrLoanDouble
	}
	a, b := decimal.NewFromInt(1000), decimal.NewFromInt(100)
	res := engine.Result{NIK: in.Emp.NIK, Lines: []engine.Line{
		{Key: engine.CompKey{Type: engine.CompT, ID: 1}, Amount: &a},
		{Key: engine.CompKey{Type: engine.CompP, ID: 1}, Amount: &b},
		{Key: engine.CompKey{Type: engine.CompP, ID: 2}},
	}}
	if in.Emp.NIK == "N00004" {
		res.Warnings = []string{engine.WarnPPhNoBracket + ": P001 tanpa bracket"}
	}
	return res, nil
}

func TestRunner_DryRun(t *testing.T) {
	r := Runner{Calc: fakeCalc, Workers: 4}
	loaded := Result{Inputs: syntheticInputs(6), Warnings: []Issue{{NIK: "N00001", Code: "THR_NOT_LOADED"}}}
	out, err := r.DryRun(context.Background(), loaded)
	if err != nil {
		t.Fatal(err)
	}
	for i := 1; i < len(out.Results); i++ {
		if out.Results[i-1].NIK >= out.Results[i].NIK {
			t.Fatalf("results not ordered by NIK: %s, %s", out.Results[i-1].NIK, out.Results[i].NIK)
		}
	}
	if out.Summary.EmpCount != 4 || out.Summary.LineCount != 12 ||
		!out.Summary.TotalT.Equal(decimal.NewFromInt(4000)) || !out.Summary.TotalP.Equal(decimal.NewFromInt(400)) {
		t.Fatalf("summary = %+v", out.Summary)
	}
	codes := map[string]string{}
	for _, w := range out.Warnings {
		codes[w.NIK+"/"+w.Code] = w.Message
	}
	for _, want := range []string{"N00001/THR_NOT_LOADED", "N00002/" + CodeNoWorkdays, "N00003/" + CodeLoanDouble, "N00004/" + engine.WarnPPhNoBracket} {
		if _, ok := codes[want]; !ok {
			t.Errorf("missing warning %s in %v", want, out.Warnings)
		}
	}
	if len(out.Warnings) != 4 {
		t.Errorf("warnings = %d, want 4: %v", len(out.Warnings), out.Warnings)
	}
}

func TestRunner_HashDeterministic(t *testing.T) {
	r := Runner{Calc: fakeCalc}
	a, err := r.DryRun(context.Background(), Result{Inputs: syntheticInputs(50)})
	if err != nil {
		t.Fatal(err)
	}
	in := syntheticInputs(50)
	in[0], in[49] = in[49], in[0] // input order must not matter
	b, _ := r.DryRun(context.Background(), Result{Inputs: in})
	if a.InputHash == "" || a.InputHash != b.InputHash {
		t.Fatalf("hash differs: %q vs %q", a.InputHash, b.InputHash)
	}
	in[3].Emp.TaxStatus = "K/1"
	c, _ := r.DryRun(context.Background(), Result{Inputs: in})
	if c.InputHash == a.InputHash {
		t.Fatal("hash must change when an input changes")
	}
}

func TestRunner_Canceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := (Runner{Calc: fakeCalc}).DryRun(ctx, Result{Inputs: syntheticInputs(3)}); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v", err)
	}
}

// BenchmarkRunner_1300 is the DoD: 1.300 synthetic inputs through the real engine in < 2 s.
func BenchmarkRunner_1300(b *testing.B) {
	loaded := Result{Inputs: syntheticInputs(1300)}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		start := time.Now()
		if _, err := NewRunner().DryRun(context.Background(), loaded); err != nil {
			b.Fatal(err)
		}
		if el := time.Since(start); el > 2*time.Second {
			b.Fatalf("1300 inputs took %s", el)
		}
	}
}
