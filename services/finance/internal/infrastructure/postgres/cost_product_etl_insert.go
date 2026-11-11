package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/costproductmaster"
)

const etlSource = "ORACLE_ETL"

// etlLockSQL uses the same lock function as SaveErpLink/CreateWithErpLink so ETL and manual ERP
// linking serialize on the same (item, shade) key.
const etlLockSQL = `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`

// etlNameLockNS namespaces the name+shade lock that guards the POSSIBLE_DUPLICATE re-check.
const etlNameLockNS = "cpm_etl_name:"

// Re-check inside the tx: match (a) item+shade / flex_01, or (c) same name+shade under another item.
const etlRecheckSQL = `
SELECT COUNT(*) FROM cost_product_master m
 WHERE (UPPER(TRIM(COALESCE(m.cpm_erp_item_code, ''))) = $1
        AND UPPER(TRIM(COALESCE(m.cpm_shade_code, ''))) = $2)
    OR (m.cpm_flex_01 IS NOT NULL
        AND regexp_replace(UPPER(m.cpm_flex_01), '-[^-]*$', '') = $1 || '-' || $2)
    OR (UPPER(TRIM(m.cpm_product_name)) = $3
        AND UPPER(TRIM(COALESCE(m.cpm_shade_code, ''))) = $2
        AND UPPER(TRIM(COALESCE(m.cpm_erp_item_code, ''))) <> $1)`

const etlInsertProductSQL = `
INSERT INTO cost_product_master (
  cpm_product_code, cpm_product_type_id, cpm_product_name, cpm_shade_code, cpm_shade_name, cpm_grade_code,
  cpm_erp_item_code, cpm_flex_01, cpm_source, cpm_is_active,
  cpm_created_at, cpm_created_by, cpm_updated_at, cpm_updated_by)
VALUES (generate_cost_product_code($1, $2), $1, $3, $4, NULLIF($5, ''), 'AX',
        $6, $7, $8, TRUE, $2, $9, $2, $9)
RETURNING cpm_product_sys_id, cpm_product_code`

const etlAttachSQL = `
INSERT INTO cost_product_applicable_param (capp_product_sys_id, capp_param_id, capp_is_required, capp_created_by)
VALUES ($1, $2, FALSE, $3) ON CONFLICT (capp_product_sys_id, capp_param_id) DO NOTHING`

const etlPutSQL = `
INSERT INTO cost_product_parameter
  (cpp_product_sys_id, cpp_param_id, cpp_value_numeric, cpp_filled_by, cpp_created_by)
VALUES ($1, $2, CAST($3 AS numeric), $4, $4)
ON CONFLICT (cpp_product_sys_id, cpp_param_id) DO NOTHING`

// InsertEtlProducts inserts all items in ONE transaction and stores the APPLY run in the same tx.
// Any SQL error rolls back the whole run.
func (r *CostProductEtlRepository) InsertEtlProducts(ctx context.Context, in costproductmaster.EtlInsertInput) (out costproductmaster.EtlInsertOutput, err error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return out, fmt.Errorf("begin tx: %w", err)
	}
	defer func() {
		if err != nil {
			if rbErr := tx.Rollback(); rbErr != nil && !errors.Is(rbErr, sql.ErrTxDone) {
				_ = rbErr
			}
		}
	}()
	paramIDs, err := etlParamIDs(ctx, tx)
	if err != nil {
		return out, err
	}
	if err = etlLockAll(ctx, tx, in.Items); err != nil {
		return out, err
	}
	now := time.Now().UTC()
	for _, it := range in.Items {
		res, ierr := insertOne(ctx, tx, it, in.Actor, now, paramIDs)
		if ierr != nil {
			err = ierr
			return out, err
		}
		out.Results = append(out.Results, res)
	}
	pid := in.PreviewRunID
	all := append(append([]costproductmaster.EtlInsertResult{}, in.Skipped...), out.Results...)
	out.RunID, err = insertRun(ctx, tx, costproductmaster.EtlRun{
		Type: costproductmaster.EtlRunApply, PreviewRunID: &pid, FromDate: in.FromDate,
		ApplyRows: all, Status: costproductmaster.EtlRunCompleted, CreatedBy: in.Actor,
	})
	if err != nil {
		return out, err
	}
	if err = tx.Commit(); err != nil {
		return out, fmt.Errorf("commit: %w", err)
	}
	return out, nil
}

