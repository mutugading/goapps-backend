package costproductmaster

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/costproductmaster"
)

type fakeEtlSrc struct {
	rows  []domain.EtlSourceRow
	calls int
}

func (f *fakeEtlSrc) ListMergeItems(context.Context, time.Time) ([]domain.EtlSourceRow, error) {
	f.calls++
	return f.rows, nil
}

type fakeEtlRepo struct {
	ms      domain.EtlMatchSet
	types   map[string]int32
	sib     map[int64]map[string]string
	runs    map[uuid.UUID]domain.EtlRun
	gotIn   domain.EtlInsertInput
	insRes  func(domain.EtlInsert) domain.EtlInsertResult
	insErr  error
	saveCnt int
}

func newFakeRepo() *fakeEtlRepo {
	return &fakeEtlRepo{
		ms: domain.EtlMatchSet{ByItemShade: map[string][]int64{}, ByItem: map[string][]int64{}, ByNameShade: map[string][]int64{}},
		types: map[string]int32{"POY": 1, "DTY": 2}, sib: map[int64]map[string]string{}, runs: map[uuid.UUID]domain.EtlRun{},
	}
}

func (f *fakeEtlRepo) MatchProducts(context.Context, []domain.EtlKey) (domain.EtlMatchSet, error) {
	return f.ms, nil
}
func (f *fakeEtlRepo) TypeIDsByCode(context.Context) (map[string]int32, error) { return f.types, nil }
func (f *fakeEtlRepo) SiblingParams(context.Context, []int64, []string) (map[int64]map[string]string, error) {
	return f.sib, nil
}
func (f *fakeEtlRepo) SaveRun(_ context.Context, r domain.EtlRun) (uuid.UUID, error) {
	f.saveCnt++
	r.ID = uuid.New()
	f.runs[r.ID] = r
	return r.ID, nil
}
func (f *fakeEtlRepo) GetRun(_ context.Context, id uuid.UUID) (domain.EtlRun, error) {
	r, ok := f.runs[id]
	if !ok {
		return r, domain.ErrEtlRunNotFound
	}
	return r, nil
}
func (f *fakeEtlRepo) InsertEtlProducts(_ context.Context, in domain.EtlInsertInput) (domain.EtlInsertOutput, error) {
	f.gotIn = in
	if f.insErr != nil {
		return domain.EtlInsertOutput{}, f.insErr
	}
	out := domain.EtlInsertOutput{RunID: uuid.New()}
	for i, it := range in.Items {
		if f.insRes != nil {
			out.Results = append(out.Results, f.insRes(it))
			continue
		}
		out.Results = append(out.Results, domain.EtlInsertResult{RowKey: it.RowKey, Result: domain.EtlResultInserted, ProductSysID: int64(100 + i)})
	}
	return out, nil
}

type fakeFill struct{ ids []int64 }

func (f *fakeFill) BestEffort(_ context.Context, ids []int64, _ string) { f.ids = ids }

func row(item, name, shade, den, fil string) domain.EtlSourceRow {
	return domain.EtlSourceRow{ItemCode: item, ItemName: name, ShadeCode: shade, ShadeName: shade + " N", Denier: den, Filament: fil}
}

func preview(t *testing.T, svc *ProductEtlService, include bool, search string) EtlPreviewResult {
	t.Helper()
	res, err := svc.Preview(context.Background(), PreviewEtlCommand{FromDate: "2026-01-01", Search: search, IncludeExisting: include, Actor: "u"})
	require.NoError(t, err)
	return res
}

func byKey(rows []domain.EtlRunRow) map[string]domain.EtlRunRow {
	m := map[string]domain.EtlRunRow{}
	for _, r := range rows {
		m[r.RowKey] = r
	}
	return m
}

