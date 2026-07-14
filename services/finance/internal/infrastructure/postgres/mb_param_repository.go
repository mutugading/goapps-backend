package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/lib/pq"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbparam"
)

// MBParamRepository implements mbparam.Repository using PostgreSQL.
type MBParamRepository struct {
	db *DB
}

// NewMBParamRepository creates a new MBParamRepository instance.
func NewMBParamRepository(db *DB) *MBParamRepository {
	return &MBParamRepository{db: db}
}

// Verify interface implementation at compile time.
var _ mbparam.Repository = (*MBParamRepository)(nil)

const errMsgRowsAffected = "mb_param_repository: rows affected: %w"

// Create persists a new parameter row.
func (r *MBParamRepository) Create(ctx context.Context, e *mbparam.Entity) error {
	const q = `
		INSERT INTO mst_mb_param
			(mbp_code, mbp_name, mbp_description, mbp_type, mbp_default_value,
			 mbp_default_option, mbp_unit, mbp_display_order, mbp_is_active, mbp_created_by)
		VALUES ($1, $2, $3, $4, NULLIF($5::numeric, 0), NULLIF($6, ''), $7, $8, $9, $10)
		RETURNING mbp_id`
	var id string
	err := r.db.QueryRowContext(ctx, q,
		e.Code(), e.Name(), e.Description(), e.Type(), e.DefaultValue(),
		e.DefaultOption(), e.Unit(), e.DisplayOrder(), e.IsActive(), e.CreatedBy(),
	).Scan(&id)
	if err != nil {
		if isUniqueViolation(err) {
			return mbparam.ErrAlreadyExists
		}
		return fmt.Errorf("mb_param_repository: create: %w", err)
	}
	return nil
}

