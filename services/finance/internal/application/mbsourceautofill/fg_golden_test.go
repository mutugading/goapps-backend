package mbsourceautofill_test

// mb-spin-fg-poy-link P4-T5 (unit level): the FG-aware chain
// [MBSpinFgProvider, MBSpinProvider, SuperbaProvider] driven through AutoFillMBSource must
// produce exactly the old chain's plans and stats when no live FG spin exists, and must
// prefer the FG spin of the product (POY) or of its upstream POY otherwise.

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/application/mbsourceautofill"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbsource"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbspin"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/parameter"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/superbacostsp"
)

// memSpin is one in-memory mst_mb_spin row.
type memSpin struct {
	e          *mbspin.Entity
	shade      string
	fg         int64 // 0 = legacy (no FG link)
	active     bool
	superseded bool
}

// memSpins emulates FindByShades (old rule: first matching row = the pick) and
// FindByShadeAndFg (live, active, non-superseded, FG-linked).
type memSpins struct{ rows []memSpin }

func (m *memSpins) FindByShades(_ context.Context, shades []string) (map[string]*mbspin.Entity, error) {
	out := map[string]*mbspin.Entity{}
	for _, s := range shades {
		for _, r := range m.rows {
			if r.active && mbsource.NormalizeShade(r.shade) == s {
				out[s] = r.e
				break
			}
		}
	}
	return out, nil
}

func (m *memSpins) FindByShadeAndFg(_ context.Context, keys []mbsource.ShadeFgKey) (map[mbsource.ShadeFgKey]*mbspin.Entity, error) {
	out := map[mbsource.ShadeFgKey]*mbspin.Entity{}
	for _, k := range keys {
		for _, r := range m.rows {
			if r.fg != 0 && r.fg == k.FgProductID && r.active && !r.superseded &&
				mbsource.NormalizeShade(r.shade) == mbsource.NormalizeShade(k.Shade) {
				out[mbsource.ShadeFgKey{Shade: mbsource.NormalizeShade(k.Shade), FgProductID: k.FgProductID}] = r.e
			}
		}
	}
	return out, nil
}

type memCands map[int64][]int64

func (m memCands) FgCandidates(_ context.Context, ids []int64) (map[int64][]int64, error) {
	out := map[int64][]int64{}
	for _, id := range ids {
		if c, ok := m[id]; ok {
			out[id] = c
		}
	}
	return out, nil
}

type memSuperba struct {
	superbacostsp.Repository
	rows map[string]string
}

func (m memSuperba) ResolveByShades(_ context.Context, s []string) (map[string]superbacostsp.Resolved, error) {
	out := map[string]superbacostsp.Resolved{}
	for _, x := range s {
		if c, ok := m.rows[x]; ok {
			out[x] = superbacostsp.Resolved{ColourName: c}
		}
	}
	return out, nil
}

// goldenParams serves the trigger and two real fill-group children so Children are non-empty.
type goldenParams struct{ kids []*parameter.Parameter }

func (goldenParams) GetByCode(ctx context.Context, c parameter.Code) (*parameter.Parameter, error) {
	return okParams{}.GetByCode(ctx, c)
}
func (g goldenParams) GetByFillGroup(context.Context, string) ([]*parameter.Parameter, error) {
	return g.kids, nil
}

func goldenChild(t *testing.T, code, dataType, col string) *parameter.Parameter {
	t.Helper()
	c, err := parameter.NewCode(code)
	require.NoError(t, err)
	dt, err := parameter.NewDataType(dataType)
	require.NoError(t, err)
	cat, err := parameter.NewParamCategory("INPUT")
	require.NoError(t, err)
	return parameter.ReconstructParameter(uuid.New(), c, code, code, dt, cat, nil, "", "", nil, nil, nil, true,
		parameter.CostingMetadata{LookupFillGroupCode: mbsource.FillGroupSPCode, LookupSourceColumn: col},
		time.Now(), "t", nil, nil, nil, nil)
}

func goldenSpin(t *testing.T, orion string, denier float64) *mbspin.Entity {
	t.Helper()
	o := orion
	e, err := mbspin.New(uuid.New(), "DYE "+orion, nil, &o, &denier, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, "t")
	require.NoError(t, err)
	return e
}

type chainFixture struct {
	spins   *memSpins
	cands   memCands
	superba memSuperba
	params  goldenParams
	targets []mbsourceautofill.Target
}

func (f chainFixture) oldChain() *mbsource.Resolver {
	return mbsource.NewResolver(mbsource.NewMBSpinProvider(f.spins, f.params), mbsource.NewSuperbaProvider(f.superba))
}

