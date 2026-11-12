package mbsource

import (
	"context"
	"fmt"
)

// ResolveTargets returns a Resolution per product id. Providers run in order and the first
// that knows a product wins: an FgAwareProvider gets the pending targets, any other
// provider gets the unique pending normalized shades and its result is mapped back to
// every pending target with that shade. Targets with an empty shade are skipped. A
// provider error aborts the whole call, exactly like Resolve.
func (r *Resolver) ResolveTargets(ctx context.Context, targets []Target) (map[int64]Resolution, error) {
	out := make(map[int64]Resolution, len(targets))
	pending := pendingTargets(targets)
	for _, p := range r.providers {
		if len(pending) == 0 {
			break
		}
		got, err := resolveProviderTargets(ctx, p, pending)
		if err != nil {
			return nil, fmt.Errorf("mb source provider %s: %w", p.Name(), err)
		}
		next := pending[:0:0]
		for _, t := range pending {
			if res, ok := got[t.ProductSysID]; ok {
				out[t.ProductSysID] = res
			} else {
				next = append(next, t)
			}
		}
		pending = next
	}
	return out, nil
}

// pendingTargets normalizes shades, drops empty ones and de-dups by product id (first wins).
func pendingTargets(targets []Target) []Target {
	seen := make(map[int64]bool, len(targets))
	out := make([]Target, 0, len(targets))
	for _, t := range targets {
		n := NormalizeShade(t.Shade)
		if n == "" || seen[t.ProductSysID] {
			continue
		}
		seen[t.ProductSysID] = true
		out = append(out, Target{ProductSysID: t.ProductSysID, Shade: n})
	}
	return out
}

// resolveProviderTargets runs one provider over normalized pending targets.
func resolveProviderTargets(ctx context.Context, p Provider, pending []Target) (map[int64]Resolution, error) {
	if fp, ok := p.(FgAwareProvider); ok {
		return fp.ResolveByTargets(ctx, pending)
	}
	shades := make([]string, 0, len(pending))
	seen := make(map[string]bool, len(pending))
	for _, t := range pending {
		if !seen[t.Shade] {
			seen[t.Shade] = true
			shades = append(shades, t.Shade)
		}
	}
	got, err := p.ResolveByShades(ctx, shades)
	if err != nil {
		return nil, err
	}
	out := make(map[int64]Resolution, len(pending))
	for _, t := range pending {
		if res, ok := got[t.Shade]; ok {
			out[t.ProductSysID] = res
		}
	}
	return out, nil
}
