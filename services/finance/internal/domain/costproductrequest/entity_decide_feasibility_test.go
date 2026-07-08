package costproductrequest_test

import (
	"errors"
	"testing"

	cpr "github.com/mutugading/goapps-backend/services/finance/internal/domain/costproductrequest"
)

func newUnderReview(t *testing.T, classification string) *cpr.Request {
	t.Helper()
	r, err := cpr.New(cpr.NewInput{
		RequestTypeID:         1,
		Title:                 "t",
		CustomerName:          "Acme",
		ProductClassification: classification,
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
	return r
}

func TestDecideFeasibility_PendingClassificationUnverified_Rejected(t *testing.T) {
	t.Parallel()

	r := newUnderReview(t, cpr.ClassPending)
	if err := r.DecideFeasibility(cpr.FeasibilityFeasible, "", "reviewer-1"); !errors.Is(err, cpr.ErrClassificationNotVerified) {
		t.Fatalf("want ErrClassificationNotVerified, got %v", err)
	}
	if r.VerifiedClassification() != "" {
		t.Fatalf("want verifiedClassification to remain unset, got %q", r.VerifiedClassification())
	}
}

func TestDecideFeasibility_PendingClassificationVerifiedFirst_Succeeds(t *testing.T) {
	t.Parallel()

	r := newUnderReview(t, cpr.ClassPending)
	if err := r.VerifyClassification(cpr.ClassNew, ""); err != nil {
		t.Fatalf("VerifyClassification: %v", err)
	}
	if err := r.DecideFeasibility(cpr.FeasibilityFeasible, "", "reviewer-1"); err != nil {
		t.Fatalf("DecideFeasibility: %v", err)
	}
	if r.VerifiedClassification() != cpr.ClassNew {
		t.Fatalf("want verifiedClassification=%s, got %s", cpr.ClassNew, r.VerifiedClassification())
	}
}

func TestDecideFeasibility_RealClassificationUnverified_PreservesIt(t *testing.T) {
	t.Parallel()

	r := newUnderReview(t, cpr.ClassExisting)
	if err := r.DecideFeasibility(cpr.FeasibilityFeasible, "", "reviewer-1"); err != nil {
		t.Fatalf("DecideFeasibility: %v", err)
	}
	if r.VerifiedClassification() != cpr.ClassExisting {
		t.Fatalf("want verifiedClassification=%s, got %s", cpr.ClassExisting, r.VerifiedClassification())
	}
}

func TestDecideFeasibility_NotFeasible_PendingClassificationUnverified_Rejected(t *testing.T) {
	t.Parallel()

	r := newUnderReview(t, cpr.ClassPending)
	if err := r.DecideFeasibility(cpr.FeasibilityNotFeasible, "not viable", "reviewer-1"); !errors.Is(err, cpr.ErrClassificationNotVerified) {
		t.Fatalf("want ErrClassificationNotVerified, got %v", err)
	}
}
