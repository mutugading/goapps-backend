package costproductmaster

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	appshade "github.com/mutugading/goapps-backend/services/finance/internal/application/shade"
	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/costproductmaster"
	shadedomain "github.com/mutugading/goapps-backend/services/finance/internal/domain/shade"
)

// DefaultEtlApplyLimit is the default max rows per apply run.
const DefaultEtlApplyLimit = 200

// ProductEtlService previews and applies the Oracle merge-item -> Product Master ETL.
// Insert-only: existing products are never modified.
type ProductEtlService struct {
	src      domain.EtlSource // may be nil (Oracle not configured)
	repo     domain.EtlRepository
	shades   shadedomain.Repository
	autofill MBSourceAutoFiller // may be nil
	limit    int
	now      func() time.Time
}

// NewProductEtlService builds the service. src and autofill may be nil; limit <= 0 uses the default.
func NewProductEtlService(src domain.EtlSource, repo domain.EtlRepository, shades shadedomain.Repository,
	autofill MBSourceAutoFiller, applyLimit int) *ProductEtlService {
	if applyLimit <= 0 {
		applyLimit = DefaultEtlApplyLimit
	}
	return &ProductEtlService{src: src, repo: repo, shades: shades, autofill: autofill, limit: applyLimit, now: time.Now}
}

// PreviewEtlCommand is the preview input.
type PreviewEtlCommand struct {
	FromDate        string
	Search          string
	IncludeExisting bool
	Actor           string
}

// EtlPreviewResult is the preview output. Counts cover ALL classified rows (before the
// include_existing / search filters); Rows is the filtered view.
type EtlPreviewResult struct {
	RunID      uuid.UUID
	FromDate   string
	Rows       []domain.EtlRunRow
	Counts     domain.EtlCounts
	ApplyLimit int
}

// ApplyEtlCommand is the apply input.
type ApplyEtlCommand struct {
	PreviewRunID uuid.UUID
	RowKeys      []string
	Actor        string
}

// EtlApplyResult is the apply output.
type EtlApplyResult struct {
	RunID    uuid.UUID
	Inserted int32
	Skipped  int32
	Failed   int32
	Rows     []domain.EtlInsertResult
}

// Preview reads Oracle (SELECT-only), classifies every (item, shade) and stores a PREVIEW run.
func (s *ProductEtlService) Preview(ctx context.Context, cmd PreviewEtlCommand) (EtlPreviewResult, error) {
	if s.src == nil {
		return EtlPreviewResult{}, domain.ErrEtlNotConfigured
	}
	from, err := time.Parse(time.DateOnly, strings.TrimSpace(cmd.FromDate))
	if err != nil || from.Format(time.DateOnly) > s.now().Format(time.DateOnly) {
		return EtlPreviewResult{}, domain.ErrEtlInvalidFromDate
	}
	srcRows, err := s.src.ListMergeItems(ctx, from)
	if err != nil {
		return EtlPreviewResult{}, fmt.Errorf("read oracle merge items: %w", err)
	}
	merged, skippedMB := etlDedupe(srcRows)
	rows, err := s.classifyAll(ctx, merged)
	if err != nil {
		return EtlPreviewResult{}, err
	}
	var counts domain.EtlCounts
	for _, r := range rows {
		etlCount(&counts, r.Status)
	}
	search := strings.TrimSpace(cmd.Search)
	runID, err := s.repo.SaveRun(ctx, domain.EtlRun{
		Type: domain.EtlRunPreview, FromDate: from, Search: search, SkippedMB: skippedMB,
		Counts: counts, Rows: rows, Status: domain.EtlRunCompleted, CreatedBy: cmd.Actor,
	})
	if err != nil {
		return EtlPreviewResult{}, fmt.Errorf("save preview run: %w", err)
	}
	log.Info().Str("run_id", runID.String()).Int("rows", len(rows)).Int("skipped_mb", skippedMB).Msg("product ETL preview")
	return EtlPreviewResult{
		RunID: runID, FromDate: from.Format(time.DateOnly), Counts: counts, ApplyLimit: s.limit,
		Rows: etlFilter(rows, search, cmd.IncludeExisting),
	}, nil
}

func etlFilter(rows []domain.EtlRunRow, search string, includeExisting bool) []domain.EtlRunRow {
	q := strings.ToLower(search)
	out := make([]domain.EtlRunRow, 0, len(rows))
	for _, r := range rows {
		if !includeExisting && etlIsExisting(r.Status) {
			continue
		}
		if etlMatchesSearch(r, q) {
			out = append(out, r)
		}
	}
	return out
}

