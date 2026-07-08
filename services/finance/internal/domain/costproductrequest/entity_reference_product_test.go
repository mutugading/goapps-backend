package costproductrequest_test

import (
	"errors"
	"testing"

	cpr "github.com/mutugading/goapps-backend/services/finance/internal/domain/costproductrequest"
)

func draftExisting(t *testing.T, referenceProductSysID int64) *cpr.Request {
	t.Helper()
	r, err := cpr.New(cpr.NewInput{
		RequestTypeID:         1,
		Title:                 "reference product test",
		CustomerName:          "Acme",
		ProductClassification: cpr.ClassExisting,
		UrgencyLevel:          cpr.UrgencyMedium,
		RequesterUserID:       "user-1",
		ReferenceProductSysID: referenceProductSysID,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return r
}

// TestNew_ReferenceProductSysID_UnsetByDefault verifies the "0 = unset"
// convention holds when NewInput omits the field entirely.
func TestNew_ReferenceProductSysID_UnsetByDefault(t *testing.T) {
	t.Parallel()

	r := draftExisting(t, 0)
	if got := r.ReferenceProductSysID(); got != 0 {
		t.Fatalf("want 0 (unset), got %d", got)
	}
}

// TestNew_ReferenceProductSysID_SetAtCreate verifies a positive value passed
// at creation is stored and readable via the getter.
func TestNew_ReferenceProductSysID_SetAtCreate(t *testing.T) {
	t.Parallel()

	r := draftExisting(t, 42)
	if got := r.ReferenceProductSysID(); got != 42 {
		t.Fatalf("want 42, got %d", got)
	}
}

// TestNew_ReferenceProductSysID_NegativeRejected verifies New() rejects a
// negative reference product sys id rather than silently accepting it.
func TestNew_ReferenceProductSysID_NegativeRejected(t *testing.T) {
	t.Parallel()

	_, err := cpr.New(cpr.NewInput{
		RequestTypeID:         1,
		Title:                 "negative reference",
		CustomerName:          "Acme",
		ProductClassification: cpr.ClassExisting,
		UrgencyLevel:          cpr.UrgencyMedium,
		RequesterUserID:       "user-1",
		ReferenceProductSysID: -1,
	})
	if !errors.Is(err, cpr.ErrInvalidReferenceProduct) {
		t.Fatalf("want ErrInvalidReferenceProduct, got %v", err)
	}
}

func validUpdateInput(referenceProductSysID *int64) cpr.UpdateInput {
	return cpr.UpdateInput{
		Title:                 "updated title",
		CustomerName:          "Acme",
		ProductClassification: cpr.ClassExisting,
		UrgencyLevel:          cpr.UrgencyMedium,
		ReferenceProductSysID: referenceProductSysID,
	}
}

// TestUpdate_ReferenceProductSysID_NilLeavesUnchanged verifies the
// pointer-optional convention: a nil ReferenceProductSysID in UpdateInput
// leaves the existing value untouched (matches how Spec/other optional
// Update fields behave elsewhere in this file).
func TestUpdate_ReferenceProductSysID_NilLeavesUnchanged(t *testing.T) {
	t.Parallel()

	r := draftExisting(t, 7)
	if err := r.Update(validUpdateInput(nil)); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if got := r.ReferenceProductSysID(); got != 7 {
		t.Fatalf("want unchanged 7, got %d", got)
	}
}

// TestUpdate_ReferenceProductSysID_SetsNewValue verifies a non-nil pointer
// replaces the stored value, including replacing a previously set value.
func TestUpdate_ReferenceProductSysID_SetsNewValue(t *testing.T) {
	t.Parallel()

	r := draftExisting(t, 7)
	newVal := int64(99)
	if err := r.Update(validUpdateInput(&newVal)); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if got := r.ReferenceProductSysID(); got != 99 {
		t.Fatalf("want 99, got %d", got)
	}
}

// TestUpdate_ReferenceProductSysID_ZeroClearsIt verifies that explicitly
// passing a pointer to 0 clears a previously set reference — 0 is the
// canonical "unset" sentinel, not a value that gets rejected.
func TestUpdate_ReferenceProductSysID_ZeroClearsIt(t *testing.T) {
	t.Parallel()

	r := draftExisting(t, 7)
	zero := int64(0)
	if err := r.Update(validUpdateInput(&zero)); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if got := r.ReferenceProductSysID(); got != 0 {
		t.Fatalf("want 0 (cleared), got %d", got)
	}
}

// TestUpdate_ReferenceProductSysID_NegativeRejected verifies Update() applies
// the same validation as New() for this field.
func TestUpdate_ReferenceProductSysID_NegativeRejected(t *testing.T) {
	t.Parallel()

	r := draftExisting(t, 7)
	negative := int64(-5)
	err := r.Update(validUpdateInput(&negative))
	if !errors.Is(err, cpr.ErrInvalidReferenceProduct) {
		t.Fatalf("want ErrInvalidReferenceProduct, got %v", err)
	}
	// Confirm the rejected update did not mutate the stored value.
	if got := r.ReferenceProductSysID(); got != 7 {
		t.Fatalf("want unchanged 7 after rejected update, got %d", got)
	}
}

// TestReconstruct_ReferenceProductSysID verifies Reconstruct round-trips the
// field from persistence without any validation applied.
func TestReconstruct_ReferenceProductSysID(t *testing.T) {
	t.Parallel()

	r := cpr.Reconstruct(cpr.ReconstructInput{
		RequestID:             1,
		Title:                 "reconstructed",
		CustomerName:          "Acme",
		ProductClassification: cpr.ClassExisting,
		UrgencyLevel:          cpr.UrgencyMedium,
		Status:                cpr.StatusDraft,
		ReferenceProductSysID: 123456,
	})
	if got := r.ReferenceProductSysID(); got != 123456 {
		t.Fatalf("want 123456, got %d", got)
	}
}

// TestReconstruct_ReferenceProductSysID_ZeroValueDefault verifies the "0 =
// unset" convention holds for rows reconstructed without the field set
// (e.g. rows persisted before this field existed).
func TestReconstruct_ReferenceProductSysID_ZeroValueDefault(t *testing.T) {
	t.Parallel()

	r := cpr.Reconstruct(cpr.ReconstructInput{
		RequestID:             1,
		Title:                 "legacy row",
		CustomerName:          "Acme",
		ProductClassification: cpr.ClassExisting,
		UrgencyLevel:          cpr.UrgencyMedium,
		Status:                cpr.StatusDraft,
	})
	if got := r.ReferenceProductSysID(); got != 0 {
		t.Fatalf("want 0 (unset), got %d", got)
	}
}
