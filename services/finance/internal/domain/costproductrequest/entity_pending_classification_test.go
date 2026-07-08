package costproductrequest_test

import (
	"errors"
	"testing"

	cpr "github.com/mutugading/goapps-backend/services/finance/internal/domain/costproductrequest"
)

func fullSpec() *cpr.SpecInput {
	return &cpr.SpecInput{
		RawMaterialType:    cpr.RawMatPOYBoughtout,
		ProductDescription: "test product",
		PaperTubeTypeID:    1,
		WeightPerBobbinKg:  "1.5",
		BoxType:            cpr.BoxTypeNormal,
		ShadeCode:          "BRIGHT",
	}
}

func TestNew_ClassificationSpecCoupling(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		classification string
		spec           *cpr.SpecInput
		wantErr        error
	}{
		{
			name:           "pending with no spec succeeds",
			classification: cpr.ClassPending,
			spec:           nil,
			wantErr:        nil,
		},
		{
			name:           "pending with full spec succeeds",
			classification: cpr.ClassPending,
			spec:           fullSpec(),
			wantErr:        nil,
		},
		{
			name:           "existing with spec still fails",
			classification: cpr.ClassExisting,
			spec:           fullSpec(),
			wantErr:        cpr.ErrSpecNotAllowed,
		},
		{
			name:           "existing with no spec succeeds",
			classification: cpr.ClassExisting,
			spec:           nil,
			wantErr:        nil,
		},
		{
			name:           "new without spec still fails",
			classification: cpr.ClassNew,
			spec:           nil,
			wantErr:        cpr.ErrSpecRequired,
		},
		{
			name:           "new with full spec succeeds",
			classification: cpr.ClassNew,
			spec:           fullSpec(),
			wantErr:        nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			r, err := cpr.New(cpr.NewInput{
				RequestTypeID:         1,
				Title:                 "t",
				CustomerName:          "Acme",
				ProductClassification: tt.classification,
				UrgencyLevel:          cpr.UrgencyMedium,
				RequesterUserID:       "user-1",
				Spec:                  tt.spec,
			})
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("want %v, got %v", tt.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if r.ProductClassification() != tt.classification {
				t.Fatalf("want classification %s, got %s", tt.classification, r.ProductClassification())
			}
		})
	}
}

func TestVerifyClassification_FromPending_NoReasonRequired(t *testing.T) {
	t.Parallel()

	r, err := cpr.New(cpr.NewInput{
		RequestTypeID:         1,
		Title:                 "pending request",
		CustomerName:          "Acme",
		ProductClassification: cpr.ClassPending,
		UrgencyLevel:          cpr.UrgencyMedium,
		RequesterUserID:       "user-1",
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := r.Submit(); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if err := r.StartReview(); err != nil {
		t.Fatalf("StartReview: %v", err)
	}
	// Resolving a pending classification is not an override — no reason needed,
	// even though verified ("new") differs from base ("pending").
	if err := r.VerifyClassification(cpr.ClassNew, ""); err != nil {
		t.Fatalf("want success without reason, got %v", err)
	}
	if r.VerifiedClassification() != cpr.ClassNew {
		t.Fatalf("want verifiedClassification=%s, got %s", cpr.ClassNew, r.VerifiedClassification())
	}
	if r.ClassificationOverrideReason() != "" {
		t.Fatalf("want empty override reason, got %q", r.ClassificationOverrideReason())
	}
}

func TestVerifyClassification_RealToRealMismatch_StillRequiresReason(t *testing.T) {
	t.Parallel()

	r, err := cpr.New(cpr.NewInput{
		RequestTypeID:         1,
		Title:                 "existing request",
		CustomerName:          "Acme",
		ProductClassification: cpr.ClassExisting,
		UrgencyLevel:          cpr.UrgencyMedium,
		RequesterUserID:       "user-1",
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := r.Submit(); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if err := r.StartReview(); err != nil {
		t.Fatalf("StartReview: %v", err)
	}
	// Regression: a real classification (existing) being overridden to a
	// different real classification (new) still requires an override reason.
	if err := r.VerifyClassification(cpr.ClassNew, ""); !errors.Is(err, cpr.ErrOverrideReasonRequired) {
		t.Fatalf("want ErrOverrideReasonRequired, got %v", err)
	}
	if err := r.VerifyClassification(cpr.ClassNew, "customer confirmed new tooling"); err != nil {
		t.Fatalf("want success with reason, got %v", err)
	}
	if r.ClassificationOverrideReason() == "" {
		t.Fatal("want non-empty override reason")
	}
}

func TestVerifyClassification_RejectsPendingAsVerifiedValue(t *testing.T) {
	t.Parallel()

	r, err := cpr.New(cpr.NewInput{
		RequestTypeID:         1,
		Title:                 "pending request",
		CustomerName:          "Acme",
		ProductClassification: cpr.ClassPending,
		UrgencyLevel:          cpr.UrgencyMedium,
		RequesterUserID:       "user-1",
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := r.Submit(); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if err := r.StartReview(); err != nil {
		t.Fatalf("StartReview: %v", err)
	}
	if err := r.VerifyClassification(cpr.ClassPending, ""); !errors.Is(err, cpr.ErrInvalidVerified) {
		t.Fatalf("want ErrInvalidVerified, got %v", err)
	}
}
