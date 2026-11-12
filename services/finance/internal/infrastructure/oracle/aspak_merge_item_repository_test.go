package oracle_test

import (
	"context"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/oracle"
	"github.com/mutugading/goapps-backend/services/finance/internal/testutil/fakeoracle"
)

// mergeRecorder wraps a querier and records query + args.
type mergeRecorder struct {
	inner oracle.ReadOnlyQuerier
	query string
	args  []any
}

func (r *mergeRecorder) QueryRO(ctx context.Context, q string, args ...any) (oracle.Rows, error) {
	r.query, r.args = q, args
	return r.inner.QueryRO(ctx, q, args...)
}

func TestASPAKMergeItemRepository_MapsRowsAndBinds(t *testing.T) {
	f := fakeoracle.New()
	f.Register("ASPAK.MMSMERGE", fakeoracle.Dataset{
		Columns: []string{"i", "n", "s", "sn", "d", "f"},
		Rows: [][]string{
			{" 1001 ", " POY 150/48 ", " S1 ", " RED ", " 150 ", "48"},
			{"1002", "DTY 75/36", "S2", "NATURAL", "", ""},
			{"", "", "", "", "", ""},
		},
	})
	rec := &mergeRecorder{inner: f}
	repo := oracle.NewASPAKMergeItemRepository(rec)

	got, err := repo.ListMergeItems(context.Background(), time.Date(2026, 3, 5, 10, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	require.Len(t, got, 3)
	assert.Equal(t, "1001", got[0].ItemCode)
	assert.Equal(t, "POY 150/48", got[0].ItemName)
	assert.Equal(t, "S1", got[0].ShadeCode)
	assert.Equal(t, "RED", got[0].ShadeName)
	assert.Equal(t, "150", got[0].Denier)
	assert.Equal(t, "48", got[0].Filament)
	assert.Equal(t, "", got[1].Denier)
	assert.Equal(t, "", got[1].Filament)
	assert.Equal(t, "", got[2].ItemCode)

	assert.Equal(t, []any{"2026-03-05"}, rec.args)
	assert.Contains(t, rec.query, "TO_DATE(:1,'YYYY-MM-DD')")
	assert.False(t, regexp.MustCompile(`\d{4}-\d{2}-\d{2}`).MatchString(rec.query), "no date literal in query")
	assert.NoError(t, oracle.CheckReadOnly(rec.query))
}

func TestASPAKMergeItemRepository_QueryErrorPropagated(t *testing.T) {
	f := fakeoracle.New()
	boom := errors.New("boom")
	f.FailWith(boom)
	_, err := oracle.NewASPAKMergeItemRepository(f).ListMergeItems(context.Background(), time.Now())
	require.ErrorIs(t, err, boom)
}

func TestASPAKMergeItemRepository_NilQuerier(t *testing.T) {
	_, err := oracle.NewASPAKMergeItemRepository(nil).ListMergeItems(context.Background(), time.Now())
	require.ErrorIs(t, err, oracle.ErrNoReadConnection)
}