// Update persists changes to an existing parameter row.
func (r *MBParamRepository) Update(ctx context.Context, e *mbparam.Entity) error {
	const q = `
		UPDATE mst_mb_param
		SET mbp_name = $2, mbp_description = $3, mbp_default_value = NULLIF($4::numeric, 0),
		    mbp_default_option = NULLIF($5, ''), mbp_unit = $6, mbp_display_order = $7,
		    mbp_is_active = $8, mbp_updated_at = NOW(), mbp_updated_by = $9
		WHERE mbp_id = $1 AND deleted_at IS NULL`
	result, err := r.db.ExecContext(ctx, q, e.ID(), e.Name(), e.Description(), e.DefaultValue(),
		e.DefaultOption(), e.Unit(), e.DisplayOrder(), e.IsActive(), e.UpdatedBy())
	if err != nil {
		return fmt.Errorf("mb_param_repository: update: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf(errMsgRowsAffected, err)
	}
	if rowsAffected == 0 {
		return mbparam.ErrNotFound
	}
	return nil
}

// Delete soft-deletes a parameter row by ID.
func (r *MBParamRepository) Delete(ctx context.Context, id string) error {
	const q = `UPDATE mst_mb_param SET deleted_at = NOW() WHERE mbp_id = $1 AND deleted_at IS NULL`
	result, err := r.db.ExecContext(ctx, q, id)
	if err != nil {
		return fmt.Errorf("mb_param_repository: delete: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf(errMsgRowsAffected, err)
	}
	if rowsAffected == 0 {
		return mbparam.ErrNotFound
	}
	return nil
}

// GetByID returns a single active parameter row by ID, with its options eager-loaded.
func (r *MBParamRepository) GetByID(ctx context.Context, id string) (*mbparam.Entity, error) {
	row := r.db.QueryRowContext(ctx, r.selectCols()+` WHERE mbp_id = $1 AND deleted_at IS NULL`, id)
	e, err := r.scanOne(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, mbparam.ErrNotFound
		}
		return nil, fmt.Errorf("mb_param_repository: get by id: %w", err)
	}
	optionsByCode, err := r.loadOptionsByParamCodes(ctx, []string{e.Code()})
	if err != nil {
		return nil, err
	}
	e.SetOptions(optionsByCode[e.Code()])
	return e, nil
}

// List returns paginated active parameter rows with each parameter's options eager-loaded in
// one batched query (WHERE mbpo_mbp_code = ANY($1)), avoiding N+1.
func (r *MBParamRepository) List(ctx context.Context, filter mbparam.ListFilter) ([]*mbparam.Entity, int64, error) {
	filter.Validate()

	where := whereNotDeleted
	args := []any{}
	if filter.Search != "" {
		where += fmt.Sprintf(" AND (mbp_code ILIKE $%d OR mbp_name ILIKE $%d)", len(args)+1, len(args)+1)
		args = append(args, "%"+filter.Search+"%")
	}
	if filter.IsActive != nil {
		where += fmt.Sprintf(" AND mbp_is_active = $%d", len(args)+1)
		args = append(args, *filter.IsActive)
	}

	var total int64
	countQ := "SELECT COUNT(*) FROM mst_mb_param " + where
	if err := r.db.QueryRowContext(ctx, countQ, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("mb_param_repository: count: %w", err)
	}

	orderCol := r.resolveSort(filter.SortBy)
	dir := sortASC
	if strings.ToUpper(filter.SortOrder) == sortDESC {
		dir = sortDESC
	}

	listQ := fmt.Sprintf("%s %s ORDER BY %s %s LIMIT $%d OFFSET $%d",
		r.selectCols(), where, orderCol, dir, len(args)+1, len(args)+2)
	args = append(args, filter.PageSize, filter.Offset())

	rows, err := r.db.QueryContext(ctx, listQ, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("mb_param_repository: list: %w", err)
	}
	defer closeRows(rows)

	var out []*mbparam.Entity
	codes := make([]string, 0, filter.PageSize)
	for rows.Next() {
		e, scanErr := r.scanRow(rows)
		if scanErr != nil {
			return nil, 0, scanErr
		}
		out = append(out, e)
		codes = append(codes, e.Code())
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("mb_param_repository: iterate: %w", err)
	}

	if len(out) == 0 {
		return out, total, nil
	}
	optionsByCode, err := r.loadOptionsByParamCodes(ctx, codes)
	if err != nil {
		return nil, 0, err
	}
	for _, e := range out {
		e.SetOptions(optionsByCode[e.Code()])
	}
	return out, total, nil
}

// ListActive returns all active, non-deleted parameter rows (unpaginated) with each
// parameter's options eager-loaded in one batched query — used to resolve the full recipe
// parameter set at MB Head VALIDATE time.
func (r *MBParamRepository) ListActive(ctx context.Context) ([]*mbparam.Entity, error) {
	const q = `WHERE deleted_at IS NULL AND mbp_is_active = true`
	listQ := r.selectCols() + q + " ORDER BY mbp_code ASC"

	rows, err := r.db.QueryContext(ctx, listQ)
	if err != nil {
		return nil, fmt.Errorf("mb_param_repository: list active: %w", err)
	}
	defer closeRows(rows)

	var out []*mbparam.Entity
	codes := make([]string, 0)
	for rows.Next() {
		e, scanErr := r.scanRow(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, e)
		codes = append(codes, e.Code())
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("mb_param_repository: iterate active: %w", err)
	}

	if len(out) == 0 {
		return out, nil
	}
	optionsByCode, err := r.loadOptionsByParamCodes(ctx, codes)
	if err != nil {
		return nil, err
	}
	for _, e := range out {
		e.SetOptions(optionsByCode[e.Code()])
	}
	return out, nil
}

// ListAll returns all non-deleted parameter rows matching filter (unpaginated, for export),
// with each parameter's options eager-loaded in one batched query.
func (r *MBParamRepository) ListAll(ctx context.Context, filter mbparam.ExportFilter) ([]*mbparam.Entity, error) {
	where := whereNotDeleted
	args := []any{}
	if filter.IsActive != nil {
		where += fmt.Sprintf(" AND mbp_is_active = $%d", len(args)+1)
		args = append(args, *filter.IsActive)
	}

	listQ := r.selectCols() + where + " ORDER BY mbp_code ASC"
	rows, err := r.db.QueryContext(ctx, listQ, args...)
	if err != nil {
		return nil, fmt.Errorf("mb_param_repository: list all: %w", err)
	}
	defer closeRows(rows)

	var out []*mbparam.Entity
	codes := make([]string, 0)
	for rows.Next() {
		e, scanErr := r.scanRow(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, e)
		codes = append(codes, e.Code())
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("mb_param_repository: iterate all: %w", err)
	}

	if len(out) == 0 {
		return out, nil
	}
	optionsByCode, err := r.loadOptionsByParamCodes(ctx, codes)
	if err != nil {
		return nil, err
	}
	for _, e := range out {
		e.SetOptions(optionsByCode[e.Code()])
	}
	return out, nil
}

// GetByCode returns a single active parameter row by its unique code, with options eager-loaded.
func (r *MBParamRepository) GetByCode(ctx context.Context, code string) (*mbparam.Entity, error) {
	row := r.db.QueryRowContext(ctx, r.selectCols()+` WHERE mbp_code = $1 AND deleted_at IS NULL`, code)
	e, err := r.scanOne(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, mbparam.ErrNotFound
		}
		return nil, fmt.Errorf("mb_param_repository: get by code: %w", err)
	}
	optionsByCode, err := r.loadOptionsByParamCodes(ctx, []string{e.Code()})
	if err != nil {
		return nil, err
	}
	e.SetOptions(optionsByCode[e.Code()])
	return e, nil
}

