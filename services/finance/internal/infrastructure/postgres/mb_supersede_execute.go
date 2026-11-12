package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/google/uuid"
	"github.com/lib/pq"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbhead"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbspin"
)

// supersedeWorkflowLogToState is the mbwl_to_state of the supersede audit row. Like the
// child-recalc row (recalcWorkflowLogToState) a supersede is NOT an entry-status change:
// the from-state carries the head's current status and the to-state names the event.
// ⚠ Writing from=to=<status> instead would break heads parked in UNLOCK_REQUESTED: the
// PreUnlockStatus subquery (mb_head_repository selectCols, mb_head_relock) reads the latest
// row with to_state='UNLOCK_REQUESTED' and would then restore UNLOCK_REQUESTED itself.
const supersedeWorkflowLogToState = "SUPERSEDE"

// mbAutoGenSupersedeCandidates locks (FOR UPDATE) and returns the live spins of other
// heads that the head's new root spin would collide with. A head without FG or shade
// never conflicts (pre-FG behavior).
func mbAutoGenSupersedeCandidates(ctx context.Context, tx *sql.Tx, headID uuid.UUID, entity *mbhead.Entity) ([]supersedeSpin, error) {
	fg := entity.FgProductID()
	if fg == nil || *fg <= 0 {
		return nil, nil
	}
	return findSupersedeCandidatesOn(ctx, tx, headID, entity.ShadeCode(), *fg, true)
}

// supersedeOnAutoGenTx runs design §2.5 inside the Validate tx, in the order the unique
// index uix_mbs_shade_fg_live requires: deactivate S_old → insert S_new → link
// S_old.mbs_superseded_by_spin_id → repoint params (one batch) → deactivate orphaned old
// heads / old MB products → workflow log rows (new head + each old head). No hard delete.
func supersedeOnAutoGenTx(ctx context.Context, tx *sql.Tx, newHeadID uuid.UUID, newSpin *mbspin.Entity, cands []supersedeSpin, actor string) (*mbhead.SupersedeOutcome, error) {
	out := &mbhead.SupersedeOutcome{BatchID: uuid.New(), NewSpinID: newSpin.ID()}
	oldIDs := make([]string, 0, len(cands))
	for _, c := range cands {
		out.OldSpinIDs = append(out.OldSpinIDs, c.SpinID)
		oldIDs = append(oldIDs, c.SpinID.String())
	}
	if err := supersedeDeactivateOldSpinsTx(ctx, tx, oldIDs, actor); err != nil {
		return nil, err
	}
	if err := createSpinOn(ctx, tx, newSpin); err != nil {
		return nil, fmt.Errorf("mb_autogen: insert superseding mb spin: %w", err)
	}
	if err := supersedeLinkOldSpinsTx(ctx, tx, oldIDs, newSpin.ID()); err != nil {
		return nil, err
	}
	if err := supersedeRepointAllTx(ctx, tx, out, cands, actor); err != nil {
		return nil, err
	}
	if err := supersedeDeactivateOldHeadsTx(ctx, tx, out, cands, newHeadID, oldIDs, actor); err != nil {
		return nil, err
	}
	if err := supersedeDeactivateOldProductsTx(ctx, tx, out, cands, oldIDs, actor); err != nil {
		return nil, err
	}
	if err := supersedeWriteWorkflowLogsTx(ctx, tx, out, cands, newHeadID, actor); err != nil {
		return nil, err
	}
	return out, nil
}

// supersedeDeactivateOldSpinsTx is step 1: S_old leaves the live set of the unique index.
func supersedeDeactivateOldSpinsTx(ctx context.Context, tx *sql.Tx, oldIDs []string, actor string) error {
	if _, err := tx.ExecContext(ctx, `
		UPDATE mst_mb_spin
		SET mbs_is_active = FALSE, mbs_superseded_at = NOW(), mbs_superseded_by = $2,
		    updated_at = NOW(), updated_by = $2
		WHERE mbs_id = ANY(CAST($1 AS uuid[])) AND deleted_at IS NULL`, pq.Array(oldIDs), actor); err != nil {
		return fmt.Errorf("supersede: deactivate old spins: %w", err)
	}
	return nil
}

