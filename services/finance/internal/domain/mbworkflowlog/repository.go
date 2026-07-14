package mbworkflowlog

import "context"

// Repository defines the persistence contract for MB head workflow-transition audit logs.
type Repository interface {
	Create(ctx context.Context, e *Entity) error
	ListByMbhID(ctx context.Context, mbhID string) ([]*Entity, error)
}
