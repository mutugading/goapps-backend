// Package postgres provides PostgreSQL implementations for domain repositories.
package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/product"
)

// productSortColumns whitelists frontend field names → DB column names.
var productSortColumns = map[string]string{
	"productCode":     "product_code",
	"productName":     "product_name",
	"productItemCode": "product_item_code",
	"workflowStatus":  "workflow_status",
	"productStatus":   "product_status",
	"purpose":         "purpose",
	"createdAt":       "created_at",
	"updatedAt":       "updated_at",
}

// ProductRepository implements product.Repository using PostgreSQL.
type ProductRepository struct {
	db *DB
}

// NewProductRepository creates a new ProductRepository instance.
func NewProductRepository(db *DB) *ProductRepository {
	return &ProductRepository{db: db}
}

// Verify interface implementation at compile time.
var _ product.Repository = (*ProductRepository)(nil)

// Create persists a new Product to the database.
func (r *ProductRepository) Create(ctx context.Context, p *product.Product) error {
	copiedJSON, err := marshalCopyOptions(p.CopiedWithOptions())
	if err != nil {
		return fmt.Errorf("failed to marshal copied_with_options: %w", err)
	}

	query := `
		INSERT INTO cst_product (
			product_id, product_code, product_name, product_item_code,
			product_shade_code, product_shade_name,
			product_status, workflow_status,
			created_by_dept_id, created_by_dept_code,
			purpose,
			duplicated_from_id, duplication_note, copied_with_options,
			template_id, template_version_pinned,
			current_request_id,
			locked_at, locked_by, locked_period, unlock_count,
			created_at, created_by,
			updated_at, updated_by,
			deleted_at, deleted_by
		) VALUES (
			$1, $2, $3, $4,
			$5, $6,
			$7, $8,
			$9, $10,
			$11,
			$12, $13, $14,
			$15, $16,
			$17,
			$18, $19, $20, $21,
			$22, $23,
			$24, $25,
			$26, $27
		)
	`

	// copiedJSON is nil when no CopyOptions are set; pass as sql.NullString so pq sends NULL.
	var copiedJSONArg sql.NullString
	if len(copiedJSON) > 0 {
		copiedJSONArg = sql.NullString{String: string(copiedJSON), Valid: true}
	}

	_, err = r.db.ExecContext(ctx, query,
		p.ID(),
		p.Code().String(),
		p.Name().String(),
		p.ItemCode().String(),
		nullableStringVal(p.ShadeCode().String()),
		nullableStringVal(p.ShadeName().String()),
		p.ProductStatus().String(),
		p.WorkflowStatus().String(),
		nullableUUIDVal(p.CreatedByDeptID()),
		nullableStringVal(p.CreatedByDeptCode()),
		p.Purpose().String(),
		nullableUUIDVal(p.DuplicatedFromID()),
		nullableStringVal(p.DuplicationNote()),
		copiedJSONArg,
		nullableUUIDVal(p.TemplateID()),
		nullableIntVal(p.TemplateVersionPinned()),
		nullableUUIDVal(p.CurrentRequestID()),
		p.LockedAt(),
		nullableStringVal(p.LockedBy()),
		nullableStringVal(p.LockedPeriod()),
		p.UnlockCount(),
		p.CreatedAt(),
		p.CreatedBy(),
		p.UpdatedAt(),
		nullableStringVal(p.UpdatedBy()),
		p.DeletedAt(),
		nullableStringVal(p.DeletedBy()),
	)
	if err != nil {
		if isUniqueViolation(err) {
			return product.ErrAlreadyExists
		}
		return fmt.Errorf("failed to create product: %w", err)
	}

	return nil
}

// GetByID retrieves a non-deleted Product by its UUID.
func (r *ProductRepository) GetByID(ctx context.Context, id uuid.UUID) (*product.Product, error) {
	query := productSelectCols() + `
		FROM cst_product
		WHERE product_id = $1 AND deleted_at IS NULL
	`
	return scanProduct(r.db.QueryRowContext(ctx, query, id))
}

// GetByCode retrieves a non-deleted Product by its product code.
func (r *ProductRepository) GetByCode(ctx context.Context, code string) (*product.Product, error) {
	query := productSelectCols() + `
		FROM cst_product
		WHERE product_code = $1 AND deleted_at IS NULL
	`
	return scanProduct(r.db.QueryRowContext(ctx, query, code))
}

