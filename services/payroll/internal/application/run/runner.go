package run

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io"
	"reflect"
	"runtime"
	"sort"
	"strings"

	"github.com/shopspring/decimal"
	"golang.org/x/sync/errgroup"

	"github.com/mutugading/goapps-backend/services/payroll/internal/engine"
)

// Per-NIK warning codes raised by the runner (D-27: a per-NIK engine error is a warning, not a failed run).
const (
	CodeNoWorkdays  = "NO_WORKDAYS"
	CodeLoanDouble  = "LOAN_DOUBLE"
	CodeCalcError   = "CALC_ERROR"
	CodeEngineWarn  = "ENGINE_WARNING"
	CodeIncomplete  = "INCOMPLETE_RESULT"
	warnCodeMaxLen  = 40
	hashSharedLabel = "shared"
)

// Summary is the dry-run totals (03§3 step 5). Totals sum the rounded line amounts (NULL = 0).
type Summary struct {
	EmpCount  int // employees with a calculated result
	LineCount int
	TotalT    decimal.Decimal // Σ Tunjangan lines
	TotalP    decimal.Decimal // Σ Potongan lines
}

// DryRunResult is the outcome of a dry run. Results and Warnings are ordered by NIK; nothing is persisted.
type DryRunResult struct {
	Results   []engine.Result
	Warnings  []Issue // loader warnings, per-NIK errors and engine warnings
	Summary   Summary
	InputHash string // sha256 hex over the inputs in NIK order (deterministic)
}

// Runner calculates a loaded batch with a worker pool (03§3 step 4).
type Runner struct {
	Workers int                                       // 0 = GOMAXPROCS
	Calc    func(engine.Input) (engine.Result, error) // nil = engine.Calculate
}

// NewRunner returns a Runner using engine.Calculate and GOMAXPROCS workers.
func NewRunner() Runner { return Runner{} }

