package mbhead_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbhead"
)

func TestHeadFg(t *testing.T) {
	e := &mbhead.Entity{}
	assert.Nil(t, e.FgProductID())
	assert.False(t, e.IsSuperseded())

	pid, zero, code, blank := int64(5), int64(0), " IT ", " "
	require.NoError(t, e.SetFg(&pid, &code))
	assert.Equal(t, int64(5), *e.FgProductID())
	assert.Equal(t, "IT", *e.FgItemCode())

	require.NoError(t, e.SetFg(&pid, &blank))
	assert.Nil(t, e.FgItemCode())
	require.NoError(t, e.SetFg(&pid, nil))
	assert.Nil(t, e.FgItemCode())

	assert.ErrorIs(t, e.SetFg(nil, nil), mbhead.ErrInvalidFgProduct)
	assert.ErrorIs(t, e.SetFg(&zero, nil), mbhead.ErrInvalidFgProduct)

	e.ClearFg()
	assert.Nil(t, e.FgProductID())
	assert.Nil(t, e.FgItemCode())
}

func TestHeadHydrateFg(t *testing.T) {
	e := &mbhead.Entity{}
	pid, code, pc, pn := int64(9), "C", "P", "N"
	sid := uuid.New()
	at := time.Now()
	e.HydrateFg(&pid, &code, &sid, &at)
	e.HydrateFgReadModel(mbhead.FgReadModel{FgProductCode: &pc, FgProductName: &pn})
	assert.True(t, e.IsSuperseded())
	assert.Equal(t, sid, *e.SupersededByHeadID())
	assert.Equal(t, at, *e.SupersededAt())
	assert.Equal(t, "P", *e.FgProductCode())
	assert.Equal(t, "N", *e.FgProductName())
	assert.Equal(t, int64(9), *e.FgProductID())
}
