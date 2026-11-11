package mbspin

import (
	"strings"
	"time"

	"github.com/google/uuid"
)

// FG link source values stored in mbs_fg_link_source (CHECK constraint, migration 000607).
const (
	// FgLinkSourceRecipe marks a spin whose FG link was inherited from the head at Validate.
	FgLinkSourceRecipe = "RECIPE"
	// FgLinkSourceDuplicate marks a spin whose FG link was chosen at Duplicate.
	FgLinkSourceDuplicate = "DUPLICATE"
	// FgLinkSourceManual marks a spin whose FG link was set manually.
	FgLinkSourceManual = "MANUAL"
	// FgLinkSourceBackfill marks a spin whose FG link was set by the backfill.
	FgLinkSourceBackfill = "BACKFILL"
)

// IsValidFgLinkSource reports whether s is one of the allowed link sources.
func IsValidFgLinkSource(s string) bool {
	switch s {
	case FgLinkSourceRecipe, FgLinkSourceDuplicate, FgLinkSourceManual, FgLinkSourceBackfill:
		return true
	}
	return false
}

// CheckFgShade applies the design §2.1 shade rule with UPPER(TRIM) comparison.
// Empty POY shade => fillPoyShade=true (caller fills it with the recipe shade);
// equal => (false, nil); different => ErrFgShadeMismatch. A blank recipe shade
// is always rejected (ErrFgShadeMismatch): an FG cannot be linked to a shade-less recipe.
func CheckFgShade(recipeShade, poyShade string) (fillPoyShade bool, err error) {
	if strings.TrimSpace(recipeShade) == "" {
		return false, ErrFgShadeMismatch
	}
	poy := strings.ToUpper(strings.TrimSpace(poyShade))
	if poy == "" {
		return true, nil
	}
	if poy != strings.ToUpper(strings.TrimSpace(recipeShade)) {
		return false, ErrFgShadeMismatch
	}
	return false, nil
}

// FgSupersede is the persisted supersede trail of a spin (migration 000607).
type FgSupersede struct {
	SupersededBySpinID *uuid.UUID
	SupersededAt       *time.Time
	SupersededBy       *string
}

// FgReadModel carries joined, display-only values (not persisted on the spin).
type FgReadModel struct {
	FgProductCode        *string
	FgProductName        *string
	SupersededBySpinCode *string
}

// FgProductID returns the linked FG (POY) cost product id, or nil.
func (e *Entity) FgProductID() *int64 { return e.fgProductID }

// FgItemCode returns the snapshot of the FG's ERP item code, or nil.
func (e *Entity) FgItemCode() *string { return e.fgItemCode }

// FgLinkSource returns how the FG link was established, or nil.
func (e *Entity) FgLinkSource() *string { return e.fgLinkSource }

// SupersededBySpinID returns the spin that superseded this one, or nil.
func (e *Entity) SupersededBySpinID() *uuid.UUID { return e.supersededBySpinID }

// SupersededAt returns when this spin was superseded, or nil.
func (e *Entity) SupersededAt() *time.Time { return e.supersededAt }

// SupersededBy returns who superseded this spin, or nil.
func (e *Entity) SupersededBy() *string { return e.supersededBy }

// IsSuperseded reports whether the spin has been superseded.
func (e *Entity) IsSuperseded() bool { return e.supersededBySpinID != nil }

// FgProductCode returns the joined FG product code (read model), or nil.
func (e *Entity) FgProductCode() *string { return e.fgProductCode }

// FgProductName returns the joined FG product name (read model), or nil.
func (e *Entity) FgProductName() *string { return e.fgProductName }

// SupersededBySpinCode returns the joined superseding spin code (read model), or nil.
func (e *Entity) SupersededBySpinCode() *string { return e.supersededBySpinCode }

// WithFg links the spin to an FG (POY) product. linkSource must be a known enum
// value (ErrInvalidFgLinkSource) and productID must be non-nil and positive
// (ErrFgNotPoy); POY-ness and shade are checked by the caller. An empty
// itemCode is stored as nil.
func (e *Entity) WithFg(productID *int64, itemCode *string, linkSource string) error {
	if !IsValidFgLinkSource(linkSource) {
		return ErrInvalidFgLinkSource
	}
	if productID == nil || *productID <= 0 {
		return ErrFgNotPoy
	}
	pid := *productID
	e.fgProductID = &pid
	e.fgItemCode = normalizeOptional(itemCode)
	src := linkSource
	e.fgLinkSource = &src
	return nil
}

// ClearFg removes the FG link.
func (e *Entity) ClearFg() {
	e.fgProductID, e.fgItemCode, e.fgLinkSource = nil, nil, nil
}

// HydrateFg applies persisted FG/supersede columns onto a Reconstruct-ed entity.
// Unvalidated on purpose (persisted data).
func (e *Entity) HydrateFg(productID *int64, itemCode, linkSource *string, s FgSupersede) {
	e.fgProductID, e.fgItemCode, e.fgLinkSource = productID, itemCode, linkSource
	e.supersededBySpinID, e.supersededAt, e.supersededBy = s.SupersededBySpinID, s.SupersededAt, s.SupersededBy
}

// HydrateFgReadModel applies joined display values.
func (e *Entity) HydrateFgReadModel(r FgReadModel) {
	e.fgProductCode, e.fgProductName, e.supersededBySpinCode = r.FgProductCode, r.FgProductName, r.SupersededBySpinCode
}

func normalizeOptional(s *string) *string {
	if s == nil {
		return nil
	}
	v := strings.TrimSpace(*s)
	if v == "" {
		return nil
	}
	return &v
}