func (f chainFixture) newChain() *mbsource.Resolver {
	return mbsource.NewResolver(mbsource.NewMBSpinFgProvider(f.cands, f.spins, f.params),
		mbsource.NewMBSpinProvider(f.spins, f.params), mbsource.NewSuperbaProvider(f.superba))
}

// run drives AutoFillMBSource with the resolver; the old chain is wrapped so it only exposes
// Resolve (the pre-P4 shade-only path), exactly as the service saw it before P4.
func (f chainFixture) run(t *testing.T, r mbsourceautofill.ShadeResolver) (mbsourceautofill.Stats, []mbsourceautofill.Plan) {
	t.Helper()
	st := &fakeStore{targets: f.targets}
	ids := make([]int64, 0, len(f.targets))
	for _, x := range f.targets {
		ids = append(ids, x.ProductSysID)
	}
	stats, err := mbsourceautofill.New(st, r, f.params).AutoFillMBSource(context.Background(), ids, "u")
	require.NoError(t, err)
	return stats, st.applied
}

// shadeOnly hides ResolveTargets so the service takes the legacy Resolve(shades) path.
type shadeOnly struct{ r *mbsource.Resolver }

func (s shadeOnly) Resolve(ctx context.Context, shades []string) (map[string]mbsource.Resolution, error) {
	return s.r.Resolve(ctx, shades)
}

const (
	pPoy, pDty, pMel, pSup, pNone, pDup, pBlank = 1, 2, 3, 4, 5, 6, 7
	poyUp1, poyUp2                              = 101, 102
)

func newGoldenFixture(t *testing.T, fgSpins ...memSpin) chainFixture {
	t.Helper()
	spins := &memSpins{rows: []memSpin{
		{e: goldenSpin(t, "ORION-A", 75), shade: "A", active: true},
		{e: goldenSpin(t, "ORION-A-OFF", 50), shade: "A", active: false},
		{e: goldenSpin(t, "ORION-BOTH", 30), shade: "BOTH", active: true},
		{e: goldenSpin(t, "ORION-M", 20), shade: " m ", active: true},
	}}
	spins.rows = append(spins.rows, fgSpins...)
	return chainFixture{
		spins:   spins,
		cands:   memCands{pPoy: {pPoy}, pDty: {poyUp1}, pMel: {poyUp1, poyUp2}, pDup: {pDup}},
		superba: memSuperba{rows: map[string]string{"BOTH": "SKY", "SUP": "RED"}},
		params: goldenParams{kids: []*parameter.Parameter{
			goldenChild(t, "MB_SP_DYE", "TEXT", "mbs_mgt_name"),
			goldenChild(t, "MB_SP_DENIER", "NUMBER", "mbs_denier"),
		}},
		targets: []mbsourceautofill.Target{
			{ProductSysID: pPoy, Shade: " a"}, {ProductSysID: pDty, Shade: "A"}, {ProductSysID: pMel, Shade: "m"},
			{ProductSysID: pSup, Shade: "sup"}, {ProductSysID: pNone, Shade: "zzz"}, {ProductSysID: pDup, Shade: "both"},
			{ProductSysID: pBlank, Shade: "  "},
		},
	}
}

// TestGolden_NoLiveFgSpin_NewChainIdenticalToOld is the P4 golden test (unit level).
func TestGolden_NoLiveFgSpin_NewChainIdenticalToOld(t *testing.T) {
	cases := map[string][]memSpin{
		"no FG spins at all": nil,
		// FG spins exist for the candidates but none is live: superseded, inactive, other shade.
		"only dead or unrelated FG spins": {
			{e: goldenSpin(t, "FG-SUPERSEDED", 1), shade: "A", fg: pPoy, active: true, superseded: true},
			{e: goldenSpin(t, "FG-INACTIVE", 2), shade: "A", fg: poyUp1, active: false},
			{e: goldenSpin(t, "FG-OTHER-SHADE", 3), shade: "QQ", fg: poyUp2, active: true},
		},
	}
	for name, fg := range cases {
		t.Run(name, func(t *testing.T) {
			f := newGoldenFixture(t, fg...)
			oldStats, oldPlans := f.run(t, shadeOnly{f.oldChain()})
			newStats, newPlans := f.run(t, f.newChain())

			assert.Equal(t, oldStats, newStats)
			require.Len(t, newPlans, len(oldPlans))
			for i := range oldPlans {
				assert.Equal(t, oldPlans[i], newPlans[i], "plan %d", i)
				assert.Nil(t, newPlans[i].Resolution.FgProductID)
			}
			// Sanity: the fixture really exercises spin, superba, no-match and a blank shade.
			assert.Equal(t, 7, oldStats.Considered)
			assert.Equal(t, 5, oldStats.Filled)
			assert.Equal(t, 2, oldStats.NoMatch)
			assert.Equal(t, 4, oldStats.BySource[mbsource.SourceMBSpin])
			assert.Equal(t, 1, oldStats.BySource[mbsource.SourceSuperbaCostSP])
			assert.Zero(t, newStats.BySource[mbsource.SourceMBSpinFg])
		})
	}
}

