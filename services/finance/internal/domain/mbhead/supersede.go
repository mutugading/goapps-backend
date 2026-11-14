package mbhead

import "github.com/google/uuid"

// AutoGenOptions tunes TransitionWithAutoGenOpts (mb-spin-fg-poy-link P3, design §2.3/§2.5).
// The zero value is exactly the behavior of TransitionWithAutoGen.
type AutoGenOptions struct {
	// ConfirmSupersede lets a first Validate whose new root spin collides (same
	// UPPER(TRIM(shade)) + FG) with live spins of OTHER heads supersede them inside the
	// same transaction. When false such a collision fails with
	// mbspin.ErrSupersedeConfirmationRequired and nothing is written.
	ConfirmSupersede bool
}

// SupersedeOutcome reports what a confirmed supersede changed. It is nil when the
// Validate superseded nothing.
type SupersedeOutcome struct {
	// BatchID is the mst_mb_spin_supersede_item.mssi_batch_id of this action.
	BatchID uuid.UUID
	// NewSpinID is the root spin created for the validated head (S_new).
	NewSpinID uuid.UUID
	// OldSpinIDs are the superseded spins (S_old), in candidate order.
	OldSpinIDs []uuid.UUID
	// DeactivatedHeadIDs are the old heads switched to mbh_is_active=false.
	DeactivatedHeadIDs []uuid.UUID
	// DeactivatedProductIDs are the old MB cost products switched to cpm_is_active=false.
	DeactivatedProductIDs []int64
	// AffectedProductIDs are the products whose MB_SP_CODE row was repointed (sorted,
	// unique) — the post-commit recalc targets.
	AffectedProductIDs []int64
	// RepointedRows is the number of cost_product_parameter rows changed.
	RepointedRows int
	// Warnings lists rows that were NOT repointed (locked product, shared ORION code, ...).
	Warnings []string
}
