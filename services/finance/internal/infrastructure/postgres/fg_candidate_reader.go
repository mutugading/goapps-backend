package postgres

import (
	"context"
	"fmt"

	"github.com/lib/pq"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbsource"
)

// FgCandidateReader implements mbsource.FgCandidateFinder over cost_product_master and the
// route DAG (cost_route_head / cost_route_seq / cost_route_rm).
type FgCandidateReader struct {
	db *DB
}

// NewFgCandidateReader creates an FgCandidateReader.
func NewFgCandidateReader(db *DB) *FgCandidateReader { return &FgCandidateReader{db: db} }

var _ mbsource.FgCandidateFinder = (*FgCandidateReader)(nil)

// fgCandidatesSQL lists (product, candidate, ordinal) rows for the products in $1:
//   - a POY product is its own single candidate (ordinal 0);
//   - otherwise the POY RM PRODUCT lines of the product's route head: the live non-LOCKED
//     head (at most one, uk_cost_route_head_active_per_product), else the LOCKED head with
//     the highest version. Live seqs only; ordered by route level, route seq, RM id.
//
// Duplicate candidates are removed in Go keeping the first occurrence.
const fgCandidatesSQL = `
	WITH tgt AS (
		SELECT pm.cpm_product_sys_id AS pid, (pt.cpt_type_code = 'POY') AS is_poy
		FROM cost_product_master pm
		LEFT JOIN cost_product_type pt ON pt.cpt_type_id = pm.cpm_product_type_id
		WHERE pm.cpm_product_sys_id = ANY($1)
	),
	head AS (
		SELECT DISTINCT ON (crh.crh_product_sys_id) crh.crh_product_sys_id AS pid, crh.crh_head_id
		FROM cost_route_head crh
		JOIN tgt ON tgt.pid = crh.crh_product_sys_id AND NOT COALESCE(tgt.is_poy, FALSE)
		WHERE crh.crh_deleted_at IS NULL
		ORDER BY crh.crh_product_sys_id,
		         CASE WHEN crh.crh_routing_status <> 'LOCKED' THEN 0 ELSE 1 END,
		         crh.crh_version DESC, crh.crh_head_id DESC
	)
	SELECT tgt.pid, tgt.pid, 0, 0, 0
	FROM tgt WHERE COALESCE(tgt.is_poy, FALSE)
	UNION ALL
	SELECT head.pid, crm.crm_rm_product_sys_id, crs.crs_route_level, crs.crs_route_seq, crm.crm_rm_id
	FROM head
	JOIN cost_route_seq crs ON crs.crs_head_id = head.crh_head_id AND crs.crs_deleted_at IS NULL
	JOIN cost_route_rm crm ON crm.crm_seq_id = crs.crs_seq_id
	                      AND crm.crm_rm_type = 'PRODUCT' AND crm.crm_rm_product_sys_id IS NOT NULL
	JOIN cost_product_master rpm ON rpm.cpm_product_sys_id = crm.crm_rm_product_sys_id
	JOIN cost_product_type rpt ON rpt.cpt_type_id = rpm.cpm_product_type_id AND rpt.cpt_type_code = 'POY'
	ORDER BY 1, 3, 4, 5`

// FgCandidates implements mbsource.FgCandidateFinder in one batched query. Empty input runs
// no query; products without any candidate are absent from the result.
func (r *FgCandidateReader) FgCandidates(ctx context.Context, productSysIDs []int64) (map[int64][]int64, error) {
	out := make(map[int64][]int64, len(productSysIDs))
	if len(productSysIDs) == 0 {
		return out, nil
	}
	rows, err := r.db.QueryContext(ctx, fgCandidatesSQL, pq.Array(productSysIDs))
	if err != nil {
		return nil, fmt.Errorf("list fg candidates: %w", err)
	}
	defer closeRows(rows)
	seen := make(map[[2]int64]struct{})
	for rows.Next() {
		var pid, cand, level, seq, rmID int64
		if err := rows.Scan(&pid, &cand, &level, &seq, &rmID); err != nil {
			return nil, fmt.Errorf("scan fg candidate: %w", err)
		}
		k := [2]int64{pid, cand}
		if _, dup := seen[k]; dup {
			continue
		}
		seen[k] = struct{}{}
		out[pid] = append(out[pid], cand)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate fg candidates: %w", err)
	}
	return out, nil
}
