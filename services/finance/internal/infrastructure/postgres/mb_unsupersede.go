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

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbspin"
)

// Un-supersede (mb-spin-fg-poy-link P3, design Revisi 1 D8). Symmetric to the supersede:
// S_new is deactivated and marked superseded by S_old, S_old is re-activated, the old head
// and old MB product are re-activated, and the params of the latest un-reverted batch are
// restored row by row. All in ONE tx, no hard delete. H_new / the new MB product are NOT
// touched (reported in the result warnings).

const unsupersedeWorkflowLogToState = "UNSUPERSEDE"

// MBUnsupersedeRepository implements mbspin.Unsuperseder.
type MBUnsupersedeRepository struct{ db *DB }

// NewMBUnsupersedeRepository creates an MBUnsupersedeRepository.
func NewMBUnsupersedeRepository(db *DB) *MBUnsupersedeRepository {
	return &MBUnsupersedeRepository{db: db}
}

var _ mbspin.Unsuperseder = (*MBUnsupersedeRepository)(nil)

// Unsupersede restores spinID (which must belong to headID) in one transaction.
func (r *MBUnsupersedeRepository) Unsupersede(ctx context.Context, headID, spinID uuid.UUID, actor string) (*mbspin.UnsupersedeResult, error) {
	var res *mbspin.UnsupersedeResult
	err := r.db.Transaction(ctx, func(tx *sql.Tx) error {
		var err error
		res, err = unsupersedeTx(ctx, tx, headID, spinID, actor)
		return err
	})
	if err != nil {
		return nil, err
	}
	return res, nil
}

// unsupersedeSpinRow is the locked S_old state.
type unsupersedeSpinRow struct {
	NewSpinID uuid.UUID
	Shade     string
	FgID      sql.NullInt64
}

func unsupersedeTx(ctx context.Context, tx *sql.Tx, headID, spinID uuid.UUID, actor string) (*mbspin.UnsupersedeResult, error) {
	old, err := lockUnsupersedeSpin(ctx, tx, headID, spinID)
	if err != nil {
		return nil, err
	}
	var newHeadCode string
	var newHeadID uuid.UUID
	if err := tx.QueryRowContext(ctx, `
		SELECT h.mbh_id, COALESCE(h.mbh_mb_costing, '') FROM mst_mb_spin s JOIN mst_mb_head h ON h.mbh_id = s.mbs_mbh_id
		WHERE s.mbs_id = $1 FOR UPDATE OF s`, old.NewSpinID).Scan(&newHeadID, &newHeadCode); err != nil {
		return nil, fmt.Errorf("unsupersede: lock superseding spin: %w", err)
	}
	batchID, err := findUnsupersedeBatch(ctx, tx, spinID, old.NewSpinID)
	if err != nil {
		return nil, err
	}
	if err := checkUnsupersedeConflict(ctx, tx, spinID, old); err != nil {
		return nil, err
	}
	if err := swapSpinsTx(ctx, tx, spinID, old.NewSpinID, actor); err != nil {
		return nil, err
	}
	res := &mbspin.UnsupersedeResult{BatchID: batchID, NewSpinID: old.NewSpinID}
	if batchID != uuid.Nil {
		if err := revertSupersedeParamsTx(ctx, tx, spinID, batchID, actor, res); err != nil {
			return nil, err
		}
	}
	react, err := reactivateOldHeadTx(ctx, tx, headID, old.NewSpinID, actor)
	if err != nil {
		return nil, err
	}
	if err := logUnsupersedeTx(ctx, tx, headID, newHeadID, spinID, old.NewSpinID, batchID, react, actor, res); err != nil {
		return nil, err
	}
	res.Warnings = append(res.Warnings, fmt.Sprintf(
		"the superseding recipe %s and its MB product stay active; deactivate them manually if no longer needed", newHeadCode))
	if res.Spin, err = loadSpinForRepointOn(ctx, tx, spinID); err != nil {
		return nil, err
	}
	return res, nil
}