// DryRun calculates every input. Errors per NIK become warnings and that NIK is left out of Results;
// only context cancellation fails the run. Incomplete results are excluded as well (never persisted).
func (r Runner) DryRun(ctx context.Context, loaded Result) (DryRunResult, error) {
	inputs := append([]engine.Input(nil), loaded.Inputs...)
	sort.SliceStable(inputs, func(i, j int) bool { return inputs[i].Emp.NIK < inputs[j].Emp.NIK })

	calc := r.Calc
	if calc == nil {
		calc = engine.Calculate
	}
	workers := r.Workers
	if workers <= 0 {
		workers = runtime.GOMAXPROCS(0)
	}

	type slot struct {
		res engine.Result
		err error
	}
	slots := make([]slot, len(inputs))
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(workers)
	for i := range inputs {
		g.Go(func() error {
			if err := gctx.Err(); err != nil {
				return err
			}
			if err := inputs[i].Validate(); err != nil {
				slots[i].err = err
				return nil //nolint:nilerr // per-NIK error is recorded in the slot and becomes a warning (D-27)
			}
			slots[i].res, slots[i].err = calc(inputs[i])
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return DryRunResult{}, err
	}

	out := DryRunResult{Warnings: append([]Issue(nil), loaded.Warnings...)}
	for i, s := range slots {
		nik := inputs[i].Emp.NIK
		if s.err != nil {
			out.Warnings = append(out.Warnings, Issue{NIK: nik, Code: errCode(s.err), Message: s.err.Error()})
			continue
		}
		for _, w := range s.res.Warnings {
			out.Warnings = append(out.Warnings, engineWarning(nik, w))
		}
		if s.res.Incomplete {
			out.Warnings = append(out.Warnings, Issue{NIK: nik, Code: CodeIncomplete,
				Message: "komponen belum terimplementasi; hasil tidak dipakai"})
			continue
		}
		out.Results = append(out.Results, s.res)
		out.Summary.add(s.res)
	}
	out.InputHash = hashInputs(inputs)
	return out, nil
}

func (s *Summary) add(res engine.Result) {
	s.EmpCount++
	s.LineCount += len(res.Lines)
	for _, l := range res.Lines {
		if l.Key.Type == engine.CompT {
			s.TotalT = s.TotalT.Add(l.AmountOrZero())
		} else {
			s.TotalP = s.TotalP.Add(l.AmountOrZero())
		}
	}
}

func errCode(err error) string {
	switch {
	case errors.Is(err, engine.ErrNoWorkdays):
		return CodeNoWorkdays
	case errors.Is(err, engine.ErrLoanDouble):
		return CodeLoanDouble
	default:
		return CodeCalcError
	}
}

// engineWarning splits "CODE: message" (PPH_NO_BRACKET, LOAN_NET_NEGATIVE, …); other texts get CodeEngineWarn.
func engineWarning(nik, w string) Issue {
	if code, msg, ok := strings.Cut(w, ": "); ok && isCode(code) {
		return Issue{NIK: nik, Code: code, Message: msg}
	}
	if isCode(w) {
		return Issue{NIK: nik, Code: w, Message: w}
	}
	return Issue{NIK: nik, Code: CodeEngineWarn, Message: w}
}

func isCode(s string) bool {
	if s == "" || len(s) > warnCodeMaxLen {
		return false
	}
	for _, c := range s {
		if (c < 'A' || c > 'Z') && c != '_' && (c < '0' || c > '9') {
			return false
		}
	}
	return true
}

// hashInputs is sha256 over the rule set/tax tables (once) and every input in the given order.
// The text form is canonical: map keys are sorted, decimals print via String().
func hashInputs(inputs []engine.Input) string {
	h := sha256.New()
	w := canonWriter{h}
	if len(inputs) > 0 {
		in := inputs[0]
		w.printf("%s|rules=%d/%v|", hashSharedLabel, in.Rules.ID(), in.Rules.LegacyCompat())
		canon(w, reflect.ValueOf(in.Tax))
		hashConstants(w, in.Rules)
	}
	for _, in := range inputs {
		in.Tax, in.Rules = engine.TaxTables{}, nil
		w.printf("\nnik=%s|", in.Emp.NIK)
		canon(w, reflect.ValueOf(in))
	}
	return hex.EncodeToString(h.Sum(nil))
}

// constantsProvider is implemented by rule sets that can list their resolved constants (postgres.RuleSet).
type constantsProvider interface {
	Constants() map[string]decimal.Decimal
}

// hashConstants adds the resolved rule constants in key order: editing a constant of the same rule set
// after a commit changes the hash, so approve/close see the result as stale (T023d).
func hashConstants(w canonWriter, r engine.RuleSet) {
	cp, ok := r.(constantsProvider)
	if !ok {
		return
	}
	cs := cp.Constants()
	keys := make([]string, 0, len(cs))
	for k := range cs {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		w.printf("const.%s=%s|", k, cs[k].String())
	}
}

// canonWriter feeds a hash.Hash, whose Write never fails (hash.Hash contract), so errors are dropped here once.
type canonWriter struct{ h hash.Hash }

func (w canonWriter) str(s string) { _, _ = io.WriteString(w.h, s) } //nolint:errcheck // hash.Hash.Write never errors

func (w canonWriter) printf(format string, a ...any) { _, _ = fmt.Fprintf(w.h, format, a...) } //nolint:errcheck // hash.Hash.Write never errors

func canon(w canonWriter, v reflect.Value) {
	if !v.IsValid() {
		w.str("nil;")
		return
	}
	if v.CanInterface() {
		if s, ok := v.Interface().(fmt.Stringer); ok && v.Kind() != reflect.Ptr && v.Kind() != reflect.Interface {
			w.printf("%s;", s.String())
			return
		}
	}
	switch v.Kind() {
	case reflect.Ptr, reflect.Interface:
		if v.IsNil() {
			w.str("nil;")
			return
		}
		canon(w, v.Elem())
	case reflect.Struct:
		canonStruct(w, v)
	case reflect.Slice, reflect.Array:
		w.printf("[%d:", v.Len())
		for i := 0; i < v.Len(); i++ {
			canon(w, v.Index(i))
		}
		w.str("]")
	case reflect.Map:
		canonMap(w, v)
	case reflect.Func, reflect.Chan:
		w.str("fn;")
	default:
		if v.CanInterface() {
			w.printf("%v;", v.Interface())
		}
	}
}

func canonStruct(w canonWriter, v reflect.Value) {
	w.str("{")
	for i := 0; i < v.NumField(); i++ {
		if v.Type().Field(i).IsExported() {
			canon(w, v.Field(i))
		}
	}
	w.str("}")
}

func canonMap(w canonWriter, v reflect.Value) {
	type kv struct {
		k string
		v reflect.Value
	}
	kvs := make([]kv, 0, v.Len())
	for it := v.MapRange(); it.Next(); {
		kvs = append(kvs, kv{fmt.Sprintf("%v", it.Key().Interface()), it.Value()})
	}
	sort.Slice(kvs, func(i, j int) bool { return kvs[i].k < kvs[j].k })
	w.printf("<%d:", len(kvs))
	for _, e := range kvs {
		w.printf("%s=", e.k)
		canon(w, e.v)
	}
	w.str(">")
}
