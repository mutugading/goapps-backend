package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/finance/internal/application/mbsourceautofill"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbsource"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbspin"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/parameter"
)

// SupersedeRepointResult is the outcome of repointSupersededParamsTx for one old spin.
type SupersedeRepointResult struct {
	// Products are the repointed product sys ids (sorted, unique) — recalc targets.
	Products []int64
	// Rows is the number of cost_product_parameter rows changed (= mssi rows written).
	Rows int
	// Warnings: locked products skipped, shared ORION code text rows skipped, children kept.
	Warnings []string
}

// repointSupersededParamsTx moves every MB_SP_CODE row referencing old (design D7 + Revisi 2:
// pointer rows whatever cpp_filled_by; pointer-less rows whose text is old's ORION code
// only while that code is unique to old) onto newSpinID inside tx, overwrites the
// fill-group children from the new spin, and writes one mst_mb_spin_supersede_item row per
// changed cpp row under batchID. Locked products are skipped and reported. The MB_SP_CODE
// rows are locked FOR UPDATE. Written values carry cpp_filled_by = mbsource.FilledBySupersede;
// the previous filled_by / text / numeric / pointer go to the audit row.
//
// mssi value convention: text values in mssi_*_value_text, numeric children in
// mssi_*_value_numeric (text NULL). A child row inserted by the repoint has all old_* NULL.
func repointSupersededParamsTx(ctx context.Context, tx *sql.Tx, batchID uuid.UUID, old supersedeSpin, newSpinID uuid.UUID, actor string) (SupersedeRepointResult, error) {
	var out SupersedeRepointResult
	rows, err := listSupersedeParamRowsOn(ctx, tx, old, true)
	if err != nil {
		return out, err
	}
	if rows.TextShared {
		out.Warnings = append(out.Warnings, fmt.Sprintf(
			"%d text-only MB_SP_CODE row(s) not repointed: ORION code %s is shared by more than one spin",
			rows.SharedTextRows, old.SpinCode))
	}
	targets := append(append([]supersedeParamRow{}, rows.Pointer...), rows.TextOnly...)
	if len(targets) == 0 {
		return out, nil
	}
	ps, children, err := loadMBSourceParamsOn(ctx, tx)
	if err != nil {
		return out, err
	}
	newSpin, err := loadSpinForRepointOn(ctx, tx, newSpinID)
	if err != nil {
		return out, err
	}
	res := mbsource.SpinResolution(newSpin, children)
	audit := supersedeAudit{batchID: batchID, oldSpinID: old.SpinID, newSpinID: newSpinID, actor: actor}
	seen := map[int64]bool{}
	for _, r := range targets {
		if r.IsLocked {
			out.Warnings = append(out.Warnings, fmt.Sprintf("product %s is locked: MB_SP_CODE not repointed", r.ProductCode))
			continue
		}
		wr, err := forceWriteMBSourceTx(ctx, tx, ps, r.ProductSysID, res, r.Kind, mbsource.FilledBySupersede, actor)
		if err != nil {
			return out, fmt.Errorf("repoint product %s: %w", r.ProductCode, err)
		}
		if err := audit.insert(ctx, tx, wr.Changes); err != nil {
			return out, err
		}
		out.Rows += len(wr.Changes)
		if len(wr.KeptChildren) > 0 {
			out.Warnings = append(out.Warnings, fmt.Sprintf("product %s: %s kept old value (new spin has none)",
				r.ProductCode, strings.Join(wr.KeptChildren, ", ")))
		}
		if !seen[r.ProductSysID] {
			seen[r.ProductSysID] = true
			out.Products = append(out.Products, r.ProductSysID)
		}
	}
	sort.Slice(out.Products, func(i, j int) bool { return out.Products[i] < out.Products[j] })
	return out, nil
}

// supersedeAudit writes mst_mb_spin_supersede_item rows for one (batch, old, new) triple.
type supersedeAudit struct {
	batchID, oldSpinID, newSpinID uuid.UUID
	actor                         string
}

const supersedeAuditInsertSQL = `INSERT INTO mst_mb_spin_supersede_item
	(mssi_batch_id, mssi_old_spin_id, mssi_new_spin_id, mssi_cpp_value_id, mssi_product_sys_id,
	 mssi_old_value_text, mssi_old_filled_by, mssi_new_value_text,
	 mssi_param_code, mssi_old_mb_spin_id, mssi_old_value_numeric, mssi_new_value_numeric, mssi_repoint_kind,
	 created_by)
	VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)`

func (a supersedeAudit) insert(ctx context.Context, tx *sql.Tx, changes []forceChange) error {
	for _, c := range changes {
		var oldFilledBy *string
		if c.OldExisted {
			fb := c.Old.FilledBy
			oldFilledBy = &fb
		}
		if _, err := tx.ExecContext(ctx, supersedeAuditInsertSQL,
			a.batchID, a.oldSpinID, a.newSpinID, c.CppValueID, c.ProductSysID,
			c.Old.Text, oldFilledBy, c.NewText,
			c.ParamCode, c.Old.SpinID, c.Old.Num, c.NewNum, c.Kind,
			a.actor); err != nil {
			return fmt.Errorf("insert supersede item %s: %w", c.ParamCode, err)
		}
	}
	return nil
}

// loadMBSourceParamsOn reads the MB_SP_CODE trigger and its active fill-group children
// (mst_parameter.lookup_fill_group_code) through q, independent of any app service.
func loadMBSourceParamsOn(ctx context.Context, q supersedeQuerier) (mbsourceautofill.ParamSet, []*parameter.Parameter, error) {
	var ps mbsourceautofill.ParamSet
	err := q.QueryRowContext(ctx, `SELECT id FROM mst_parameter WHERE param_code = $1 AND deleted_at IS NULL`,
		mbsource.ParamSPCode).Scan(&ps.TriggerID)
	if err != nil {
		return ps, nil, fmt.Errorf("get %s param: %w", mbsource.ParamSPCode, err)
	}
	rows, err := q.QueryContext(ctx, selectWithUOMJoin()+`
		WHERE p.lookup_fill_group_code = $1 AND p.deleted_at IS NULL AND p.is_active = TRUE
		ORDER BY p.display_order, p.param_code`, mbsource.FillGroupSPCode)
	if err != nil {
		return ps, nil, fmt.Errorf("get %s fill group: %w", mbsource.FillGroupSPCode, err)
	}
	defer closeRows(rows)
	var (
		scanner  ParameterRepository
		children []*parameter.Parameter
	)
	ps.Children = map[string]uuid.UUID{}
	for rows.Next() {
		p, err := scanner.scanParameterFromRows(rows)
		if err != nil {
			return ps, nil, err
		}
		children = append(children, p)
		ps.Children[p.Code().String()] = p.ID()
	}
	return ps, children, rows.Err()
}

// loadSpinForRepointOn reads a non-deleted spin through q.
func loadSpinForRepointOn(ctx context.Context, q supersedeQuerier, id uuid.UUID) (*mbspin.Entity, error) {
	var repo MBSpinRepository
	spin, err := repo.scanOne(q.QueryRowContext(ctx, repo.selectCols()+` WHERE mbs_id = $1 AND deleted_at IS NULL`, id))
	if err != nil {
		return nil, fmt.Errorf("load new spin %s: %w", id, err)
	}
	return spin, nil
}
