package grpc

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	financev1 "github.com/mutugading/goapps-backend/gen/finance/v1"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbhead"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbspin"
)

// A shade+FG collision without confirm_supersede → 409 pointing at the preview + retry.
func TestValidateMBHead_SupersedeConflict_Returns409WithHint(t *testing.T) {
	h, repo, id := newApproveHandler(t, mbhead.StatusSubmitted, false)
	repo.autoGenErr = mbspin.ErrSupersedeConfirmationRequired

	resp, err := h.ValidateMBHead(context.Background(), &financev1.ValidateMBHeadRequest{MbhId: id.String()})

	require.NoError(t, err)
	require.False(t, resp.Base.IsSuccess)
	assert.Equal(t, "409", resp.Base.StatusCode)
	assert.Contains(t, resp.Base.Message, "SUPERSEDE_CONFIRMATION_REQUIRED")
	assert.Contains(t, resp.Base.Message, "/api/v1/finance/mb-heads/"+id.String()+"/supersede-preview")
	assert.Contains(t, resp.Base.Message, "confirm_supersede=true")
	assert.False(t, repo.lastOpts.ConfirmSupersede)
}

// confirm_supersede=true is threaded down to the repository option.
func TestValidateMBHead_ConfirmSupersede_ThreadsOption(t *testing.T) {
	h, repo, id := newApproveHandler(t, mbhead.StatusSubmitted, false)

	resp, err := h.ValidateMBHead(context.Background(), &financev1.ValidateMBHeadRequest{MbhId: id.String(), ConfirmSupersede: true})

	require.NoError(t, err)
	require.Truef(t, resp.Base.IsSuccess, "validate failed: %s", resp.Base.Message)
	assert.True(t, repo.lastOpts.ConfirmSupersede)
	assert.Equal(t, mbhead.StatusValidated, resp.Data.EntryStatus)
}

// Approve lands VALIDATED (Opsi A) and can therefore hit the same conflict: 409 too.
func TestApproveMBHead_SupersedeConflict_Returns409(t *testing.T) {
	h, repo, id := newApproveHandler(t, mbhead.StatusSubmitted, false)
	repo.autoGenErr = mbspin.ErrSupersedeConfirmationRequired

	resp, err := h.ApproveMBHead(context.Background(), &financev1.ApproveMBHeadRequest{MbhId: id.String()})

	require.NoError(t, err)
	require.False(t, resp.Base.IsSuccess)
	assert.Equal(t, "409", resp.Base.StatusCode)
	assert.Contains(t, resp.Base.Message, "supersede-preview")
}

func TestValidateSupersedeErrToBase_OtherErrorsNil(t *testing.T) {
	assert.Nil(t, validateSupersedeErrToBase(mbspin.ErrSpinShadeFgExists, [16]byte{}))
}