func planBy(plans []mbsourceautofill.Plan) map[int64]mbsource.Resolution {
	out := map[int64]mbsource.Resolution{}
	for _, p := range plans {
		out[p.ProductSysID] = p.Resolution
	}
	return out
}

// TestChain_FgSpinWins covers (a) POY self, (b) DTY via upstream POY and (e) MEL ordering.
func TestChain_FgSpinWins(t *testing.T) {
	fgPoy := goldenSpin(t, "FG-POY", 10)
	fgUp1 := goldenSpin(t, "FG-UP1", 11)
	fgUp2M := goldenSpin(t, "FG-UP2-M", 12)
	f := newGoldenFixture(t,
		memSpin{e: fgPoy, shade: "a ", fg: pPoy, active: true},
		memSpin{e: fgUp1, shade: "A", fg: poyUp1, active: true},
		// MEL (shade M): only the 2nd candidate has an FG spin.
		memSpin{e: fgUp2M, shade: "M", fg: poyUp2, active: true},
	)
	stats, plans := f.run(t, f.newChain())
	got := planBy(plans)

	// (a) POY: own FG spin beats the shade-only pick ORION-A.
	assert.Equal(t, "FG-POY", got[pPoy].SPCode)
	assert.Equal(t, mbsource.SourceMBSpinFg, got[pPoy].Source)
	require.NotNil(t, got[pPoy].FgProductID)
	assert.Equal(t, int64(pPoy), *got[pPoy].FgProductID)
	assert.Equal(t, fgPoy.ID(), *got[pPoy].SpinID)
	require.NotNil(t, got[pPoy].Children["MB_SP_DENIER"].Num)
	assert.InDelta(t, 10.0, *got[pPoy].Children["MB_SP_DENIER"].Num, 1e-9)
	// (b) DTY: FG spin of the upstream POY.
	assert.Equal(t, "FG-UP1", got[pDty].SPCode)
	assert.Equal(t, int64(poyUp1), *got[pDty].FgProductID)
	// (e) MEL, only 2nd POY has an FG spin -> 2nd chosen.
	assert.Equal(t, "FG-UP2-M", got[pMel].SPCode)
	assert.Equal(t, int64(poyUp2), *got[pMel].FgProductID)
	// Products without an FG hit keep the old rule.
	assert.Equal(t, mbsource.SourceSuperbaCostSP, got[pSup].Source)
	assert.Equal(t, "ORION-BOTH", got[pDup].SPCode)
	assert.Equal(t, mbsource.SourceMBSpin, got[pDup].Source)
	assert.Nil(t, got[pDup].FgProductID)

	assert.Equal(t, 3, stats.BySource[mbsource.SourceMBSpinFg])
	assert.Equal(t, 1, stats.BySource[mbsource.SourceMBSpin])
	assert.Equal(t, 1, stats.BySource[mbsource.SourceSuperbaCostSP])
	assert.Equal(t, 2, stats.NoMatch)
}

// TestChain_MELBothCandidatesHit_FirstInSeqOrderWins covers (e) with both POY RMs linked.
func TestChain_MELBothCandidatesHit_FirstInSeqOrderWins(t *testing.T) {
	f := newGoldenFixture(t,
		memSpin{e: goldenSpin(t, "FG-UP2-M", 1), shade: "M", fg: poyUp2, active: true},
		memSpin{e: goldenSpin(t, "FG-UP1-M", 2), shade: "M", fg: poyUp1, active: true},
	)
	_, plans := f.run(t, f.newChain())
	got := planBy(plans)
	assert.Equal(t, "FG-UP1-M", got[pMel].SPCode)
	assert.Equal(t, int64(poyUp1), *got[pMel].FgProductID)
}

// TestChain_SupersededOrInactiveFgSpinFallsBackToOldRule: dead FG spins never win.
func TestChain_SupersededOrInactiveFgSpinFallsBackToOldRule(t *testing.T) {
	f := newGoldenFixture(t,
		memSpin{e: goldenSpin(t, "FG-OLD", 1), shade: "A", fg: pPoy, active: true, superseded: true},
		memSpin{e: goldenSpin(t, "FG-OFF", 2), shade: "A", fg: poyUp1, active: false},
	)
	_, plans := f.run(t, f.newChain())
	got := planBy(plans)
	for _, pid := range []int64{pPoy, pDty} {
		assert.Equal(t, "ORION-A", got[pid].SPCode)
		assert.Equal(t, mbsource.SourceMBSpin, got[pid].Source)
		assert.Nil(t, got[pid].FgProductID)
	}
}
