package costproductrequest_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	app "github.com/mutugading/goapps-backend/services/finance/internal/application/costproductrequest"
	cpr "github.com/mutugading/goapps-backend/services/finance/internal/domain/costproductrequest"
)

// fakeSADRequestRepo is a minimal cpr.Repository fake used only by
// SubmitAndDecide tests. It records every Save call so the test can assert
// the exact sequence of domain-state transitions that were persisted.
type fakeSADRequestRepo struct {
	req         *cpr.Request
	savedStatus []string // status recorded at each Save call, in order
}

func (r *fakeSADRequestRepo) Create(_ context.Context, _ *cpr.Request) error { return nil }

func (r *fakeSADRequestRepo) GetByID(_ context.Context, _ int64) (*cpr.Request, error) {
	return r.req, nil
}

func (r *fakeSADRequestRepo) GetByNo(_ context.Context, _ string) (*cpr.Request, error) {
	return nil, nil
}

func (r *fakeSADRequestRepo) Save(_ context.Context, req *cpr.Request) error {
	r.savedStatus = append(r.savedStatus, req.Status())
	r.req = req
	return nil
}

func (r *fakeSADRequestRepo) List(_ context.Context, _ cpr.Filter) ([]*cpr.Request, int64, error) {
	return nil, 0, nil
}

func (r *fakeSADRequestRepo) ListAll(_ context.Context, _ cpr.Filter) ([]*cpr.Request, error) {
	return nil, nil
}

// fakeSADNotifier records every CPREvent dispatched via CPRNotifier.
type fakeSADNotifier struct {
	events []app.CPREvent
}

func (n *fakeSADNotifier) NotifyEvent(_ context.Context, event app.CPREvent) error {
	n.events = append(n.events, event)
	return nil
}

// newDraftRequest builds a DRAFT request with the given base classification,
// ready to be driven through SubmitAndDecide.
func newDraftRequest(t *testing.T, classification string) *cpr.Request {
	t.Helper()
	req, err := cpr.New(cpr.NewInput{
		RequestTypeID:         1,
		Title:                 "t",
		CustomerName:          "Acme",
		ProductClassification: classification,
		UrgencyLevel:          cpr.UrgencyMedium,
		RequesterUserID:       "user-1",
	})
	require.NoError(t, err)
	req.SetIDs(1, "CR-2026-0001")
	return req
}

func newSubmitAndDecideHandler(reqRepo cpr.Repository, notifier *fakeSADNotifier) *app.TransitionHandler {
	h := app.NewTransitionHandler(reqRepo)
	if notifier != nil {
		h = h.WithCPRNotifier(notifier)
	}
	return h
}

// assertConsolidatedNotificationPair asserts that exactly the 2 consolidated
// notifications (CPR_SUBMITTED_REVIEWER + CPR_SUBMITTED_ACK) were emitted,
// with no duplicates and nothing else (e.g. no CPR_UNDER_REVIEW, CPR_FEASIBLE,
// CPR_NOT_FEASIBLE, or CPR_ROUTING_NEEDED).
func assertConsolidatedNotificationPair(t *testing.T, events []app.CPREvent) {
	t.Helper()
	require.Len(t, events, 2, "want exactly 2 notifications, got %d: %+v", len(events), events)
	assert.Equal(t, "CPR_SUBMITTED_REVIEWER", events[0].EventType)
	assert.Equal(t, "CPR_SUBMITTED_ACK", events[1].EventType)
}

func TestSubmitAndDecide_Feasible_ExistingClassification_Sequence(t *testing.T) {
	t.Parallel()

	req := newDraftRequest(t, cpr.ClassExisting)
	repo := &fakeSADRequestRepo{req: req}
	notifier := &fakeSADNotifier{}
	h := newSubmitAndDecideHandler(repo, notifier)

	got, err := h.SubmitAndDecide(context.Background(), 1, cpr.ClassExisting, "", cpr.FeasibilityFeasible, "", 42, "reviewer-1", "Reviewer One")

	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, cpr.StatusRoutingDefined, got.Status())
	assert.Equal(t, cpr.ClassExisting, got.VerifiedClassification())
	assert.Equal(t, int64(42), got.LinkedRouteHeadID())

	// Exact sequence of persisted states: SUBMITTED -> UNDER_REVIEW -> (verify,
	// no status change) -> ROUTING_DEFINED -> (link route, no status change).
	require.Equal(t, []string{
		cpr.StatusSubmitted,
		cpr.StatusUnderReview,
		cpr.StatusUnderReview, // VerifyClassification does not change status
		cpr.StatusRoutingDefined,
		cpr.StatusRoutingDefined, // LinkRoute does not change status
	}, repo.savedStatus)

	assertConsolidatedNotificationPair(t, notifier.events)
}