func (s *ProductEtlService) classifyAll(ctx context.Context, merged []etlMerged) ([]domain.EtlRunRow, error) {
	ms, err := s.repo.MatchProducts(ctx, etlKeys(merged))
	if err != nil {
		return nil, fmt.Errorf("match products: %w", err)
	}
	types, err := s.repo.TypeIDsByCode(ctx)
	if err != nil {
		return nil, fmt.Errorf("load product types: %w", err)
	}
	rows := make([]domain.EtlRunRow, 0, len(merged))
	sibSet := map[int64]struct{}{}
	for _, m := range merged {
		r := etlClassify(m, ms, types)
		if r.Status == domain.EtlNewShade {
			for _, id := range r.MatchedIDs {
				sibSet[id] = struct{}{}
			}
		}
		rows = append(rows, r)
	}
	var sib map[int64]map[string]string
	if len(sibSet) > 0 {
		ids := make([]int64, 0, len(sibSet))
		for id := range sibSet {
			ids = append(ids, id)
		}
		if sib, err = s.repo.SiblingParams(ctx, ids, etlParamCodes); err != nil {
			return nil, fmt.Errorf("load sibling params: %w", err)
		}
	}
	for i := range rows {
		if rows[i].Selectable {
			etlProposeParams(&rows[i], merged[i].src, sib)
		}
	}
	return rows, nil
}

// Apply inserts the selected rows of a stored PREVIEW run. Oracle is never read here.
func (s *ProductEtlService) Apply(ctx context.Context, cmd ApplyEtlCommand) (EtlApplyResult, error) {
	keys := etlDedupeKeys(cmd.RowKeys)
	if len(keys) > s.limit {
		return EtlApplyResult{}, domain.ErrEtlApplyLimitExceeded
	}
	run, err := s.repo.GetRun(ctx, cmd.PreviewRunID)
	if err != nil {
		return EtlApplyResult{}, err
	}
	if run.Type != domain.EtlRunPreview {
		return EtlApplyResult{}, domain.ErrEtlRunNotPreview
	}
	byKey := make(map[string]domain.EtlRunRow, len(run.Rows))
	for _, r := range run.Rows {
		byKey[r.RowKey] = r
	}
	var inserts []domain.EtlInsert
	var skipped []domain.EtlInsertResult
	for _, k := range keys {
		r, ok := byKey[k]
		switch {
		case !ok:
			skipped = append(skipped, etlSkip(k, "row_key tidak ada di preview run"))
		case !r.Selectable:
			skipped = append(skipped, etlSkip(k, "baris tidak dapat dipilih ("+r.Status+")"))
		default:
			inserts = append(inserts, domain.EtlInsert{
				RowKey: k, ItemCode: r.ItemCode, ItemName: r.ItemName, ShadeCode: r.ShadeCode, ShadeName: r.ShadeName,
				TypeID: r.TypeID, TypeCode: r.TypeCode, Denier: r.Denier, Filament: r.Filament, ActDenier: r.ActDenier,
			})
		}
	}
	out, err := s.repo.InsertEtlProducts(ctx, domain.EtlInsertInput{
		PreviewRunID: run.ID, FromDate: run.FromDate, Items: inserts, Skipped: skipped, Actor: cmd.Actor,
	})
	if err != nil {
		s.recordFailed(ctx, run, cmd.Actor, err)
		return EtlApplyResult{}, fmt.Errorf("apply product ETL: %w", err)
	}
	res := EtlApplyResult{RunID: out.RunID, Rows: append(append([]domain.EtlInsertResult{}, skipped...), out.Results...)}
	s.afterCommit(ctx, &res, byKey, cmd.Actor)
	return res, nil
}

func etlSkip(key, msg string) domain.EtlInsertResult {
	return domain.EtlInsertResult{RowKey: key, Result: domain.EtlResultSkipped, Message: msg}
}

func etlDedupeKeys(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, k := range in {
		k = strings.TrimSpace(k)
		if k == "" {
			continue
		}
		if _, dup := seen[k]; dup {
			continue
		}
		seen[k] = struct{}{}
		out = append(out, k)
	}
	return out
}

// recordFailed stores a FAILED APPLY run outside the (rolled back) tx, best effort.
func (s *ProductEtlService) recordFailed(ctx context.Context, run domain.EtlRun, actor string, cause error) {
	pid := run.ID
	_, err := s.repo.SaveRun(ctx, domain.EtlRun{
		Type: domain.EtlRunApply, PreviewRunID: &pid, FromDate: run.FromDate,
		Status: domain.EtlRunFailed, Error: cause.Error(), CreatedBy: actor,
	})
	if err != nil {
		log.Warn().Err(err).Msg("product ETL: could not record failed apply run")
	}
}

// afterCommit tallies results, then ensures shade masters and runs the MB autofill for the
// inserted products only (both best effort).
func (s *ProductEtlService) afterCommit(ctx context.Context, res *EtlApplyResult, byKey map[string]domain.EtlRunRow, actor string) {
	var ids []int64
	for _, r := range res.Rows {
		switch r.Result {
		case domain.EtlResultInserted:
			res.Inserted++
			ids = append(ids, r.ProductSysID)
			row := byKey[r.RowKey]
			appshade.EnsureShadeMaster(ctx, s.shades, row.ShadeCode, row.ShadeName, actor)
		case domain.EtlResultFailed:
			res.Failed++
		default:
			res.Skipped++
		}
	}
	if s.autofill != nil && len(ids) > 0 {
		s.autofill.BestEffort(ctx, ids, actor)
	}
}
