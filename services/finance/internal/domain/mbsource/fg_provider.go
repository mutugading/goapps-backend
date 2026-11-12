package mbsource

import (
	"context"
	"fmt"
)

// SourceMBSpinFg marks a resolution found in mst_mb_spin through the FG (POY) link
// (mbs_fg_product_id), mb-spin-fg-poy-link P4.
const SourceMBSpinFg = "MB_SPIN_FG"

// Target is one product to resolve: its id (needed for the FG link) and its raw shade.
type Target struct {
	ProductSysID int64
	Shade        string
}

// FgAwareProvider is an OPTIONAL Provider extension that resolves per product (not per
// shade). Resolver.ResolveTargets uses ResolveByTargets when a provider implements it;
// Resolver.Resolve keeps calling ResolveByShades only.
type FgAwareProvider interface {
	Provider
	// ResolveByTargets returns a Resolution per product id it resolves. Missing products are absent.
	ResolveByTargets(ctx context.Context, targets []Target) (map[int64]Resolution, error)
}

// FgExplain describes the FG lookup of one product: the ordered FG (POY) candidates and
// the FG-linked spin resolution that won (nil when no candidate has a spin for the shade).
type FgExplain struct {
	Candidates []int64
	Hit        *Resolution
}

// MBSpinFgProvider resolves a product against the MB spin linked to its FG (POY): the
// product itself when POY, else its upstream POY RM products in route order. The first
// candidate with a live spin for (shade, candidate) wins.
type MBSpinFgProvider struct {
	cands  FgCandidateFinder
	spins  SpinShadeFgFinder
	params FillGroupLister
}

// NewMBSpinFgProvider constructs the FG-aware spin provider.
func NewMBSpinFgProvider(cands FgCandidateFinder, spins SpinShadeFgFinder, params FillGroupLister) *MBSpinFgProvider {
	return &MBSpinFgProvider{cands: cands, spins: spins, params: params}
}

// Name implements Provider.
func (p *MBSpinFgProvider) Name() string { return SourceMBSpinFg }

// ResolveByShades implements Provider: without a product id no FG match is possible, so
// shade-only callers (Resolver.Resolve) always fall through to the next provider.
func (p *MBSpinFgProvider) ResolveByShades(_ context.Context, _ []string) (map[string]Resolution, error) {
	return map[string]Resolution{}, nil
}

// ResolveByTargets implements FgAwareProvider (1 candidate query, 1 spin query, and the
// fill-group listing only when at least one product hits).
func (p *MBSpinFgProvider) ResolveByTargets(ctx context.Context, targets []Target) (map[int64]Resolution, error) {
	_, out, err := p.resolve(ctx, targets)
	return out, err
}

// Explain reports the FG candidates and the winning FG spin resolution for one product.
func (p *MBSpinFgProvider) Explain(ctx context.Context, t Target) (FgExplain, error) {
	cands, out, err := p.resolve(ctx, []Target{t})
	if err != nil {
		return FgExplain{}, err
	}
	ex := FgExplain{Candidates: cands[t.ProductSysID]}
	if res, ok := out[t.ProductSysID]; ok {
		ex.Hit = &res
	}
	return ex, nil
}

func (p *MBSpinFgProvider) resolve(ctx context.Context, targets []Target) (map[int64][]int64, map[int64]Resolution, error) {
	out := make(map[int64]Resolution, len(targets))
	shades := make(map[int64]string, len(targets))
	ids := make([]int64, 0, len(targets))
	for _, t := range targets {
		n := NormalizeShade(t.Shade)
		if n == "" {
			continue
		}
		if _, dup := shades[t.ProductSysID]; !dup {
			ids = append(ids, t.ProductSysID)
		}
		shades[t.ProductSysID] = n
	}
	if len(ids) == 0 {
		return map[int64][]int64{}, out, nil
	}
	cands, err := p.cands.FgCandidates(ctx, ids)
	if err != nil {
		return nil, nil, fmt.Errorf("find FG candidates: %w", err)
	}
	keys := fgKeys(ids, shades, cands)
	if len(keys) == 0 {
		return cands, out, nil
	}
	spins, err := p.spins.FindByShadeAndFg(ctx, keys)
	if err != nil {
		return nil, nil, fmt.Errorf("find spins by shade and FG: %w", err)
	}
	if len(spins) == 0 {
		return cands, out, nil
	}
	children, err := p.params.GetByFillGroup(ctx, FillGroupSPCode)
	if err != nil {
		return nil, nil, fmt.Errorf("list %s fill group: %w", FillGroupSPCode, err)
	}
	for _, id := range ids {
		for _, fg := range cands[id] {
			spin, ok := spins[ShadeFgKey{Shade: shades[id], FgProductID: fg}]
			if !ok || spin == nil {
				continue
			}
			res := SpinResolution(spin, children)
			res.Source = SourceMBSpinFg
			fgID := fg
			res.FgProductID = &fgID
			out[id] = res
			break
		}
	}
	return cands, out, nil
}

// fgKeys builds the unique (shade, candidate) lookup keys in product/candidate order.
func fgKeys(ids []int64, shades map[int64]string, cands map[int64][]int64) []ShadeFgKey {
	seen := make(map[ShadeFgKey]bool)
	keys := make([]ShadeFgKey, 0, len(ids))
	for _, id := range ids {
		for _, fg := range cands[id] {
			k := ShadeFgKey{Shade: shades[id], FgProductID: fg}
			if fg <= 0 || seen[k] {
				continue
			}
			seen[k] = true
			keys = append(keys, k)
		}
	}
	return keys
}
