package etl

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/rs/zerolog/log"
)

// PruneOptions configures PruneExcluded. Apply=false is a dry run (report only, nothing is deleted).
type PruneOptions struct {
	CompanyID int16
	Apply     bool
}

// PrunedRun is one LEGACY run (and its period) that falls outside the D-35.1 scope.
type PrunedRun struct {
	RunID    int64
	YYMM     string
	Division string
	Heads    int
	Lines    int
}

// PruneExcluded removes LEGACY runs of periods that salary-legacy now SKIPs (D-35.1), left over from loads made before
// that decision. It needs the table-owner DSN: payroll_etl has no DELETE and fn_guard_closed forbids touching CLOSED
// periods, so the period is reopened first. Everything runs in ONE transaction; a period that holds any non-LEGACY run
// aborts the whole prune. Without Apply it only reports what would be deleted.
func PruneExcluded(ctx context.Context, db *sql.DB, o PruneOptions) ([]PrunedRun, error) {
	conn, err := db.Conn(ctx)
	if err != nil {
		return nil, err
	}
	defer closeQuiet(conn)
	var out []PrunedRun
	err = conn.Raw(func(dc any) error {
		sc, ok := dc.(*stdlib.Conn)
		if !ok {
			return fmt.Errorf("driver bukan pgx")
		}
		tx, err := sc.Conn().Begin(ctx)
		if err != nil {
			return err
		}
		defer func() {
			if rbErr := tx.Rollback(ctx); rbErr != nil && !errors.Is(rbErr, pgx.ErrTxClosed) {
				log.Warn().Err(rbErr).Msg("etl: rollback failed")
			}
		}()
		out, err = pruneTx(ctx, tx, o)
		if err != nil {
			return err
		}
		return tx.Commit(ctx)
	})
	return out, err
}

func pruneTx(ctx context.Context, tx pgx.Tx, o PruneOptions) ([]PrunedRun, error) {
	rows, err := tx.Query(ctx, `SELECT r.pr_id, r.pr_yymm, r.pr_division, r.pr_kind,
  (SELECT count(*) FROM payroll.pyr_salary_head h WHERE h.psh_run_id = r.pr_id),
  (SELECT count(*) FROM payroll.pyr_salary_detail d WHERE d.psd_run_id = r.pr_id)
FROM payroll.pyr_run r WHERE r.pr_company_id = $1 ORDER BY r.pr_yymm, r.pr_division, r.pr_id`, o.CompanyID)
	if err != nil {
		return nil, err
	}
	type cand struct {
		PrunedRun
		kind string
	}
	var all []cand
	for rows.Next() {
		var c cand
		var yymm int
		if err := rows.Scan(&c.RunID, &yymm, &c.Division, &c.kind, &c.Heads, &c.Lines); err != nil {
			rows.Close()
			return nil, err
		}
		c.YYMM = strconv.Itoa(yymm)
		all = append(all, c)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make([]PrunedRun, 0, len(all))
	periods := map[snapKey]bool{}
	for _, c := range all {
		k := snapKey{c.YYMM, c.Division}
		if !legacySkipped(k) {
			continue
		}
		if c.kind != "LEGACY" {
			return nil, fmt.Errorf("periode %s/%s punya run %d bukan LEGACY (%s); prune dibatalkan", c.YYMM, c.Division, c.RunID, c.kind)
		}
		out = append(out, c.PrunedRun)
		periods[k] = true
	}
	if !o.Apply {
		return out, nil
	}
	return out, deleteRuns(ctx, tx, o, out, periods)
}

func deleteRuns(ctx context.Context, tx pgx.Tx, o PruneOptions, out []PrunedRun, periods map[snapKey]bool) error {
	for k := range periods {
		if _, err := tx.Exec(ctx, `UPDATE payroll.pyr_period SET pp_status='OPEN', pp_active_run_id=NULL, closed_by=NULL, closed_at=NULL
WHERE pp_company_id=$1 AND pp_yymm=$2 AND pp_division=$3`, o.CompanyID, k.yymm, k.division); err != nil {
			return fmt.Errorf("buka periode %s/%s: %w", k.yymm, k.division, err)
		}
	}
	for _, r := range out {
		if _, err := tx.Exec(ctx, `DELETE FROM payroll.pyr_run WHERE pr_id=$1 AND pr_kind='LEGACY'`, r.RunID); err != nil {
			return fmt.Errorf("hapus run %d: %w", r.RunID, err)
		}
		if _, err := tx.Exec(ctx, `INSERT INTO payroll.pyr_audit_log (pal_actor, pal_action, pal_entity, pal_entity_key, pal_division, pal_after, pal_company_id)
VALUES ('etl','ETL_PRUNE_EXCLUDED','pyr_run',$1,$2,jsonb_build_object('run_id',$3::bigint,'heads',$4::int,'lines',$5::int),$6)`,
			r.YYMM+"/"+r.Division, r.Division, r.RunID, r.Heads, r.Lines, o.CompanyID); err != nil {
			return fmt.Errorf("audit: %w", err)
		}
	}
	for k := range periods {
		if _, err := tx.Exec(ctx, `DELETE FROM payroll.pyr_period WHERE pp_company_id=$1 AND pp_yymm=$2 AND pp_division=$3`,
			o.CompanyID, k.yymm, k.division); err != nil {
			return fmt.Errorf("hapus periode %s/%s: %w", k.yymm, k.division, err)
		}
	}
	return nil
}
