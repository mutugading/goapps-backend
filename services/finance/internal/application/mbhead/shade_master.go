package mbhead

import (
	"context"
	"strings"

	appshade "github.com/mutugading/goapps-backend/services/finance/internal/application/shade"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbhead"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/shade"
)

// headShadeCode returns the shade code a recipe carries: the user-visible "Shade
// Code" (mbh_code), falling back to mbh_shade_code when mbh_code is empty.
func headShadeCode(entity *mbhead.Entity) string {
	if c := entity.MBHCode(); c != nil && strings.TrimSpace(*c) != "" {
		return *c
	}
	return entity.ShadeCode()
}

// ensureShadeMaster is the MB recipe wrapper around the shared best-effort
// shade.EnsureShadeMaster: it derives the shade code (headShadeCode) and name from
// the recipe. A nil entity disables the hook.
func ensureShadeMaster(ctx context.Context, repo shade.Repository, entity *mbhead.Entity, actor string) {
	if entity == nil {
		return
	}
	appshade.EnsureShadeMaster(ctx, repo, headShadeCode(entity), entity.ShadeName(), actor)
}
