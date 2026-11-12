package mbspin

import "sort"

// Backfill (design §6 + Revisi 3 "Backfill final"): links legacy live spins to
// their FG POY product. The SQL matcher (infrastructure) computes the per-spin
// counts exactly like docs/mb-spin-fg/sql/postgres/P-13-backfill-match-by-spec-prefix.sql;
// this file holds the pure classification so it is unit-testable without a DB.

// BackfillAction is the dry-run decision written in the CSV `action` column.
type BackfillAction string

// Backfill actions. Only APPLY rows are ever processed by --apply.
const (
	// BackfillActionApply: spec + shade match exactly one active POY.
	BackfillActionApply BackfillAction = "APPLY"
	// BackfillActionReview: spin has no spec, its shade matches exactly one POY (human review).
	BackfillActionReview BackfillAction = "REVIEW"
	// BackfillActionSkip: everything else (reason explains why).
	BackfillActionSkip BackfillAction = "SKIP"
)

// Backfill buckets — identical labels and order to the P-13 SQL CASE.
const (
	BackfillBucketNoShade            = "0_spin_tanpa_shade"
	BackfillBucketSpecShadeUnique    = "1_spec+shade_tepat_1"
	BackfillBucketSpecShadeAmbiguous = "2_spec+shade_ambigu"
	BackfillBucketShadeOnlyUnique    = "3_tanpa_spec_shade_tepat_1_POY"
	BackfillBucketShadeOnlyAmbiguous = "4_tanpa_spec_shade_ambigu"
	BackfillBucketNoPoyWithShade     = "5_tidak_ada_POY_dengan_shade_itu"
	BackfillBucketSpecNoMatch        = "6_spec_tidak_cocok"
)

// Backfill reasons (dry-run classification, overrides and apply results).
const (
	BackfillReasonNoShade              = "no_shade"
	BackfillReasonSpecShadeUnique      = "spec_shade_unique"
	BackfillReasonSpecShadeAmbiguous   = "spec_shade_ambiguous"
	BackfillReasonShadeOnlyUnique      = "shade_only_unique"
	BackfillReasonShadeOnlyAmbiguous   = "shade_only_ambiguous"
	BackfillReasonNoPoyWithShade       = "no_poy_with_shade"
	BackfillReasonSpecNoMatch          = "spec_no_match"
	BackfillReasonFgAlreadySet         = "fg_already_set"
	BackfillReasonConflictExistingLive = "conflict_existing_live"
	BackfillReasonConflictSameTarget   = "conflict_same_target"
	BackfillReasonSpinNotLive          = "spin_not_live"
	BackfillReasonAlreadyLinked        = "already_linked"
	BackfillReasonPoyInvalid           = "poy_invalid"
	BackfillReasonShadeMismatch        = "shade_mismatch"
	BackfillReasonConflictUnique       = "conflict_unique"
	BackfillReasonMalformedRow         = "malformed_row"
	BackfillReasonDBError              = "db_error"
)

// BackfillResult is the per-row outcome of --apply.
type BackfillResult string

// Apply results.
const (
	BackfillResultApplied BackfillResult = "APPLIED"
	BackfillResultSkipped BackfillResult = "SKIPPED"
	BackfillResultFailed  BackfillResult = "FAILED"
)

// BackfillApplyRequest is one reviewed APPLY row to write.
type BackfillApplyRequest struct {
	SpinID          string
	PoyProductSysID int64
	Actor           string
}

// BackfillApplyOutcome is the per-row result of --apply.
type BackfillApplyOutcome struct {
	Result      BackfillResult
	Reason      string
	HeadUpdated bool
	Message     string
}

// BackfillPoy is a candidate FG POY product (display + item code).
type BackfillPoy struct {
	ProductSysID int64
	ProductCode  string
	ProductName  string
	ErpItemCode  string
}

// BackfillCandidate is one live spin with the matcher counts computed in SQL.
// NormShade = UPPER(TRIM(mbs_shade_code)) (as in P-13 and uix_mbs_shade_fg_live);
// NormSpec = P-13 `nfp` (” when the legacy final product is empty/NULL).
type BackfillCandidate struct {
	SpinID        string
	HeadID        string
	IsRoot        bool
	OrionItemCode string
	MgtName       string
	ShadeCode     string
	FinalProduct  string
	NormShade     string
	NormSpec      string
	NSpecShade    int64
	NShadeOnly    int64
	// SpecShadePoy is set only when NSpecShade == 1; ShadeOnlyPoy only when NShadeOnly == 1.
	SpecShadePoy *BackfillPoy
	ShadeOnlyPoy *BackfillPoy
	// FgProductID is the spin's current FG (0 = none). Superseded marks
	// mbs_superseded_by_spin_id IS NOT NULL (not live for the unique index).
	FgProductID int64
	Superseded  bool
}

// BackfillRow is a classified candidate, ready for the dry-run CSV.
type BackfillRow struct {
	Candidate      BackfillCandidate
	Bucket         string
	Action         BackfillAction
	Reason         string
	Poy            *BackfillPoy
	ConflictSpinID string
}

