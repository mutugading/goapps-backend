package mbhead_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/application/mbhead"
	mbheaddomain "github.com/mutugading/goapps-backend/services/finance/internal/domain/mbhead"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbspin"
)

type fakeSupersedeReader struct {
	cands []mbspin.SupersedeCandidate
	err   error
	calls int
	shade string
	fg    int64
	head  uuid.UUID
}

func (f *fakeSupersedeReader) PreviewSupersede(_ context.Context, headID uuid.UUID, shade string, fg int64) ([]mbspin.SupersedeCandidate, error) {
	f.calls++
	f.head, f.shade, f.fg = headID, shade, fg
	return f.cands, f.err
}

func TestPreviewSupersedeHandler(t *testing.T) {
	ctx := context.Background()
	newHead := func(t *testing.T, withFg bool) *mbheaddomain.Entity {
		e, err := mbheaddomain.New(mbheaddomain.NewParams{MBCosting: "MB-SUP", ShadeCode: "S1", CreatedBy: "a"})
		require.NoError(t, err)
		if withFg {
			fg := int64(7)
			require.NoError(t, e.SetFg(&fg, strPtr("ERP-7")))
		}
		return e
	}

	t.Run("head without FG never requires confirmation", func(t *testing.T) {
		head := newHead(t, false)
		repo := new(MockRepository)
		repo.On("GetByID", ctx, head.ID()).Return(head, nil)
		r := &fakeSupersedeReader{}
		got, err := mbhead.NewPreviewSupersedeHandler(repo, r).Handle(ctx, mbhead.PreviewSupersedeQuery{MbhID: head.ID()})
		require.NoError(t, err)
		assert.False(t, got.RequiresConfirmation)
		assert.Empty(t, got.Candidates)
		assert.Zero(t, r.calls)
	})

	t.Run("head with FG and candidates requires confirmation", func(t *testing.T) {
		head := newHead(t, true)
		repo := new(MockRepository)
		repo.On("GetByID", ctx, head.ID()).Return(head, nil)
		r := &fakeSupersedeReader{cands: []mbspin.SupersedeCandidate{{OldSpinCode: "OLD-1"}}}
		got, err := mbhead.NewPreviewSupersedeHandler(repo, r).Handle(ctx, mbhead.PreviewSupersedeQuery{MbhID: head.ID()})
		require.NoError(t, err)
		assert.True(t, got.RequiresConfirmation)
		require.Len(t, got.Candidates, 1)
		assert.Equal(t, head.ID(), r.head)
		assert.Equal(t, "S1", r.shade)
		assert.Equal(t, int64(7), r.fg)
	})

	t.Run("head with FG without candidates", func(t *testing.T) {
		head := newHead(t, true)
		repo := new(MockRepository)
		repo.On("GetByID", ctx, head.ID()).Return(head, nil)
		got, err := mbhead.NewPreviewSupersedeHandler(repo, &fakeSupersedeReader{}).Handle(ctx, mbhead.PreviewSupersedeQuery{MbhID: head.ID()})
		require.NoError(t, err)
		assert.False(t, got.RequiresConfirmation)
	})

	t.Run("head not found propagates", func(t *testing.T) {
		id := uuid.New()
		repo := new(MockRepository)
		repo.On("GetByID", ctx, id).Return(nil, mbheaddomain.ErrNotFound)
		_, err := mbhead.NewPreviewSupersedeHandler(repo, &fakeSupersedeReader{}).Handle(ctx, mbhead.PreviewSupersedeQuery{MbhID: id})
		assert.ErrorIs(t, err, mbheaddomain.ErrNotFound)
	})

	t.Run("reader error propagates", func(t *testing.T) {
		head := newHead(t, true)
		repo := new(MockRepository)
		repo.On("GetByID", ctx, head.ID()).Return(head, nil)
		boom := errors.New("boom")
		_, err := mbhead.NewPreviewSupersedeHandler(repo, &fakeSupersedeReader{err: boom}).Handle(ctx, mbhead.PreviewSupersedeQuery{MbhID: head.ID()})
		assert.ErrorIs(t, err, boom)
	})

	t.Run("nil reader is an error", func(t *testing.T) {
		_, err := mbhead.NewPreviewSupersedeHandler(new(MockRepository), nil).Handle(ctx, mbhead.PreviewSupersedeQuery{MbhID: uuid.New()})
		assert.Error(t, err)
	})
}
