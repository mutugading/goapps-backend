package costproductmaster

import (
	"context"
	"errors"
	"time"
)

// ETL classification statuses (Oracle merge items -> Product Master, design Revisi 3).
const (
	EtlExists            = "EXISTS"
	EtlExistsMulti       = "EXISTS_MULTI"
	EtlNewShade          = "NEW_SHADE"
	EtlNewItem           = "NEW_ITEM"
	EtlPossibleDuplicate = "POSSIBLE_DUPLICATE"
	EtlInvalidType       = "INVALID_TYPE"
)

// Domain errors of the Oracle ETL use case.
var (
	// ErrEtlNotConfigured is returned when no Oracle source is wired (server booted without Oracle).
	ErrEtlNotConfigured = errors.New("Oracle not configured") //nolint:staticcheck // user-facing message
	// ErrEtlRunNotFound is returned when the preview run id does not exist.
	ErrEtlRunNotFound = errors.New("ETL run not found")
	// ErrEtlRunNotPreview is returned when apply references a run that is not a PREVIEW run.
	ErrEtlRunNotPreview = errors.New("ETL run is not a preview run")
	// ErrEtlApplyLimitExceeded is returned when apply selects more rows than the per-run limit.
	ErrEtlApplyLimitExceeded = errors.New("ETL apply row limit exceeded")
	// ErrEtlInvalidFromDate is returned for an unparsable or future from_date.
	ErrEtlInvalidFromDate = errors.New("invalid from_date (expected YYYY-MM-DD, not in the future)")
)

// EtlSourceRow is one distinct (item, shade) merge row read from Oracle. All values are trimmed
// text; NULL columns are empty strings. Denier/Filament are Oracle numbers as text.
type EtlSourceRow struct {
	ItemCode  string
	ItemName  string
	ShadeCode string
	ShadeName string
	Denier    string
	Filament  string
}

// EtlSource is the read-only Oracle port for the product ETL.
type EtlSource interface {
	ListMergeItems(ctx context.Context, from time.Time) ([]EtlSourceRow, error)
}