func TestEtlPreview_AllStatuses(t *testing.T) {
	repo := newFakeRepo()
	repo.ms.ByItemShade["I1|S1"] = []int64{1}
	repo.ms.ByItemShade["I2|S1"] = []int64{2, 3}
	repo.ms.ByNameShade[domain.EtlNameShadeKey("POY DUP", "S1")] = []int64{9}
	repo.ms.ByItem["I4"] = []int64{7, 8}
	repo.sib[8] = map[string]string{"DENIER": "150", "NO_OF_FILAMENTS": "48", "ACT_DENIER": "151.5"}
	src := &fakeEtlSrc{rows: []domain.EtlSourceRow{
		row("I1", "POY A", "S1", "", ""),
		row("i2", "POY B", "s1", "", ""),
		row("I3", "POY DUP", "S1", "", ""),
		row("I4", "POY C", "S2", "99", "10"),
		row("I5", "DTY D", "S1", "75.5", "36"),
		row("I6", "RMP E", "S1", "", ""),
		row("", "POY F", "S1", "", ""),
		row("I7", "MB G", "S1", "", ""),
		row("I5", "DTY D", "S1", "", ""), // dup
	}}
	fill := &fakeFill{}
	svc := NewProductEtlService(src, repo, nil, fill, 5)
	res := preview(t, svc, true, "")
	m := byKey(res.Rows)
	assert.Equal(t, domain.EtlExists, m["I1|S1"].Status)
	assert.Equal(t, domain.EtlExistsMulti, m["I2|S1"].Status)
	assert.Equal(t, domain.EtlPossibleDuplicate, m["I3|S1"].Status)
	assert.Equal(t, domain.EtlNewShade, m["I4|S2"].Status)
	assert.Equal(t, domain.EtlNewItem, m["I5|S1"].Status)
	assert.Equal(t, domain.EtlInvalidType, m["I6|S1"].Status)
	assert.Equal(t, domain.EtlInvalidType, m["|S1"].Status)
	assert.Len(t, res.Rows, 7) // MB skipped, dup merged
	assert.Equal(t, int32(7), res.Counts.Total)
	assert.Equal(t, 5, res.ApplyLimit)
	assert.False(t, m["I1|S1"].Selectable)
	assert.False(t, m["I3|S1"].Selectable)
	// NEW_SHADE: sibling wins over Oracle, ACT_DENIER from sibling.
	assert.Equal(t, "150", m["I4|S2"].Denier)
	assert.Equal(t, "48", m["I4|S2"].Filament)
	assert.Equal(t, "151.5", m["I4|S2"].ActDenier)
	// NEW_ITEM: Oracle only, no ACT_DENIER.
	assert.Equal(t, "75.5", m["I5|S1"].Denier)
	assert.Equal(t, "36", m["I5|S1"].Filament)
	assert.Empty(t, m["I5|S1"].ActDenier)
	assert.Equal(t, int32(1), int32(repo.runs[res.RunID].SkippedMB))
}

func TestEtlPreview_NewShadeFallsBackToOracleWhenSiblingEmpty(t *testing.T) {
	repo := newFakeRepo()
	repo.ms.ByItem["I4"] = []int64{7}
	svc := NewProductEtlService(&fakeEtlSrc{rows: []domain.EtlSourceRow{row("I4", "POY C", "S2", "99", "abc")}}, repo, nil, nil, 0)
	r := preview(t, svc, true, "").Rows[0]
	assert.Equal(t, "99", r.Denier)
	assert.Empty(t, r.Filament) // unparsable not written
	assert.Empty(t, r.ActDenier)
}

func TestEtlPreview_IncludeExistingAndSearch(t *testing.T) {
	repo := newFakeRepo()
	repo.ms.ByItemShade["I1|S1"] = []int64{1}
	src := &fakeEtlSrc{rows: []domain.EtlSourceRow{row("I1", "POY A", "S1", "", ""), row("I2", "POY B", "S1", "", ""), row("I3", "DTY Z", "S1", "", "")}}
	svc := NewProductEtlService(src, repo, nil, nil, 0)
	res := preview(t, svc, false, "")
	assert.Len(t, res.Rows, 2)
	assert.Equal(t, int32(1), res.Counts.Exists) // hidden but counted
	res = preview(t, svc, true, "dty")
	assert.Len(t, res.Rows, 1)
	assert.Equal(t, int32(3), res.Counts.Total) // counts before search
}

