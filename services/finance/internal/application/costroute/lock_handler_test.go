package costroute_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	app "github.com/mutugading/goapps-backend/services/finance/internal/application/costroute"
	costroute "github.com/mutugading/goapps-backend/services/finance/internal/domain/costroute"
)

// headWithStatus returns a *costroute.Head with the given routing status.
func headWithStatus(id int64, status string) *costroute.Head {
	return &costroute.Head{
		HeadID:        id,
		ProductSysID:  1,
		ProductCode:   "FG-001",
		RoutingStatus: status,
	}
}

// --- fakes for LockHandler / UnlockHandler ---

// fakeRepo is a configurable fake for costroute.Repository.
type fakeRepo struct {
	fakeRepoForDup // embed no-op implementations for all other methods
	head           *costroute.Head
	getHeadErr     error
	saveHeadErr    error
	savedHead      *costroute.Head
	saveHeadCalled bool
}

func (r *fakeRepo) GetHead(_ context.Context, _ int64) (*costroute.Head, error) {
	return r.head, r.getHeadErr
}

func (r *fakeRepo) SaveHead(_ context.Context, h *costroute.Head, _ string) error {
	r.saveHeadCalled = true
	r.savedHead = h
	return r.saveHeadErr
}

// fakeChecker is a configurable fake for app.ParamCompletenessChecker.
type fakeChecker struct {
	unfilled int
	err      error
}

func (c *fakeChecker) CountUnfilledParams(_ context.Context, _ int64) (int, error) {
	return c.unfilled, c.err
}

// fakeNotifier is a configurable fake for app.RouteNotifier.
type fakeNotifier struct {
	lockedCalled   bool
	unlockedCalled bool
	lockErr        error
	unlockErr      error
}

func (n *fakeNotifier) NotifyRouteLocked(_ context.Context, _ int64, _, _ string) error {
	n.lockedCalled = true
	return n.lockErr
}

func (n *fakeNotifier) NotifyRouteUnlocked(_ context.Context, _ int64, _, _ string) error {
	n.unlockedCalled = true
	return n.unlockErr
}

// --- LockHandler tests ---

func TestLockHandler_Handle(t *testing.T) {
	t.Run("locks when all params filled", func(t *testing.T) {
		repo := &fakeRepo{head: headWithStatus(10, costroute.StatusComplete)}
		checker := &fakeChecker{unfilled: 0}
		notifier := &fakeNotifier{}

		h := app.NewLockHandler(repo).
			WithParamChecker(checker).
			WithNotifier(notifier)

		got, err := h.Handle(context.Background(), 10, "user-1", "Alice")

		require.NoError(t, err)
		require.NotNil(t, got)
		assert.Equal(t, costroute.StatusLocked, got.RoutingStatus)
		assert.True(t, repo.saveHeadCalled, "repo.SaveHead must be called")
		assert.True(t, notifier.lockedCalled, "notifier.NotifyRouteLocked must be called")
	})

	t.Run("rejects when params incomplete", func(t *testing.T) {
		repo := &fakeRepo{head: headWithStatus(10, costroute.StatusComplete)}
		checker := &fakeChecker{unfilled: 3}
		notifier := &fakeNotifier{}

		h := app.NewLockHandler(repo).
			WithParamChecker(checker).
			WithNotifier(notifier)

		got, err := h.Handle(context.Background(), 10, "user-1", "Alice")

		require.Error(t, err)
		assert.Nil(t, got)
		assert.True(t, errors.Is(err, costroute.ErrParamIncomplete), "error must wrap ErrParamIncomplete")
		assert.False(t, repo.saveHeadCalled, "repo.SaveHead must NOT be called when params incomplete")
		assert.False(t, notifier.lockedCalled, "notifier must NOT be called when params incomplete")
	})

	t.Run("works without checker", func(t *testing.T) {
		repo := &fakeRepo{head: headWithStatus(20, costroute.StatusComplete)}
		notifier := &fakeNotifier{}

		h := app.NewLockHandler(repo).WithNotifier(notifier)

		got, err := h.Handle(context.Background(), 20, "user-2", "Bob")

		require.NoError(t, err)
		require.NotNil(t, got)
		assert.Equal(t, costroute.StatusLocked, got.RoutingStatus)
		assert.True(t, repo.saveHeadCalled)
		assert.True(t, notifier.lockedCalled)
	})

	t.Run("works without notifier", func(t *testing.T) {
		repo := &fakeRepo{head: headWithStatus(30, costroute.StatusComplete)}
		checker := &fakeChecker{unfilled: 0}

		h := app.NewLockHandler(repo).WithParamChecker(checker)

		got, err := h.Handle(context.Background(), 30, "user-3", "Carol")

		require.NoError(t, err)
		require.NotNil(t, got)
		assert.Equal(t, costroute.StatusLocked, got.RoutingStatus)
		assert.True(t, repo.saveHeadCalled)
	})

	t.Run("propagates repo GetHead error", func(t *testing.T) {
		repoErr := errors.New("db unavailable")
		repo := &fakeRepo{getHeadErr: repoErr}

		h := app.NewLockHandler(repo)

		got, err := h.Handle(context.Background(), 99, "user-4", "Dave")

		require.Error(t, err)
		assert.Nil(t, got)
		assert.True(t, errors.Is(err, repoErr))
		assert.False(t, repo.saveHeadCalled)
	})

	t.Run("propagates checker error", func(t *testing.T) {
		repo := &fakeRepo{head: headWithStatus(10, costroute.StatusComplete)}
		checkerErr := errors.New("checker failed")
		checker := &fakeChecker{err: checkerErr}

		h := app.NewLockHandler(repo).WithParamChecker(checker)

		got, err := h.Handle(context.Background(), 10, "user-5", "Eve")

		require.Error(t, err)
		assert.Nil(t, got)
		assert.True(t, errors.Is(err, checkerErr))
		assert.False(t, repo.saveHeadCalled)
	})

	t.Run("notify failure is non-blocking", func(t *testing.T) {
		repo := &fakeRepo{head: headWithStatus(40, costroute.StatusComplete)}
		notifier := &fakeNotifier{lockErr: errors.New("notification service down")}

		h := app.NewLockHandler(repo).WithNotifier(notifier)

		// Lock should succeed even if notifier returns an error.
		got, err := h.Handle(context.Background(), 40, "user-6", "Frank")

		require.NoError(t, err)
		require.NotNil(t, got)
		assert.Equal(t, costroute.StatusLocked, got.RoutingStatus)
		assert.True(t, notifier.lockedCalled)
	})
}

