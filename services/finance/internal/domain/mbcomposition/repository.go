package mbcomposition

import "context"

// Repository defines the persistence contract for MB composition rows.
type Repository interface {
	Create(ctx context.Context, e *Entity) error
	Update(ctx context.Context, e *Entity) error
	Delete(ctx context.Context, id string) error
	GetByID(ctx context.Context, id string) (*Entity, error)
	ListByMbhID(ctx context.Context, mbhID string) ([]*Entity, error)
	SumPercentageByMbhID(ctx context.Context, mbhID string) (string, error)

	// ListVersionsByMbhID returns the frozen composition snapshot rows for mbhID at the given
	// version. version == 0 resolves to the latest version available for mbhID.
	ListVersionsByMbhID(ctx context.Context, mbhID string, version int32) ([]VersionRow, error)
}

// VersionRow is one frozen composition line from a VALIDATED snapshot.
type VersionRow struct {
	ID             string
	MbhID          string
	Version        int32
	ValidatedAt    string
	ValidatedBy    string
	SeqNo          int32
	GroupHeadID    string
	CompositionPct string
	SourceType     string
	MbRefMbhID     string
	IsCarrier      bool
}
