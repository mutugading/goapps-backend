package mbsource_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbsource"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbspin"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/parameter"
)

func fillChild(t *testing.T, code, dataType, col string) *parameter.Parameter {
	t.Helper()
	c, err := parameter.NewCode(code)
	require.NoError(t, err)
	dt, err := parameter.NewDataType(dataType)
	require.NoError(t, err)
	cat, err := parameter.NewParamCategory("INPUT")
	require.NoError(t, err)
	return parameter.ReconstructParameter(uuid.New(), c, code, code, dt, cat, nil, "", "", nil, nil, nil, true,
		parameter.CostingMetadata{LookupFillGroupCode: mbsource.FillGroupSPCode, LookupSourceColumn: col},
		time.Now(), "t", nil, nil, nil, nil)
}

func TestSpinResolution(t *testing.T) {
	orion := "ORION-1"
	denier := 75.0
	spin, err := mbspin.New(uuid.New(), "RED DYE", nil, &orion, &denier, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, "t")
	require.NoError(t, err)
	children := []*parameter.Parameter{
		fillChild(t, "MB_SP_DYE", "TEXT", "mbs_mgt_name"),
		fillChild(t, "MB_SP_DENIER", "NUMBER", "mbs_denier"),
		fillChild(t, "MB_SP_FILAMENT", "NUMBER", "mbs_filament"), // spin has no filament → absent
	}
	res := mbsource.SpinResolution(spin, children)
	assert.Equal(t, "ORION-1", res.SPCode)
	assert.Equal(t, mbsource.SourceMBSpin, res.Source)
	require.NotNil(t, res.SpinID)
	assert.Equal(t, spin.ID(), *res.SpinID)
	require.Contains(t, res.Children, "MB_SP_DYE")
	assert.Equal(t, "RED DYE", *res.Children["MB_SP_DYE"].Text)
	require.Contains(t, res.Children, "MB_SP_DENIER")
	assert.InDelta(t, 75.0, *res.Children["MB_SP_DENIER"].Num, 1e-9)
	assert.NotContains(t, res.Children, "MB_SP_FILAMENT")
}
