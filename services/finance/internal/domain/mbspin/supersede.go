package mbspin

import (
	"context"

	"github.com/google/uuid"
)

// Supersede read model (mb-spin-fg-poy-link P3, design §2.3 / §2.5 / Revisi 1 D7 / Revisi 2).
//
// The types live in package mbspin (not mbhead) because mbspin has no domain imports:
// both the mbhead application layer (Validate, Preview) and the mbspin one (Unsupersede)
// can use them without an import cycle.

// Supersede repoint kinds of a cost_product_parameter MB_SP_CODE row.
const (
	// SupersedeRepointPointer is a row whose cpp_value_mb_spin_id points at the old spin
	// (repointed whatever its cpp_filled_by — D7).
	SupersedeRepointPointer = "POINTER"
	// SupersedeRepointTextOnly is a row with a NULL pointer whose text equals the old spin's
	// ORION code (Revisi 2) — repointed only while that code is unique to the old spin.
	SupersedeRepointTextOnly = "TEXT_ONLY"
)

// SupersedeFilledByCount is the number of repointable MB_SP_CODE rows per cpp_filled_by.
type SupersedeFilledByCount struct {
	FilledBy string
	Count    int
}

// SupersedeAffectedProduct is one product whose MB_SP_CODE row would be repointed.
type SupersedeAffectedProduct struct {
	ProductSysID int64
	ProductCode  string
	ProductName  string
	FilledBy     string
	// RepointKind is SupersedeRepointPointer or SupersedeRepointTextOnly.
	RepointKind string
}

// SupersedeCandidate is one live spin of ANOTHER head that a new root spin with the same
// UPPER(TRIM(shade)) + FG would supersede, plus what the supersede would touch.
type SupersedeCandidate struct {
	OldSpinID     uuid.UUID
	OldSpinCode   string
	ShadeCode     string
	FgProductID   int64
	FgProductCode string

	OldHeadID   uuid.UUID
	OldHeadCode string
	// OldHeadWillDeactivate: the old head is active and has no other live+active spin
	// besides the candidates (design §2.5 step 3).
	OldHeadWillDeactivate bool

	// OldMBProductSysID is the old head's mbh_cost_product_id (0 when none).
	OldMBProductSysID int64
	// OldMBProductWillDeactivate: the product is active and no live+active spin other than
	// the candidates points at it through mbs_cost_product_id (design §2.5 step 4).
	OldMBProductWillDeactivate bool

	// ParamRowsPointer / ParamRowsTextOnly count the repointable (unlocked) MB_SP_CODE rows.
	ParamRowsPointer  int
	ParamRowsTextOnly int
	// FilledByCounts groups both kinds of repointable rows by cpp_filled_by (sorted).
	FilledByCounts   []SupersedeFilledByCount
	AffectedProducts []SupersedeAffectedProduct
	// Warnings lists rows that will NOT be repointed (shared ORION code, locked product).
	Warnings []string
}

// SupersedePreview is the read-only answer of PreviewMBHeadSupersede.
type SupersedePreview struct {
	RequiresConfirmation bool
	Candidates           []SupersedeCandidate
}

// SupersedeReader finds supersede candidates for a head's (future) root spin.
type SupersedeReader interface {
	// PreviewSupersede returns the live, active, not-superseded spins of heads other than
	// newHeadID with the same UPPER(TRIM(shadeCode)) and FG, with their repoint counts.
	// Read-only. An empty result means no supersede is needed.
	PreviewSupersede(ctx context.Context, newHeadID uuid.UUID, shadeCode string, fgProductID int64) ([]SupersedeCandidate, error)
}
