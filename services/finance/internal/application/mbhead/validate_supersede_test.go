package mbhead_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/application/mbhead"
	mbheaddomain "github.com/mutugading/goapps-backend/services/finance/internal/domain/mbhead"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbspin"
)

// Without confirm the handler keeps calling TransitionWithAutoGen (unchanged call) and
// surfaces the repository's ErrSupersedeConfirmationRequired untouched.
func TestValidateHandler_NoConfirm_UsesLegacyCallAndPropagatesConflict(t *testing.T) {
	mockRepo := new(MockRepository)
	mockParams := new(MockParamRepository)
	ctx := context.Background()
	entity := approvedHeadWithParams("", "", nil)

	mockRepo.On("GetByID", ctx, entity.ID()).Return(entity, nil)
	mockParams.On("ListActive", ctx).Return(activeParamMasters(t), nil)
	mockRepo.On("TransitionWithAutoGen", ctx, entity.ID(), mbheaddomain.StatusApproved, mbheaddomain.StatusValidated,
		mock.Anything, "", "tester", mock.Anything, mock.Anything).Return(mbspin.ErrSupersedeConfirmationRequired)

	got, outcome, err := mbhead.NewValidateHandler(mockRepo, mockParams).HandleWithOutcome(ctx,
		mbhead.ValidateCommand{MbhID: entity.ID(), ActorUserID: "tester"})

	require.ErrorIs(t, err, mbspin.ErrSupersedeConfirmationRequired)
	assert.Nil(t, got)
	assert.Nil(t, outcome)
	mockRepo.AssertNotCalled(t, "TransitionWithAutoGenOpts", mock.Anything, mock.Anything, mock.Anything,
		mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything)
}

// With confirm the handler calls TransitionWithAutoGenOpts{ConfirmSupersede:true} and
// returns the outcome.
func TestValidateHandler_Confirm_PassesOptionAndReturnsOutcome(t *testing.T) {
	mockRepo := new(MockRepository)
	mockParams := new(MockParamRepository)
	ctx := context.Background()
	entity := approvedHeadWithParams("", "", nil)
	want := &mbheaddomain.SupersedeOutcome{BatchID: uuid.New(), NewSpinID: uuid.New(), AffectedProductIDs: []int64{7, 9}}

	mockRepo.On("GetByID", ctx, entity.ID()).Return(entity, nil)
	mockParams.On("ListActive", ctx).Return(activeParamMasters(t), nil)
	mockRepo.On("TransitionWithAutoGenOpts", ctx, entity.ID(), mbheaddomain.StatusApproved, mbheaddomain.StatusValidated,
		mock.Anything, "", "tester", mock.Anything, mock.Anything,
		mbheaddomain.AutoGenOptions{ConfirmSupersede: true}).Return(want, nil)

	got, outcome, err := mbhead.NewValidateHandler(mockRepo, mockParams).HandleWithOutcome(ctx,
		mbhead.ValidateCommand{MbhID: entity.ID(), ActorUserID: "tester", ConfirmSupersede: true})

	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, mbheaddomain.StatusValidated, got.EntryStatus())
	assert.Same(t, want, outcome)
	mockRepo.AssertNotCalled(t, "TransitionWithAutoGen", mock.Anything, mock.Anything, mock.Anything,
		mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything)
}

// Handle (the legacy entry point) with confirm also goes through the opts path.
func TestValidateHandler_Handle_ConfirmErrorRollsUp(t *testing.T) {
	mockRepo := new(MockRepository)
	mockParams := new(MockParamRepository)
	ctx := context.Background()
	entity := approvedHeadWithParams("", "", nil)

	mockRepo.On("GetByID", ctx, entity.ID()).Return(entity, nil)
	mockParams.On("ListActive", ctx).Return(activeParamMasters(t), nil)
	mockRepo.On("TransitionWithAutoGenOpts", mock.Anything, mock.Anything, mock.Anything, mock.Anything,
		mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(nil, mbspin.ErrSpinShadeFgExists)

	got, err := mbhead.NewValidateHandler(mockRepo, mockParams).Handle(ctx,
		mbhead.ValidateCommand{MbhID: entity.ID(), ActorUserID: "tester", ConfirmSupersede: true})

	require.ErrorIs(t, err, mbspin.ErrSpinShadeFgExists)
	assert.Nil(t, got)
}

func runValidateWithTrigger(t *testing.T, outcome *mbheaddomain.SupersedeOutcome, trigger *fakeLookupSyncTrigger) error {
	t.Helper()
	mockRepo := new(MockRepository)
	mockParams := new(MockParamRepository)
	ctx := context.Background()
	entity := approvedHeadWithParams("", "", nil)

	mockRepo.On("GetByID", ctx, entity.ID()).Return(entity, nil)
	mockParams.On("ListActive", ctx).Return(activeParamMasters(t), nil)
	mockRepo.On("TransitionWithAutoGenOpts", ctx, entity.ID(), mbheaddomain.StatusApproved, mbheaddomain.StatusValidated,
		mock.Anything, "", "tester", mock.Anything, mock.Anything, mock.Anything).Return(outcome, nil)
	mockRepo.On("TransitionWithAutoGen", ctx, entity.ID(), mbheaddomain.StatusApproved, mbheaddomain.StatusValidated,
		mock.Anything, "", "tester", mock.Anything, mock.Anything).Return(nil)

	_, _, err := mbhead.NewValidateHandler(mockRepo, mockParams).WithLookupSyncTrigger(trigger).HandleWithOutcome(ctx,
		mbhead.ValidateCommand{MbhID: entity.ID(), ActorUserID: "tester", ConfirmSupersede: outcome != nil})
	return err
}

func TestValidateHandler_Supersede_TriggersNewSpinSync(t *testing.T) {
	trigger := &fakeLookupSyncTrigger{}
	out := &mbheaddomain.SupersedeOutcome{BatchID: uuid.New(), NewSpinID: uuid.New(), AffectedProductIDs: []int64{7}}

	require.NoError(t, runValidateWithTrigger(t, out, trigger))
	require.Eventually(t, func() bool { return len(trigger.calls()) == 1 }, 2*time.Second, 10*time.Millisecond)
	assert.Equal(t, out.NewSpinID, trigger.calls()[0])
}

func TestValidateHandler_NoSupersede_DoesNotTrigger(t *testing.T) {
	trigger := &fakeLookupSyncTrigger{}

	require.NoError(t, runValidateWithTrigger(t, nil, trigger))
	time.Sleep(50 * time.Millisecond)
	assert.Empty(t, trigger.calls())
}

func TestValidateHandler_Supersede_TriggerErrorDoesNotFail(t *testing.T) {
	trigger := &fakeLookupSyncTrigger{spinErr: errors.New("publisher down")}
	out := &mbheaddomain.SupersedeOutcome{NewSpinID: uuid.New(), AffectedProductIDs: []int64{7}}

	require.NoError(t, runValidateWithTrigger(t, out, trigger))
	require.Eventually(t, func() bool { return len(trigger.calls()) == 1 }, 2*time.Second, 10*time.Millisecond)
}
