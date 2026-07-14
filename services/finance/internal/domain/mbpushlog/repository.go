package mbpushlog

import "context"

// Repository defines the persistence contract for MB cost-push audit logs.
type Repository interface {
	Create(ctx context.Context, e *Entity) error
	List(ctx context.Context, page, pageSize int32, period string) ([]*Entity, int64, error)
}
