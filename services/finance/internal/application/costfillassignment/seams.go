package costfillassignment

import "context"

// Notifier sends SLA/overdue notifications. Implementation provided externally.
type Notifier interface {
	NotifyOverdue(ctx context.Context, taskID int64) error
}

// CompletionGate checks if all fill tasks for a request are approved and fires next state.
type CompletionGate interface {
	CheckAndAdvance(ctx context.Context, requestID int64) error
}
