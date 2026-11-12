package mbspin_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbspin"
)

func poy(id int64) *mbspin.BackfillPoy {
	return &mbspin.BackfillPoy{ProductSysID: id, ProductCode: "P", ProductName: "POY X", ErpItemCode: "E"}
}

func TestClassifyBackfill_AllP13Buckets(t *testing.T) {
	tests := []struct {
		name   string
		c      mbspin.BackfillCandidate
		bucket string
		action mbspin.BackfillAction
		reason string
		poyID  int64
	}{
		{"no shade", mbspin.BackfillCandidate{NormSpec: "250/48", NSpecShade: 0, NShadeOnly: 0},
			mbspin.BackfillBucketNoShade, mbspin.BackfillActionSkip, mbspin.BackfillReasonNoShade, 0},
		{"no shade wins over counts", mbspin.BackfillCandidate{NSpecShade: 1, SpecShadePoy: poy(1)},
			mbspin.BackfillBucketNoShade, mbspin.BackfillActionSkip, mbspin.BackfillReasonNoShade, 0},
		{"spec+shade unique", mbspin.BackfillCandidate{NormShade: "R", NormSpec: "250/48", NSpecShade: 1, NShadeOnly: 3, SpecShadePoy: poy(7)},
			mbspin.BackfillBucketSpecShadeUnique, mbspin.BackfillActionApply, mbspin.BackfillReasonSpecShadeUnique, 7},
		{"spec+shade ambiguous", mbspin.BackfillCandidate{NormShade: "R", NormSpec: "250/48", NSpecShade: 2, NShadeOnly: 3},
			mbspin.BackfillBucketSpecShadeAmbiguous, mbspin.BackfillActionSkip, mbspin.BackfillReasonSpecShadeAmbiguous, 0},
		{"no spec shade unique", mbspin.BackfillCandidate{NormShade: "R", NShadeOnly: 1, ShadeOnlyPoy: poy(9)},
			mbspin.BackfillBucketShadeOnlyUnique, mbspin.BackfillActionReview, mbspin.BackfillReasonShadeOnlyUnique, 9},
		{"no spec shade ambiguous", mbspin.BackfillCandidate{NormShade: "R", NShadeOnly: 4},
			mbspin.BackfillBucketShadeOnlyAmbiguous, mbspin.BackfillActionSkip, mbspin.BackfillReasonShadeOnlyAmbiguous, 0},
		{"no poy with shade (no spec)", mbspin.BackfillCandidate{NormShade: "R"},
			mbspin.BackfillBucketNoPoyWithShade, mbspin.BackfillActionSkip, mbspin.BackfillReasonNoPoyWithShade, 0},
		{"no poy with shade (spec)", mbspin.BackfillCandidate{NormShade: "R", NormSpec: "250/48"},
			mbspin.BackfillBucketNoPoyWithShade, mbspin.BackfillActionSkip, mbspin.BackfillReasonNoPoyWithShade, 0},
		{"spec no match", mbspin.BackfillCandidate{NormShade: "R", NormSpec: "250/48", NShadeOnly: 1, ShadeOnlyPoy: poy(3)},
			mbspin.BackfillBucketSpecNoMatch, mbspin.BackfillActionSkip, mbspin.BackfillReasonSpecNoMatch, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			bucket, action, reason, p := mbspin.ClassifyBackfill(tc.c)
			assert.Equal(t, tc.bucket, bucket)
			assert.Equal(t, tc.action, action)
			assert.Equal(t, tc.reason, reason)
			if tc.poyID == 0 {
				assert.Nil(t, p)
				return
			}
			require.NotNil(t, p)
			assert.Equal(t, tc.poyID, p.ProductSysID)
		})
	}
}

func byID(rows []mbspin.BackfillRow) map[string]mbspin.BackfillRow {
	m := make(map[string]mbspin.BackfillRow, len(rows))
	for _, r := range rows {
		m[r.Candidate.SpinID] = r
	}
	return m
}