func etlParamIDs(ctx context.Context, tx *sql.Tx) (map[string]uuid.UUID, error) {
	rows, err := tx.QueryContext(ctx,
		`SELECT param_code, id FROM mst_parameter WHERE deleted_at IS NULL AND param_code = ANY($1)`,
		pq.Array([]string{"DENIER", "NO_OF_FILAMENTS", "ACT_DENIER"}))
	if err != nil {
		return nil, fmt.Errorf("lookup etl params: %w", err)
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil {
			_ = cerr
		}
	}()
	out := map[string]uuid.UUID{}
	for rows.Next() {
		var code string
		var id uuid.UUID
		if err := rows.Scan(&code, &id); err != nil {
			return nil, err
		}
		out[code] = id
	}
	return out, rows.Err()
}

func insertOne(ctx context.Context, tx *sql.Tx, it costproductmaster.EtlInsert, actor string, now time.Time, paramIDs map[string]uuid.UUID) (costproductmaster.EtlInsertResult, error) {
	res := costproductmaster.EtlInsertResult{RowKey: it.RowKey}
	item, shade := costproductmaster.EtlNorm(it.ItemCode), costproductmaster.EtlNorm(it.ShadeCode)
	var n int
	if err := tx.QueryRowContext(ctx, etlRecheckSQL, item, shade, costproductmaster.EtlNorm(it.ItemName)).Scan(&n); err != nil {
		return res, fmt.Errorf("recheck: %w", err)
	}
	if n > 0 {
		res.Result, res.Message = costproductmaster.EtlResultSkipped, "already exists"
		return res, nil
	}
	flex01 := it.ItemCode + "-" + it.ShadeCode + "-" + it.TypeCode
	if err := tx.QueryRowContext(ctx, etlInsertProductSQL, it.TypeID, now, it.ItemName, it.ShadeCode, it.ShadeName,
		it.ItemCode, flex01, etlSource, actor).Scan(&res.ProductSysID, &res.ProductCode); err != nil {
		return res, fmt.Errorf("insert product %s: %w", it.RowKey, err)
	}
	vals := map[string]string{"DENIER": it.Denier, "NO_OF_FILAMENTS": it.Filament, "ACT_DENIER": it.ActDenier}
	for code, v := range vals {
		pid, ok := paramIDs[code]
		if v == "" || !ok {
			continue
		}
		if _, err := tx.ExecContext(ctx, etlAttachSQL, res.ProductSysID, pid, actor); err != nil {
			return res, fmt.Errorf("attach %s: %w", code, err)
		}
		if _, err := tx.ExecContext(ctx, etlPutSQL, res.ProductSysID, pid, v, actor); err != nil {
			return res, fmt.Errorf("put %s: %w", code, err)
		}
	}
	res.Result = costproductmaster.EtlResultInserted
	return res, nil
}

// etlLockAll takes every advisory lock of the run up front, in one global sorted order, so two
// concurrent applies can never deadlock. Per item it locks the ERP-link key (shared with
// SaveErpLink/CreateWithErpLink) and the name+shade key (guards the POSSIBLE_DUPLICATE check).
func etlLockAll(ctx context.Context, tx *sql.Tx, items []costproductmaster.EtlInsert) error {
	seen := map[string]struct{}{}
	keys := make([]string, 0, len(items)*2)
	add := func(k string) {
		if _, ok := seen[k]; !ok {
			seen[k] = struct{}{}
			keys = append(keys, k)
		}
	}
	for _, it := range items {
		shade := costproductmaster.EtlNorm(it.ShadeCode)
		add(cpmErpLinkLockNS + costproductmaster.EtlNorm(it.ItemCode) + "|" + shade)
		add(etlNameLockNS + costproductmaster.EtlNorm(it.ItemName) + "|" + shade)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if _, err := tx.ExecContext(ctx, etlLockSQL, k); err != nil {
			return fmt.Errorf("advisory lock: %w", err)
		}
	}
	return nil
}
