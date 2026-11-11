package postgres

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbspin"
)

func TestBuildMBSpinFgWhere(t *testing.T) {
	t.Parallel()

	sqlText, args := buildMBSpinFgWhere(mbspin.ListFilter{}, 1)
	assert.Equal(t, " AND mbs_superseded_by_spin_id IS NULL", sqlText)
	assert.Empty(t, args)

	sqlText, args = buildMBSpinFgWhere(mbspin.ListFilter{IncludeSuperseded: true}, 1)
	assert.Empty(t, sqlText)
	assert.Empty(t, args)

	sqlText, args = buildMBSpinFgWhere(mbspin.ListFilter{IncludeSuperseded: true, FgProductID: 7, FgSearch: " poy "}, 3)
	assert.Contains(t, sqlText, "mbs_fg_product_id = $3")
	assert.Contains(t, sqlText, "ILIKE $4")
	assert.NotContains(t, sqlText, "poy")
	require.Len(t, args, 2)
	assert.Equal(t, int64(7), args[0])
	assert.Equal(t, "%poy%", args[1])
}

func TestMapMBSpinUniqueErr(t *testing.T) {
	t.Parallel()

	fg := &pgconn.PgError{Code: "23505", ConstraintName: spinShadeFgIndex}
	other := &pgconn.PgError{Code: "23505", ConstraintName: "something_else"}

	assert.ErrorIs(t, mapMBSpinUniqueErr(fg), mbspin.ErrSpinShadeFgExists)
	assert.ErrorIs(t, mapMBSpinUniqueErr(other), mbspin.ErrAlreadyExists)
	assert.NoError(t, mapMBSpinUniqueErr(errors.New("boom")))
}