func TestClassifyBackfillAll_Overrides(t *testing.T) {
	cands := []mbspin.BackfillCandidate{
		// fg_already_set: APPLY bucket but spin already has FG.
		{SpinID: "a1", NormShade: "R", NormSpec: "S", NSpecShade: 1, SpecShadePoy: poy(1), FgProductID: 1},
		// fg_already_set even from a SKIP bucket.
		{SpinID: "a2", NormShade: "Q", FgProductID: 50},
		// existing live holder of (G, 2) and a candidate targeting it.
		{SpinID: "live", NormShade: "G", FgProductID: 2},
		{SpinID: "b1", NormShade: "G", NormSpec: "S", NSpecShade: 1, SpecShadePoy: poy(2)},
		// superseded holder does not block.
		{SpinID: "sup", NormShade: "B", FgProductID: 3, Superseded: true},
		{SpinID: "c1", NormShade: "B", NormSpec: "S", NSpecShade: 1, SpecShadePoy: poy(3)},
		// same target: APPLY + REVIEW on (Y, 4) -> both skipped.
		{SpinID: "d1", NormShade: "Y", NormSpec: "S", NSpecShade: 1, SpecShadePoy: poy(4)},
		{SpinID: "d2", NormShade: "Y", NShadeOnly: 1, ShadeOnlyPoy: poy(4)},
		// same POY, different shade -> no conflict.
		{SpinID: "e1", NormShade: "Z", NormSpec: "S", NSpecShade: 1, SpecShadePoy: poy(4)},
		// unique REVIEW untouched.
		{SpinID: "f1", NormShade: "W", NShadeOnly: 1, ShadeOnlyPoy: poy(5)},
		// unique bucket without POY id -> defensive skip.
		{SpinID: "g1", NormShade: "V", NormSpec: "S", NSpecShade: 1},
	}
	rows := mbspin.ClassifyBackfillAll(cands)
	require.Len(t, rows, len(cands))
	m := byID(rows)

	tests := []struct {
		id       string
		action   mbspin.BackfillAction
		reason   string
		bucket   string
		conflict string
		hasPoy   bool
	}{
		{"a1", mbspin.BackfillActionSkip, mbspin.BackfillReasonFgAlreadySet, mbspin.BackfillBucketSpecShadeUnique, "", false},
		{"a2", mbspin.BackfillActionSkip, mbspin.BackfillReasonFgAlreadySet, mbspin.BackfillBucketNoPoyWithShade, "", false},
		{"b1", mbspin.BackfillActionSkip, mbspin.BackfillReasonConflictExistingLive, mbspin.BackfillBucketSpecShadeUnique, "live", true},
		{"c1", mbspin.BackfillActionApply, mbspin.BackfillReasonSpecShadeUnique, mbspin.BackfillBucketSpecShadeUnique, "", true},
		{"d1", mbspin.BackfillActionSkip, mbspin.BackfillReasonConflictSameTarget, mbspin.BackfillBucketSpecShadeUnique, "", true},
		{"d2", mbspin.BackfillActionSkip, mbspin.BackfillReasonConflictSameTarget, mbspin.BackfillBucketShadeOnlyUnique, "", true},
		{"e1", mbspin.BackfillActionApply, mbspin.BackfillReasonSpecShadeUnique, mbspin.BackfillBucketSpecShadeUnique, "", true},
		{"f1", mbspin.BackfillActionReview, mbspin.BackfillReasonShadeOnlyUnique, mbspin.BackfillBucketShadeOnlyUnique, "", true},
		{"g1", mbspin.BackfillActionSkip, mbspin.BackfillReasonPoyInvalid, mbspin.BackfillBucketSpecShadeUnique, "", false},
	}
	for _, tc := range tests {
		t.Run(tc.id, func(t *testing.T) {
			r := m[tc.id]
			assert.Equal(t, tc.action, r.Action)
			assert.Equal(t, tc.reason, r.Reason)
			assert.Equal(t, tc.bucket, r.Bucket)
			assert.Equal(t, tc.conflict, r.ConflictSpinID)
			assert.Equal(t, tc.hasPoy, r.Poy != nil)
		})
	}
}

func TestClassifyBackfillAll_SortAndCounts(t *testing.T) {
	cands := []mbspin.BackfillCandidate{
		{SpinID: "3", NormShade: "B"},
		{SpinID: "2", NormShade: "A", NormSpec: "S", NSpecShade: 1, SpecShadePoy: poy(1)},
		{SpinID: "1", NormShade: "B"},
		{SpinID: "4", NormShade: "C", NShadeOnly: 1, ShadeOnlyPoy: poy(2)},
		{SpinID: "0"},
	}
	rows := mbspin.ClassifyBackfillAll(cands)
	got := make([]string, 0, len(rows))
	for _, r := range rows {
		got = append(got, r.Candidate.SpinID)
	}
	// APPLY < REVIEW < SKIP, then shade, then id.
	assert.Equal(t, []string{"2", "4", "0", "1", "3"}, got)

	assert.Equal(t, map[string]int{
		"APPLY/spec_shade_unique": 1, "REVIEW/shade_only_unique": 1,
		"SKIP/no_shade": 1, "SKIP/no_poy_with_shade": 2,
	}, mbspin.CountBackfillRows(rows))
	assert.Equal(t, 2, mbspin.CountBackfillBuckets(rows)[mbspin.BackfillBucketNoPoyWithShade])
}