// ClassifyBackfill maps a candidate to its P-13 bucket, action and reason, plus
// the target POY for APPLY/REVIEW. Order of checks = P-13 CASE order.
func ClassifyBackfill(c BackfillCandidate) (bucket string, action BackfillAction, reason string, poy *BackfillPoy) {
	switch {
	case c.NormShade == "":
		return BackfillBucketNoShade, BackfillActionSkip, BackfillReasonNoShade, nil
	case c.NSpecShade == 1:
		return BackfillBucketSpecShadeUnique, BackfillActionApply, BackfillReasonSpecShadeUnique, c.SpecShadePoy
	case c.NSpecShade > 1:
		return BackfillBucketSpecShadeAmbiguous, BackfillActionSkip, BackfillReasonSpecShadeAmbiguous, nil
	case c.NormSpec == "" && c.NShadeOnly == 1:
		return BackfillBucketShadeOnlyUnique, BackfillActionReview, BackfillReasonShadeOnlyUnique, c.ShadeOnlyPoy
	case c.NormSpec == "" && c.NShadeOnly > 1:
		return BackfillBucketShadeOnlyAmbiguous, BackfillActionSkip, BackfillReasonShadeOnlyAmbiguous, nil
	case c.NShadeOnly == 0:
		return BackfillBucketNoPoyWithShade, BackfillActionSkip, BackfillReasonNoPoyWithShade, nil
	default:
		return BackfillBucketSpecNoMatch, BackfillActionSkip, BackfillReasonSpecNoMatch, nil
	}
}

// backfillKey is the (normalized shade, FG POY) pair guarded by uix_mbs_shade_fg_live.
type backfillKey struct {
	shade string
	poyID int64
}

// isTargeting reports whether r currently proposes a write (APPLY or REVIEW with a POY).
func (r *BackfillRow) isTargeting() bool {
	return (r.Action == BackfillActionApply || r.Action == BackfillActionReview) && r.Poy != nil
}

func (r *BackfillRow) key() backfillKey {
	return backfillKey{shade: r.Candidate.NormShade, poyID: r.Poy.ProductSysID}
}

func (r *BackfillRow) skip(reason string) {
	r.Action = BackfillActionSkip
	r.Reason = reason
}

// ClassifyBackfillAll classifies every candidate, applies the overrides and
// returns rows sorted by (action, shade, spin id). Overrides, in order:
//  1. spin already has an FG -> SKIP fg_already_set (any bucket; bucket kept);
//     a superseded spin -> SKIP spin_not_live;
//  2. target (shade, POY) already used by another live spin with an FG -> SKIP conflict_existing_live;
//  3. two or more APPLY/REVIEW rows targeting the same (shade, POY) -> ALL SKIP conflict_same_target
//     (no silent winner).
func ClassifyBackfillAll(cands []BackfillCandidate) []BackfillRow {
	rows := make([]BackfillRow, 0, len(cands))
	live := make(map[backfillKey]string, len(cands))
	for _, c := range cands {
		if c.FgProductID != 0 && !c.Superseded && c.NormShade != "" {
			live[backfillKey{shade: c.NormShade, poyID: c.FgProductID}] = c.SpinID
		}
		bucket, action, reason, poy := ClassifyBackfill(c)
		if (action == BackfillActionApply || action == BackfillActionReview) && poy == nil {
			// Defensive: a unique bucket without its POY id cannot be applied.
			action, reason = BackfillActionSkip, BackfillReasonPoyInvalid
		}
		rows = append(rows, BackfillRow{Candidate: c, Bucket: bucket, Action: action, Reason: reason, Poy: poy})
	}
	markBackfillOverrides(rows, live)
	sortBackfillRows(rows)
	return rows
}

func markBackfillOverrides(rows []BackfillRow, live map[backfillKey]string) {
	targets := make(map[backfillKey]int, len(rows))
	for i := range rows {
		r := &rows[i]
		if r.Candidate.FgProductID != 0 {
			// Never overwrite an existing FG, whatever the bucket was.
			r.skip(BackfillReasonFgAlreadySet)
			r.Poy = nil
			continue
		}
		if !r.isTargeting() {
			continue
		}
		if r.Candidate.Superseded {
			r.skip(BackfillReasonSpinNotLive)
			r.Poy = nil
			continue
		}
		if other, ok := live[r.key()]; ok {
			r.skip(BackfillReasonConflictExistingLive)
			r.ConflictSpinID = other
			continue
		}
		targets[r.key()]++
	}
	for i := range rows {
		r := &rows[i]
		if r.isTargeting() && targets[r.key()] > 1 {
			r.skip(BackfillReasonConflictSameTarget)
		}
	}
}

func sortBackfillRows(rows []BackfillRow) {
	sort.SliceStable(rows, func(i, j int) bool {
		a, b := rows[i], rows[j]
		if a.Action != b.Action {
			return a.Action < b.Action
		}
		if a.Candidate.NormShade != b.Candidate.NormShade {
			return a.Candidate.NormShade < b.Candidate.NormShade
		}
		return a.Candidate.SpinID < b.Candidate.SpinID
	})
}

// CountBackfillRows returns counts keyed by "ACTION/reason" for the summary log.
func CountBackfillRows(rows []BackfillRow) map[string]int {
	out := make(map[string]int)
	for _, r := range rows {
		out[string(r.Action)+"/"+r.Reason]++
	}
	return out
}

// CountBackfillBuckets returns counts per P-13 bucket (before overrides) for
// comparison with the P-13 SQL output.
func CountBackfillBuckets(rows []BackfillRow) map[string]int {
	out := make(map[string]int)
	for _, r := range rows {
		out[r.Bucket]++
	}
	return out
}
