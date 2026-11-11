package mbspin

import (
	"context"

	"github.com/google/uuid"
)

// Skip reasons of a param row that un-supersede could not revert.
const (
	UnsupersedeSkipChanged        = "changed after supersede"
	UnsupersedeSkipRowDeleted     = "row deleted"
	UnsupersedeSkipProductLocked  = "product locked"
	UnsupersedeSkipNoPreviousData = "value added by supersede (no previous value to restore)"
	// UnsupersedeSkipTriggerKept: a CHILD row is kept because its product's MB_SP_CODE row
	// was not reverted (avoids a mixed state neither spin produced).
	UnsupersedeSkipTriggerKept = "MB_SP_CODE not reverted"
)

// UnsupersedeSkipped is one cost_product_parameter row left as it is by un-supersede.
type UnsupersedeSkipped struct {
	CppValueID   int64
	ProductSysID int64
	ProductCode  string
	ParamCode    string
	Reason       string
}

// UnsupersedeResult is the outcome of a committed un-supersede.
type UnsupersedeResult struct {
	// Spin is the restored (re-activated) spin.
	Spin *Entity
	// BatchID is the supersede batch that was reverted.
	BatchID uuid.UUID
	// NewSpinID is the spin that had superseded Spin (now deactivated, superseded by Spin).
	NewSpinID uuid.UUID
	// ParamsReverted is the number of param rows restored.
	ParamsReverted int
	Skipped        []UnsupersedeSkipped
	// AffectedProductIDs are the products whose params were reverted (sorted, unique).
	AffectedProductIDs []int64
	// Warnings, e.g. the new head / MB product that stay active.
	Warnings []string
}

// Unsuperseder restores a superseded spin in one transaction (symmetric to supersede).
type Unsuperseder interface {
	// Unsupersede restores spinID, which must belong to headID. Errors: ErrNotFound,
	// ErrNotSuperseded, ErrUnsupersedeNoBatch, ErrUnsupersedeConflict.
	Unsupersede(ctx context.Context, headID, spinID uuid.UUID, actor string) (*UnsupersedeResult, error)
}
