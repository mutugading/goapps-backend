package lookupmaster

import "context"

// Repository is the persistence contract for mst_lookup_master + mst_lookup_master_column.
type Repository interface {
	// ListMasters returns all (or only active) registered master lookup codes.
	ListMasters(ctx context.Context, activeOnly bool) ([]*LookupMaster, error)
	// ListColumns returns fillable columns for a given master code, ordered by sort_order.
	ListColumns(ctx context.Context, masterCode string) ([]*Column, error)
}
