// Command backfill-mb-validate drives the real Submit→Approve→Validate (or
// direct Validate for boughtout) workflow on seeded DRAFT MB Heads that are
// VALIDATED-candidates from the Oracle 202606 composition backfill. It reuses
// the same application-layer handlers the gRPC VALIDATE button calls, so the
// resulting auto-gen chain (cost_product_master → cost_route_head → …) is
// byte-for-byte identical to a manual VALIDATE.
package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/lib/pq"

	"github.com/mutugading/goapps-backend/services/finance/internal/application/mbhead"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/config"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/postgres"
)

const actor = "backfill-mb-validate"

func main() {
	if err := run(); err != nil {
		log.Fatalf("backfill-mb-validate: %v", err)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	db, err := postgres.NewConnection(&cfg.Database)
	if err != nil {
		return fmt.Errorf("connect db: %w", err)
	}
	defer logClose("database", db)

	compositionRepo := postgres.NewMBCompositionRepository(db)
	headRepo := postgres.NewMBHeadRepository(db, compositionRepo)
	paramRepo := postgres.NewMBParamRepository(db)

	submitH := mbhead.NewSubmitHandler(headRepo)
	approveH := mbhead.NewApproveHandler(headRepo)
	validateH := mbhead.NewValidateHandler(headRepo, paramRepo)

	ctx := context.Background()

	candidates, err := listCandidates(ctx, db.DB)
	if err != nil {
		return fmt.Errorf("list candidates: %w", err)
	}
	log.Printf("found %d VALIDATED-candidate heads (DRAFT with ≥1 composition row)", len(candidates))
	if len(candidates) == 0 {
		return nil
	}

	edges, err := listMBEdges(ctx, db.DB, candidates)
	if err != nil {
		return fmt.Errorf("list mb edges: %w", err)
	}
	ordered, err := topoSort(candidates, edges)
	if err != nil {
		return fmt.Errorf("topo sort: %w", err)
	}
	log.Printf("topo-sorted %d candidates (%d MB-to-MB edges)", len(ordered), len(edges))

	var stats struct {
		validated, skipped, errored int
	}
	start := time.Now()

	for i, c := range ordered {
		if c.CostProductID != 0 {
			stats.skipped++
			continue
		}

		logPrefix := fmt.Sprintf("[%d/%d] %s (%s)", i+1, len(ordered), c.Code, c.MBHID)

		if err := driveValidate(ctx, c, submitH, approveH, validateH); err != nil {
			log.Printf("%s ERROR: %v", logPrefix, err)
			stats.errored++
			continue
		}

		stats.validated++
		if stats.validated%50 == 0 {
			log.Printf("  progress: %d validated, %d skipped, %d errors (%.1fs)",
				stats.validated, stats.skipped, stats.errored, time.Since(start).Seconds())
		}
	}

	log.Printf("done in %.1fs — validated=%d skipped=%d errors=%d total=%d",
		time.Since(start).Seconds(), stats.validated, stats.skipped, stats.errored, len(ordered))

	if stats.errored > 0 {
		return fmt.Errorf("%d heads failed — check logs above", stats.errored)
	}
	return nil
}

type candidate struct {
	MBHID         string
	Code          string
	IsBoughtout   bool
	CostProductID int64
}

func listCandidates(ctx context.Context, db *sql.DB) ([]candidate, error) {
	const q = `
		SELECT h.mbh_id::text, h.mbh_mb_costing, h.mbh_is_boughtout, COALESCE(h.mbh_cost_product_id, 0)
		FROM mst_mb_head h
		WHERE h.mbh_entry_status = 'DRAFT'
		  AND h.deleted_at IS NULL
		  AND h.mbh_oracle_sys_id IS NOT NULL
		  AND EXISTS (SELECT 1 FROM mst_mb_composition c
		              WHERE c.mbcm_mbh_id = h.mbh_id AND c.deleted_at IS NULL)
		ORDER BY h.mbh_mb_costing`
	rows, err := db.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer logCloseRows(rows)

	var out []candidate
	for rows.Next() {
		var c candidate
		if err := rows.Scan(&c.MBHID, &c.Code, &c.IsBoughtout, &c.CostProductID); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

type mbEdge struct {
	MBHID, RefMBHID string
}

func listMBEdges(ctx context.Context, db *sql.DB, candidates []candidate) ([]mbEdge, error) {
	if len(candidates) == 0 {
		return nil, nil
	}
	// MB-to-MB composition edges from the live table — these heads are still DRAFT
	// (no version snapshot yet).
	const q = `
		SELECT c.mbcm_mbh_id::text, c.mbcm_mb_ref_mbh_id::text
		FROM mst_mb_composition c
		WHERE c.mbcm_source_type = 'MB'
		  AND c.mbcm_mb_ref_mbh_id IS NOT NULL
		  AND c.deleted_at IS NULL
		  AND c.mbcm_mbh_id = ANY($1::uuid[])`
	ids := make([]string, len(candidates))
	for i, c := range candidates {
		ids[i] = c.MBHID
	}

	rows, err := db.QueryContext(ctx, q, pq.Array(ids))
	if err != nil {
		return nil, err
	}
	defer logCloseRows(rows)

	var out []mbEdge
	for rows.Next() {
		var e mbEdge
		if err := rows.Scan(&e.MBHID, &e.RefMBHID); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func topoSort(candidates []candidate, edges []mbEdge) ([]candidate, error) {
	byID := make(map[string]candidate, len(candidates))
	inDegree := make(map[string]int, len(candidates))
	adj := make(map[string][]string, len(candidates))
	for _, c := range candidates {
		byID[c.MBHID] = c
		inDegree[c.MBHID] = 0
	}
	seen := make(map[[2]string]struct{}, len(edges))
	for _, e := range edges {
		if _, ok := byID[e.RefMBHID]; !ok {
			continue
		}
		if e.MBHID == e.RefMBHID {
			continue
		}
		key := [2]string{e.MBHID, e.RefMBHID}
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		adj[e.RefMBHID] = append(adj[e.RefMBHID], e.MBHID)
		inDegree[e.MBHID]++
	}

	queue := make([]string, 0, len(candidates))
	for _, c := range candidates {
		if inDegree[c.MBHID] == 0 {
			queue = append(queue, c.MBHID)
		}
	}

	sorted := make([]candidate, 0, len(candidates))
	visited := 0
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		visited++
		sorted = append(sorted, byID[id])
		for _, next := range adj[id] {
			inDegree[next]--
			if inDegree[next] == 0 {
				queue = append(queue, next)
			}
		}
	}
	if visited != len(candidates) {
		return nil, fmt.Errorf("cycle detected in MB-to-MB composition graph")
	}
	return sorted, nil
}

func driveValidate(ctx context.Context, c candidate, submitH *mbhead.SubmitHandler, approveH *mbhead.ApproveHandler, validateH *mbhead.ValidateHandler) error {
	uid, err := uuid.Parse(c.MBHID)
	if err != nil {
		return fmt.Errorf("parse uuid %q: %w", c.MBHID, err)
	}

	if c.IsBoughtout {
		if _, err := validateH.Handle(ctx, mbhead.ValidateCommand{MbhID: uid, ActorUserID: actor}); err != nil {
			return fmt.Errorf("validate (boughtout): %w", err)
		}
		return nil
	}

	if _, err := submitH.Handle(ctx, mbhead.SubmitCommand{MbhID: uid, ActorUserID: actor}); err != nil {
		return fmt.Errorf("submit: %w", err)
	}
	if _, err := approveH.Handle(ctx, mbhead.ApproveCommand{MbhID: uid, ActorUserID: actor}); err != nil {
		return fmt.Errorf("approve: %w", err)
	}
	if _, err := validateH.Handle(ctx, mbhead.ValidateCommand{MbhID: uid, ActorUserID: actor}); err != nil {
		return fmt.Errorf("validate: %w", err)
	}
	return nil
}

type closer interface{ Close() error }

func logClose(name string, c closer) {
	if err := c.Close(); err != nil {
		log.Printf("WARN: failed to close %s: %v", name, err)
	}
}

func logCloseRows(rows *sql.Rows) {
	if err := rows.Close(); err != nil {
		log.Printf("WARN: failed to close rows: %v", err)
	}
}
