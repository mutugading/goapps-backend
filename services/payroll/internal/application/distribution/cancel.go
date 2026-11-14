package distribution

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/mutugading/goapps-backend/services/payroll/internal/application/report"
	"github.com/mutugading/goapps-backend/services/payroll/internal/domain/audit"
)

// Cancel ends a batch by hand (status CANCELLED, audited with the reason). Send and Resend refuse a CANCELLED batch, a
// running send stops at its next item, and a new batch may then be created for the same period. Items already SENT
// keep their history.
func (s *Service) Cancel(ctx context.Context, q report.Query, batchID int64, reason string) error {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return errors.New("cancel: alasan wajib (-reason)")
	}
	return s.DB.ScopedTx(ctx, func(tx *sql.Tx) error {
		var status string
		err := tx.QueryRowContext(ctx, `SELECT pdb_status FROM payroll.pyr_distribution_batch
WHERE pdb_id=$1 AND pdb_company_id=$2 AND pdb_division=$3 AND pdb_yymm=$4 FOR UPDATE`, batchID, q.CompanyID, q.Division, q.YYMM).Scan(&status)
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("batch %d tidak ada (atau di luar scope)", batchID)
		}
		if err != nil {
			return fmt.Errorf("read batch: %w", err)
		}
		if status == StCancelled || status == StSuperseded {
			return fmt.Errorf("batch %d sudah berstatus %s", batchID, status)
		}
		var sent, pending int
		if err := tx.QueryRowContext(ctx, `SELECT count(*) FILTER (WHERE pdi_status='SENT'),
count(*) FILTER (WHERE pdi_status IN ('QUEUED','SENDING','FAILED')) FROM payroll.pyr_distribution_item WHERE pdi_batch_id=$1`, batchID).Scan(&sent, &pending); err != nil {
			return fmt.Errorf("count items: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `UPDATE payroll.pyr_distribution_batch SET pdb_status='CANCELLED',
pdb_finished_at=COALESCE(pdb_finished_at,$2) WHERE pdb_id=$1`, batchID, s.Now().UTC()); err != nil {
			return fmt.Errorf("cancel batch: %w", err)
		}
		return s.auditChange(ctx, tx, q.CompanyID, q.Division, "DISTRIBUTION_CANCEL", batchID,
			map[string]any{keyStatus: status}, map[string]any{keyStatus: StCancelled, "alasan": reason, "terkirim": sent, "tertunda": pending})
	})
}

// supersedeStale marks the batches of the same period that were built from another run as SUPERSEDED (the period was
// reopened and a new run is ACTIVE, 16§2.6). It runs when the next batch is created, the only moment the old one is
// in the way, so run commit stays unaware of distribution (T028g2). Returns the newest superseded batch id (0 = none).
func (s *Service) supersedeStale(ctx context.Context, tx *sql.Tx, q report.Query, activeRun int64) (int64, error) {
	rows, err := tx.QueryContext(ctx, `UPDATE payroll.pyr_distribution_batch SET pdb_status='SUPERSEDED', pdb_finished_at=COALESCE(pdb_finished_at,$5)
WHERE pdb_company_id=$1 AND pdb_division=$2 AND pdb_doc_type=$3 AND pdb_yymm=$4
  AND pdb_status NOT IN ('CANCELLED','SUPERSEDED') AND pdb_run_id IS NOT NULL AND pdb_run_id <> $6
RETURNING pdb_id, pdb_run_id`, q.CompanyID, q.Division, DocType, q.YYMM, s.Now().UTC(), activeRun)
	if err != nil {
		return 0, fmt.Errorf("supersede: %w", err)
	}
	type old struct{ id, run int64 }
	var olds []old
	for rows.Next() {
		var o old
		if err := rows.Scan(&o.id, &o.run); err != nil {
			closeRows(rows)
			return 0, fmt.Errorf("scan superseded: %w", err)
		}
		olds = append(olds, o)
	}
	closeRows(rows)
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("supersede: %w", err)
	}
	var newest int64
	for _, o := range olds {
		if err := s.auditChange(ctx, tx, q.CompanyID, q.Division, "DISTRIBUTION_SUPERSEDE", o.id,
			map[string]any{"run": o.run}, map[string]any{keyStatus: StSuperseded, "run_aktif": activeRun}); err != nil {
			return 0, err
		}
		newest = max(newest, o.id)
	}
	return newest, nil
}

func (s *Service) auditChange(ctx context.Context, tx *sql.Tx, company int16, division, action string, batchID int64, before, after map[string]any) error {
	return s.Audit.Write(ctx, tx, audit.Entry{CompanyID: company, Division: division, Action: action,
		Entity: auditEntity, EntityKey: fmt.Sprint(batchID), Before: before, After: after})
}

// keyStatus is the audit before/after map key for a distribution status.
const keyStatus = "status"
