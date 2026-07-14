package mbhead

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newEntityAt builds a bare Entity with the given entryStatus/isBoughtout, bypassing
// New/Reconstruct (neither wires entryStatus yet). Test-only helper for this package.
func newEntityAt(status string, boughtout bool) *Entity {
	return &Entity{entryStatus: status, isBoughtout: boughtout}
}

func TestWorkflow_DraftSubmitApproveValidate_HappyPath(t *testing.T) {
	e := newEntityAt(StatusDraft, false)

	require.NoError(t, e.Submit())
	assert.Equal(t, StatusSubmitted, e.EntryStatus())

	require.NoError(t, e.Approve())
	assert.Equal(t, StatusApproved, e.EntryStatus())

	startVersion := e.CurrentVersion()
	require.NoError(t, e.Validate())
	assert.Equal(t, StatusValidated, e.EntryStatus())
	assert.Equal(t, startVersion+1, e.CurrentVersion())
}

func TestWorkflow_BoughtoutDraftToValidated_Shortcut(t *testing.T) {
	e := newEntityAt(StatusDraft, true)

	require.NoError(t, e.Validate())
	assert.Equal(t, StatusValidated, e.EntryStatus())
	assert.Equal(t, int32(1), e.CurrentVersion())
}

// TestValidate_DraftNonBoughtout_AllowedByStateMap documents actual behavior rather than the
// checklist's literal wording ("Validate from Draft fails for non-boughtout"): the non-boughtout
// branch of Validate delegates to canTransition(e.entryStatus, StatusValidated), and
// allowedTransitions[StatusDraft] includes StatusValidated (added for the boughtout shortcut but
// not conditioned on isBoughtout). Per task-9-brief.md, Validate/state_machine.go are transcribed
// verbatim and must not be modified, so this test asserts the transition succeeds as coded — the
// isBoughtout gate for this path is enforced by the caller/handler layer per design.md §2.1, not
// by this domain method alone. See task-9-report.md for the discrepancy write-up.
func TestValidate_DraftNonBoughtout_AllowedByStateMap(t *testing.T) {
	e := newEntityAt(StatusDraft, false)

	require.NoError(t, e.Validate())
	assert.Equal(t, StatusValidated, e.EntryStatus())
}

// TestValidate_SubmittedNonBoughtout_Fails covers a transition that IS rejected for non-boughtout
// entities under the given state map (Submitted has no direct path to Validated).
func TestValidate_SubmittedNonBoughtout_Fails(t *testing.T) {
	e := newEntityAt(StatusSubmitted, false)

	err := e.Validate()
	assert.ErrorIs(t, err, ErrInvalidTransition)
	assert.Equal(t, StatusSubmitted, e.EntryStatus())
}

func TestUnApprove_RequiresReason(t *testing.T) {
	e := newEntityAt(StatusApproved, false)

	err := e.UnApprove("")
	assert.ErrorIs(t, err, ErrReasonRequired)
	assert.Equal(t, StatusApproved, e.EntryStatus())

	require.NoError(t, e.UnApprove("quality issue"))
	assert.Equal(t, StatusUnApproved, e.EntryStatus())
	assert.Equal(t, "quality issue", e.StateReason())
}

func TestRevoke_FromAnyNonTerminalState(t *testing.T) {
	tests := []struct {
		name   string
		status string
	}{
		{"from draft", StatusDraft},
		{"from submitted", StatusSubmitted},
		{"from approved", StatusApproved},
		{"from validated", StatusValidated},
		{"from un_approved", StatusUnApproved},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newEntityAt(tt.status, false)

			require.NoError(t, e.Revoke("no longer needed"))
			assert.Equal(t, StatusRevoked, e.EntryStatus())
			assert.Equal(t, "no longer needed", e.StateReason())
		})
	}
}

func TestRevoke_FromRevoked_Fails(t *testing.T) {
	e := newEntityAt(StatusRevoked, false)

	err := e.Revoke("try again")
	assert.ErrorIs(t, err, ErrInvalidTransition)
	assert.Equal(t, StatusRevoked, e.EntryStatus())
}

func TestRevoke_RequiresReason(t *testing.T) {
	e := newEntityAt(StatusDraft, false)

	err := e.Revoke("")
	assert.ErrorIs(t, err, ErrReasonRequired)
	assert.Equal(t, StatusDraft, e.EntryStatus())
}

func TestSubmit_InvalidFromNonDraft(t *testing.T) {
	e := newEntityAt(StatusApproved, false)

	err := e.Submit()
	assert.ErrorIs(t, err, ErrInvalidTransition)
	assert.Equal(t, StatusApproved, e.EntryStatus())
}

func TestApprove_InvalidFromDraft(t *testing.T) {
	e := newEntityAt(StatusDraft, false)

	err := e.Approve()
	assert.ErrorIs(t, err, ErrInvalidTransition)
	assert.Equal(t, StatusDraft, e.EntryStatus())
}

func TestApprove_FromUnApproved_RevalidatePath(t *testing.T) {
	e := newEntityAt(StatusUnApproved, false)

	require.NoError(t, e.Approve())
	assert.Equal(t, StatusApproved, e.EntryStatus())
}
