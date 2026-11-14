package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"

	"github.com/google/uuid"
	"github.com/lib/pq"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/costproductmaster"
)

// CostProductEtlRepository implements costproductmaster.EtlRepository. It is insert-only for
// products: it never UPDATEs or deactivates an existing cost_product_master row.
type CostProductEtlRepository struct{ db *DB }

// NewCostProductEtlRepository constructs the repository.
func NewCostProductEtlRepository(db *DB) *CostProductEtlRepository {
	return &CostProductEtlRepository{db: db}
}

var _ costproductmaster.EtlRepository = (*CostProductEtlRepository)(nil)

// Match (a1): erp item + shade. Pure equality join, so Postgres can hash-join the key list.
// All products (active + inactive).
const etlMatchItemShadeSQL = `
SELECT k.item, k.shade, m.cpm_product_sys_id
  FROM unnest(CAST($1 AS text[]), CAST($2 AS text[])) AS k(item, shade)
  JOIN cost_product_master m
    ON UPPER(TRIM(COALESCE(m.cpm_erp_item_code, ''))) = k.item
   AND UPPER(TRIM(COALESCE(m.cpm_shade_code, ''))) = k.shade
 ORDER BY m.cpm_product_sys_id`

// Match (a2): legacy flex_01 "ITEM-SHADE-TYPE". The type suffix has no '-', so stripping the last
// segment leaves exactly ITEM-SHADE (equality join, no LIKE).
const etlMatchFlexSQL = `
SELECT k.item, k.shade, m.cpm_product_sys_id
  FROM unnest(CAST($1 AS text[]), CAST($2 AS text[])) AS k(item, shade)
  JOIN cost_product_master m
    ON regexp_replace(UPPER(m.cpm_flex_01), '-[^-]*$', '') = k.item || '-' || k.shade
 WHERE m.cpm_flex_01 IS NOT NULL
 ORDER BY m.cpm_product_sys_id`

// Match (b): siblings = any product of the item (erp item code, or the ITEM part of a legacy
// flex_01 "ITEM-SHADE-TYPE"), whatever the shade.
const etlMatchItemSQL = `
SELECT item, id FROM (
  SELECT UPPER(TRIM(cpm_erp_item_code)) AS item, cpm_product_sys_id AS id
    FROM cost_product_master
   WHERE UPPER(TRIM(COALESCE(cpm_erp_item_code, ''))) = ANY($1)
  UNION ALL
  SELECT regexp_replace(UPPER(cpm_flex_01), '-[^-]*-[^-]*$', '') AS item, cpm_product_sys_id AS id
    FROM cost_product_master
   WHERE cpm_flex_01 IS NOT NULL
     AND regexp_replace(UPPER(cpm_flex_01), '-[^-]*-[^-]*$', '') = ANY($1)
) u ORDER BY id`

// Match (c): same name + shade under a different (or empty) ERP item.
const etlMatchNameSQL = `
SELECT k.name, k.shade, m.cpm_product_sys_id
  FROM unnest(CAST($1 AS text[]), CAST($2 AS text[]), CAST($3 AS text[])) AS k(name, shade, item)
  JOIN cost_product_master m
    ON UPPER(TRIM(m.cpm_product_name)) = k.name
   AND UPPER(TRIM(COALESCE(m.cpm_shade_code, ''))) = k.shade
   AND UPPER(TRIM(COALESCE(m.cpm_erp_item_code, ''))) <> k.item
 ORDER BY m.cpm_product_sys_id`