// findUnsupersedeBatch returns the un-reverted batch that linked spinID to newSpinID, or
// uuid.Nil when that supersede wrote no param rows (nothing referenced the old spin).
func findUnsupersedeBatch(ctx context.Context, tx *sql.Tx, spinID, newSpinID uuid.UUID) (uuid.UUID, error) {
	var batchID uuid.UUID
	err := tx.QueryRowContext(ctx, `
		SELECT mssi_batch_id FROM mst_mb_spin_supersede_item
		WHERE mssi_old_spin_id = $1 AND mssi_new_spin_id = $2 AND mssi_reverted_at IS NULL
		ORDER BY created_at DESC, mssi_id DESC LIMIT 1`, spinID, newSpinID).Scan(&batchID)
	if errors.Is(err, sql.ErrNoRows) {
		return uuid.Nil, nil
	}
	if err != nil {
		return uuid.Nil, fmt.Errorf("unsupersede: find batch: %w", err)
	}
	return batchID, nil
}

// lockUnsupersedeSpin locks S_old and validates it belongs to headID and is superseded.
func lockUnsupersedeSpin(ctx context.Context, tx *sql.Tx, headID, spinID uuid.UUID) (unsupersedeSpinRow, error) {
	var (
		out    unsupersedeSpinRow
		by     sql.NullString
		spinHd uuid.UUID
	)
	err := tx.QueryRowContext(ctx, `
		SELECT mbs_mbh_id, mbs_superseded_by_spin_id, COALESCE(mbs_shade_code, ''), mbs_fg_product_id
		FROM mst_mb_spin WHERE mbs_id = $1 AND deleted_at IS NULL FOR UPDATE`, spinID).
		Scan(&spinHd, &by, &out.Shade, &out.FgID)
	if errors.Is(err, sql.ErrNoRows) {
		return out, mbspin.ErrNotFound
	}
	if err != nil {
		return out, fmt.Errorf("unsupersede: lock spin: %w", err)
	}
	if spinHd != headID {
		return out, mbspin.ErrNotFound
	}
	if !by.Valid {
		return out, mbspin.ErrNotSuperseded
	}
	if out.NewSpinID, err = uuid.Parse(by.String); err != nil {
		return out, fmt.Errorf("unsupersede: bad superseded_by_spin_id: %w", err)
	}
	return out, nil
}

// checkUnsupersedeConflict rejects when a live spin other than S_old / S_new holds the
// same (shade, FG) identity (uix_mbs_shade_fg_live).
func checkUnsupersedeConflict(ctx context.Context, tx *sql.Tx, spinID uuid.UUID, old unsupersedeSpinRow) error {
	if !old.FgID.Valid || old.FgID.Int64 <= 0 || old.Shade == "" {
		return nil
	}
	var exists bool
	if err := tx.QueryRowContext(ctx, `
		SELECT EXISTS (SELECT 1 FROM mst_mb_spin s
			WHERE s.deleted_at IS NULL AND s.mbs_is_active AND s.mbs_superseded_by_spin_id IS NULL
			  AND s.mbs_fg_product_id = $1 AND UPPER(TRIM(s.mbs_shade_code)) = UPPER(TRIM($2))
			  AND s.mbs_id <> $3 AND s.mbs_id <> $4)`,
		old.FgID.Int64, old.Shade, spinID, old.NewSpinID).Scan(&exists); err != nil {
		return fmt.Errorf("unsupersede: conflict check: %w", err)
	}
	if exists {
		return mbspin.ErrUnsupersedeConflict
	}
	return nil
}

