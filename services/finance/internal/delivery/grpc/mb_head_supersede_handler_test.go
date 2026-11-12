package grpc

import (
	"context"
	"math"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	financev1 "github.com/mutugading/goapps-backend/gen/finance/v1"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbhead"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbspin"
)

type stubSupersedeReader struct{ cands []mbspin.SupersedeCandidate }

func (s stubSupersedeReader) PreviewSupersede(_ context.Context, _ uuid.UUID, _ string, _ int64) ([]mbspin.SupersedeCandidate, error) {
	return s.cands, nil
}

func TestPreviewMBHeadSupersede(t *testing.T) {
	ctx := context.Background()
	oldSpin, oldHead := uuid.New(), uuid.New()
	cand := mbspin.SupersedeCandidate{
		OldSpinID: oldSpin, OldSpinCode: "ORION-OLD", ShadeCode: "S1", FgProductID: 7, FgProductCode: "POY-7",
		OldHeadID: oldHead, OldHeadCode: "MB-OLD", OldHeadWillDeactivate: true,
		OldMBProductSysID: 99, OldMBProductWillDeactivate: true,
		ParamRowsPointer: 2, ParamRowsTextOnly: 1,
		FilledByCounts:   []mbspin.SupersedeFilledByCount{{FilledBy: "auto_mb_source", Count: 2}, {FilledBy: "import", Count: 1}},
		AffectedProducts: []mbspin.SupersedeAffectedProduct{{ProductSysID: 5, ProductCode: "P5", ProductName: "Prod 5", FilledBy: "import"}},
		Warnings:         []string{"w1"},
	}

	newHandler := func(t *testing.T, withFg bool, cands []mbspin.SupersedeCandidate) (*MBHeadHandler, uuid.UUID) {
		t.Helper()
		e, err := mbhead.New(mbhead.NewParams{MBCosting: "MB-NEW", ShadeCode: "S1", CreatedBy: "a"})
		require.NoError(t, err)
		if withFg {
			fg := int64(7)
			require.NoError(t, e.SetFg(&fg, nil))
		}
		repo := &fakeFrozenMBHeadRepo{stored: e}
		h, err := NewMBHeadHandler(repo, nil, fakeMBMachineRepo{})
		require.NoError(t, err)
		return h.WithSupersedeReader(repo, stubSupersedeReader{cands: cands}), e.ID()
	}

	t.Run("maps candidates", func(t *testing.T) {
		h, id := newHandler(t, true, []mbspin.SupersedeCandidate{cand})
		resp, err := h.PreviewMBHeadSupersede(ctx, &financev1.PreviewMBHeadSupersedeRequest{MbhId: id.String()})
		require.NoError(t, err)
		require.True(t, resp.Base.IsSuccess, resp.Base.Message)
		require.True(t, resp.Data.RequiresConfirmation)
		require.Len(t, resp.Data.Candidates, 1)
		c := resp.Data.Candidates[0]
		assert.Equal(t, oldSpin.String(), c.OldSpinId)
		assert.Equal(t, "ORION-OLD", c.OldSpinCode)
		assert.Equal(t, "S1", c.ShadeCode)
		assert.Equal(t, int64(7), c.FgProductId)
		assert.Equal(t, "POY-7", c.FgProductCode)
		assert.Equal(t, oldHead.String(), c.OldHeadId)
		assert.Equal(t, "MB-OLD", c.OldHeadCode)
		assert.True(t, c.OldHeadWillDeactivate)
		assert.Equal(t, int64(99), c.OldMbProductSysId)
		assert.True(t, c.OldMbProductWillDeactivate)
		assert.Equal(t, int32(2), c.ParamRowsPointer)
		assert.Equal(t, int32(1), c.ParamRowsTextOnly)
		require.Len(t, c.FilledByCounts, 2)
		assert.Equal(t, "import", c.FilledByCounts[1].FilledBy)
		assert.Equal(t, int32(1), c.FilledByCounts[1].Count)
		require.Len(t, c.AffectedProducts, 1)
		assert.Equal(t, int64(5), c.AffectedProducts[0].ProductSysId)
		assert.Equal(t, "Prod 5", c.AffectedProducts[0].ProductName)
		assert.Equal(t, []string{"w1"}, c.Warnings)
	})

	t.Run("head without FG", func(t *testing.T) {
		h, id := newHandler(t, false, []mbspin.SupersedeCandidate{cand})
		resp, err := h.PreviewMBHeadSupersede(ctx, &financev1.PreviewMBHeadSupersedeRequest{MbhId: id.String()})
		require.NoError(t, err)
		require.True(t, resp.Base.IsSuccess)
		assert.False(t, resp.Data.RequiresConfirmation)
		assert.Empty(t, resp.Data.Candidates)
	})

	t.Run("not found is 404", func(t *testing.T) {
		repo := &fakeFrozenMBHeadRepo{}
		h, err := NewMBHeadHandler(repo, nil, fakeMBMachineRepo{})
		require.NoError(t, err)
		h = h.WithSupersedeReader(repo, stubSupersedeReader{})
		resp, err := h.PreviewMBHeadSupersede(ctx, &financev1.PreviewMBHeadSupersedeRequest{MbhId: uuid.NewString()})
		require.NoError(t, err)
		assert.Equal(t, "404", resp.Base.StatusCode)
	})

	t.Run("not configured is 500", func(t *testing.T) {
		h, err := NewMBHeadHandler(&fakeFrozenMBHeadRepo{}, nil, fakeMBMachineRepo{})
		require.NoError(t, err)
		resp, err := h.PreviewMBHeadSupersede(ctx, &financev1.PreviewMBHeadSupersedeRequest{MbhId: uuid.NewString()})
		require.NoError(t, err)
		assert.Equal(t, "500", resp.Base.StatusCode)
	})

	t.Run("invalid id is rejected", func(t *testing.T) {
		h, _ := newHandler(t, true, nil)
		resp, err := h.PreviewMBHeadSupersede(ctx, &financev1.PreviewMBHeadSupersedeRequest{MbhId: "nope"})
		require.NoError(t, err)
		assert.False(t, resp.Base.IsSuccess)
	})
}

func TestSupersedeCandidateToProto_ClampsCounts(t *testing.T) {
	c := supersedeCandidateToProto(&mbspin.SupersedeCandidate{ParamRowsPointer: math.MaxInt32 + 10})
	assert.Equal(t, int32(math.MaxInt32), c.ParamRowsPointer)
}

func TestSupersedeErrToBase(t *testing.T) {
	assert.Equal(t, "409", supersedeErrToBase(mbspin.ErrSupersedeConfirmationRequired).StatusCode)
	assert.Equal(t, "404", supersedeErrToBase(mbhead.ErrNotFound).StatusCode)
}
