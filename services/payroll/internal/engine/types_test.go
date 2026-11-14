package engine

import (
	"errors"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/shopspring/decimal"
)

type stubRules struct{}

func (stubRules) ID() int                                 { return 1 }
func (stubRules) LegacyCompat() bool                      { return true }
func (stubRules) Constant(string) (decimal.Decimal, bool) { return decimal.Zero, false }

func validInput() Input {
	return Input{
		Period: Period{YYMM: 202610},
		Emp:    Employee{NIK: "12345"},
		Components: []EmpComponent{
			{Key: CompKey{CompT, 1}}, {Key: CompKey{CompP, 1}},
		},
		Rules: stubRules{},
	}
}

func TestInputValidate(t *testing.T) {
	cases := []struct {
		name string
		mut  func(*Input)
		want error
	}{
		{"ok", func(*Input) {}, nil},
		{"no rules", func(in *Input) { in.Rules = nil }, ErrNoRules},
		{"bad month", func(in *Input) { in.Period.YYMM = 202613 }, ErrBadPeriod},
		{"no nik", func(in *Input) { in.Emp.NIK = "" }, ErrNoNIK},
		{"bad type", func(in *Input) { in.Components[0].Key.Type = "X" }, ErrBadComponentType},
		{"dup", func(in *Input) { in.Components[1].Key = CompKey{CompT, 1} }, ErrDupComponent},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			in := validInput()
			c.mut(&in)
			if err := in.Validate(); !errors.Is(err, c.want) {
				t.Fatalf("Validate() = %v, want %v", err, c.want)
			}
		})
	}
}

func TestPeriodFlags(t *testing.T) {
	if p := (Period{YYMM: 202601}); !p.IsJanuary() || p.IsDecember() || p.Year() != 2026 {
		t.Fatalf("202601 flags wrong: %+v", p)
	}
	if p := (Period{YYMM: 202612}); !p.IsDecember() || p.Month() != 12 {
		t.Fatalf("202612 flags wrong: %+v", p)
	}
}

func TestSortLines_TBeforeP(t *testing.T) {
	lines := []Line{
		{Key: CompKey{CompP, 1}}, {Key: CompKey{CompT, 40}}, {Key: CompKey{CompP, 0}}, {Key: CompKey{CompT, 1}},
	}
	SortLines(lines)
	var got []string
	for _, l := range lines {
		got = append(got, l.Key.String())
	}
	if s := strings.Join(got, ","); s != "T001,T040,P000,P001" {
		t.Fatalf("order = %s", s)
	}
}

func TestLineAmountOrZero(t *testing.T) {
	if !(Line{}).AmountOrZero().IsZero() {
		t.Fatal("NULL amount must count as zero")
	}
	v := decimal.NewFromInt(7)
	if !(Line{Amount: &v}).AmountOrZero().Equal(v) {
		t.Fatal("amount lost")
	}
}

func TestAttRekapGetMissingIsZero(t *testing.T) {
	r := AttRekap{"pay_tot": decimal.NewFromInt(23)}
	if !r.Get("alpa").IsZero() || !r.Get("pay_tot").Equal(decimal.NewFromInt(23)) {
		t.Fatal("Get mismatch")
	}
	var nilRekap AttRekap
	if !nilRekap.Get("pay_tot").IsZero() {
		t.Fatal("nil rekap must read as zero")
	}
}

// D-03: engine and its sub-packages may import only stdlib, decimal and expr-lang.
func TestEngineImportsArePure(t *testing.T) {
	allowed := []string{"github.com/shopspring/decimal", "github.com/expr-lang/expr"}
	self := "github.com/mutugading/goapps-backend/services/payroll/internal/engine"
	fset := token.NewFileSet()
	err := filepath.WalkDir(".", func(path string, de fs.DirEntry, err error) error {
		if err != nil || de.IsDir() || !strings.HasSuffix(path, ".go") {
			return err
		}
		f, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, imp := range f.Imports {
			p, _ := strconv.Unquote(imp.Path.Value)
			if !strings.Contains(strings.SplitN(p, "/", 2)[0], ".") || strings.HasPrefix(p, self) {
				continue // stdlib or engine itself
			}
			ok := false
			for _, a := range allowed {
				ok = ok || p == a || strings.HasPrefix(p, a+"/")
			}
			if !ok {
				t.Errorf("%s imports forbidden package %q", path, p)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestInputHasRekap_NilVersusEmpty(t *testing.T) {
	in := validInput()
	if in.HasRekap() {
		t.Fatal("nil Att must mean no recap (D2)")
	}
	in.Att = AttRekap{}
	if !in.HasRekap() {
		t.Fatal("empty non-nil Att must count as a recap")
	}
}