func TestEtlPreview_Errors(t *testing.T) {
	repo := newFakeRepo()
	_, err := NewProductEtlService(nil, repo, nil, nil, 0).Preview(context.Background(), PreviewEtlCommand{FromDate: "2026-01-01"})
	assert.ErrorIs(t, err, domain.ErrEtlNotConfigured)
	svc := NewProductEtlService(&fakeEtlSrc{}, repo, nil, nil, 0)
	for _, d := range []string{"", "x", "2999-01-01"} {
		_, err = svc.Preview(context.Background(), PreviewEtlCommand{FromDate: d})
		assert.ErrorIs(t, err, domain.ErrEtlInvalidFromDate, d)
	}
}

func applySetup(t *testing.T) (*ProductEtlService, *fakeEtlRepo, *fakeEtlSrc, *fakeFill, uuid.UUID) {
	t.Helper()
	repo := newFakeRepo()
	repo.ms.ByItemShade["I1|S1"] = []int64{1}
	src := &fakeEtlSrc{rows: []domain.EtlSourceRow{row("I1", "POY A", "S1", "", ""), row("I2", "POY B", "S1", "5", "6"), row("I3", "POY C", "S1", "", "")}}
	fill := &fakeFill{}
	svc := NewProductEtlService(src, repo, nil, fill, 3)
	res := preview(t, svc, true, "")
	return svc, repo, src, fill, res.RunID
}

func TestEtlApply_Flow(t *testing.T) {
	svc, repo, src, fill, run := applySetup(t)
	calls := src.calls
	res, err := svc.Apply(context.Background(), ApplyEtlCommand{PreviewRunID: run, RowKeys: []string{"I2|S1", "I1|S1", "NOPE|X", "I2|S1"}, Actor: "u"})
	require.NoError(t, err)
	assert.Equal(t, calls, src.calls, "Oracle must not be read in Apply")
	assert.Equal(t, int32(1), res.Inserted)
	assert.Equal(t, int32(2), res.Skipped)
	assert.Len(t, repo.gotIn.Items, 1)
	assert.Equal(t, "5", repo.gotIn.Items[0].Denier)
	assert.Len(t, repo.gotIn.Skipped, 2)
	assert.Equal(t, []int64{100}, fill.ids)
}

func TestEtlApply_RecheckSkippedNotAutofilled(t *testing.T) {
	svc, repo, _, fill, run := applySetup(t)
	repo.insRes = func(it domain.EtlInsert) domain.EtlInsertResult {
		return domain.EtlInsertResult{RowKey: it.RowKey, Result: domain.EtlResultSkipped, Message: "already exists"}
	}
	res, err := svc.Apply(context.Background(), ApplyEtlCommand{PreviewRunID: run, RowKeys: []string{"I2|S1"}})
	require.NoError(t, err)
	assert.Equal(t, int32(1), res.Skipped)
	assert.Empty(t, fill.ids)
}

func TestEtlApply_Errors(t *testing.T) {
	svc, repo, _, _, run := applySetup(t)
	_, err := svc.Apply(context.Background(), ApplyEtlCommand{PreviewRunID: run, RowKeys: []string{"a", "b", "c", "d"}})
	assert.ErrorIs(t, err, domain.ErrEtlApplyLimitExceeded)
	_, err = svc.Apply(context.Background(), ApplyEtlCommand{PreviewRunID: uuid.New(), RowKeys: []string{"a"}})
	assert.ErrorIs(t, err, domain.ErrEtlRunNotFound)
	aid := uuid.New()
	repo.runs[aid] = domain.EtlRun{ID: aid, Type: domain.EtlRunApply}
	_, err = svc.Apply(context.Background(), ApplyEtlCommand{PreviewRunID: aid, RowKeys: []string{"a"}})
	assert.ErrorIs(t, err, domain.ErrEtlRunNotPreview)
	repo.insErr = errors.New("boom")
	before := repo.saveCnt
	_, err = svc.Apply(context.Background(), ApplyEtlCommand{PreviewRunID: run, RowKeys: []string{"I2|S1"}})
	assert.Error(t, err)
	assert.Equal(t, before+1, repo.saveCnt, "failed run recorded")
}