func TestSubmitAndDecide_Feasible_NewClassification_Sequence(t *testing.T) {
	t.Parallel()

	// Base classification starts pending; VerifyClassification resolves it to "new".
	req := newDraftRequest(t, cpr.ClassPending)
	repo := &fakeSADRequestRepo{req: req}
	notifier := &fakeSADNotifier{}
	h := newSubmitAndDecideHandler(repo, notifier)

	got, err := h.SubmitAndDecide(context.Background(), 1, cpr.ClassNew, "marketing miscoded this", cpr.FeasibilityFeasible, "", 99, "reviewer-1", "Reviewer One")

	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, cpr.StatusRoutingDefined, got.Status())
	assert.Equal(t, cpr.ClassNew, got.VerifiedClassification())
	assert.Equal(t, int64(99), got.LinkedRouteHeadID())

	require.Equal(t, []string{
		cpr.StatusSubmitted,
		cpr.StatusUnderReview,
		cpr.StatusUnderReview,
		cpr.StatusRoutingDefined,
		cpr.StatusRoutingDefined,
	}, repo.savedStatus)

	assertConsolidatedNotificationPair(t, notifier.events)
}

func TestSubmitAndDecide_NotFeasible_NoLinkRoute(t *testing.T) {
	t.Parallel()

	req := newDraftRequest(t, cpr.ClassExisting)
	repo := &fakeSADRequestRepo{req: req}
	notifier := &fakeSADNotifier{}
	h := newSubmitAndDecideHandler(repo, notifier)

	got, err := h.SubmitAndDecide(context.Background(), 1, cpr.ClassExisting, "", cpr.FeasibilityNotFeasible, "not viable", 0, "reviewer-1", "Reviewer One")

	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, cpr.StatusRejected, got.Status())
	assert.Equal(t, int64(0), got.LinkedRouteHeadID(), "LinkRoute must NOT be called on NOT_FEASIBLE")

	// Only 4 saves: submit, start review, verify, decide feasibility — no 5th
	// save for LinkRoute since the decision was NOT_FEASIBLE.
	require.Equal(t, []string{
		cpr.StatusSubmitted,
		cpr.StatusUnderReview,
		cpr.StatusUnderReview,
		cpr.StatusRejected,
	}, repo.savedStatus)

	assertConsolidatedNotificationPair(t, notifier.events)
}

func TestSubmitAndDecide_Feasible_ZeroHeadID_SkipsLinkRoute(t *testing.T) {
	t.Parallel()

	// Even on FEASIBLE, a referenceProductHeadID of 0 (unresolved routing) must
	// not attempt LinkRoute (which would fail domain validation on headID<=0).
	req := newDraftRequest(t, cpr.ClassExisting)
	repo := &fakeSADRequestRepo{req: req}
	notifier := &fakeSADNotifier{}
	h := newSubmitAndDecideHandler(repo, notifier)

	got, err := h.SubmitAndDecide(context.Background(), 1, cpr.ClassExisting, "", cpr.FeasibilityFeasible, "", 0, "reviewer-1", "Reviewer One")

	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, cpr.StatusRoutingDefined, got.Status())
	assert.Equal(t, int64(0), got.LinkedRouteHeadID())

	require.Equal(t, []string{
		cpr.StatusSubmitted,
		cpr.StatusUnderReview,
		cpr.StatusUnderReview,
		cpr.StatusRoutingDefined,
	}, repo.savedStatus)

	assertConsolidatedNotificationPair(t, notifier.events)
}

func TestSubmitAndDecide_PendingClassificationUnverified_Rejected(t *testing.T) {
	t.Parallel()

	// Passing "pending" as verified is rejected by VerifyClassification's own
	// validation (allowedVerifiedClassification only allows existing|new) —
	// the composition must stop there and emit no notifications at all.
	req := newDraftRequest(t, cpr.ClassPending)
	repo := &fakeSADRequestRepo{req: req}
	notifier := &fakeSADNotifier{}
	h := newSubmitAndDecideHandler(repo, notifier)

	got, err := h.SubmitAndDecide(context.Background(), 1, cpr.ClassPending, "", cpr.FeasibilityFeasible, "", 42, "reviewer-1", "Reviewer One")

	require.Error(t, err)
	assert.Nil(t, got)
	assert.Empty(t, notifier.events, "no notification should fire when the composition fails partway through")
}

// TestSubmitAndDecide_NoWorkflowClientWiring is a compile-time-flavored guard:
// TransitionHandler no longer exposes WithWorkflowClient at all (the field and
// setter were removed per design.md §3 B3), so SubmitAndDecide cannot
// accidentally depend on IAM workflow instances. If this package fails to
// compile because WithWorkflowClient still exists and this test references a
// removed symbol, that itself is the regression signal — so this test simply
// exercises the handler end-to-end without ever wiring a workflow client and
// asserts no panic/timeout occurs.
func TestSubmitAndDecide_NoWorkflowClientWiring(t *testing.T) {
	t.Parallel()

	req := newDraftRequest(t, cpr.ClassExisting)
	repo := &fakeSADRequestRepo{req: req}
	h := app.NewTransitionHandler(repo) // no notifier, no workflow client attached at all

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	got, err := h.SubmitAndDecide(ctx, 1, cpr.ClassExisting, "", cpr.FeasibilityFeasible, "", 7, "reviewer-1", "Reviewer One")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, cpr.StatusRoutingDefined, got.Status())
}