func (r *MBParamRepository) loadOptionsByParamCodes(ctx context.Context, codes []string) (map[string][]*mbparam.Option, error) {
	const q = `
		SELECT mbpo_id, mbpo_mbp_code, mbpo_code, mbpo_numeric_value,
		       COALESCE(mbpo_description, ''), COALESCE(mbpo_display_order, 0), mbpo_is_active
		FROM mst_mb_param_option
		WHERE mbpo_mbp_code = ANY($1) AND deleted_at IS NULL
		ORDER BY mbpo_mbp_code ASC, mbpo_display_order ASC`
	rows, err := r.db.QueryContext(ctx, q, pq.Array(codes))
	if err != nil {
		return nil, fmt.Errorf("mb_param_repository: load options: %w", err)
	}
	defer closeRows(rows)

	byCode := make(map[string][]*mbparam.Option, len(codes))
	for rows.Next() {
		var id, paramCode, code, numericValue, description string
		var displayOrder int32
		var isActive bool
		if scanErr := rows.Scan(&id, &paramCode, &code, &numericValue, &description, &displayOrder, &isActive); scanErr != nil {
			return nil, fmt.Errorf("mb_param_repository: scan option: %w", scanErr)
		}
		byCode[paramCode] = append(byCode[paramCode], mbparam.ReconstructOption(
			id, paramCode, code, numericValue, description, displayOrder, isActive,
		))
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("mb_param_repository: iterate options: %w", err)
	}
	return byCode, nil
}

// CreateOption persists a new picklist option row.
func (r *MBParamRepository) CreateOption(ctx context.Context, o *mbparam.Option) error {
	const q = `
		INSERT INTO mst_mb_param_option
			(mbpo_mbp_code, mbpo_code, mbpo_numeric_value, mbpo_description,
			 mbpo_display_order, mbpo_is_active)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING mbpo_id`
	var id string
	err := r.db.QueryRowContext(ctx, q,
		o.ParamCode(), o.Code(), o.NumericValue(), o.Description(), o.DisplayOrder(), o.IsActive(),
	).Scan(&id)
	if err != nil {
		return fmt.Errorf("mb_param_repository: create option: %w", err)
	}
	return nil
}

