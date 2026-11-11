package mbsource_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbsource"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbspin"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/parameter"
)

type fakeCands struct {
	m     map[int64][]int64
	err   error
	calls [][]int64
}

func (f *fakeCands) FgCandidates(_ context.Context, ids []int64) (map[int64][]int64, error) {
	f.calls = append(f.calls, ids)
	if f.err != nil {
		return nil, f.err
	}
	out := map[int64][]int64{}
	for _, id := range ids {
		if c, ok := f.m[id]; ok {
			out[id] = c
		}
	}
	return out, nil
}

type fakeFgSpins struct {
	m     map[mbsource.ShadeFgKey]*mbspin.Entity
	err   error
	calls [][]mbsource.ShadeFgKey
}

func (f *fakeFgSpins) FindByShadeAndFg(_ context.Context, keys []mbsource.ShadeFgKey) (map[mbsource.ShadeFgKey]*mbspin.Entity, error) {
	f.calls = append(f.calls, keys)
	if f.err != nil {
		return nil, f.err
	}
	out := map[mbsource.ShadeFgKey]*mbspin.Entity{}
	for _, k := range keys {
		if s, ok := f.m[k]; ok {
			out[k] = s
		}
	}
	return out, nil
}

type fakeShadeSpins struct{ m map[string]*mbspin.Entity }

func (f *fakeShadeSpins) FindByShades(_ context.Context, shades []string) (map[string]*mbspin.Entity, error) {
	out := map[string]*mbspin.Entity{}
	for _, s := range shades {
		if e, ok := f.m[s]; ok {
			out[s] = e
		}
	}
	return out, nil
}

type fakeParams struct{ calls int }

func (f *fakeParams) GetByFillGroup(_ context.Context, _ string) ([]*parameter.Parameter, error) {
	f.calls++
	return nil, nil
}

func newSpin(t *testing.T, orion string) *mbspin.Entity {
	t.Helper()
	o := orion
	e, err := mbspin.New(uuid.New(), "DYE "+orion, nil, &o, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, "t")
	require.NoError(t, err)
	return e
}

func key(s string, fg int64) mbsource.ShadeFgKey { return mbsource.ShadeFgKey{Shade: s, FgProductID: fg} }

func TestMBSpinFgProvider_FgHitWinsOverShadeOnly(t *testing.T) {
	cands := &fakeCands{m: map[int64][]int64{10: {10}}}
	fg := &fakeFgSpins{m: map[mbsource.ShadeFgKey]*mbspin.Entity{key("A-1", 10): newSpin(t, "FG-SPIN")}}
	params := &fakeParams{}
	legacy := mbsource.NewMBSpinProvider(&fakeShadeSpins{m: map[string]*mbspin.Entity{"A-1": newSpin(t, "SHADE-SPIN")}}, params)
	r := mbsource.NewResolver(mbsource.NewMBSpinFgProvider(cands, fg, params), legacy)

	got, err := r.ResolveTargets(context.Background(), []mbsource.Target{{ProductSysID: 10, Shade: " a-1 "}})
	require.NoError(t, err)
	res := got[10]
	assert.Equal(t, "FG-SPIN", res.SPCode)
	assert.Equal(t, mbsource.SourceMBSpinFg, res.Source)
	require.NotNil(t, res.FgProductID)
	assert.Equal(t, int64(10), *res.FgProductID)
	assert.Equal(t, []mbsource.ShadeFgKey{key("A-1", 10)}, fg.calls[0])
}

func TestMBSpinFgProvider_MELSecondCandidateHit(t *testing.T) {
	cands := &fakeCands{m: map[int64][]int64{20: {11, 12, 13}}}
	fg := &fakeFgSpins{m: map[mbsource.ShadeFgKey]*mbspin.Entity{
		key("B", 12): newSpin(t, "S12"),
		key("B", 13): newSpin(t, "S13"),
	}}
	p := mbsource.NewMBSpinFgProvider(cands, fg, &fakeParams{})
	got, err := p.ResolveByTargets(context.Background(), []mbsource.Target{{ProductSysID: 20, Shade: "b"}})
	require.NoError(t, err)
	assert.Equal(t, "S12", got[20].SPCode)
	assert.Equal(t, int64(12), *got[20].FgProductID)

	ex, err := p.Explain(context.Background(), mbsource.Target{ProductSysID: 20, Shade: "b"})
	require.NoError(t, err)
	assert.Equal(t, []int64{11, 12, 13}, ex.Candidates)
	require.NotNil(t, ex.Hit)
	assert.Equal(t, "S12", ex.Hit.SPCode)
}

func TestMBSpinFgProvider_ExplainNoHit(t *testing.T) {
	p := mbsource.NewMBSpinFgProvider(&fakeCands{m: map[int64][]int64{5: {7}}}, &fakeFgSpins{}, &fakeParams{})
	ex, err := p.Explain(context.Background(), mbsource.Target{ProductSysID: 5, Shade: "x"})
	require.NoError(t, err)
	assert.Equal(t, []int64{7}, ex.Candidates)
	assert.Nil(t, ex.Hit)
}