// List retrieves Products matching the filter with pagination.
// Returns the matching items, the total count across all pages, and any error.
func (r *ProductRepository) List(ctx context.Context, f product.ListFilter) ([]*product.Product, int, error) {
	f = normalizeListFilter(f)

	orderCol, err := resolveProductSortColumn(f.SortField)
	if err != nil {
		return nil, 0, err
	}
	orderDir := sortASC
	if f.SortDesc {
		orderDir = sortDESC
	}

	baseQuery, args, argIndex := buildProductWhereClause(f)

	var total int
	countQuery := "SELECT COUNT(*) FROM cst_product " + baseQuery
	if err := r.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed to count products: %w", err)
	}

	selectQuery := productSelectCols() + " FROM cst_product " + baseQuery +
		fmt.Sprintf(" ORDER BY %s %s LIMIT $%d OFFSET $%d",
			orderCol, orderDir, argIndex, argIndex+1)
	args = append(args, f.PageSize, (f.Page-1)*f.PageSize)

	rows, err := r.db.QueryContext(ctx, selectQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list products: %w", err)
	}
	defer closeRows(rows)

	var items []*product.Product
	for rows.Next() {
		p, scanErr := scanProductFromRows(rows)
		if scanErr != nil {
			return nil, 0, scanErr
		}
		items = append(items, p)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("error iterating product rows: %w", err)
	}

	return items, total, nil
}

