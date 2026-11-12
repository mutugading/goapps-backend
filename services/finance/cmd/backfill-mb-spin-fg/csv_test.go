package main

import (
	"bytes"
	"context"
	"encoding/csv"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbspin"
)

const (
	spinA = "11111111-1111-1111-1111-111111111111"
	spinB = "22222222-2222-2222-2222-222222222222"
)

func TestWriteDryRunCSV_HeaderAndRoundTrip(t *testing.T) {
	rows := mbspin.ClassifyBackfillAll([]mbspin.BackfillCandidate{
		{SpinID: spinA, HeadID: "h1", IsRoot: true, ShadeCode: " r ", NormShade: "R", FinalProduct: "POY 250/48, x",
			NormSpec: "250/48/X", NSpecShade: 1, NShadeOnly: 2,
			SpecShadePoy: &mbspin.BackfillPoy{ProductSysID: 42, ProductCode: "C42", ProductName: `POY "q"`, ErpItemCode: "E42"}},
		{SpinID: spinB, HeadID: "h2"},
	})
	var buf bytes.Buffer
	require.NoError(t, writeDryRunCSV(&buf, rows))

	recs, err := csv.NewReader(bytes.NewReader(buf.Bytes())).ReadAll()
	require.NoError(t, err)
	require.Len(t, recs, 3)
	assert.Equal(t, "action,reason,bucket,mbs_id,mbh_id,is_root,orion_item_code,mgt_name,shade_code,final_product,norm_spec,"+
		"n_spec_shade,n_shade_only,poy_product_sys_id,poy_product_code,poy_product_name,poy_erp_item_code,conflict_spin_id",
		strings.Join(recs[0], ","))
	assert.Equal(t, []string{"APPLY", "spec_shade_unique", mbspin.BackfillBucketSpecShadeUnique, spinA, "h1", "true", "", "",
		" r ", "POY 250/48, x", "250/48/X", "1", "2", "42", "C42", `POY "q"`, "E42", ""}, recs[1])
	assert.Equal(t, "SKIP", recs[2][0])
	assert.Equal(t, mbspin.BackfillReasonNoShade, recs[2][1])
	assert.Empty(t, recs[2][13])

	// The dry-run output is directly usable as --apply input.
	in, err := parseApplyCSV(bytes.NewReader(buf.Bytes()))
	require.NoError(t, err)
	require.Len(t, in.Entries, 1)
	assert.NoError(t, in.Entries[0].Err)
	assert.Equal(t, spinA, in.Entries[0].SpinID)
	assert.Equal(t, int64(42), in.Entries[0].PoySys)
	assert.Equal(t, map[string]int{"SKIP": 1}, in.Ignored)
}

func TestParseApplyCSV_OnlyApplyRowsAndMalformed(t *testing.T) {
	src := "\uFEFFpoy_product_sys_id,Action,mbs_id,extra\n" +
		"10, apply ," + spinA + ",x\n" + // trimmed + upper-cased -> APPLY
		"11,REVIEW," + spinB + ",x\n" +
		"12,SKIP," + spinB + ",x\n" +
		"13,,," + "\n" +
		"abc,APPLY," + spinB + "\n" + // bad POY id
		"0,APPLY," + spinB + "\n" + // non-positive POY id
		"14,APPLY,not-a-uuid\n" + // bad uuid
		"15,APPLYX," + spinA + "\n" // not exactly APPLY
	in, err := parseApplyCSV(strings.NewReader(src))
	require.NoError(t, err)
	assert.Equal(t, map[string]int{"REVIEW": 1, "SKIP": 1, actionMiss: 1, "APPLYX": 1}, in.Ignored)
	require.Len(t, in.Entries, 4)
	assert.NoError(t, in.Entries[0].Err)
	assert.Equal(t, int64(10), in.Entries[0].PoySys)
	for _, e := range in.Entries[1:] {
		assert.Error(t, e.Err, "line %d", e.Line)
	}
}

func TestParseApplyCSV_MissingColumn(t *testing.T) {
	_, err := parseApplyCSV(strings.NewReader("action,mbs_id\nAPPLY," + spinA + "\n"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), colPoySys)
}

