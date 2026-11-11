package mbspin_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbspin"
)

func TestCheckFgShade(t *testing.T) {
	tests := []struct {
		name, recipe, poy string
		fill              bool
		err               error
	}{
		{"empty poy fills", "RED", "", true, nil},
		{"blank poy fills", "RED", "  ", true, nil},
		{"equal", "RED", "RED", false, nil},
		{"equal case trim", " red ", "Red  ", false, nil},
		{"mismatch", "RED", "BLUE", false, mbspin.ErrFgShadeMismatch},
		{"empty recipe nonempty poy", "", "BLUE", false, mbspin.ErrFgShadeMismatch},
		{"blank recipe empty poy", "  ", "", false, mbspin.ErrFgShadeMismatch},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fill, err := mbspin.CheckFgShade(tc.recipe, tc.poy)
			assert.Equal(t, tc.fill, fill)
			assert.ErrorIs(t, err, tc.err)
		})
	}
}

func TestIsValidFgLinkSource(t *testing.T) {
	for _, s := range []string{"RECIPE", "DUPLICATE", "MANUAL", "BACKFILL"} {
		assert.True(t, mbspin.IsValidFgLinkSource(s), s)
	}
	for _, s := range []string{"", "recipe", "OTHER"} {
		assert.False(t, mbspin.IsValidFgLinkSource(s), s)
	}
}

func newFgSpin(t *testing.T) *mbspin.Entity {
	t.Helper()
	e, err := mbspin.New(uuid.New(), "n", nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, "u")
	require.NoError(t, err)
	return e
}

func TestWithFg(t *testing.T) {
	pid := int64(7)
	zero := int64(0)
	code := "  ITM-1 "
	empty := " "

	e := newFgSpin(t)
	require.NoError(t, e.WithFg(&pid, &code, mbspin.FgLinkSourceDuplicate))
	assert.Equal(t, int64(7), *e.FgProductID())
	assert.Equal(t, "ITM-1", *e.FgItemCode())
	assert.Equal(t, "DUPLICATE", *e.FgLinkSource())

	require.NoError(t, e.WithFg(&pid, &empty, mbspin.FgLinkSourceRecipe))
	assert.Nil(t, e.FgItemCode())
	require.NoError(t, e.WithFg(&pid, nil, mbspin.FgLinkSourceManual))
	assert.Nil(t, e.FgItemCode())

	assert.ErrorIs(t, e.WithFg(&pid, nil, "BAD"), mbspin.ErrInvalidFgLinkSource)
	assert.ErrorIs(t, e.WithFg(nil, nil, mbspin.FgLinkSourceRecipe), mbspin.ErrFgNotPoy)
	assert.ErrorIs(t, e.WithFg(&zero, nil, mbspin.FgLinkSourceRecipe), mbspin.ErrFgNotPoy)

	e.ClearFg()
	assert.Nil(t, e.FgProductID())
	assert.Nil(t, e.FgItemCode())
	assert.Nil(t, e.FgLinkSource())
}

func TestHydrateFg(t *testing.T) {
	e := newFgSpin(t)
	assert.False(t, e.IsSuperseded())
	pid := int64(3)
	src, code, by, pcode, pname, scode := "BACKFILL", "X", "bob", "POY1", "Poy One", "S-2"
	sid := uuid.New()
	at := time.Now()
	e.HydrateFg(&pid, &code, &src, mbspin.FgSupersede{SupersededBySpinID: &sid, SupersededAt: &at, SupersededBy: &by})
	e.HydrateFgReadModel(mbspin.FgReadModel{FgProductCode: &pcode, FgProductName: &pname, SupersededBySpinCode: &scode})
	assert.True(t, e.IsSuperseded())
	assert.Equal(t, sid, *e.SupersededBySpinID())
	assert.Equal(t, at, *e.SupersededAt())
	assert.Equal(t, "bob", *e.SupersededBy())
	assert.Equal(t, "POY1", *e.FgProductCode())
	assert.Equal(t, "Poy One", *e.FgProductName())
	assert.Equal(t, "S-2", *e.SupersededBySpinCode())
	assert.Equal(t, "BACKFILL", *e.FgLinkSource())
}
