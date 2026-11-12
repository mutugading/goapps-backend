package costproductmaster

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
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

// ETL apply result codes (per row).
const (
	EtlResultInserted = "INSERTED"
	EtlResultSkipped  = "SKIPPED"
	EtlResultFailed   = "FAILED"
)

// ETL run types / statuses (cost_product_etl_run).
const (
	EtlRunPreview   = "PREVIEW"
	EtlRunApply     = "APPLY"
	EtlRunCompleted = "COMPLETED"
	EtlRunFailed    = "FAILED"
	EtlRunPartial   = "PARTIAL"
)

// EtlNorm normalizes a key part: trimmed + upper-cased.
func EtlNorm(s string) string { return strings.ToUpper(strings.TrimSpace(s)) }

// EtlRowKey builds the stable row key "<ITEM>|<SHADE>" (normalized) of an (item, shade) pair.
func EtlRowKey(item, shade string) string { return EtlNorm(item) + "|" + EtlNorm(shade) }

// EtlKey is one normalized lookup key of a source row (all parts upper-cased + trimmed).
type EtlKey struct {
	Item      string
	Shade     string
	NameUpper string
}

// EtlMatchSet holds existing-product ids (active AND inactive) per lookup, ids ascending.
//   - ByItemShade: key EtlRowKey(item, shade), match (a) erp item + shade, or flex_01 prefix.
//   - ByItem:      key normalized item, any shade (siblings, match b).
//   - ByNameShade: key NameUpper|SHADE, same name + shade under a different/empty item (match c).
type EtlMatchSet struct {
	ByItemShade map[string][]int64
	ByItem      map[string][]int64
	ByNameShade map[string][]int64
}

// EtlNameShadeKey builds the ByNameShade key.
func EtlNameShadeKey(nameUpper, shade string) string {
	return EtlNorm(nameUpper) + "|" + EtlNorm(shade)
}

// EtlRunRow is one classified row stored in a PREVIEW run snapshot. Denier/Filament/ActDenier are
// the numeric values (decimal text) that WILL be written for a newly inserted product; empty means
// "not written".
type EtlRunRow struct {
	RowKey     string  `json:"row_key"`
	ItemCode   string  `json:"item_code"`
	ItemName   string  `json:"item_name"`
	ShadeCode  string  `json:"shade_code"`
	ShadeName  string  `json:"shade_name"`
	TypeCode   string  `json:"type_code"`
	TypeID     int32   `json:"type_id"`
	Status     string  `json:"status"`
	Selectable bool    `json:"selectable"`
	MatchedIDs []int64 `json:"matched_ids,omitempty"`
	Denier     string  `json:"denier,omitempty"`
	Filament   string  `json:"filament,omitempty"`
	ActDenier  string  `json:"act_denier,omitempty"`
	Note       string  `json:"note,omitempty"`
}

// EtlCounts are the per-status totals of a run (before the search filter).
type EtlCounts struct {
	Exists            int32 `json:"exists"`
	ExistsMulti       int32 `json:"exists_multi"`
	NewShade          int32 `json:"new_shade"`
	NewItem           int32 `json:"new_item"`
	PossibleDuplicate int32 `json:"possible_duplicate"`
	InvalidType       int32 `json:"invalid_type"`
	Total             int32 `json:"total"`
}

// EtlInsertResult is the outcome of one apply row.
type EtlInsertResult struct {
	RowKey       string `json:"row_key"`
	Result       string `json:"result"`
	ProductSysID int64  `json:"product_sys_id,omitempty"`
	ProductCode  string `json:"product_code,omitempty"`
	Message      string `json:"message,omitempty"`
}

// EtlRun is a persisted ETL run (cost_product_etl_run). PREVIEW runs carry Rows; APPLY runs carry
// ApplyRows and PreviewRunID.
type EtlRun struct {
	ID           uuid.UUID
	Type         string
	PreviewRunID *uuid.UUID
	FromDate     time.Time
	Search       string
	SkippedMB    int
	Counts       EtlCounts
	Rows         []EtlRunRow
	ApplyRows    []EtlInsertResult
	Status       string
	Error        string
	CreatedBy    string
}

// EtlInsert is one product to insert (taken from a preview snapshot row).
type EtlInsert struct {
	RowKey    string
	ItemCode  string
	ItemName  string
	ShadeCode string
	ShadeName string
	TypeID    int32
	TypeCode  string
	Denier    string
	Filament  string
	ActDenier string
}

// EtlInsertInput is the batch given to InsertEtlProducts; the APPLY run row is saved in the same tx.
type EtlInsertInput struct {
	PreviewRunID uuid.UUID
	FromDate     time.Time
	Items        []EtlInsert
	Skipped      []EtlInsertResult // rows already SKIPPED by the use case (recorded in the run)
	Actor        string
}

// EtlInsertOutput is the result of InsertEtlProducts (Results only covers Items).
type EtlInsertOutput struct {
	RunID   uuid.UUID
	Results []EtlInsertResult
}

// EtlRepository is the Postgres port of the product ETL (insert-only; never touches old products).
type EtlRepository interface {
	// MatchProducts resolves existing products for all keys in a few batched queries.
	MatchProducts(ctx context.Context, keys []EtlKey) (EtlMatchSet, error)
	// TypeIDsByCode returns active product types (upper-cased code -> type id).
	TypeIDsByCode(ctx context.Context) (map[string]int32, error)
	// SiblingParams returns numeric param values (decimal text) per product id and param code.
	SiblingParams(ctx context.Context, productIDs []int64, codes []string) (map[int64]map[string]string, error)
	// SaveRun persists a run and returns its id.
	SaveRun(ctx context.Context, run EtlRun) (uuid.UUID, error)
	// GetRun loads a run (ErrEtlRunNotFound when absent).
	GetRun(ctx context.Context, id uuid.UUID) (EtlRun, error)
	// InsertEtlProducts inserts all items in ONE transaction (advisory lock + re-check per key)
	// and stores the APPLY run row in the same transaction.
	InsertEtlProducts(ctx context.Context, in EtlInsertInput) (EtlInsertOutput, error)
}