// UpdateOption persists changes to an existing picklist option row.
func (r *MBParamRepository) UpdateOption(ctx context.Context, o *mbparam.Option) error {
	const q = `
		UPDATE mst_mb_param_option
		SET mbpo_numeric_value = $2, mbpo_description = $3, mbpo_display_order = $4,
		    mbpo_is_active = $5
		WHERE mbpo_id = $1 AND deleted_at IS NULL`
	result, err := r.db.ExecContext(ctx, q, o.ID(), o.NumericValue(), o.Description(), o.DisplayOrder(), o.IsActive())
	if err != nil {
		return fmt.Errorf("mb_param_repository: update option: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf(errMsgRowsAffected, err)
	}
	if rowsAffected == 0 {
		return mbparam.ErrNotFound
	}
	return nil
}

// DeleteOption soft-deletes a picklist option row by ID.
func (r *MBParamRepository) DeleteOption(ctx context.Context, id string) error {
	const q = `UPDATE mst_mb_param_option SET deleted_at = NOW() WHERE mbpo_id = $1 AND deleted_at IS NULL`
	result, err := r.db.ExecContext(ctx, q, id)
	if err != nil {
		return fmt.Errorf("mb_param_repository: delete option: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf(errMsgRowsAffected, err)
	}
	if rowsAffected == 0 {
		return mbparam.ErrNotFound
	}
	return nil
}

func (r *MBParamRepository) resolveSort(sortBy string) string {
	m := map[string]string{
		"code":           "mbp_code",
		"name":           "mbp_name",
		"type":           "mbp_type",
		sortKeyCreatedAt: "mbp_created_at",
	}
	if col, ok := m[sortBy]; ok {
		return col
	}
	return "mbp_code"
}

func (r *MBParamRepository) selectCols() string {
	return `
		SELECT mbp_id, mbp_code, mbp_name, COALESCE(mbp_description, ''), mbp_type,
		       COALESCE(mbp_default_value::text, ''), COALESCE(mbp_default_option, ''),
		       COALESCE(mbp_unit, ''), COALESCE(mbp_display_order, 0), mbp_is_active,
		       mbp_created_at, mbp_created_by,
		       COALESCE(mbp_updated_at::text, ''), COALESCE(mbp_updated_by, ''),
		       COALESCE(deleted_at::text, ''), COALESCE(deleted_by, '')
		FROM mst_mb_param
	`
}

type mbParamDTO struct {
	ID            string
	Code          string
	Name          string
	Description   string
	ParamType     string
	DefaultValue  string
	DefaultOption string
	Unit          string
	DisplayOrder  int32
	IsActive      bool
	CreatedAt     string
	CreatedBy     string
	UpdatedAt     string
	UpdatedBy     string
	DeletedAt     string
	DeletedBy     string
}

func (d *mbParamDTO) toEntity() *mbparam.Entity {
	return mbparam.Reconstruct(
		d.ID, d.Code, d.Name, d.Description, d.ParamType, d.DefaultValue, d.DefaultOption,
		d.Unit, d.DisplayOrder, d.IsActive, d.CreatedAt, d.CreatedBy, d.UpdatedAt, d.UpdatedBy,
		d.DeletedAt, d.DeletedBy,
	)
}

func (r *MBParamRepository) scanRow(rows *sql.Rows) (*mbparam.Entity, error) {
	var d mbParamDTO
	err := rows.Scan(&d.ID, &d.Code, &d.Name, &d.Description, &d.ParamType, &d.DefaultValue,
		&d.DefaultOption, &d.Unit, &d.DisplayOrder, &d.IsActive, &d.CreatedAt, &d.CreatedBy,
		&d.UpdatedAt, &d.UpdatedBy, &d.DeletedAt, &d.DeletedBy)
	if err != nil {
		return nil, fmt.Errorf("mb_param_repository: scan row: %w", err)
	}
	return d.toEntity(), nil
}

func (r *MBParamRepository) scanOne(row *sql.Row) (*mbparam.Entity, error) {
	var d mbParamDTO
	err := row.Scan(&d.ID, &d.Code, &d.Name, &d.Description, &d.ParamType, &d.DefaultValue,
		&d.DefaultOption, &d.Unit, &d.DisplayOrder, &d.IsActive, &d.CreatedAt, &d.CreatedBy,
		&d.UpdatedAt, &d.UpdatedBy, &d.DeletedAt, &d.DeletedBy)
	if err != nil {
		return nil, err
	}
	return d.toEntity(), nil
}
