package grpc

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	financev1 "github.com/mutugading/goapps-backend/gen/finance/v1"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbhead"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbspin"
)

type stubFgReader struct{ poy *mbspin.FgProduct }

func (s stubFgReader) GetFgProduct(_ context.Context, _ int64) (*mbspin.FgProduct, error) {
	return s.poy, nil
}

func (stubFgReader) FillEmptyFgShade(_ context.Context, _ int64, _, _ string) (bool, error) {
	return true, nil
}

func newFgHandler(t *testing.T, poy *mbspin.FgProduct) (*MBHeadHandler, *fakeFrozenMBHeadRepo) {
	t.Helper()
	h, repo := newFrozenSealHandler(t)
	return h.WithFgReader(stubFgReader{poy: poy}), repo
}

func okPoy() *mbspin.FgProduct {
	shade := "SH-1"
	return &mbspin.FgProduct{ID: 9, TypeCode: "POY", IsActive: true, ShadeCode: &shade}
}

func TestUpdateMBHead_FgTriState(t *testing.T) {
	ctx := context.Background()
	seed := func(t *testing.T, repo *fakeFrozenMBHeadRepo) {
		seedFrozenHead(t, repo, "")
		pid := int64(4)
		require.NoError(t, repo.stored.SetFg(&pid, nil))
	}
	id := func(repo *fakeFrozenMBHeadRepo) string { return repo.stored.ID().String() }

	t.Run("absent leaves FG unchanged", func(t *testing.T) {
		h, repo := newFgHandler(t, okPoy())
		seed(t, repo)
		resp, err := h.UpdateMBHead(ctx, &financev1.UpdateMBHeadRequest{MbhId: id(repo)})
		require.NoError(t, err)
		require.True(t, resp.Base.IsSuccess, resp.Base.Message)
		assert.Equal(t, int64(4), resp.Data.FgProductId)
	})
	t.Run("zero clears FG", func(t *testing.T) {
		h, repo := newFgHandler(t, okPoy())
		seed(t, repo)
		zero := int64(0)
		resp, err := h.UpdateMBHead(ctx, &financev1.UpdateMBHeadRequest{MbhId: id(repo), FgProductId: &zero})
		require.NoError(t, err)
		require.True(t, resp.Base.IsSuccess, resp.Base.Message)
		assert.Zero(t, resp.Data.FgProductId)
	})
	t.Run("positive sets FG", func(t *testing.T) {
		h, repo := newFgHandler(t, okPoy())
		seed(t, repo)
		v := int64(9)
		resp, err := h.UpdateMBHead(ctx, &financev1.UpdateMBHeadRequest{MbhId: id(repo), MbhCode: strPtr("SH-1"), FgProductId: &v})
		require.NoError(t, err)
		require.True(t, resp.Base.IsSuccess, resp.Base.Message)
		assert.Equal(t, int64(9), resp.Data.FgProductId)
	})
	t.Run("non POY maps to 400", func(t *testing.T) {
		p := okPoy()
		p.TypeCode = "FDY"
		h, repo := newFgHandler(t, p)
		seed(t, repo)
		v := int64(9)
		resp, err := h.UpdateMBHead(ctx, &financev1.UpdateMBHeadRequest{MbhId: id(repo), FgProductId: &v})
		require.NoError(t, err)
		assert.False(t, resp.Base.IsSuccess)
		assert.Equal(t, "400", resp.Base.StatusCode)
	})
}

func TestFgErrorMapping(t *testing.T) {
	assert.Equal(t, "400", mbHeadWriteErrToBase(mbspin.ErrFgNotPoy).StatusCode)
	assert.Equal(t, "400", mbHeadWriteErrToBase(fmt.Errorf("w: %w", mbspin.ErrFgShadeMismatch)).StatusCode)
	assert.Equal(t, "409", mbHeadWriteErrToBase(mbspin.ErrSpinShadeFgExists).StatusCode)
	assert.Equal(t, "400", mbSpinErrorToBaseResponse(mbspin.ErrFgNotPoy).StatusCode)
	assert.Equal(t, "400", mbSpinErrorToBaseResponse(mbspin.ErrFgShadeMismatch).StatusCode)
	assert.Equal(t, "409", mbSpinErrorToBaseResponse(mbspin.ErrSpinShadeFgExists).StatusCode)
}

func TestMBSpinEntityToProto_Fg(t *testing.T) {
	e, err := mbspin.New(uuid.New(), "spin", nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, "a")
	require.NoError(t, err)
	pid, item, src := int64(5), "ERP-5", mbspin.FgLinkSourceDuplicate
	sup, at, by := uuid.New(), time.Date(2026, 11, 11, 1, 2, 3, 0, time.UTC), "bob"
	e.HydrateFg(&pid, &item, &src, mbspin.FgSupersede{SupersededBySpinID: &sup, SupersededAt: &at, SupersededBy: &by})
	code, name, supCode := "POY-5", "Poy five", "SPIN-X"
	e.HydrateFgReadModel(mbspin.FgReadModel{FgProductCode: &code, FgProductName: &name, SupersededBySpinCode: &supCode})

	p := mbSpinEntityToProto(e)
	assert.Equal(t, int64(5), p.FgProductId)
	assert.Equal(t, "ERP-5", p.FgItemCode)
	assert.Equal(t, "POY-5", p.FgProductCode)
	assert.Equal(t, "Poy five", p.FgProductName)
	assert.Equal(t, "DUPLICATE", p.FgLinkSource)
	assert.Equal(t, sup.String(), p.SupersededBySpinId)
	assert.Equal(t, "2026-11-11T01:02:03Z", p.SupersededAt)
	assert.Equal(t, "SPIN-X", p.SupersededBySpinCode)

	plain, err := mbspin.New(uuid.New(), "s", nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, "a")
	require.NoError(t, err)
	q := mbSpinEntityToProto(plain)
	assert.Zero(t, q.FgProductId)
	assert.False(t, q.FgFollowsRecipe)
	assert.False(t, p.FgFollowsRecipe)
	rec := mbspin.FgLinkSourceRecipe
	e.HydrateFg(&pid, &item, &rec, mbspin.FgSupersede{})
	assert.True(t, mbSpinEntityToProto(e).FgFollowsRecipe)
	assert.Empty(t, q.SupersededAt)
}

func TestMBHeadEntityToProto_Fg(t *testing.T) {
	e, err := mbhead.New(mbhead.NewParams{MBCosting: "MB", CreatedBy: "a"})
	require.NoError(t, err)
	pid, item := int64(8), "ERP-8"
	sup, at := uuid.New(), time.Date(2026, 11, 11, 0, 0, 0, 0, time.UTC)
	e.HydrateFg(&pid, &item, &sup, &at)
	code, name := "POY-8", "Eight"
	e.HydrateFgReadModel(mbhead.FgReadModel{FgProductCode: &code, FgProductName: &name})
	p := mbHeadEntityToProto(e)
	assert.Equal(t, int64(8), p.FgProductId)
	assert.Equal(t, "ERP-8", p.FgItemCode)
	assert.Equal(t, "POY-8", p.FgProductCode)
	assert.Equal(t, "Eight", p.FgProductName)
	assert.Equal(t, sup.String(), p.SupersededByHeadId)
	assert.Equal(t, "2026-11-11T00:00:00Z", p.SupersededAt)
}
