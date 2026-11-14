package postgres

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbspin"
)

func TestSupersedeTextKey(t *testing.T) {
	id := uuid.New()
	orion := " ab-1 "
	key, fromOrion := supersedeTextKey(supersedeSpin{SpinID: id, OrionItemCode: &orion})
	assert.Equal(t, "AB-1", key)
	assert.True(t, fromOrion)

	blank := "  "
	key, fromOrion = supersedeTextKey(supersedeSpin{SpinID: id, OrionItemCode: &blank})
	assert.Equal(t, uuid.MustParse(key), id)
	assert.False(t, fromOrion)
}

func TestApplySupersedeParamRows(t *testing.T) {
	ptr := mbspin.SupersedeRepointPointer
	txt := mbspin.SupersedeRepointTextOnly
	rows := &supersedeParamRows{
		Pointer: []supersedeParamRow{
			{ProductSysID: 1, FilledBy: "manual", Kind: ptr},
			{ProductSysID: 2, FilledBy: "auto_mb_source", Kind: ptr},
			{ProductSysID: 3, FilledBy: "manual", Kind: ptr, IsLocked: true},
		},
		TextOnly: []supersedeParamRow{{ProductSysID: 4, FilledBy: "manual", Kind: txt}},
	}
	var c mbspin.SupersedeCandidate
	applySupersedeParamRows(&c, rows)
	assert.Equal(t, 2, c.ParamRowsPointer)
	assert.Equal(t, 1, c.ParamRowsTextOnly)
	require.Len(t, c.FilledByCounts, 2)
	assert.Equal(t, mbspin.SupersedeFilledByCount{FilledBy: "auto_mb_source", Count: 1}, c.FilledByCounts[0])
	assert.Equal(t, mbspin.SupersedeFilledByCount{FilledBy: "manual", Count: 2}, c.FilledByCounts[1])
	assert.Len(t, c.AffectedProducts, 3)
	require.Len(t, c.Warnings, 1)
	assert.Contains(t, c.Warnings[0], "locked")

	var shared mbspin.SupersedeCandidate
	shared.OldSpinCode = "DUP"
	applySupersedeParamRows(&shared, &supersedeParamRows{TextShared: true, SharedTextRows: 3})
	assert.Zero(t, shared.ParamRowsTextOnly)
	require.Len(t, shared.Warnings, 1)
	assert.Contains(t, shared.Warnings[0], "3 text-only")
	assert.Contains(t, shared.Warnings[0], "DUP")
}
