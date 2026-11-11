package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"

	"github.com/google/uuid"
	"github.com/lib/pq"

	"github.com/mutugading/goapps-backend/services/finance/internal/application/mbsourceautofill"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/costproductparameter"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbsource"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbspin"
)

// Forced MB source writes (mb-spin-fg-poy-link P3, design §2.5 / D7). Unlike Apply (fill
// only empty values, unchanged), a forced write OVERWRITES the MB_SP_CODE row (text +
// spin pointer) and every fill-group child the resolution carries. Children the resolution
// has no value for are left untouched (clearing would need a row delete — never done here)
// and reported. Used by the confirmed supersede (repointSupersededParamsTx, inside the
// Validate tx) and by ApplyForce (own tx, future explicit "Re-link", P4).

// forceUpsertSQL overwrites one value row; $7 is the actor (created_by / updated_by).
const forceUpsertSQL = `INSERT INTO cost_product_parameter
	   (cpp_product_sys_id, cpp_param_id, cpp_value_numeric, cpp_value_text, cpp_value_mb_spin_id,
	    cpp_filled_by, cpp_created_by)
	 VALUES ($1, $2, $3, $4, $5, $6, $7)
	 ON CONFLICT (cpp_product_sys_id, cpp_param_id) DO UPDATE SET
	    cpp_value_numeric = EXCLUDED.cpp_value_numeric, cpp_value_text = EXCLUDED.cpp_value_text,
	    cpp_value_flag = NULL, cpp_value_mb_spin_id = EXCLUDED.cpp_value_mb_spin_id,
	    cpp_filled_at = NOW(), cpp_filled_by = EXCLUDED.cpp_filled_by,
	    cpp_updated_at = NOW(), cpp_updated_by = $7
	 RETURNING cpp_value_id`

// forceAttachSQL is Apply's CAPP attach (idempotent).
const forceAttachSQL = `INSERT INTO cost_product_applicable_param (capp_product_sys_id, capp_param_id, capp_is_required, capp_created_by)
	VALUES ($1, $2, FALSE, $3) ON CONFLICT (capp_product_sys_id, capp_param_id) DO NOTHING`

// forceOldRow is a value row as it was before a forced write.
type forceOldRow struct {
	Text     *string
	Num      *float64
	SpinID   *uuid.UUID
	FilledBy string
}

// forceChange is one cost_product_parameter row a forced write changed. Old* are nil
// (OldExisted false) when the row was inserted by the write.
type forceChange struct {
	CppValueID   int64
	ProductSysID int64
	ParamCode    string
	OldExisted   bool
	Old          forceOldRow
	NewText      *string
	NewNum       *float64
	// Kind is mbspin.SupersedeRepointPointer / TextOnly for the MB_SP_CODE row,
	// mbspin.SupersedeRepointChild for a child.
	Kind string
}

// forceWriteResult is the outcome of forceWriteMBSourceTx for one product.
type forceWriteResult struct {
	Changes []forceChange
	// KeptChildren are child codes holding a value the new resolution has none for.
	KeptChildren []string
}

// forceValue is one value to write.
type forceValue struct {
	paramID   uuid.UUID
	code      string
	num       *float64
	text      *string
	spinID    *uuid.UUID
	kind      string
	isTrigger bool
}

// forceWriteMBSourceTx attaches the MB_SP_CODE group (CAPP) to productSysID and overwrites
// MB_SP_CODE (text + pointer) and each child the resolution has a value for. triggerKind
// labels the MB_SP_CODE change. Rows whose value is already identical are not rewritten.
func forceWriteMBSourceTx(ctx context.Context, tx *sql.Tx, ps mbsourceautofill.ParamSet, productSysID int64,
	res mbsource.Resolution, triggerKind, filledBy, actor string) (forceWriteResult, error) {
	var out forceWriteResult
	ids := forceParamIDs(ps)
	for _, id := range ids {
		if _, err := tx.ExecContext(ctx, forceAttachSQL, productSysID, id, actor); err != nil {
			return out, fmt.Errorf("attach capp: %w", err)
		}
	}
	old, err := lockForceOldRows(ctx, tx, productSysID, ids)
	if err != nil {
		return out, err
	}
	for _, v := range forceValues(ps, res, triggerKind) {
		prev, existed := old[v.paramID]
		if existed && forceUnchanged(prev, v) {
			continue
		}
		var cppID int64
		if err := tx.QueryRowContext(ctx, forceUpsertSQL, productSysID, v.paramID, v.num, v.text,
			v.spinID, filledBy, actor).Scan(&cppID); err != nil {
			return out, fmt.Errorf("force write %s: %w", v.code, err)
		}
		out.Changes = append(out.Changes, forceChange{
			CppValueID: cppID, ProductSysID: productSysID, ParamCode: v.code,
			OldExisted: existed, Old: prev, NewText: v.text, NewNum: v.num, Kind: v.kind,
		})
	}
	out.KeptChildren = forceKeptChildren(ps, res, old)
	return out, nil
}

// forceParamIDs is the trigger id followed by the child ids in code order.
func forceParamIDs(ps mbsourceautofill.ParamSet) []uuid.UUID {
	ids := []uuid.UUID{ps.TriggerID}
	for _, code := range sortedChildCodes(ps) {
		ids = append(ids, ps.Children[code])
	}
	return ids
}