// supersedeLinkOldSpinsTx is step 3 (after S_new exists, FK mbs_superseded_by_spin_id).
func supersedeLinkOldSpinsTx(ctx context.Context, tx *sql.Tx, oldIDs []string, newSpinID uuid.UUID) error {
	if _, err := tx.ExecContext(ctx, `
		UPDATE mst_mb_spin SET mbs_superseded_by_spin_id = $2
		WHERE mbs_id = ANY(CAST($1 AS uuid[])) AND deleted_at IS NULL`, pq.Array(oldIDs), newSpinID); err != nil {
		return fmt.Errorf("supersede: link old spins to new spin: %w", err)
	}
	return nil
}

// supersedeRepointAllTx is step 4: repoint every candidate's MB_SP_CODE rows (one batch).
func supersedeRepointAllTx(ctx context.Context, tx *sql.Tx, out *mbhead.SupersedeOutcome, cands []supersedeSpin, actor string) error {
	seen := map[int64]bool{}
	for _, c := range cands {
		res, err := repointSupersededParamsTx(ctx, tx, out.BatchID, c, out.NewSpinID, actor)
		if err != nil {
			return err
		}
		out.RepointedRows += res.Rows
		out.Warnings = append(out.Warnings, res.Warnings...)
		for _, p := range res.Products {
			if !seen[p] {
				seen[p] = true
				out.AffectedProductIDs = append(out.AffectedProductIDs, p)
			}
		}
	}
	sort.Slice(out.AffectedProductIDs, func(i, j int) bool { return out.AffectedProductIDs[i] < out.AffectedProductIDs[j] })
	return nil
}

// supersedeDeactivateOldHeadsTx is step 5: an old head left without any live active spin is
// deactivated and marked superseded by the new head. Its workflow status is NOT changed.
func supersedeDeactivateOldHeadsTx(ctx context.Context, tx *sql.Tx, out *mbhead.SupersedeOutcome, cands []supersedeSpin, newHeadID uuid.UUID, oldIDs []string, actor string) error {
	done := map[uuid.UUID]bool{}
	for _, c := range cands {
		if done[c.HeadID] {
			continue
		}
		done[c.HeadID] = true
		will, err := supersedeHeadWillDeactivateOn(ctx, tx, c.HeadID, oldIDs)
		if err != nil {
			return err
		}
		if !will {
			continue
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE mst_mb_head
			SET mbh_is_active = FALSE, mbh_superseded_by_head_id = $2, mbh_superseded_at = NOW(),
			    mbh_superseded_by = $3, updated_at = NOW(), updated_by = $3
			WHERE mbh_id = $1 AND deleted_at IS NULL`, c.HeadID, newHeadID, actor); err != nil {
			return fmt.Errorf("supersede: deactivate old head %s: %w", c.HeadCode, err)
		}
		out.DeactivatedHeadIDs = append(out.DeactivatedHeadIDs, c.HeadID)
	}
	return nil
}

// supersedeDeactivateOldProductsTx is step 6: an old MB product no live active spin points
// at any more gets cpm_is_active=false (no delete).
func supersedeDeactivateOldProductsTx(ctx context.Context, tx *sql.Tx, out *mbhead.SupersedeOutcome, cands []supersedeSpin, oldIDs []string, actor string) error {
	done := map[int64]bool{}
	for _, c := range cands {
		if c.MBProductID <= 0 || done[c.MBProductID] {
			continue
		}
		done[c.MBProductID] = true
		will, err := supersedeProductWillDeactivateOn(ctx, tx, c.MBProductID, oldIDs)
		if err != nil {
			return err
		}
		if !will {
			continue
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE cost_product_master
			SET cpm_is_active = FALSE, cpm_updated_at = NOW(), cpm_updated_by = $2
			WHERE cpm_product_sys_id = $1`, c.MBProductID, actor); err != nil {
			return fmt.Errorf("supersede: deactivate old mb product %d: %w", c.MBProductID, err)
		}
		out.DeactivatedProductIDs = append(out.DeactivatedProductIDs, c.MBProductID)
	}
	return nil
}

// supersedeLogMeta is the mbwl_meta document of a supersede workflow log row.
type supersedeLogMeta struct {
	Event            string      `json:"event"`
	Role             string      `json:"role"`
	BatchID          uuid.UUID   `json:"batch_id"`
	NewHeadID        uuid.UUID   `json:"new_head_id"`
	NewSpinID        uuid.UUID   `json:"new_spin_id"`
	OldSpinIDs       []uuid.UUID `json:"old_spin_ids"`
	HeadDeactivated  bool        `json:"head_deactivated"`
	AffectedProducts []int64     `json:"affected_product_ids"`
	RepointedRows    int         `json:"repointed_rows"`
	// DeactivatedProductIDs / DeactivatedHeadIDs (OLD rows only) record exactly what this
	// supersede deactivated for that old head, so un-supersede restores only those.
	DeactivatedProductIDs []int64     `json:"deactivated_product_ids,omitempty"`
	DeactivatedHeadIDs    []uuid.UUID `json:"deactivated_head_ids,omitempty"`
}

