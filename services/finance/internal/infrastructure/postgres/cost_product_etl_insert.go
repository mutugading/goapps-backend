package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/costproductmaster"
)

const etlSource = "ORACLE_ETL"

const etlLockSQL = `SELECT pg_advisory_xact_lock(hashtext($1))`

// Re-check inside the tx: match (a) item+shade / flex_01, or (c) same name+shade under another item.
const etlRecheckSQL = `
SELECT COUNT(*) FROM cost_product_master m
 WHERE (UPPER(TRIM(COALESCE(m.cpm_erp_item_code, ''))) = $1
        AND UPPER(TRIM(COALESCE(m.cpm_shade_code, ''))) = $2)
    OR UPPER(COALESCE(m.cpm_flex_01, '')) LIKE $3 ESCAPE '\'
    OR (UPPER(TRIM(m.cpm_product_name)) = $4
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
	if _, err := tx.ExecContext(ctx, etlLockSQL, "cpm_etl:"+item+"|"+shade); err != nil {
		return res, fmt.Errorf("advisory lock: %w", err)
	}
	var n int
	pat := escapeLike(item) + "-" + escapeLike(shade) + "-%"
	if err := tx.QueryRowContext(ctx, etlRecheckSQL, item, shade, pat, costproductmaster.EtlNorm(it.ItemName)).Scan(&n); err != nil {
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