func sortedChildCodes(ps mbsourceautofill.ParamSet) []string {
	codes := make([]string, 0, len(ps.Children))
	for c := range ps.Children {
		codes = append(codes, c)
	}
	sort.Strings(codes)
	return codes
}

// forceValues lists the values to write: MB_SP_CODE first, then children (code order)
// that have a non-empty value in res.
func forceValues(ps mbsourceautofill.ParamSet, res mbsource.Resolution, triggerKind string) []forceValue {
	code := res.SPCode
	out := []forceValue{{paramID: ps.TriggerID, code: mbsource.ParamSPCode, text: &code,
		spinID: res.SpinID, kind: triggerKind, isTrigger: true}}
	for _, c := range sortedChildCodes(ps) {
		v, ok := res.Children[c]
		if !ok {
			continue
		}
		fv := forceValue{paramID: ps.Children[c], code: c, kind: mbspin.SupersedeRepointChild}
		switch {
		case v.Num != nil:
			n := *v.Num
			fv.num = &n
		case v.Text != nil && *v.Text != "":
			t := *v.Text
			fv.text = &t
		default:
			continue
		}
		out = append(out, fv)
	}
	return out
}

// forceKeptChildren lists child codes that have a stored value but no new value.
func forceKeptChildren(ps mbsourceautofill.ParamSet, res mbsource.Resolution, old map[uuid.UUID]forceOldRow) []string {
	kept := make([]string, 0, len(ps.Children))
	for _, c := range sortedChildCodes(ps) {
		if _, has := old[ps.Children[c]]; !has {
			continue
		}
		v, ok := res.Children[c]
		if ok && (v.Num != nil || (v.Text != nil && *v.Text != "")) {
			continue
		}
		kept = append(kept, c)
	}
	return kept
}

// forceUnchanged: the stored row already holds v (the trigger also compares the pointer).
func forceUnchanged(prev forceOldRow, v forceValue) bool {
	if !forceSameValue(prev, v) {
		return false
	}
	return !v.isTrigger || forceSameSpin(prev.SpinID, v.spinID)
}

func forceSameValue(prev forceOldRow, v forceValue) bool {
	switch {
	case v.num != nil:
		return prev.Num != nil && prev.Text == nil && *prev.Num == *v.num
	case v.text != nil:
		return prev.Text != nil && prev.Num == nil && *prev.Text == *v.text
	default:
		return false
	}
}

func forceSameSpin(a, b *uuid.UUID) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

// lockForceOldRows reads (FOR UPDATE) the existing value rows of the group params.
func lockForceOldRows(ctx context.Context, tx *sql.Tx, productSysID int64, ids []uuid.UUID) (map[uuid.UUID]forceOldRow, error) {
	strIDs := make([]string, 0, len(ids))
	for _, id := range ids {
		strIDs = append(strIDs, id.String())
	}
	rows, err := tx.QueryContext(ctx, `
		SELECT cpp_param_id, cpp_value_text, cpp_value_numeric, cpp_value_mb_spin_id, cpp_filled_by
		FROM cost_product_parameter
		WHERE cpp_product_sys_id = $1 AND cpp_param_id = ANY(CAST($2 AS uuid[]))
		FOR UPDATE`, productSysID, pq.Array(strIDs))
	if err != nil {
		return nil, fmt.Errorf("lock mb source rows: %w", err)
	}
	defer closeRows(rows)
	out := make(map[uuid.UUID]forceOldRow, len(ids))
	for rows.Next() {
		var (
			pid  uuid.UUID
			text sql.NullString
			num  sql.NullFloat64
			spin sql.NullString
			r    forceOldRow
		)
		if err := rows.Scan(&pid, &text, &num, &spin, &r.FilledBy); err != nil {
			return nil, fmt.Errorf("scan mb source row: %w", err)
		}
		r.Text, r.Num, r.SpinID = nullableStringPtr(text), nullableFloat64Ptr(num), nullableUUIDPtr(spin)
		out[pid] = r
	}
	return out, rows.Err()
}

// ApplyForce overwrites the MB source group of plan.ProductSysID with plan.Resolution in
// its own transaction (explicit "Re-link", P4). Unlike Apply it replaces existing values;
// values are tagged cpp_filled_by = mbsource.FilledBy, cpp_updated_by = actor. A locked
// product returns costproductparameter.ErrProductLocked without writes. No supersede audit.
func (s *MBSourceAutoFillStore) ApplyForce(ctx context.Context, ps mbsourceautofill.ParamSet, plan mbsourceautofill.Plan, actor string) error {
	if actor == "" {
		actor = mbsource.FilledBy
	}
	return s.db.Transaction(ctx, func(tx *sql.Tx) error {
		var locked bool
		err := tx.QueryRowContext(ctx, `SELECT cpm_is_locked FROM cost_product_master
			WHERE cpm_product_sys_id = $1 FOR UPDATE`, plan.ProductSysID).Scan(&locked)
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("product %d not found", plan.ProductSysID)
		}
		if err != nil {
			return fmt.Errorf("lock product: %w", err)
		}
		if locked {
			return costproductparameter.ErrProductLocked
		}
		_, err = forceWriteMBSourceTx(ctx, tx, ps, plan.ProductSysID, plan.Resolution,
			mbspin.SupersedeRepointPointer, mbsource.FilledBy, actor)
		return err
	})
}
