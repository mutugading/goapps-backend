package postgres

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbhead"
)

func TestSupersedeOldHeadIDs_DistinctInOrder(t *testing.T) {
	h1, h2 := uuid.New(), uuid.New()
	got := supersedeOldHeadIDs([]supersedeSpin{{HeadID: h1}, {HeadID: h2}, {HeadID: h1}})
	assert.Equal(t, []uuid.UUID{h1, h2}, got)
	assert.Empty(t, supersedeOldHeadIDs(nil))
}

func TestSupersedeLogMeta_JSONKeys(t *testing.T) {
	m := supersedeLogMeta{Event: supersedeWorkflowLogToState, Role: "NEW", BatchID: uuid.New(), NewSpinID: uuid.New(),
		OldSpinIDs: []uuid.UUID{uuid.New()}, AffectedProducts: []int64{3}}
	b, err := json.Marshal(m)
	require.NoError(t, err)
	var doc map[string]any
	require.NoError(t, json.Unmarshal(b, &doc))
	for _, k := range []string{"event", "role", "batch_id", "new_head_id", "new_spin_id", "old_spin_ids", "affected_product_ids", "repointed_rows", "head_deactivated"} {
		assert.Contains(t, doc, k)
	}
	assert.LessOrEqual(t, len(supersedeWorkflowLogToState), 20, "mbwl_to_state is VARCHAR(20)")
}

// A head without FG never looks for candidates (no DB access → nil tx is safe).
func TestMBAutoGenSupersedeCandidates_NoFg_NoQuery(t *testing.T) {
	e, err := mbhead.New(mbhead.NewParams{MBCosting: "MBX", CreatedBy: "t", ShadeCode: "SH"})
	require.NoError(t, err)
	got, err := mbAutoGenSupersedeCandidates(context.Background(), nil, uuid.New(), e)
	require.NoError(t, err)
	assert.Nil(t, got)
}
