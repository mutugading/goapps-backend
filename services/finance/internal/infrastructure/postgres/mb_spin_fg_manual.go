package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbspin"
)

var _ mbspin.ManualFgRepository = (*MBSpinRepository)(nil)

// ApplyManualFg links a spin to a POY product (MANUAL) or clears the link (fgProductID == 0)
// in one transaction: FOR UPDATE on the spin, FOR SHARE on the POY. Writes only the three
// FG columns plus audit stamps; never the head, cost_product_parameter or any recalc/sync.
func (r *MBSpinRepository) ApplyManualFg(ctx context.Context, spinID uuid.UUID, fgProductID int64, actor string) (err error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	committed := false
	defer func() {
		if committed {
			return
		}
		if rbErr := tx.Rollback(); rbErr != nil && !errors.Is(rbErr, sql.ErrTxDone) && err == nil {
			err = fmt.Errorf("rollback: %w", rbErr)
		}
	}()

	if err = applyManualFgInTx(ctx, tx, spinID, fgProductID, actor); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	committed = true
	return nil
}

func applyManualFgInTx(ctx context.Context, tx *sql.Tx, spinID uuid.UUID, fgProductID int64, actor string) error {
	var (
		deleted, active, superseded bool
		curFg                       int64
		curSource                   sql.NullString
		normShade                   string
		root, headHasFg             bool
	)
	err := tx.QueryRowContext(ctx, `
		SELECT deleted_at IS NOT NULL, mbs_is_active, mbs_superseded_by_spin_id IS NOT NULL,
		       COALESCE(mbs_fg_product_id, 0), mbs_fg_link_source, COALESCE(UPPER(TRIM(mbs_shade_code)), ''),
		       mbs_parent_spin_id IS NULL,
		       EXISTS (SELECT 1 FROM mst_mb_head h WHERE h.mbh_id = mst_mb_spin.mbs_mbh_id AND h.mbh_fg_product_id IS NOT NULL)
		  FROM mst_mb_spin
		 WHERE mbs_id = $1
		   FOR UPDATE`, spinID).Scan(&deleted, &active, &superseded, &curFg, &curSource, &normShade, &root, &headHasFg)
	if errors.Is(err, sql.ErrNoRows) {
		return mbspin.ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("lock spin: %w", err)
	}
	if deleted {
		return mbspin.ErrAlreadyDeleted
	}
	if !active || superseded {
		return mbspin.ErrFgSpinNotLive
	}
	// Same predicate as the FG-follows-recipe read flag. A root spin whose head carries an FG
	// is re-synced from the head on every recipe save (syncRootSpinFgTx), so a manual edit
	// would be silently reverted (P8-R backend BE-3).
	var srcPtr *string
	if curSource.Valid {
		srcPtr = &curSource.String
	}
	if mbspin.FgFollowsRecipe(srcPtr, root, headHasFg) {
		return mbspin.ErrFgFollowsRecipe
	}

	if fgProductID == 0 {
		return execManualFg(ctx, tx, spinID, sql.NullInt64{}, sql.NullString{}, sql.NullString{}, actor)
	}
	if fgProductID == curFg {
		return nil
	}
	poy, err := validateManualPoy(ctx, tx, spinID, normShade, fgProductID)
	if err != nil {
		return err
	}

	var item sql.NullString
	if poy.erpItem.Valid && strings.TrimSpace(poy.erpItem.String) != "" {
		item = sql.NullString{String: strings.TrimSpace(poy.erpItem.String), Valid: true}
	}
	return execManualFg(ctx, tx, spinID, sql.NullInt64{Int64: fgProductID, Valid: true}, item,
		sql.NullString{String: mbspin.FgLinkSourceManual, Valid: true}, actor)
}

func execManualFg(ctx context.Context, tx *sql.Tx, spinID uuid.UUID, fg sql.NullInt64, item, source sql.NullString, actor string) error {
	_, err := tx.ExecContext(ctx, `
		UPDATE mst_mb_spin
		   SET mbs_fg_product_id = $2, mbs_fg_item_code = $3, mbs_fg_link_source = $4,
		       updated_at = NOW(), updated_by = $5
		 WHERE mbs_id = $1`, spinID, fg, item, source, actor)
	if err != nil {
		if isSpinShadeFgViolation(err) {
			return mbspin.ErrSpinShadeFgExists
		}
		return fmt.Errorf("update spin fg: %w", err)
	}
	return nil
}

// validateManualPoy checks the POY (active, type POY, same non-blank shade) and that no other
// live spin holds (shade, FG). Manual edit never writes the product master, so a blank shade is rejected.
func validateManualPoy(ctx context.Context, tx *sql.Tx, spinID uuid.UUID, normShade string, fgProductID int64) (backfillPoyState, error) {
	poy, found, err := lockBackfillPoy(ctx, tx, fgProductID)
	if err != nil {
		return poy, err
	}
	if !found || !poy.isPoy || !poy.active {
		return poy, mbspin.ErrFgNotPoy
	}
	if normShade == "" || poy.normShade != normShade {
		return poy, mbspin.ErrFgShadeMismatch
	}
	other, err := findLiveSpinWithShadeFg(ctx, tx, spinID.String(), normShade, fgProductID)
	if err != nil {
		return poy, err
	}
	if other != "" {
		return poy, mbspin.ErrSpinShadeFgExists
	}
	return poy, nil
}

// UpdateWithManualFg saves the entity columns and applies the manual FG change in ONE
// transaction, so a failure of either leaves nothing persisted (P8-R backend BE-5).
func (r *MBSpinRepository) UpdateWithManualFg(ctx context.Context, entity *mbspin.Entity, fgProductID int64, actor string) error {
	return r.db.Transaction(ctx, func(tx *sql.Tx) error {
		if err := applyManualFgInTx(ctx, tx, entity.ID(), fgProductID, actor); err != nil {
			return err
		}
		return r.updateOn(ctx, tx, entity)
	})
}