// MatchProducts resolves matches (a), (b), (c) for all keys in three batched queries.
func (r *CostProductEtlRepository) MatchProducts(ctx context.Context, keys []costproductmaster.EtlKey) (costproductmaster.EtlMatchSet, error) {
	ms := costproductmaster.EtlMatchSet{
		ByItemShade: map[string][]int64{}, ByItem: map[string][]int64{}, ByNameShade: map[string][]int64{},
	}
	if len(keys) == 0 {
		return ms, nil
	}
	items := make([]string, len(keys))
	shades := make([]string, len(keys))
	names := make([]string, len(keys))
	for i, k := range keys {
		items[i], shades[i], names[i] = k.Item, k.Shade, k.NameUpper
	}
	addItemShade := func(a, b string, id int64) {
		key := costproductmaster.EtlRowKey(a, b)
		ms.ByItemShade[key] = appendUnique(ms.ByItemShade[key], id)
	}
	if err := r.collect(ctx, etlMatchItemShadeSQL, addItemShade, true, pq.Array(items), pq.Array(shades)); err != nil {
		return ms, fmt.Errorf("match item+shade: %w", err)
	}
	if err := r.collect(ctx, etlMatchFlexSQL, addItemShade, true, pq.Array(items), pq.Array(shades)); err != nil {
		return ms, fmt.Errorf("match flex_01: %w", err)
	}
	for k, ids := range ms.ByItemShade { // keep ids ascending after merging the two queries
		sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
		ms.ByItemShade[k] = ids
	}
	if err := r.collect(ctx, etlMatchItemSQL, func(a, _ string, id int64) {
		ms.ByItem[a] = appendUnique(ms.ByItem[a], id)
	}, false, pq.Array(items)); err != nil {
		return ms, fmt.Errorf("match item: %w", err)
	}
	if err := r.collect(ctx, etlMatchNameSQL, func(a, b string, id int64) {
		key := costproductmaster.EtlNameShadeKey(a, b)
		ms.ByNameShade[key] = appendUnique(ms.ByNameShade[key], id)
	}, true, pq.Array(names), pq.Array(shades), pq.Array(items)); err != nil {
		return ms, fmt.Errorf("match name+shade: %w", err)
	}
	return ms, nil
}

// collect runs q and feeds (col1, col2, id) rows (col2 absent when twoCols is false).
func (r *CostProductEtlRepository) collect(ctx context.Context, q string, add func(a, b string, id int64), twoCols bool, args ...any) error {
	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil {
			_ = cerr
		}
	}()
	for rows.Next() {
		var a, b string
		var id int64
		if twoCols {
			err = rows.Scan(&a, &b, &id)
		} else {
			err = rows.Scan(&a, &id)
		}
		if err != nil {
			return err
		}
		add(a, b, id)
	}
	return rows.Err()
}

func appendUnique(ids []int64, id int64) []int64 {
	for _, v := range ids {
		if v == id {
			return ids
		}
	}
	return append(ids, id)
}

