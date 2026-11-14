package shade

import (
	"context"
	"errors"
	"strings"

	"github.com/rs/zerolog/log"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/shade"
)

// EnsureShadeMaster auto-creates a MANUAL shade master row (cost_erp_shade) for the
// given shade code when none exists yet. It is strictly best-effort: every failure
// is logged as a warning and swallowed, so a shade-master problem never fails the
// caller's save. An existing shade (any source) is left untouched. A nil repo or an
// empty code disables the hook. An empty name falls back to the code.
func EnsureShadeMaster(ctx context.Context, repo shade.Repository, code, name, actor string) {
	if repo == nil {
		return
	}
	code = shade.NormalizeCode(code)
	if code == "" {
		return
	}
	logger := log.With().Str("shade_code", code).Logger()

	existing, err := repo.GetByCode(ctx, code)
	if err != nil && !errors.Is(err, shade.ErrNotFound) {
		logger.Warn().Err(err).Msg("shade master lookup failed; shade not auto-created")
		return
	}
	if existing != nil {
		return
	}

	name = strings.TrimSpace(name)
	if name == "" {
		name = code
	}
	s, err := shade.New(shade.NewParams{Code: code, Name: name, CreatedBy: actor})
	if err != nil {
		logger.Warn().Err(err).Msg("shade master auto-create rejected by domain")
		return
	}
	if err := repo.Create(ctx, s); err != nil {
		logger.Warn().Err(err).Msg("shade master auto-create failed")
		return
	}
	logger.Info().Msg("shade master auto-created (MANUAL)")
}