// --- UnlockHandler tests ---

func TestUnlockHandler_Handle(t *testing.T) {
	t.Run("unlocks when route is LOCKED", func(t *testing.T) {
		repo := &fakeRepo{head: headWithStatus(10, costroute.StatusLocked)}
		notifier := &fakeNotifier{}

		h := app.NewUnlockHandler(repo).WithNotifier(notifier)

		got, err := h.Handle(context.Background(), 10, "user-1", "Alice")

		require.NoError(t, err)
		require.NotNil(t, got)
		assert.Equal(t, costroute.StatusComplete, got.RoutingStatus)
		assert.True(t, repo.saveHeadCalled, "repo.SaveHead must be called")
		assert.True(t, notifier.unlockedCalled, "notifier.NotifyRouteUnlocked must be called")
	})

	t.Run("fails when route is not LOCKED", func(t *testing.T) {
		tests := []struct {
			name   string
			status string
		}{
			{"draft status", costroute.StatusDraft},
			{"complete status", costroute.StatusComplete},
		}
		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				repo := &fakeRepo{head: headWithStatus(20, tc.status)}
				notifier := &fakeNotifier{}

				h := app.NewUnlockHandler(repo).WithNotifier(notifier)

				got, err := h.Handle(context.Background(), 20, "user-1", "Alice")

				require.Error(t, err)
				assert.Nil(t, got)
				assert.True(t, errors.Is(err, costroute.ErrInvalidStatusTransition))
				assert.False(t, repo.saveHeadCalled, "repo.SaveHead must NOT be called on invalid transition")
				assert.False(t, notifier.unlockedCalled)
			})
		}
	})

	t.Run("works without notifier", func(t *testing.T) {
		repo := &fakeRepo{head: headWithStatus(30, costroute.StatusLocked)}

		h := app.NewUnlockHandler(repo)

		got, err := h.Handle(context.Background(), 30, "user-2", "Bob")

		require.NoError(t, err)
		require.NotNil(t, got)
		assert.Equal(t, costroute.StatusComplete, got.RoutingStatus)
		assert.True(t, repo.saveHeadCalled)
	})

	t.Run("propagates repo GetHead error", func(t *testing.T) {
		repoErr := errors.New("connection reset")
		repo := &fakeRepo{getHeadErr: repoErr}

		h := app.NewUnlockHandler(repo)

		got, err := h.Handle(context.Background(), 99, "user-3", "Carol")

		require.Error(t, err)
		assert.Nil(t, got)
		assert.True(t, errors.Is(err, repoErr))
		assert.False(t, repo.saveHeadCalled)
	})

	t.Run("notify failure is non-blocking", func(t *testing.T) {
		repo := &fakeRepo{head: headWithStatus(40, costroute.StatusLocked)}
		notifier := &fakeNotifier{unlockErr: errors.New("notification service down")}

		h := app.NewUnlockHandler(repo).WithNotifier(notifier)

		got, err := h.Handle(context.Background(), 40, "user-4", "Dave")

		require.NoError(t, err)
		require.NotNil(t, got)
		assert.Equal(t, costroute.StatusComplete, got.RoutingStatus)
		assert.True(t, notifier.unlockedCalled)
	})
}