// TypeIDsByCode returns active product types keyed by upper-cased code.
func (r *CostProductEtlRepository) TypeIDsByCode(ctx context.Context) (map[string]int32, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT cpt_type_code, cpt_type_id FROM cost_product_type WHERE cpt_is_active`)
	if err != nil {
		return nil, fmt.Errorf("list product types: %w", err)
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil {
			_ = cerr
		}
	}()
	out := map[string]int32{}
	for rows.Next() {
		var code string
		var id int32
		if err := rows.Scan(&code, &id); err != nil {
			return nil, err
		}
		out[costproductmaster.EtlNorm(code)] = id
	}
	return out, rows.Err()
}

// SiblingParams returns numeric values (decimal text) of the given params per product.
func (r *CostProductEtlRepository) SiblingParams(ctx context.Context, productIDs []int64, codes []string) (map[int64]map[string]string, error) {
	out := map[int64]map[string]string{}
	if len(productIDs) == 0 || len(codes) == 0 {
		return out, nil
	}
	const q = `
SELECT c.cpp_product_sys_id, p.param_code, CAST(c.cpp_value_numeric AS text)
  FROM cost_product_parameter c
  JOIN mst_parameter p ON p.id = c.cpp_param_id AND p.deleted_at IS NULL
 WHERE c.cpp_product_sys_id = ANY($1) AND p.param_code = ANY($2) AND c.cpp_value_numeric IS NOT NULL`
	rows, err := r.db.QueryContext(ctx, q, pq.Array(productIDs), pq.Array(codes))
	if err != nil {
		return nil, fmt.Errorf("sibling params: %w", err)
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil {
			_ = cerr
		}
	}()
	for rows.Next() {
		var id int64
		var code, v string
		if err := rows.Scan(&id, &code, &v); err != nil {
			return nil, err
		}
		if out[id] == nil {
			out[id] = map[string]string{}
		}
		out[id][code] = v
	}
	return out, rows.Err()
}

// sqlExecer is satisfied by *sql.DB and *sql.Tx.
type sqlExecer interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

const etlInsertRunSQL = `
INSERT INTO cost_product_etl_run
  (cper_run_type, cper_preview_run_id, cper_from_date, cper_search, cper_params, cper_counts, cper_rows,
   cper_status, cper_error, created_by)
VALUES ($1, $2, $3, NULLIF($4, ''), CAST($5 AS jsonb), CAST($6 AS jsonb), CAST($7 AS jsonb), $8, NULLIF($9, ''), $10)
RETURNING cper_id`

func insertRun(ctx context.Context, q sqlExecer, run costproductmaster.EtlRun) (uuid.UUID, error) {
	params, err := json.Marshal(map[string]int{"skipped_mb": run.SkippedMB})
	if err != nil {
		return uuid.Nil, fmt.Errorf("marshal run params: %w", err)
	}
	counts, err := json.Marshal(run.Counts)
	if err != nil {
		return uuid.Nil, fmt.Errorf("marshal run counts: %w", err)
	}
	var rowsJSON []byte
	if run.Type == costproductmaster.EtlRunApply {
		rowsJSON, err = json.Marshal(nonNil(run.ApplyRows))
	} else {
		rowsJSON, err = json.Marshal(nonNilRows(run.Rows))
	}
	if err != nil {
		return uuid.Nil, fmt.Errorf("marshal run rows: %w", err)
	}
	var preview any
	if run.PreviewRunID != nil {
		preview = *run.PreviewRunID
	}
	status := run.Status
	if status == "" {
		status = costproductmaster.EtlRunCompleted
	}
	var id uuid.UUID
	if err := q.QueryRowContext(ctx, etlInsertRunSQL, run.Type, preview, run.FromDate, run.Search,
		string(params), string(counts), string(rowsJSON), status, run.Error, run.CreatedBy).Scan(&id); err != nil {
		return uuid.Nil, fmt.Errorf("insert etl run: %w", err)
	}
	return id, nil
}

func nonNil(r []costproductmaster.EtlInsertResult) []costproductmaster.EtlInsertResult {
	if r == nil {
		return []costproductmaster.EtlInsertResult{}
	}
	return r
}

func nonNilRows(r []costproductmaster.EtlRunRow) []costproductmaster.EtlRunRow {
	if r == nil {
		return []costproductmaster.EtlRunRow{}
	}
	return r
}

// SaveRun persists a run (PREVIEW, or a FAILED APPLY record).
func (r *CostProductEtlRepository) SaveRun(ctx context.Context, run costproductmaster.EtlRun) (uuid.UUID, error) {
	return insertRun(ctx, r.db, run)
}

// GetRun loads a run by id.
func (r *CostProductEtlRepository) GetRun(ctx context.Context, id uuid.UUID) (costproductmaster.EtlRun, error) {
	const q = `
SELECT cper_id, cper_run_type, cper_preview_run_id, cper_from_date, COALESCE(cper_search, ''),
       cper_counts, cper_rows, cper_status, COALESCE(cper_error, ''), created_by
  FROM cost_product_etl_run WHERE cper_id = $1`
	var run costproductmaster.EtlRun
	var preview uuid.NullUUID
	var counts, rows []byte
	err := r.db.QueryRowContext(ctx, q, id).Scan(&run.ID, &run.Type, &preview, &run.FromDate, &run.Search,
		&counts, &rows, &run.Status, &run.Error, &run.CreatedBy)
	if errors.Is(err, sql.ErrNoRows) {
		return run, costproductmaster.ErrEtlRunNotFound
	}
	if err != nil {
		return run, fmt.Errorf("get etl run: %w", err)
	}
	if preview.Valid {
		p := preview.UUID
		run.PreviewRunID = &p
	}
	if err := json.Unmarshal(counts, &run.Counts); err != nil {
		return run, fmt.Errorf("decode etl counts: %w", err)
	}
	if run.Type == costproductmaster.EtlRunApply {
		err = json.Unmarshal(rows, &run.ApplyRows)
	} else {
		err = json.Unmarshal(rows, &run.Rows)
	}
	if err != nil {
		return run, fmt.Errorf("decode etl rows: %w", err)
	}
	return run, nil
}
