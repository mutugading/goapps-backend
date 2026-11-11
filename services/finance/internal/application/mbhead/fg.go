package mbhead

import (
	"context"
	"errors"
	"strings"

	"github.com/rs/zerolog/log"

	appshade "github.com/mutugading/goapps-backend/services/finance/internal/application/shade"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbhead"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbspin"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/shade"
)

// ErrFgReaderUnavailable is returned when an FG link is requested but no
// FgProductReader is wired (server misconfiguration).
var ErrFgReaderUnavailable = errors.New("mbhead: fg product reader is not configured")

// fgLink is a validated FG selection waiting to be persisted on the head.
type fgLink struct {
	poy *mbspin.FgProduct
	// fillShade is true when the POY has no shade yet and must receive the recipe shade.
	fillShade bool
}

// resolveFg loads the POY and enforces design section 2.1 against the recipe shade:
// active POY (ErrFgNotPoy), shade equal or empty (ErrFgShadeMismatch otherwise).
func resolveFg(ctx context.Context, reader mbspin.FgProductReader, fgID int64, recipeShade string) (fgLink, error) {
	if reader == nil {
		return fgLink{}, ErrFgReaderUnavailable
	}
	poy, err := reader.GetFgProduct(ctx, fgID)
	if err != nil {
		return fgLink{}, err
	}
	if poy == nil || !poy.IsActive || poy.TypeCode != "POY" {
		return fgLink{}, mbspin.ErrFgNotPoy
	}
	poyShade := ""
	if poy.ShadeCode != nil {
		poyShade = *poy.ShadeCode
	}
	fill, err := mbspin.CheckFgShade(recipeShade, poyShade)
	if err != nil {
		return fgLink{}, err
	}
	return fgLink{poy: poy, fillShade: fill}, nil
}

// apply snapshots the FG onto the head entity.
func (l fgLink) apply(entity *mbhead.Entity) error {
	id := l.poy.ID
	var code *string
	if l.poy.ErpItemCode != nil {
		if v := strings.TrimSpace(*l.poy.ErpItemCode); v != "" {
			code = &v
		}
	}
	return entity.SetFg(&id, code)
}

// afterSave fills an empty POY shade with the recipe shade and ensures the shade
// master. Best-effort: failures are logged, never fail the save.
func (l fgLink) afterSave(ctx context.Context, reader mbspin.FgProductReader, shadeRepo shade.Repository, entity *mbhead.Entity, actor string) {
	if l.poy == nil || !l.fillShade {
		return
	}
	code := strings.TrimSpace(headShadeCode(entity))
	if code == "" {
		return
	}
	if _, err := reader.FillEmptyFgShade(ctx, l.poy.ID, code, actor); err != nil {
		log.Warn().Err(err).Int64("fg_product_id", l.poy.ID).Msg("MB head FG: fill empty POY shade failed")
	}
	appshade.EnsureShadeMaster(ctx, shadeRepo, code, entity.ShadeName(), actor)
}