// swapSpinsTx deactivates S_new first (frees the unique index) then restores S_old.
func swapSpinsTx(ctx context.Context, tx *sql.Tx, oldID, newID uuid.UUID, actor string) error {
	if _, err := tx.ExecContext(ctx, `
		UPDATE mst_mb_spin SET mbs_is_active = FALSE, mbs_superseded_by_spin_id = $2,
		       mbs_superseded_at = NOW(), mbs_superseded_by = $3, updated_at = NOW(), updated_by = $3
		WHERE mbs_id = $1`, newID, oldID, actor); err != nil {
		return fmt.Errorf("unsupersede: deactivate superseding spin: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE mst_mb_spin SET mbs_is_active = TRUE, mbs_superseded_by_spin_id = NULL,
		       mbs_superseded_at = NULL, mbs_superseded_by = NULL, updated_at = NOW(), updated_by = $2
		WHERE mbs_id = $1`, oldID, actor); err != nil {
		if isSpinShadeFgViolation(err) {
			return mbspin.ErrUnsupersedeConflict
		}
		return fmt.Errorf("unsupersede: reactivate spin: %w", err)
	}
	return nil
}

// unsupersedeItem is one mst_mb_spin_supersede_item row joined with its current cpp row.
type unsupersedeItem struct {
	ItemID       int64
	CppValueID   int64
	ProductSysID int64
	ProductCode  string
	ParamCode    string
	Kind         string
	CppExists    bool
	Locked       bool
	Matches      bool // current cpp row still equals the superseded-in values
	OldExisted   bool
}

const unsupersedeItemsSQL = `
	SELECT i.mssi_id, i.mssi_cpp_value_id, i.mssi_product_sys_id, COALESCE(m.cpm_product_code, ''),
	       COALESCE(i.mssi_param_code, ''), COALESCE(i.mssi_repoint_kind, ''),
	       c.cpp_value_id IS NOT NULL, COALESCE(m.cpm_is_locked, FALSE),
	       COALESCE(c.cpp_value_text IS NOT DISTINCT FROM i.mssi_new_value_text
	            AND ROUND(c.cpp_value_numeric, 6) IS NOT DISTINCT FROM ROUND(i.mssi_new_value_numeric, 6)
	            AND (COALESCE(i.mssi_repoint_kind, '') NOT IN ('POINTER', 'TEXT_ONLY')
	                 OR c.cpp_value_mb_spin_id = i.mssi_new_spin_id), FALSE),
	       i.mssi_old_filled_by IS NOT NULL
	FROM mst_mb_spin_supersede_item i
	LEFT JOIN cost_product_parameter c ON c.cpp_value_id = i.mssi_cpp_value_id
	LEFT JOIN cost_product_master m ON m.cpm_product_sys_id = i.mssi_product_sys_id
	WHERE i.mssi_batch_id = $1 AND i.mssi_old_spin_id = $2 AND i.mssi_reverted_at IS NULL
	ORDER BY i.mssi_id
	FOR UPDATE OF i`

func listUnsupersedeItems(ctx context.Context, tx *sql.Tx, batchID, oldID uuid.UUID) ([]unsupersedeItem, error) {
	// Lock the cpp rows first (FOR UPDATE cannot target the nullable side of an outer join).
	if _, err := tx.ExecContext(ctx, `
		SELECT 1 FROM cost_product_parameter WHERE cpp_value_id IN (
			SELECT mssi_cpp_value_id FROM mst_mb_spin_supersede_item
			WHERE mssi_batch_id = $1 AND mssi_old_spin_id = $2 AND mssi_reverted_at IS NULL)
		FOR UPDATE`, batchID, oldID); err != nil {
		return nil, fmt.Errorf("unsupersede: lock param rows: %w", err)
	}
	rows, err := tx.QueryContext(ctx, unsupersedeItemsSQL, batchID, oldID)
	if err != nil {
		return nil, fmt.Errorf("unsupersede: list items: %w", err)
	}
	defer closeRows(rows)
	var out []unsupersedeItem
	for rows.Next() {
		var it unsupersedeItem
		if err := rows.Scan(&it.ItemID, &it.CppValueID, &it.ProductSysID, &it.ProductCode, &it.ParamCode,
			&it.Kind, &it.CppExists, &it.Locked, &it.Matches, &it.OldExisted); err != nil {
			return nil, fmt.Errorf("unsupersede: scan item: %w", err)
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

// skipReason returns why the item cannot be reverted, "" when it can.
func (it unsupersedeItem) skipReason() string {
	switch {
	case !it.CppExists:
		return mbspin.UnsupersedeSkipRowDeleted
	case it.Locked:
		return mbspin.UnsupersedeSkipProductLocked
	case !it.Matches:
		return mbspin.UnsupersedeSkipChanged
	case !it.OldExisted:
		// cpp_one_value_chk forbids an all-NULL row and rows are never hard deleted.
		return mbspin.UnsupersedeSkipNoPreviousData
	}
	return ""
}

const unsupersedeRevertSQL = `
	UPDATE cost_product_parameter c
	SET cpp_value_text = i.mssi_old_value_text, cpp_value_numeric = i.mssi_old_value_numeric,
	    cpp_value_flag = NULL, cpp_value_mb_spin_id = i.mssi_old_mb_spin_id,
	    cpp_filled_by = i.mssi_old_filled_by, cpp_filled_at = NOW(),
	    cpp_updated_at = NOW(), cpp_updated_by = $2
	FROM mst_mb_spin_supersede_item i
	WHERE i.mssi_id = $1 AND c.cpp_value_id = i.mssi_cpp_value_id`

// itemReasons computes the skip reason per item. A CHILD row is also skipped when its
// product's trigger (POINTER / TEXT_ONLY) row is skipped, so no mixed state is produced.
func itemReasons(items []unsupersedeItem) []string {
	reasons := make([]string, len(items))
	triggerKept := map[int64]bool{}
	for i, it := range items {
		reasons[i] = it.skipReason()
		if reasons[i] != "" && it.Kind != mbspin.SupersedeRepointChild {
			triggerKept[it.ProductSysID] = true
		}
	}
	for i, it := range items {
		if reasons[i] == "" && it.Kind == mbspin.SupersedeRepointChild && triggerKept[it.ProductSysID] {
			reasons[i] = mbspin.UnsupersedeSkipTriggerKept
		}
	}
	return reasons
}

// revertSupersedeParamsTx restores every revertable row of the batch and reports the rest.
func revertSupersedeParamsTx(ctx context.Context, tx *sql.Tx, oldID, batchID uuid.UUID, actor string, res *mbspin.UnsupersedeResult) error {
	items, err := listUnsupersedeItems(ctx, tx, batchID, oldID)
	if err != nil {
		return err
	}
	reasons := itemReasons(items)
	seen := map[int64]bool{}
	for i, it := range items {
		if reason := reasons[i]; reason != "" {
			res.Skipped = append(res.Skipped, mbspin.UnsupersedeSkipped{
				CppValueID: it.CppValueID, ProductSysID: it.ProductSysID, ProductCode: it.ProductCode,
				ParamCode: it.ParamCode, Reason: reason})
			continue
		}
		if _, err := tx.ExecContext(ctx, unsupersedeRevertSQL, it.ItemID, actor); err != nil {
			return fmt.Errorf("unsupersede: revert %s of product %s: %w", it.ParamCode, it.ProductCode, err)
		}
		if _, err := tx.ExecContext(ctx, `UPDATE mst_mb_spin_supersede_item SET mssi_reverted_at = NOW(), mssi_reverted_by = $2
			WHERE mssi_id = $1`, it.ItemID, actor); err != nil {
			return fmt.Errorf("unsupersede: mark item reverted: %w", err)
		}
		res.ParamsReverted++
		if !seen[it.ProductSysID] {
			seen[it.ProductSysID] = true
			res.AffectedProductIDs = append(res.AffectedProductIDs, it.ProductSysID)
		}
	}
	sort.Slice(res.AffectedProductIDs, func(i, j int) bool { return res.AffectedProductIDs[i] < res.AffectedProductIDs[j] })
	return nil
}

// unsupersedeReactivation records what un-supersede re-activated.
type unsupersedeReactivation struct {
	Head     bool
	Products []int64
}

// reactivateOldHeadTx re-activates exactly what the matching SUPERSEDE log row of the old
// head recorded as deactivated (head and products independently). Without such a row
// (legacy) it falls back to the head's superseded marker and its own MB product.
func reactivateOldHeadTx(ctx context.Context, tx *sql.Tx, headID, newSpinID uuid.UUID, actor string) (unsupersedeReactivation, error) {
	var out unsupersedeReactivation
	var raw []byte
	err := tx.QueryRowContext(ctx, `
		SELECT mbwl_meta FROM mst_mb_workflow_log
		WHERE mbwl_mbh_id = $1 AND mbwl_to_state = $2 AND mbwl_meta->>'role' = 'OLD'
		  AND mbwl_meta->>'new_spin_id' = $3
		ORDER BY mbwl_actor_at DESC LIMIT 1`, headID, supersedeWorkflowLogToState, newSpinID.String()).Scan(&raw)
	logged := err == nil
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return out, fmt.Errorf("unsupersede: read supersede log: %w", err)
	}
	var meta supersedeLogMeta
	if logged {
		if err := json.Unmarshal(raw, &meta); err != nil {
			return out, fmt.Errorf("unsupersede: parse supersede log: %w", err)
		}
	}
	if !logged || meta.HeadDeactivated {
		var product sql.NullInt64
		err := tx.QueryRowContext(ctx, `
			UPDATE mst_mb_head
			SET mbh_is_active = TRUE, mbh_superseded_by_head_id = NULL, mbh_superseded_at = NULL,
			    mbh_superseded_by = NULL, updated_at = NOW(), updated_by = $2
			WHERE mbh_id = $1 AND deleted_at IS NULL AND mbh_superseded_by_head_id IS NOT NULL
			RETURNING mbh_cost_product_id`, headID, actor).Scan(&product)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return out, fmt.Errorf("unsupersede: reactivate head: %w", err)
		}
		out.Head = err == nil
		if !logged && product.Valid && product.Int64 > 0 {
			meta.DeactivatedProductIDs = []int64{product.Int64}
		}
	}
	if len(meta.DeactivatedProductIDs) > 0 {
		if _, err := tx.ExecContext(ctx, `
			UPDATE cost_product_master SET cpm_is_active = TRUE, cpm_updated_at = NOW(), cpm_updated_by = $2
			WHERE cpm_product_sys_id = ANY($1) AND NOT cpm_is_active`, pq.Array(meta.DeactivatedProductIDs), actor); err != nil {
			return out, fmt.Errorf("unsupersede: reactivate mb product: %w", err)
		}
		out.Products = meta.DeactivatedProductIDs
	}
	return out, nil
}

// unsupersedeLogMeta is the mbwl_meta document of the UNSUPERSEDE audit row.
type unsupersedeLogMeta struct {
	Event               string      `json:"event"`
	Role                string      `json:"role"`
	BatchID             uuid.UUID   `json:"batch_id"`
	RestoredSpinID      uuid.UUID   `json:"restored_spin_id"`
	SupersedingSpinID   uuid.UUID   `json:"superseding_spin_id"`
	SupersedingHeadID   uuid.UUID   `json:"superseding_head_id"`
	HeadReactivated     bool        `json:"head_reactivated"`
	ProductsReactivated []int64     `json:"products_reactivated,omitempty"`
	AffectedProducts    []int64     `json:"affected_product_ids"`
	ParamsReverted      int         `json:"params_reverted"`
	SkippedCount        int         `json:"skipped_count"`
	OldSpinIDs          []uuid.UUID `json:"old_spin_ids"`
}

func logUnsupersedeTx(ctx context.Context, tx *sql.Tx, headID, newHeadID, oldID, newID, batchID uuid.UUID, react unsupersedeReactivation, actor string, res *mbspin.UnsupersedeResult) error {
	meta := unsupersedeLogMeta{
		Event: unsupersedeWorkflowLogToState, Role: "OLD", BatchID: batchID, RestoredSpinID: oldID,
		SupersedingSpinID: newID, SupersedingHeadID: newHeadID, HeadReactivated: react.Head,
		ProductsReactivated: react.Products, AffectedProducts: res.AffectedProductIDs,
		ParamsReverted: res.ParamsReverted, SkippedCount: len(res.Skipped), OldSpinIDs: []uuid.UUID{oldID},
	}
	return insertSupersedeEventLogTx(ctx, tx, headID, actor, "UNSUPERSEDE: superseded spin restored",
		unsupersedeWorkflowLogToState, meta)
}
