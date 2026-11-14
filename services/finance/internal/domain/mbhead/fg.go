package mbhead

import (
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

// ErrInvalidFgProduct is returned when SetFg receives a non-positive product id.
// FG business errors (ErrFgNotPoy, ErrFgShadeMismatch, ErrSpinShadeFgExists) live
// in package mbspin, which has no domain imports, so application code of both
// aggregates can share them without an import cycle.
var ErrInvalidFgProduct = errors.New("mbhead: fg product id must be positive")

// FgReadModel carries joined, display-only values (not persisted on the head).
type FgReadModel struct {
	FgProductCode *string
	FgProductName *string
}

// FgProductID returns the linked FG (POY) cost product id, or nil.
func (e *Entity) FgProductID() *int64 { return e.fgProductID }

// FgItemCode returns the snapshot of the FG's ERP item code, or nil.
func (e *Entity) FgItemCode() *string { return e.fgItemCode }

// SupersededByHeadID returns the head that superseded this one, or nil.
func (e *Entity) SupersededByHeadID() *uuid.UUID { return e.supersededByHeadID }

// SupersededAt returns when this head was superseded, or nil.
func (e *Entity) SupersededAt() *time.Time { return e.supersededAt }

// IsSuperseded reports whether the head has been superseded.
func (e *Entity) IsSuperseded() bool { return e.supersededByHeadID != nil }

// FgProductCode returns the joined FG product code (read model), or nil.
func (e *Entity) FgProductCode() *string { return e.fgProductCode }

// FgProductName returns the joined FG product name (read model), or nil.
func (e *Entity) FgProductName() *string { return e.fgProductName }

// SetFg links the head to an FG (POY) product. POY-ness and shade are validated
// by the caller. An empty itemCode is stored as nil.
func (e *Entity) SetFg(productID *int64, itemCode *string) error {
	if productID == nil || *productID <= 0 {
		return ErrInvalidFgProduct
	}
	pid := *productID
	e.fgProductID = &pid
	e.fgItemCode = nil
	if itemCode != nil {
		if v := strings.TrimSpace(*itemCode); v != "" {
			e.fgItemCode = &v
		}
	}
	return nil
}

// ClearFg removes the FG link.
func (e *Entity) ClearFg() {
	e.fgProductID, e.fgItemCode = nil, nil
}

// HydrateFg applies persisted FG/supersede columns onto a Reconstruct-ed entity.
func (e *Entity) HydrateFg(productID *int64, itemCode *string, supersededByHeadID *uuid.UUID, supersededAt *time.Time) {
	e.fgProductID, e.fgItemCode = productID, itemCode
	e.supersededByHeadID, e.supersededAt = supersededByHeadID, supersededAt
}

// HydrateFgReadModel applies joined display values.
func (e *Entity) HydrateFgReadModel(r FgReadModel) {
	e.fgProductCode, e.fgProductName = r.FgProductCode, r.FgProductName
}
