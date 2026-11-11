package shade_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	appshade "github.com/mutugading/goapps-backend/services/finance/internal/application/shade"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/shade"
)

func TestEnsureShadeMaster(t *testing.T) {
	ctx := context.Background()

	t.Run("nil repo is a no-op", func(t *testing.T) {
		appshade.EnsureShadeMaster(ctx, nil, "ABC", "n", "u")
	})

	t.Run("empty code is a no-op", func(t *testing.T) {
		repo := new(MockRepository)
		appshade.EnsureShadeMaster(ctx, repo, "   ", "n", "u")
		repo.AssertNotCalled(t, "GetByCode", mock.Anything, mock.Anything)
	})

	t.Run("creates when missing, name falls back to code", func(t *testing.T) {
		repo := new(MockRepository)
		repo.On("GetByCode", ctx, "ABC").Return(nil, shade.ErrNotFound)
		repo.On("Create", ctx, mock.MatchedBy(func(s *shade.Shade) bool {
			return s.Code() == "ABC" && s.Name() == "ABC"
		})).Return(nil)
		appshade.EnsureShadeMaster(ctx, repo, "abc", "  ", "u")
		repo.AssertExpectations(t)
	})

	t.Run("uses provided name", func(t *testing.T) {
		repo := new(MockRepository)
		repo.On("GetByCode", ctx, "ABC").Return(nil, shade.ErrNotFound)
		repo.On("Create", ctx, mock.MatchedBy(func(s *shade.Shade) bool {
			return s.Name() == "Shade Name"
		})).Return(nil)
		appshade.EnsureShadeMaster(ctx, repo, "ABC", "Shade Name", "u")
		repo.AssertExpectations(t)
	})

	t.Run("existing is untouched", func(t *testing.T) {
		repo := new(MockRepository)
		s, err := shade.New(shade.NewParams{Code: "ABC", Name: "ABC", CreatedBy: "seed"})
		require.NoError(t, err)
		repo.On("GetByCode", ctx, "ABC").Return(s, nil)
		appshade.EnsureShadeMaster(ctx, repo, "ABC", "n", "u")
		repo.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
	})

	t.Run("lookup failure is swallowed", func(t *testing.T) {
		repo := new(MockRepository)
		repo.On("GetByCode", ctx, "ABC").Return(nil, errors.New("db down"))
		appshade.EnsureShadeMaster(ctx, repo, "ABC", "n", "u")
		repo.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
	})

	t.Run("create failure is swallowed", func(t *testing.T) {
		repo := new(MockRepository)
		repo.On("GetByCode", ctx, "ABC").Return(nil, shade.ErrNotFound)
		repo.On("Create", ctx, mock.Anything).Return(errors.New("boom"))
		appshade.EnsureShadeMaster(ctx, repo, "ABC", "n", "u")
		repo.AssertExpectations(t)
	})
}