// Update persists mutations to an existing Product.
func (r *ProductRepository) Update(ctx context.Context, p *product.Product) error {
	query := `
		UPDATE cst_product SET
			product_name        = $2,
			product_shade_code  = $3,
			product_shade_name  = $4,
			purpose             = $5,
			workflow_status     = $6,
			product_status      = $7,
			locked_at           = $8,
			locked_by           = $9,
			locked_period       = $10,
			unlock_count        = $11,
			current_request_id  = $12,
			updated_at          = $13,
			updated_by          = $14
		WHERE product_id = $1 AND deleted_at IS NULL
	`

	result, err := r.db.ExecContext(ctx, query,
		p.ID(),
		p.Name().String(),
		nullableStringVal(p.ShadeCode().String()),
		nullableStringVal(p.ShadeName().String()),
		p.Purpose().String(),
		p.WorkflowStatus().String(),
		p.ProductStatus().String(),
		p.LockedAt(),
		nullableStringVal(p.LockedBy()),
		nullableStringVal(p.LockedPeriod()),
		p.UnlockCount(),
		nullableUUIDVal(p.CurrentRequestID()),
		p.UpdatedAt(),
		nullableStringVal(p.UpdatedBy()),
	)
	if err != nil {
		return fmt.Errorf("failed to update product: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return product.ErrNotFound
	}

	return nil
}

// Delete soft-deletes a Product by its UUID.
func (r *ProductRepository) Delete(ctx context.Context, id uuid.UUID, deletedBy string) error {
	query := `
		UPDATE cst_product SET
			deleted_at = $2,
			deleted_by = $3
		WHERE product_id = $1 AND deleted_at IS NULL
	`

	result, err := r.db.ExecContext(ctx, query, id, time.Now().UTC(), deletedBy)
	if err != nil {
		return fmt.Errorf("failed to delete product: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return product.ErrNotFound
	}

	return nil
}

// SearchByText performs a full-text search using the idx_cst_product_fts GIN index.
func (r *ProductRepository) SearchByText(ctx context.Context, opts product.SearchOptions) ([]*product.Product, error) {
	query := strings.TrimSpace(opts.Query)
	if query == "" {
		return []*product.Product{}, nil
	}

	limit := clampSearchLimit(opts.Limit)

	// Order by ts_rank for relevance but do not include rank in SELECT to keep scanProductFromRows compatible.
	sqlQuery := productSelectCols() + `
		FROM cst_product
		WHERE deleted_at IS NULL
		  AND to_tsvector('simple',
			  coalesce(product_name,'') || ' ' ||
			  coalesce(product_shade_name,'') || ' ' ||
			  coalesce(product_code,'')
		  ) @@ plainto_tsquery('simple', $1)
		  AND ($2 = '' OR product_shade_code = $2)
		ORDER BY ts_rank(
			to_tsvector('simple',
				coalesce(product_name,'') || ' ' ||
				coalesce(product_shade_name,'') || ' ' ||
				coalesce(product_code,'')),
			plainto_tsquery('simple', $1)
		) DESC
		LIMIT $3
	`

	rows, err := r.db.QueryContext(ctx, sqlQuery, query, opts.ShadeCode, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to search products: %w", err)
	}
	defer closeRows(rows)

	var items []*product.Product
	for rows.Next() {
		p, scanErr := scanProductFromRows(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		items = append(items, p)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating search rows: %w", err)
	}

	return items, nil
}

// ListByRequestID retrieves Products linked to a specific request UUID, with pagination.
func (r *ProductRepository) ListByRequestID(ctx context.Context, requestID uuid.UUID, page, pageSize int) ([]*product.Product, int, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	var total int
	countQuery := `SELECT COUNT(*) FROM cst_product WHERE current_request_id = $1 AND deleted_at IS NULL`
	if err := r.db.QueryRowContext(ctx, countQuery, requestID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed to count products by request id: %w", err)
	}

	selectQuery := productSelectCols() + `
		FROM cst_product
		WHERE current_request_id = $1 AND deleted_at IS NULL
		ORDER BY created_at ASC
		LIMIT $2 OFFSET $3
	`

	rows, err := r.db.QueryContext(ctx, selectQuery, requestID, pageSize, (page-1)*pageSize)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list products by request id: %w", err)
	}
	defer closeRows(rows)

	var items []*product.Product
	for rows.Next() {
		p, scanErr := scanProductFromRows(rows)
		if scanErr != nil {
			return nil, 0, scanErr
		}
		items = append(items, p)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("error iterating product rows: %w", err)
	}

	return items, total, nil
}

// =============================================================================
// SQL construction helpers
// =============================================================================

// productSelectCols returns the SELECT column list for cst_product.
func productSelectCols() string {
	return `
		SELECT
			product_id, product_code, product_name, product_item_code,
			product_shade_code, product_shade_name,
			product_status, workflow_status,
			created_by_dept_id, created_by_dept_code,
			purpose,
			duplicated_from_id, duplication_note, copied_with_options,
			template_id, template_version_pinned,
			current_request_id,
			locked_at, locked_by, locked_period, unlock_count,
			created_at, created_by,
			updated_at, updated_by,
			deleted_at, deleted_by
	`
}

// buildProductWhereClause builds the WHERE clause and args for List queries.
// Returns the clause string (starting with WHERE), args slice, and the next arg index.
func buildProductWhereClause(f product.ListFilter) (string, []interface{}, int) {
	clause := "WHERE deleted_at IS NULL"
	args := []interface{}{}
	idx := 1

	if f.Search != "" {
		clause += fmt.Sprintf(` AND to_tsvector('simple',
			coalesce(product_name,'') || ' ' ||
			coalesce(product_shade_name,'') || ' ' ||
			coalesce(product_code,'')
		) @@ plainto_tsquery('simple', $%d)`, idx)
		args = append(args, f.Search)
		idx++
	}

	if f.WorkflowStatus != "" {
		clause += fmt.Sprintf(" AND workflow_status = $%d", idx)
		args = append(args, f.WorkflowStatus)
		idx++
	}

	if f.ProductStatus != "" {
		clause += fmt.Sprintf(" AND product_status = $%d", idx)
		args = append(args, f.ProductStatus)
		idx++
	}

	if f.Purpose != "" {
		clause += fmt.Sprintf(" AND purpose = $%d", idx)
		args = append(args, f.Purpose)
		idx++
	}

	if f.CreatedByDeptID != nil {
		clause += fmt.Sprintf(" AND created_by_dept_id = $%d", idx)
		args = append(args, *f.CreatedByDeptID)
		idx++
	}

	return clause, args, idx
}

// resolveProductSortColumn maps the frontend sort field to a DB column.
// Returns the default "created_at" if the field is empty.
func resolveProductSortColumn(field string) (string, error) {
	if field == "" {
		return "created_at", nil
	}
	col, ok := productSortColumns[field]
	if !ok {
		return "", fmt.Errorf("invalid sort field %q", field)
	}
	return col, nil
}

// normalizeListFilter applies defaults to a ListFilter.
func normalizeListFilter(f product.ListFilter) product.ListFilter {
	if f.Page < 1 {
		f.Page = 1
	}
	if f.PageSize < 1 || f.PageSize > 100 {
		f.PageSize = 20
	}
	return f
}

// clampSearchLimit clamps the search limit to the range [1, 50].
func clampSearchLimit(limit int) int {
	if limit < 1 {
		return 20
	}
	if limit > 50 {
		return 50
	}
	return limit
}

// =============================================================================
// Scan helpers
// =============================================================================

// productDTO is a data transfer object for cst_product rows.
type productDTO struct {
	ID                    uuid.UUID
	Code                  string
	Name                  string
	ItemCode              string
	ShadeCode             sql.NullString
	ShadeName             sql.NullString
	ProductStatus         string
	WorkflowStatus        string
	CreatedByDeptID       *uuid.UUID
	CreatedByDeptCode     sql.NullString
	Purpose               string
	DuplicatedFromID      *uuid.UUID
	DuplicationNote       sql.NullString
	CopiedWithOptions     []byte
	TemplateID            *uuid.UUID
	TemplateVersionPinned sql.NullInt64
	CurrentRequestID      *uuid.UUID
	LockedAt              sql.NullTime
	LockedBy              sql.NullString
	LockedPeriod          sql.NullString
	UnlockCount           int
	CreatedAt             time.Time
	CreatedBy             string
	UpdatedAt             sql.NullTime
	UpdatedBy             sql.NullString
	DeletedAt             sql.NullTime
	DeletedBy             sql.NullString
}

// scanProductDTO scans the standard column set of cst_product into a productDTO.
func scanProductDTO(scanner interface {
	Scan(dest ...interface{}) error
}) (*productDTO, error) {
	var dto productDTO
	err := scanner.Scan(
		&dto.ID,
		&dto.Code,
		&dto.Name,
		&dto.ItemCode,
		&dto.ShadeCode,
		&dto.ShadeName,
		&dto.ProductStatus,
		&dto.WorkflowStatus,
		&dto.CreatedByDeptID,
		&dto.CreatedByDeptCode,
		&dto.Purpose,
		&dto.DuplicatedFromID,
		&dto.DuplicationNote,
		&dto.CopiedWithOptions,
		&dto.TemplateID,
		&dto.TemplateVersionPinned,
		&dto.CurrentRequestID,
		&dto.LockedAt,
		&dto.LockedBy,
		&dto.LockedPeriod,
		&dto.UnlockCount,
		&dto.CreatedAt,
		&dto.CreatedBy,
		&dto.UpdatedAt,
		&dto.UpdatedBy,
		&dto.DeletedAt,
		&dto.DeletedBy,
	)
	return &dto, err
}

// scanProduct scans a single *sql.Row into a Product entity.
func scanProduct(row *sql.Row) (*product.Product, error) {
	dto, err := scanProductDTO(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, product.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to scan product: %w", err)
	}
	return dtoToProduct(dto)
}

// scanProductFromRows scans a *sql.Rows row into a Product entity.
func scanProductFromRows(rows *sql.Rows) (*product.Product, error) {
	dto, err := scanProductDTO(rows)
	if err != nil {
		return nil, fmt.Errorf("failed to scan product row: %w", err)
	}
	return dtoToProduct(dto)
}

// dtoToProduct converts a productDTO to a domain Product.
func dtoToProduct(dto *productDTO) (*product.Product, error) {
	shadeCode := ""
	if dto.ShadeCode.Valid {
		shadeCode = dto.ShadeCode.String
	}
	shadeName := ""
	if dto.ShadeName.Valid {
		shadeName = dto.ShadeName.String
	}
	deptCode := ""
	if dto.CreatedByDeptCode.Valid {
		deptCode = dto.CreatedByDeptCode.String
	}

	dupFromID := uuid.Nil
	if dto.DuplicatedFromID != nil {
		dupFromID = *dto.DuplicatedFromID
	}
	dupNote := ""
	if dto.DuplicationNote.Valid {
		dupNote = dto.DuplicationNote.String
	}

	var copyOpts *product.CopyOptions
	if len(dto.CopiedWithOptions) > 0 {
		opts, err := unmarshalCopyOptions(dto.CopiedWithOptions)
		if err != nil {
			return nil, err
		}
		copyOpts = opts
	}

	templateID := uuid.Nil
	if dto.TemplateID != nil {
		templateID = *dto.TemplateID
	}
	templateVersion := 0
	if dto.TemplateVersionPinned.Valid {
		templateVersion = int(dto.TemplateVersionPinned.Int64)
	}

	currentReqID := uuid.Nil
	if dto.CurrentRequestID != nil {
		currentReqID = *dto.CurrentRequestID
	}

	var lockedAt *time.Time
	if dto.LockedAt.Valid {
		lockedAt = &dto.LockedAt.Time
	}
	lockedBy := ""
	if dto.LockedBy.Valid {
		lockedBy = dto.LockedBy.String
	}
	lockedPeriod := ""
	if dto.LockedPeriod.Valid {
		lockedPeriod = dto.LockedPeriod.String
	}

	deptID := uuid.Nil
	if dto.CreatedByDeptID != nil {
		deptID = *dto.CreatedByDeptID
	}

	var updatedAt *time.Time
	if dto.UpdatedAt.Valid {
		updatedAt = &dto.UpdatedAt.Time
	}
	updatedBy := ""
	if dto.UpdatedBy.Valid {
		updatedBy = dto.UpdatedBy.String
	}

	var deletedAt *time.Time
	if dto.DeletedAt.Valid {
		deletedAt = &dto.DeletedAt.Time
	}
	deletedBy := ""
	if dto.DeletedBy.Valid {
		deletedBy = dto.DeletedBy.String
	}

	return product.ReconstructProduct(
		dto.ID,
		dto.Code,
		dto.Name,
		dto.ItemCode,
		shadeCode,
		shadeName,
		dto.ProductStatus,
		dto.WorkflowStatus,
		deptID,
		deptCode,
		dto.Purpose,
		dupFromID,
		dupNote,
		copyOpts,
		templateID,
		templateVersion,
		currentReqID,
		lockedAt,
		lockedBy,
		lockedPeriod,
		dto.UnlockCount,
		dto.CreatedAt,
		dto.CreatedBy,
		updatedAt,
		updatedBy,
		deletedAt,
		deletedBy,
	), nil
}

// =============================================================================
// JSONB helpers
// =============================================================================

// copyOptionsJSON is the JSONB wire format for copied_with_options.
type copyOptionsJSON struct {
	IncludeValues      bool `json:"include_values"`
	IncludeRouting     bool `json:"include_routing"`
	IncludeRM          bool `json:"include_rm"`
	IncludeAttachments bool `json:"include_attachments"`
}

// marshalCopyOptions marshals *product.CopyOptions to JSON bytes (or nil if opts is nil).
func marshalCopyOptions(opts *product.CopyOptions) ([]byte, error) {
	if opts == nil {
		return nil, nil
	}
	return json.Marshal(copyOptionsJSON{
		IncludeValues:      opts.IncludeValues,
		IncludeRouting:     opts.IncludeRouting,
		IncludeRM:          opts.IncludeRM,
		IncludeAttachments: opts.IncludeAttachments,
	})
}

// unmarshalCopyOptions unmarshals JSON bytes into *product.CopyOptions.
func unmarshalCopyOptions(data []byte) (*product.CopyOptions, error) {
	var raw copyOptionsJSON
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("unmarshal copied_with_options: %w", err)
	}
	return &product.CopyOptions{
		IncludeValues:      raw.IncludeValues,
		IncludeRouting:     raw.IncludeRouting,
		IncludeRM:          raw.IncludeRM,
		IncludeAttachments: raw.IncludeAttachments,
	}, nil
}

// =============================================================================
// Nullable conversion helpers
// =============================================================================

// nullableStringVal returns sql.NullString for an empty-string-means-null pattern.
func nullableStringVal(s string) sql.NullString {
	return sql.NullString{String: s, Valid: s != ""}
}

// nullableUUIDVal returns nil for uuid.Nil, or a pointer otherwise.
func nullableUUIDVal(id uuid.UUID) *uuid.UUID {
	if id == uuid.Nil {
		return nil
	}
	cp := id
	return &cp
}

// nullableIntVal returns nil for zero, or a pointer otherwise.
func nullableIntVal(v int) *int {
	if v == 0 {
		return nil
	}
	cp := v
	return &cp
}
