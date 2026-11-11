package costproductmaster_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	app "github.com/mutugading/goapps-backend/services/finance/internal/application/costproductmaster"
	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/costproductmaster"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/shade"
)

// spyShadeRepo records Create calls; GetByCode always reports not found.
type spyShadeRepo struct {
	shade.Repository
	created []string
}

func (s *spyShadeRepo) GetByCode(context.Context, string) (*shade.Shade, error) {
	return nil, shade.ErrNotFound
}

func (s *spyShadeRepo) Create(_ context.Context, e *shade.Shade) error {
	s.created = append(s.created, e.Code())
	return nil
}

// updateTrackingRepo records Update calls and serves one stored product.
type updateTrackingRepo struct {
	createTrackingRepo
	updated []*domain.CostProductMaster
	stored  *domain.CostProductMaster

	linkedCreates int
	linkedErr     error
}

func (r *updateTrackingRepo) CreateWithErpLink(_ context.Context, _ *domain.CostProductMaster) error {
	if r.linkedErr != nil {
		return r.linkedErr
	}
	r.linkedCreates++
	return nil
}

func (r *updateTrackingRepo) Update(_ context.Context, p *domain.CostProductMaster) error {
	r.updated = append(r.updated, p)
	return nil
}

func (r *updateTrackingRepo) GetBySysID(context.Context, int64) (*domain.CostProductMaster, error) {
	return r.stored, nil
}

func TestCreateHandler_ErpItemCodeAndShade(t *testing.T) {
	types := &fakeTypeRepo{byID: map[int32]string{2: "POY"}}
	base := app.CreateCommand{ProductTypeID: 2, ProductName: "POY 1", GradeCode: "AX", ShadeCode: "mc-1", ActorUserID: "u"}

	t.Run("erp item code is linked and shade ensured", func(t *testing.T) {
		repo := &updateTrackingRepo{}
		sh := &spyShadeRepo{}
		cmd := base
		cmd.ErpItemCode = " ITM-1 "
		p, err := app.NewCreateHandler(repo, types).WithErpLink(&fakeErpLinkRepo{}, repo, nil).WithShadeMaster(sh).Handle(context.Background(), cmd)
		require.NoError(t, err)
		assert.Equal(t, "ITM-1", p.ErpItemCode())
		assert.Equal(t, 1, repo.linkedCreates)
		assert.False(t, repo.createCalled)
		assert.Equal(t, []string{"MC-1"}, sh.created)
	})

	t.Run("empty erp item code keeps old behavior", func(t *testing.T) {
		repo := &updateTrackingRepo{}
		p, err := app.NewCreateHandler(repo, types).Handle(context.Background(), base)
		require.NoError(t, err)
		assert.Empty(t, p.ErpItemCode())
		assert.Empty(t, repo.updated)
	})

	t.Run("empty shade does not touch shade master", func(t *testing.T) {
		sh := &spyShadeRepo{}
		cmd := base
		cmd.ShadeCode = ""
		_, err := app.NewCreateHandler(&updateTrackingRepo{}, types).WithShadeMaster(sh).Handle(context.Background(), cmd)
		require.NoError(t, err)
		assert.Empty(t, sh.created)
	})

	t.Run("invalid erp item code is rejected before create", func(t *testing.T) {
		repo := &updateTrackingRepo{}
		cmd := base
		cmd.ErpItemCode = "bad\x01code"
		_, err := app.NewCreateHandler(repo, types).Handle(context.Background(), cmd)
		require.Error(t, err)
		assert.False(t, repo.createCalled)
	})

	t.Run("non-POY product with erp item code is rejected", func(t *testing.T) {
		repo := &updateTrackingRepo{}
		cmd := base
		cmd.ProductTypeID = 3
		cmd.ErpItemCode = "ITM-1"
		tt := &fakeTypeRepo{byID: map[int32]string{2: "POY", 3: "YARN"}}
		_, err := app.NewCreateHandler(repo, tt).WithErpLink(&fakeErpLinkRepo{}, repo, nil).Handle(context.Background(), cmd)
		require.ErrorIs(t, err, domain.ErrErpItemRequiresPoy)
		assert.False(t, repo.createCalled)
		assert.Zero(t, repo.linkedCreates)
	})

	t.Run("duplicate key is rejected before anything is created", func(t *testing.T) {
		repo := &updateTrackingRepo{}
		cmd := base
		cmd.ErpItemCode = "ITM-1"
		_, err := app.NewCreateHandler(repo, types).WithErpLink(&fakeErpLinkRepo{dups: []int64{9}}, repo, nil).Handle(context.Background(), cmd)
		require.ErrorIs(t, err, domain.ErrLinkDuplicate)
		assert.False(t, repo.createCalled)
		assert.Zero(t, repo.linkedCreates)
	})

	t.Run("link failure leaves no plain create behind", func(t *testing.T) {
		repo := &updateTrackingRepo{linkedErr: domain.ErrLinkDuplicate}
		cmd := base
		cmd.ErpItemCode = "ITM-1"
		_, err := app.NewCreateHandler(repo, types).WithErpLink(&fakeErpLinkRepo{}, repo, nil).Handle(context.Background(), cmd)
		require.ErrorIs(t, err, domain.ErrLinkDuplicate)
		assert.False(t, repo.createCalled)
	})
}

func TestUpdateHandler_EnsuresShadeOnlyWhenChanged(t *testing.T) {
	newStored := func(t *testing.T) *domain.CostProductMaster {
		p, err := domain.New(domain.NewInput{ProductTypeID: 2, ProductName: "P", ShadeCode: "AA-1", GradeCode: "AX", ActorUserID: "u"})
		require.NoError(t, err)
		return p
	}
	cmd := app.UpdateCommand{ProductName: "P", GradeCode: "AX", ActorUserID: "u"}

	t.Run("changed shade creates master", func(t *testing.T) {
		sh := &spyShadeRepo{}
		repo := &updateTrackingRepo{stored: newStored(t)}
		c := cmd
		c.ShadeCode = "bb-2"
		_, err := app.NewUpdateHandler(repo).WithShadeMaster(sh).Handle(context.Background(), c)
		require.NoError(t, err)
		assert.Equal(t, []string{"BB-2"}, sh.created)
	})

	t.Run("same shade is a no-op", func(t *testing.T) {
		sh := &spyShadeRepo{}
		repo := &updateTrackingRepo{stored: newStored(t)}
		c := cmd
		c.ShadeCode = " aa-1 "
		_, err := app.NewUpdateHandler(repo).WithShadeMaster(sh).Handle(context.Background(), c)
		require.NoError(t, err)
		assert.Empty(t, sh.created)
	})
}