type fakeApplier struct {
	calls []mbspin.BackfillApplyRequest
	fail  map[string]error
}

func (f *fakeApplier) ApplyBackfillRow(_ context.Context, in mbspin.BackfillApplyRequest) (mbspin.BackfillApplyOutcome, error) {
	f.calls = append(f.calls, in)
	if err := f.fail[in.SpinID]; err != nil {
		return mbspin.BackfillApplyOutcome{}, err
	}
	return mbspin.BackfillApplyOutcome{Result: mbspin.BackfillResultApplied, HeadUpdated: true}, nil
}

func TestApplyEntries_MalformedFailedAndDBErrorContinues(t *testing.T) {
	src := "action,mbs_id,poy_product_sys_id\n" +
		"APPLY," + spinA + ",10\n" +
		"APPLY,bad,10\n" +
		"REVIEW," + spinB + ",11\n" +
		"APPLY," + spinB + ",12\n"
	in, err := parseApplyCSV(strings.NewReader(src))
	require.NoError(t, err)
	f := &fakeApplier{fail: map[string]error{spinB: errors.New("boom")}}
	var buf bytes.Buffer
	rw, err := newApplyResultWriter(&buf)
	require.NoError(t, err)
	res, err := applyEntries(context.Background(), f, in.Entries, "tester", rw.write)
	require.NoError(t, err)
	require.Len(t, res, 3)
	require.Len(t, f.calls, 2, "malformed and non-APPLY rows never reach the repository")
	assert.Equal(t, "tester", f.calls[0].Actor)

	assert.Equal(t, mbspin.BackfillResultApplied, res[0].Outcome.Result)
	assert.Equal(t, mbspin.BackfillResultFailed, res[1].Outcome.Result)
	assert.Equal(t, mbspin.BackfillReasonMalformedRow, res[1].Outcome.Reason)
	assert.Equal(t, mbspin.BackfillResultFailed, res[2].Outcome.Result)
	assert.Equal(t, mbspin.BackfillReasonDBError, res[2].Outcome.Reason)

	recs, err := csv.NewReader(&buf).ReadAll()
	require.NoError(t, err)
	assert.Equal(t, applyResultHeader, recs[0])
	assert.Equal(t, []string{spinA, "10", "APPLIED", "", "true", "", ""}, recs[1])
	assert.Equal(t, map[string]int{"APPLIED/": 1, "FAILED/malformed_row": 1, "FAILED/db_error": 1}, countResults(res))
}

func TestApplyEntries_CancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := applyEntries(ctx, &fakeApplier{}, []applyEntry{{SpinID: spinA, PoySys: 1}}, "x", func(applyResultRow) error { return nil })
	assert.Error(t, err)
}

func TestParseFlags(t *testing.T) {
	o, err := parseFlags(nil)
	require.NoError(t, err)
	assert.True(t, o.dryRun, "dry-run is the default")
	assert.Equal(t, defaultActor, o.actor)

	o, err = parseFlags([]string{"--apply", "--input", "r.csv"})
	require.NoError(t, err)
	assert.True(t, o.apply)
	assert.False(t, o.dryRun)

	for _, args := range [][]string{
		{"--apply"},
		{"--dry-run", "--apply", "--input", "r.csv"},
		{"--report", "--apply", "--input", "r.csv"},
		{"--apply", "--input", "r.csv", "--actor", ""},
		{"--input", "r.csv"},
		{"--actor", "someone"},
	} {
		_, err := parseFlags(args)
		assert.Error(t, err, "%v", args)
	}

	o, err = parseFlags([]string{"--report"})
	require.NoError(t, err)
	assert.True(t, o.report)
	assert.False(t, o.dryRun)

	o, err = parseFlags([]string{"--report", "--input", "r.csv", "--out", "x.csv"})
	require.NoError(t, err)
	assert.True(t, o.report)
	assert.Equal(t, "r.csv", o.input)

	_, err = parseFlags([]string{"--report", "--dry-run"})
	assert.Error(t, err)
}
