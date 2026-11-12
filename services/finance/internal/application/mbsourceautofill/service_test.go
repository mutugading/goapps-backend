package mbsourceautofill_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/application/mbsourceautofill"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbsource"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/parameter"
)

type fakeStore struct {
	targets  []mbsourceautofill.Target
	listErr  error
	applyErr map[int64]error
	applied  []mbsourceautofill.Plan
}

func (f *fakeStore) ListTargets(context.Context, []int64) ([]mbsourceautofill.Target, error) {
	return f.targets, f.listErr
}
func (f *fakeStore) Apply(_ context.Context, _ mbsourceautofill.ParamSet, p mbsourceautofill.Plan, _ string) error {
	if e := f.applyErr[p.ProductSysID]; e != nil {
		return e
	}
	f.applied = append(f.applied, p)
	return nil
}

type fakeResolver struct {
	res map[string]mbsource.Resolution
	err error
}

func (f fakeResolver) Resolve(context.Context, []string) (map[string]mbsource.Resolution, error) {
	return f.res, f.err
}

// params is built via the parameter package constructor path would need DB; the service only
// needs ids, so use a reader that returns reconstructed params.
type fakeParams struct{ err error }

func (f fakeParams) GetByCode(context.Context, parameter.Code) (*parameter.Parameter, error) {
	return nil, f.err
}
func (f fakeParams) GetByFillGroup(context.Context, string) ([]*parameter.Parameter, error) {
	return nil, nil
}

func TestAutoFill_NoMatchAndEmptyInput(t *testing.T) {
	st := &fakeStore{targets: []mbsourceautofill.Target{{ProductSysID: 1, Shade: "x"}}}
	svc := mbsourceautofill.New(st, fakeResolver{res: map[string]mbsource.Resolution{}}, fakeParams{})
	stats, err := svc.AutoFillMBSource(context.Background(), []int64{1}, "u")
	require.NoError(t, err)
	assert.Equal(t, 1, stats.NoMatch)
	assert.Empty(t, st.applied)

	stats, err = svc.AutoFillMBSource(context.Background(), nil, "u")
	require.NoError(t, err)
	assert.Zero(t, stats.Considered)
}

func TestAutoFill_NoTargets_SkipsResolver(t *testing.T) {
	svc := mbsourceautofill.New(&fakeStore{}, fakeResolver{err: errors.New("must not be called")}, fakeParams{})
	_, err := svc.AutoFillMBSource(context.Background(), []int64{1}, "u")
	require.NoError(t, err)
}

func TestBestEffort_NeverPropagates(t *testing.T) {
	// store error
	svc := mbsourceautofill.New(&fakeStore{listErr: errors.New("db")}, fakeResolver{}, fakeParams{})
	assert.NotPanics(t, func() { svc.BestEffort(context.Background(), []int64{1}, "u") })
	// resolver error
	svc = mbsourceautofill.New(&fakeStore{targets: []mbsourceautofill.Target{{ProductSysID: 1, Shade: "x"}}},
		fakeResolver{err: errors.New("boom")}, fakeParams{})
	assert.NotPanics(t, func() { svc.BestEffort(context.Background(), []int64{1}, "u") })
	// param lookup error after a hit
	svc = mbsourceautofill.New(&fakeStore{targets: []mbsourceautofill.Target{{ProductSysID: 1, Shade: "x"}}},
		fakeResolver{res: map[string]mbsource.Resolution{"X": {SPCode: "X"}}}, fakeParams{err: errors.New("no param")})
	assert.NotPanics(t, func() { svc.BestEffort(context.Background(), []int64{1}, "u") })
}

// okParams returns a reconstructed trigger param so the run can reach Store.Apply.
type okParams struct{}

func (okParams) GetByCode(_ context.Context, c parameter.Code) (*parameter.Parameter, error) {
	return parameter.ReconstructParameter(uuid.New(), c, "n", "n", "", "", nil, "", "", nil, nil, nil,
		true, parameter.CostingMetadata{}, time.Now(), "t", nil, nil, nil, nil), nil
}
func (okParams) GetByFillGroup(context.Context, string) ([]*parameter.Parameter, error) {
	return nil, nil
}

// targetResolver implements both ShadeResolver and TargetResolver.
type targetResolver struct {
	fakeResolver
	byProduct map[int64]mbsource.Resolution
	gotTarget []mbsource.Target
	err       error
}

func (r *targetResolver) ResolveTargets(_ context.Context, ts []mbsource.Target) (map[int64]mbsource.Resolution, error) {
	r.gotTarget = ts
	return r.byProduct, r.err
}

func TestAutoFill_TargetResolver_PerProduct(t *testing.T) {
	st := &fakeStore{targets: []mbsourceautofill.Target{{ProductSysID: 1, Shade: "S"}, {ProductSysID: 2, Shade: "S"}, {ProductSysID: 3, Shade: "S"}}}
	r := &targetResolver{
		fakeResolver: fakeResolver{err: errors.New("Resolve must not be called")},
		byProduct: map[int64]mbsource.Resolution{
			1: {SPCode: "A", Source: mbsource.SourceMBSpinFg},
			2: {SPCode: "B", Source: mbsource.SourceMBSpin},
		},
	}
	svc := mbsourceautofill.New(st, r, okParams{})
	stats, err := svc.AutoFillMBSource(context.Background(), []int64{1, 2, 3}, "u")
	require.NoError(t, err)
	require.Len(t, r.gotTarget, 3)
	assert.Equal(t, mbsource.Target{ProductSysID: 2, Shade: "S"}, r.gotTarget[1])
	require.Len(t, st.applied, 2)
	assert.Equal(t, "A", st.applied[0].Resolution.SPCode)
	assert.Equal(t, "B", st.applied[1].Resolution.SPCode)
	assert.Equal(t, 2, stats.Filled)
	assert.Equal(t, 1, stats.NoMatch)
	assert.Equal(t, 1, stats.BySource[mbsource.SourceMBSpinFg])
	assert.Equal(t, 1, stats.BySource[mbsource.SourceMBSpin])
}

func TestAutoFill_TargetResolver_Error(t *testing.T) {
	st := &fakeStore{targets: []mbsourceautofill.Target{{ProductSysID: 1, Shade: "S"}}}
	r := &targetResolver{err: errors.New("boom")}
	_, err := mbsourceautofill.New(st, r, okParams{}).AutoFillMBSource(context.Background(), []int64{1}, "u")
	require.Error(t, err)
}

func TestAutoFill_PlainResolver_OldPath(t *testing.T) {
	st := &fakeStore{targets: []mbsourceautofill.Target{{ProductSysID: 1, Shade: " s "}, {ProductSysID: 2, Shade: "zz"}}}
	r := fakeResolver{res: map[string]mbsource.Resolution{"S": {SPCode: "A", Source: mbsource.SourceMBSpin}}}
	stats, err := mbsourceautofill.New(st, r, okParams{}).AutoFillMBSource(context.Background(), []int64{1, 2}, "u")
	require.NoError(t, err)
	require.Len(t, st.applied, 1)
	assert.Equal(t, int64(1), st.applied[0].ProductSysID)
	assert.Equal(t, 1, stats.NoMatch)
	assert.Equal(t, 1, stats.BySource[mbsource.SourceMBSpin])
}
