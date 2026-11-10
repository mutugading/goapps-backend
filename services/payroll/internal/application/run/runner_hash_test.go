package run

import (
	"testing"

	"github.com/shopspring/decimal"

	"github.com/mutugading/goapps-backend/services/payroll/internal/engine"
)

type constRules struct {
	stubRules
	c map[string]decimal.Decimal
}

func (r constRules) Constants() map[string]decimal.Decimal { return r.c }

func TestHashInputsIncludesRuleConstants(t *testing.T) {
	mk := func(v int64) []engine.Input {
		ins := syntheticInputs(2)
		for i := range ins {
			ins[i].Rules = constRules{c: map[string]decimal.Decimal{"DAYS_MONTH": decimal.NewFromInt(v)}}
		}
		return ins
	}
	a, b, a2 := hashInputs(mk(25)), hashInputs(mk(26)), hashInputs(mk(25))
	if a == b {
		t.Fatal("changing a rule constant must change the input hash")
	}
	if a != a2 {
		t.Fatal("hash must be deterministic")
	}
}
