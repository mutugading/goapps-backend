package chartdata

import (
	"context"
	"fmt"
	"time"

	dashboarddomain "github.com/mutugading/goapps-backend/services/finance/internal/domain/bi/dashboard"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/bi/factmetric"
)

// Cache is the minimal interface this handler needs.
type Cache interface {
	Get(ctx context.Context, dashboardCode, filtersHash string, out any) (bool, error)
	Set(ctx context.Context, dashboardCode, filtersHash string, value any, ttl time.Duration) error
}

// HashFilters is injected by the wiring layer (typically redisinfra.HashFilters).
type HashFilters func(any) string

// GetDataQuery is the payload for GetDataHandler.
type GetDataQuery struct {
	DashboardCode string
	Filters       ViewerFilters
	UserRoles     []string
	IsSuperAdmin  bool
}

// GetDataHandler is the viewer-side orchestrator: loads dashboard, applies access check,
// reads cache, executes query, computes KPIs, formats result, writes cache, returns.
type GetDataHandler struct {
	repo  dashboarddomain.Repository
	fact  factmetric.Repository
	cache Cache
	hash  HashFilters
	now   func() time.Time // injectable for tests
}

// NewGetDataHandler constructs a GetDataHandler.
// cache and hash may be nil for tests (cache becomes pass-through).
func NewGetDataHandler(
	repo dashboarddomain.Repository,
	fact factmetric.Repository,
	cache Cache,
	hash HashFilters,
) *GetDataHandler {
	return &GetDataHandler{
		repo:  repo,
		fact:  fact,
		cache: cache,
		hash:  hash,
		now:   func() time.Time { return time.Now().UTC() },
	}
}

// WithNow overrides the clock source. Used by tests for deterministic period resolution.
func (h *GetDataHandler) WithNow(now func() time.Time) *GetDataHandler {
	if now != nil {
		h.now = now
	}
	return h
}

// Handle executes the full viewer pipeline.
func (h *GetDataHandler) Handle(ctx context.Context, q GetDataQuery) (*Result, error) {
	// 1. Load dashboard
	d, err := h.repo.GetByCode(ctx, q.DashboardCode)
	if err != nil {
		return nil, err
	}
	// 2. Access check
	if !d.IsAccessibleBy(q.UserRoles, q.IsSuperAdmin) {
		return nil, dashboarddomain.ErrForbidden
	}

	// 3. Try cache
	var hashKey string
	if h.cache != nil && h.hash != nil {
		hashKey = h.hash(q.Filters)
		var cached Result
		if hit, _ := h.cache.Get(ctx, q.DashboardCode, hashKey, &cached); hit {
			cached.Meta.CacheHit = true
			cached.Meta.QueryHash = hashKey
			return &cached, nil
		}
	}

	// 4. Plan + execute aggregate query
	now := h.now()
	plan, err := Plan(d, q.Filters, now)
	if err != nil {
		return nil, fmt.Errorf("plan: %w", err)
	}
	rows, err := h.fact.QueryAggregate(ctx, plan)
	if err != nil {
		return nil, fmt.Errorf("execute aggregate: %w", err)
	}

	// 5. Compute KPIs
	period := ResolvePeriod(q.Filters.PeriodPreset, q.Filters.PeriodFrom, q.Filters.PeriodTo,
		d.PeriodGrain().String(), now)
	kpis, err := ComputeKPIs(ctx, h.fact, d, period, now)
	if err != nil {
		return nil, fmt.Errorf("compute kpis: %w", err)
	}

	// 6. Shape + format
	drillCtx := BuildDrillContext(d, q.Filters)
	result := Shape(d, rows, kpis, q.Filters, drillCtx)
	result.Meta.QueryHash = hashKey

	// 7. Cache (best-effort)
	ttl := time.Duration(d.CacheTTL().Seconds()) * time.Second
	if h.cache != nil && h.hash != nil && ttl > 0 {
		_ = h.cache.Set(ctx, q.DashboardCode, hashKey, result, ttl)
	}

	return &result, nil
}
