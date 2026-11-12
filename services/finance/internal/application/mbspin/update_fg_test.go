package mbspin_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/application/mbspin"
	mbspindomain "github.com/mutugading/goapps-backend/services/finance/internal/domain/mbspin"
)

type fakeManualFg struct {
	calls  int
	gotFg  int64
	retErr error
}

func (f *fakeManualFg) ApplyManualFg(_ context.Context, _ uuid.UUID, fg int64, _ string) error {
	f.calls++
	f.gotFg = fg
	return f.retErr
}

func (f *fakeManualFg) UpdateWithManualFg(_ context.Context, _ *mbspindomain.Entity, fg int64, _ string) error {
	f.calls++
	f.gotFg = fg
	return f.retErr
}

func newFgSpin(t *testing.T, source string, fg int64) *mbspindomain.Entity {
	t.Helper()
	e, err := mbspindomain.New(uuid.New(), "Spin", nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, "u")
	require.NoError(t, err)
	if source != "" {
		id := fg
		require.NoError(t, e.WithFg(&id, nil, source))
	}
	return e
}

func i64(v int64) *int64 { return &v }

func TestUpdateHandler_ManualFg(t *testing.T) {
	ctx := context.Background()

	run := func(e *mbspindomain.Entity, fg *int64, fake *fakeManualFg) (*mbspindomain.Entity, error, *MockRepository) {
		repo := new(MockRepository)
		repo.On("GetByID", mock.Anything, mock.Anything).Return(e, nil)
		repo.On("Update", mock.Anything, mock.Anything).Return(nil)
		h := mbspin.NewUpdateHandler(repo).WithManualFg(fake)
		got, err := h.Handle(ctx, mbspin.UpdateCommand{ID: e.ID(), FgProductID: fg, UpdatedBy: "u"})
		return got, err, repo
	}

	t.Run("nil FgProductID leaves FG untouched", func(t *testing.T) {
		fake := &fakeManualFg{}
		_, err, _ := run(newFgSpin(t, "", 0), nil, fake)
		require.NoError(t, err)
		assert.Zero(t, fake.calls)
	})

	t.Run("recipe-linked spin rejects set and clear", func(t *testing.T) {
		for _, v := range []int64{0, 9} {
			fake := &fakeManualFg{}
			_, err, repo := run(newFgSpin(t, mbspindomain.FgLinkSourceRecipe, 5), i64(v), fake)
			require.ErrorIs(t, err, mbspindomain.ErrFgFollowsRecipe)
			assert.Zero(t, fake.calls)
			repo.AssertNotCalled(t, "Update")
		}
	})

	t.Run("set calls repo with product id", func(t *testing.T) {
		fake := &fakeManualFg{}
		_, err, _ := run(newFgSpin(t, "", 0), i64(7), fake)
		require.NoError(t, err)
		assert.Equal(t, 1, fake.calls)
		assert.Equal(t, int64(7), fake.gotFg)
	})

	t.Run("clear on linked manual spin calls repo with 0", func(t *testing.T) {
		fake := &fakeManualFg{}
		_, err, _ := run(newFgSpin(t, mbspindomain.FgLinkSourceManual, 5), i64(0), fake)
		require.NoError(t, err)
		assert.Equal(t, 1, fake.calls)
		assert.Equal(t, int64(0), fake.gotFg)
	})

	t.Run("same value and clear-when-empty are no-ops", func(t *testing.T) {
		fake := &fakeManualFg{}
		_, err, _ := run(newFgSpin(t, mbspindomain.FgLinkSourceManual, 5), i64(5), fake)
		require.NoError(t, err)
		_, err, _ = run(newFgSpin(t, "", 0), i64(0), fake)
		require.NoError(t, err)
		assert.Zero(t, fake.calls)
	})

	t.Run("inactive spin rejected", func(t *testing.T) {
		fake := &fakeManualFg{}
		e := newFgSpin(t, "", 0)
		off := false
		require.NoError(t, e.Update(mbspindomain.UpdateInput{IsActive: &off}, "u"))
		_, err, _ := run(e, i64(7), fake)
		require.ErrorIs(t, err, mbspindomain.ErrFgSpinNotLive)
		assert.Zero(t, fake.calls)
	})

	t.Run("repo error propagates and skips save", func(t *testing.T) {
		fake := &fakeManualFg{retErr: mbspindomain.ErrFgShadeMismatch}
		_, err, repo := run(newFgSpin(t, "", 0), i64(7), fake)
		require.ErrorIs(t, err, mbspindomain.ErrFgShadeMismatch)
		repo.AssertNotCalled(t, "Update")
	})
}

func TestUpdateHandler_RejectsReactivatingSupersededSpin(t *testing.T) {
	e := newFgSpin(t, "", 0)
	by := uuid.New()
	e.HydrateFg(nil, nil, nil, mbspindomain.FgSupersede{SupersededBySpinID: &by})
	off := false
	require.NoError(t, e.Update(mbspindomain.UpdateInput{IsActive: &off}, "u"))
	repo := new(MockRepository)
	repo.On("GetByID", mock.Anything, mock.Anything).Return(e, nil)
	on := true
	_, err := mbspin.NewUpdateHandler(repo).Handle(context.Background(),
		mbspin.UpdateCommand{ID: e.ID(), IsActive: &on, UpdatedBy: "u"})
	require.ErrorIs(t, err, mbspindomain.ErrSpinSuperseded)
	repo.AssertNotCalled(t, "Update")
}

func TestUpdateHandler_ManualFgSaveIsAtomic(t *testing.T) {
	// BE-5: the FG write and the column update go through one repo call.
	fake := &fakeManualFg{}
	e := newFgSpin(t, "", 0)
	repo := new(MockRepository)
	repo.On("GetByID", mock.Anything, mock.Anything).Return(e, nil)
	_, err := mbspin.NewUpdateHandler(repo).WithManualFg(fake).Handle(context.Background(),
		mbspin.UpdateCommand{ID: e.ID(), FgProductID: i64(7), UpdatedBy: "u"})
	require.NoError(t, err)
	assert.Equal(t, 1, fake.calls)
	repo.AssertNotCalled(t, "Update")
}
