package costproductrequest

import "context"

// Filter for List.
type Filter struct {
	Search          string
	Status          string
	RequestTypeID   int32
	RequesterUserID string
	AssigneeUserID  string
	Page            int
	PageSize        int
	SortBy          string
	SortOrder       string
}

// Repository persists the Request aggregate.
//
// Lifecycle:
//   - Create: INSERT request (request_no via generate_cost_request_no()) + optional spec, single tx.
//   - Save:   mutate fields + spec; replaces spec row if present.
type Repository interface {
	Create(ctx context.Context, r *Request) error
	GetByID(ctx context.Context, id int64) (*Request, error)
	GetByNo(ctx context.Context, requestNo string) (*Request, error)
	Save(ctx context.Context, r *Request) error
	List(ctx context.Context, f Filter) (items []*Request, total int64, err error)
	// ListAll returns every request matching f (Page/PageSize/SortBy/SortOrder
	// are ignored — no pagination cap), each with its spec eagerly loaded.
	// Used by the export handler (design.md §4 Area D6).
	ListAll(ctx context.Context, f Filter) (items []*Request, err error)
}