// supersedeWriteWorkflowLogsTx is step 7: one mst_mb_workflow_log row for the new head and
// one per old head (from_state = current status, to_state = SUPERSEDE, version unchanged).
func supersedeWriteWorkflowLogsTx(ctx context.Context, tx *sql.Tx, out *mbhead.SupersedeOutcome, cands []supersedeSpin, newHeadID uuid.UUID, actor string) error {
	meta := supersedeLogMeta{
		Event: supersedeWorkflowLogToState, Role: "NEW", BatchID: out.BatchID, NewHeadID: newHeadID,
		NewSpinID: out.NewSpinID, OldSpinIDs: out.OldSpinIDs,
		AffectedProducts: out.AffectedProductIDs, RepointedRows: out.RepointedRows,
	}
	if err := insertSupersedeLogTx(ctx, tx, newHeadID, actor, "SUPERSEDE: superseded live spin(s) of other recipe(s) with the same shade and FG", meta); err != nil {
		return err
	}
	deactivated := map[uuid.UUID]bool{}
	for _, id := range out.DeactivatedHeadIDs {
		deactivated[id] = true
	}
	for _, headID := range supersedeOldHeadIDs(cands) {
		m := meta
		m.Role = "OLD"
		m.HeadDeactivated = deactivated[headID]
		m.DeactivatedHeadIDs, m.DeactivatedProductIDs = nil, nil
		if m.HeadDeactivated {
			m.DeactivatedHeadIDs = []uuid.UUID{headID}
		}
		m.DeactivatedProductIDs = deactivatedProductsOfHead(cands, headID, out.DeactivatedProductIDs)
		if err := insertSupersedeLogTx(ctx, tx, headID, actor, "SUPERSEDE: spin superseded by a recipe with the same shade and FG", m); err != nil {
			return err
		}
	}
	return nil
}

// deactivatedProductsOfHead returns the deactivated MB products that belong to headID's candidates.
func deactivatedProductsOfHead(cands []supersedeSpin, headID uuid.UUID, deactivated []int64) []int64 {
	set := map[int64]bool{}
	for _, id := range deactivated {
		set[id] = true
	}
	var out []int64
	for _, c := range cands {
		if c.HeadID == headID && set[c.MBProductID] {
			out = append(out, c.MBProductID)
			set[c.MBProductID] = false
		}
	}
	return out
}

// supersedeOldHeadIDs returns the distinct heads of the superseded spins (candidate order).
func supersedeOldHeadIDs(cands []supersedeSpin) []uuid.UUID {
	seen := map[uuid.UUID]bool{}
	out := make([]uuid.UUID, 0, len(cands))
	for _, c := range cands {
		if !seen[c.HeadID] {
			seen[c.HeadID] = true
			out = append(out, c.HeadID)
		}
	}
	return out
}

func insertSupersedeLogTx(ctx context.Context, tx *sql.Tx, headID uuid.UUID, actor, reason string, meta supersedeLogMeta) error {
	return insertSupersedeEventLogTx(ctx, tx, headID, actor, reason, supersedeWorkflowLogToState, meta)
}

// insertSupersedeEventLogTx writes the audit row with an explicit to_state (SUPERSEDE or
// UNSUPERSEDE); from_state is the head's current status and the version is unchanged.
func insertSupersedeEventLogTx(ctx context.Context, tx *sql.Tx, headID uuid.UUID, actor, reason, toState string, meta any) error {
	var (
		status  string
		version int32
	)
	if err := tx.QueryRowContext(ctx,
		`SELECT COALESCE(mbh_entry_status, ''), COALESCE(mbh_current_version, 0) FROM mst_mb_head WHERE mbh_id = $1`,
		headID).Scan(&status, &version); err != nil {
		return fmt.Errorf("supersede: read head %s status: %w", headID, err)
	}
	doc, err := json.Marshal(meta)
	if err != nil {
		return fmt.Errorf("supersede: marshal workflow log meta: %w", err)
	}
	if _, err := tx.ExecContext(ctx, insertWorkflowLogQuery,
		headID, status, toState, actor, reason, version, string(doc)); err != nil {
		return fmt.Errorf("supersede: insert workflow log for head %s: %w", headID, err)
	}
	return nil
}
