package costproductrequest_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	app "github.com/mutugading/goapps-backend/services/finance/internal/application/costproductrequest"
)

// fakeParamSummaryRepo is a configurable fake for app.ParamSummaryRepository.
type fakeParamSummaryRepo struct {
	rows []app.ProductSummaryRow
	err  error
}

func (r *fakeParamSummaryRepo) GetParamSummary(_ context.Context, _ int64) ([]app.ProductSummaryRow, error) {
	return r.rows, r.err
}

func TestGetParamSummaryHandler_Handle(t *testing.T) {
	t.Run("returns correct totals for multiple products and levels", func(t *testing.T) {
		rows := []app.ProductSummaryRow{
			{
				ProductSysID: 1,
				ProductCode:  "FG-001",
				ProductName:  "Product One",
				Levels: []app.LevelSummaryRow{
					{RouteLevel: 1, TotalParams: 5, FilledParams: 3},
					{RouteLevel: 2, TotalParams: 4, FilledParams: 4},
				},
			},
		}
		repo := &fakeParamSummaryRepo{rows: rows}
		h := app.NewGetParamSummaryHandler(repo)

		products, total, filled, err := h.Handle(context.Background(), app.GetParamSummaryQuery{RequestID: 1})

		require.NoError(t, err)
		require.Len(t, products, 1)
		assert.Equal(t, int32(9), total, "total = 5+4")
		assert.Equal(t, int32(7), filled, "filled = 3+4")
	})

	t.Run("sums across multiple products", func(t *testing.T) {
		rows := []app.ProductSummaryRow{
			{
				ProductSysID: 1,
				Levels: []app.LevelSummaryRow{
					{TotalParams: 3, FilledParams: 2},
				},
			},
			{
				ProductSysID: 2,
				Levels: []app.LevelSummaryRow{
					{TotalParams: 6, FilledParams: 6},
					{TotalParams: 2, FilledParams: 1},
				},
			},
		}
		repo := &fakeParamSummaryRepo{rows: rows}
		h := app.NewGetParamSummaryHandler(repo)

		products, total, filled, err := h.Handle(context.Background(), app.GetParamSummaryQuery{RequestID: 7})

		require.NoError(t, err)
		require.Len(t, products, 2)
		assert.Equal(t, int32(11), total, "total = 3+6+2")
		assert.Equal(t, int32(9), filled, "filled = 2+6+1")
	})

	t.Run("empty products returns zero totals", func(t *testing.T) {
		repo := &fakeParamSummaryRepo{rows: []app.ProductSummaryRow{}}
		h := app.NewGetParamSummaryHandler(repo)

		products, total, filled, err := h.Handle(context.Background(), app.GetParamSummaryQuery{RequestID: 2})

		require.NoError(t, err)
		assert.Empty(t, products)
		assert.Equal(t, int32(0), total)
		assert.Equal(t, int32(0), filled)
	})

	t.Run("propagates repo error", func(t *testing.T) {
		repoErr := errors.New("database connection lost")
		repo := &fakeParamSummaryRepo{err: repoErr}
		h := app.NewGetParamSummaryHandler(repo)

		products, total, filled, err := h.Handle(context.Background(), app.GetParamSummaryQuery{RequestID: 3})

		require.Error(t, err)
		assert.Nil(t, products)
		assert.Equal(t, int32(0), total)
		assert.Equal(t, int32(0), filled)
		assert.True(t, errors.Is(err, repoErr))
	})

	t.Run("rejects invalid request ID zero", func(t *testing.T) {
		repo := &fakeParamSummaryRepo{}
		h := app.NewGetParamSummaryHandler(repo)

		products, total, filled, err := h.Handle(context.Background(), app.GetParamSummaryQuery{RequestID: 0})

		require.Error(t, err)
		assert.Nil(t, products)
		assert.Equal(t, int32(0), total)
		assert.Equal(t, int32(0), filled)
	})

	t.Run("rejects negative request ID", func(t *testing.T) {
		repo := &fakeParamSummaryRepo{}
		h := app.NewGetParamSummaryHandler(repo)

		products, total, filled, err := h.Handle(context.Background(), app.GetParamSummaryQuery{RequestID: -5})

		require.Error(t, err)
		assert.Nil(t, products)
		assert.Equal(t, int32(0), total)
		assert.Equal(t, int32(0), filled)
	})
}
