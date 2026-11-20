package run

import (
	"testing"
	"time"

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

// T033m: the resign date and the YTD run months feed the tax v2 job cost cap and the leaver true-up, so they are
// part of the input fingerprint (a change after commit makes approve see the run as stale).
func TestHashInputsIncludesTaxV2Fields(t *testing.T) {
	base := hashInputs(syntheticInputs(2))
	ins := syntheticInputs(2)
	r := time.Date(2026, 6, 10, 0, 0, 0, 0, time.UTC)
	ins[1].Emp.ResignDate = &r
	if hashInputs(ins) == base {
		t.Error("ResignDate must change the input hash")
	}
	ins = syntheticInputs(2)
	ins[0].YTD.Months = 7
	if hashInputs(ins) == base {
		t.Error("YTD.Months must change the input hash")
	}
}