func TestMBSpinFgProvider_NoFgSpinFallsThroughIdentical(t *testing.T) {
	shadeSpin := newSpin(t, "SHADE-SPIN")
	legacyFinder := &fakeShadeSpins{m: map[string]*mbspin.Entity{"A-1": shadeSpin}}
	params := &fakeParams{}
	fgP := mbsource.NewMBSpinFgProvider(&fakeCands{m: map[int64][]int64{10: {99}}}, &fakeFgSpins{}, params)
	r := mbsource.NewResolver(fgP, mbsource.NewMBSpinProvider(legacyFinder, params))

	got, err := r.ResolveTargets(context.Background(), []mbsource.Target{{ProductSysID: 10, Shade: "a-1"}})
	require.NoError(t, err)
	want, err := mbsource.NewResolver(mbsource.NewMBSpinProvider(legacyFinder, params)).Resolve(context.Background(), []string{"a-1"})
	require.NoError(t, err)
	assert.Equal(t, want["A-1"], got[10])
	assert.Nil(t, got[10].FgProductID)
	assert.Equal(t, mbsource.SourceMBSpin, got[10].Source)
}

func TestMBSpinFgProvider_GetByFillGroupOnlyOnHit(t *testing.T) {
	params := &fakeParams{}
	p := mbsource.NewMBSpinFgProvider(&fakeCands{m: map[int64][]int64{1: {2}}}, &fakeFgSpins{}, params)
	got, err := p.ResolveByTargets(context.Background(), []mbsource.Target{{ProductSysID: 1, Shade: "x"}})
	require.NoError(t, err)
	assert.Empty(t, got)
	assert.Zero(t, params.calls)
}

func TestMBSpinFgProvider_EmptyShadeSkippedAndShadeOnlyEmpty(t *testing.T) {
	cands := &fakeCands{}
	p := mbsource.NewMBSpinFgProvider(cands, &fakeFgSpins{}, &fakeParams{})
	got, err := p.ResolveByTargets(context.Background(), []mbsource.Target{{ProductSysID: 1, Shade: "  "}})
	require.NoError(t, err)
	assert.Empty(t, got)
	assert.Empty(t, cands.calls, "no query without a shade")

	byShade, err := p.ResolveByShades(context.Background(), []string{"A"})
	require.NoError(t, err)
	assert.Empty(t, byShade)
	assert.Equal(t, mbsource.SourceMBSpinFg, p.Name())
}

func TestMBSpinFgProvider_ErrorsAbort(t *testing.T) {
	sup := &fakeProvider{name: "sup", data: map[string]mbsource.Resolution{"A": {}}}
	for name, p := range map[string]*mbsource.MBSpinFgProvider{
		"candidates": mbsource.NewMBSpinFgProvider(&fakeCands{err: errors.New("db")}, &fakeFgSpins{}, &fakeParams{}),
		"spins":      mbsource.NewMBSpinFgProvider(&fakeCands{m: map[int64][]int64{1: {2}}}, &fakeFgSpins{err: errors.New("db")}, &fakeParams{}),
	} {
		t.Run(name, func(t *testing.T) {
			_, err := mbsource.NewResolver(p, sup).ResolveTargets(context.Background(), []mbsource.Target{{ProductSysID: 1, Shade: "a"}})
			require.Error(t, err)
			assert.Contains(t, err.Error(), "mb source provider MB_SPIN_FG")
		})
	}
	assert.Empty(t, sup.calls, "must not fall through after a provider failure")
}

func TestResolver_ResolveTargetsLegacyEqualsResolve(t *testing.T) {
	spin := &fakeProvider{name: "spin", data: map[string]mbsource.Resolution{"A-1": {SPCode: "ORION1", Source: mbsource.SourceMBSpin}}}
	sup := &fakeProvider{name: "sup", data: map[string]mbsource.Resolution{
		"A-1":  {SPCode: "A-1", Source: mbsource.SourceSuperbaCostSP},
		"MC-9": {SPCode: "MC-9", Source: mbsource.SourceSuperbaCostSP},
	}}
	r := mbsource.NewResolver(spin, sup)
	cases := []struct {
		name    string
		targets []mbsource.Target
	}{
		{"empty", nil},
		{"blank shades", []mbsource.Target{{1, ""}, {2, "  "}}},
		{"mixed", []mbsource.Target{{1, " a-1 "}, {2, "mc-9"}, {3, "A-1"}, {4, "zzz"}, {5, ""}}},
		{"dup product", []mbsource.Target{{1, "mc-9"}, {1, "a-1"}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := r.ResolveTargets(context.Background(), tc.targets)
			require.NoError(t, err)
			shades := make([]string, 0, len(tc.targets))
			for _, x := range tc.targets {
				shades = append(shades, x.Shade)
			}
			byShade, err := r.Resolve(context.Background(), shades)
			require.NoError(t, err)
			seen := map[int64]bool{}
			want := map[int64]mbsource.Resolution{}
			for _, x := range tc.targets {
				if seen[x.ProductSysID] || mbsource.NormalizeShade(x.Shade) == "" {
					continue
				}
				seen[x.ProductSysID] = true
				if res, ok := byShade[mbsource.NormalizeShade(x.Shade)]; ok {
					want[x.ProductSysID] = res
				}
			}
			assert.Equal(t, want, got)
		})
	}
}

func TestResolver_ResolveTargetsSendsUniqueShadesToLegacy(t *testing.T) {
	spin := &fakeProvider{name: "spin", data: map[string]mbsource.Resolution{}}
	_, err := mbsource.NewResolver(spin).ResolveTargets(context.Background(),
		[]mbsource.Target{{1, "a"}, {2, " A "}, {3, "b"}})
	require.NoError(t, err)
	require.Len(t, spin.calls, 1)
	assert.Equal(t, []string{"A", "B"}, spin.calls[0])
}
