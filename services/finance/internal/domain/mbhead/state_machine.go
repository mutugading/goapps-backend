package mbhead

// Workflow status constants for an MB head record.
const (
	StatusDraft      = "DRAFT"
	StatusSubmitted  = "SUBMITTED"
	StatusApproved   = "APPROVED"
	StatusValidated  = "VALIDATED"
	StatusUnApproved = "UN_APPROVED"
	StatusRevoked    = "REVOKED"
)

var allowedTransitions = map[string]map[string]struct{}{
	StatusDraft:      {StatusSubmitted: {}, StatusValidated: {}}, // Validated only via boughtout shortcut
	StatusSubmitted:  {StatusApproved: {}, StatusDraft: {}},      // Draft = reject path
	StatusApproved:   {StatusValidated: {}, StatusUnApproved: {}},
	StatusValidated:  {},
	StatusUnApproved: {StatusApproved: {}}, // Revalidate re-enters Approved before Validate again
}

func canTransition(from, to string) bool {
	if from == to {
		return false
	}
	// Revoke is legal from any non-terminal state, checked separately by callers.
	targets, ok := allowedTransitions[from]
	if !ok {
		return false
	}
	_, ok = targets[to]
	return ok
}

func isTerminal(status string) bool {
	return status == StatusRevoked
}

func canRevoke(from string) bool {
	return !isTerminal(from)
}
