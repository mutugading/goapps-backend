package grpc

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	financev1 "github.com/mutugading/goapps-backend/gen/finance/v1"
	cpmapp "github.com/mutugading/goapps-backend/services/finance/internal/application/costproductmaster"
	cpm "github.com/mutugading/goapps-backend/services/finance/internal/domain/costproductmaster"
)

type fakeEtlSvc struct {
	prev   cpmapp.EtlPreviewResult
	apply  cpmapp.EtlApplyResult
	err    error
	gotCmd cpmapp.ApplyEtlCommand
}

func (f *fakeEtlSvc) Preview(context.Context, cpmapp.PreviewEtlCommand) (cpmapp.EtlPreviewResult, error) {
	return f.prev, f.err
}

func (f *fakeEtlSvc) Apply(_ context.Context, c cpmapp.ApplyEtlCommand) (cpmapp.EtlApplyResult, error) {
	f.gotCmd = c
	return f.apply, f.err
}

func TestEtlHandler_NilServiceIs503(t *testing.T) {
	h := &CostProductMasterHandler{}
	p, err := h.PreviewProductEtlFromOracle(context.Background(), &financev1.PreviewProductEtlFromOracleRequest{})
	require.NoError(t, err)
	assert.Equal(t, "503", p.Base.StatusCode)
	a, err := h.ApplyProductEtlFromOracle(context.Background(), &financev1.ApplyProductEtlFromOracleRequest{})
	require.NoError(t, err)
	assert.Equal(t, "503", a.Base.StatusCode)
}

func TestEtlHandler_ErrorMapping(t *testing.T) {
	cases := map[string]error{
		"503": cpm.ErrEtlNotConfigured, "404": cpm.ErrEtlRunNotFound, "400": cpm.ErrEtlApplyLimitExceeded,
	}
	for code, e := range cases {
		h := &CostProductMasterHandler{}
		h.WithProductEtl(&fakeEtlSvc{err: e})
		a, err := h.ApplyProductEtlFromOracle(context.Background(),
			&financev1.ApplyProductEtlFromOracleRequest{PreviewRunId: uuid.NewString(), RowKeys: []string{"A|B"}})
		require.NoError(t, err)
		assert.Equal(t, code, a.Base.StatusCode, e.Error())
	}
	assert.Equal(t, "400", etlErrToBase(cpm.ErrEtlRunNotPreview).StatusCode)
	assert.Equal(t, "400", etlErrToBase(cpm.ErrEtlInvalidFromDate).StatusCode)
	assert.Equal(t, "500", etlErrToBase(assert.AnError).StatusCode)
}

func TestEtlHandler_InvalidRunID(t *testing.T) {
	h := &CostProductMasterHandler{}
	h.WithProductEtl(&fakeEtlSvc{})
	a, err := h.ApplyProductEtlFromOracle(context.Background(), &financev1.ApplyProductEtlFromOracleRequest{PreviewRunId: "x"})
	require.NoError(t, err)
	assert.Equal(t, "400", a.Base.StatusCode)
}

func TestEtlHandler_PreviewMapping(t *testing.T) {
	id := uuid.New()
	h := &CostProductMasterHandler{}
	h.WithProductEtl(&fakeEtlSvc{prev: cpmapp.EtlPreviewResult{
		RunID: id, FromDate: "2026-01-01", ApplyLimit: 200,
		Counts: cpm.EtlCounts{NewItem: 2, Exists: 1, Total: 3},
		Rows: []cpm.EtlRunRow{{RowKey: "I|S", ItemCode: "I", ItemName: "POY 1", ShadeCode: "S", ShadeName: "Sh",
			TypeCode: "POY", Status: cpm.EtlNewShade, Selectable: true, MatchedIDs: []int64{7}, Denier: "75", Filament: "36", Note: "n"}},
	}})
	r, err := h.PreviewProductEtlFromOracle(context.Background(), &financev1.PreviewProductEtlFromOracleRequest{FromDate: "2026-01-01"})
	require.NoError(t, err)
	assert.Equal(t, "200", r.Base.StatusCode)
	assert.Equal(t, id.String(), r.Data.RunId)
	assert.Equal(t, int32(200), r.Data.ApplyLimit)
	assert.Equal(t, int32(2), r.Data.Counts.NewItem)
	assert.Equal(t, int32(3), r.Data.Counts.Total)
	require.Len(t, r.Data.Rows, 1)
	row := r.Data.Rows[0]
	assert.Equal(t, "I", row.MergeItemOrion)
	assert.Equal(t, "NEW_SHADE", row.Status)
	assert.True(t, row.Selectable)
	assert.Equal(t, []int64{7}, row.MatchedProductIds)
	assert.Equal(t, "75", row.Denier)
	assert.Equal(t, "36", row.Filament)
}

func TestEtlHandler_ApplyMapping(t *testing.T) {
	id := uuid.New()
	f := &fakeEtlSvc{apply: cpmapp.EtlApplyResult{RunID: id, Inserted: 1, Skipped: 1, Failed: 0,
		Rows: []cpm.EtlInsertResult{
			{RowKey: "A|B", Result: cpm.EtlResultInserted, ProductSysID: 9, ProductCode: "P9"},
			{RowKey: "C|D", Result: cpm.EtlResultSkipped, Message: "already exists"},
		}}}
	h := &CostProductMasterHandler{}
	h.WithProductEtl(f)
	pid := uuid.New()
	r, err := h.ApplyProductEtlFromOracle(context.Background(),
		&financev1.ApplyProductEtlFromOracleRequest{PreviewRunId: pid.String(), RowKeys: []string{"A|B", "C|D"}})
	require.NoError(t, err)
	assert.Equal(t, pid, f.gotCmd.PreviewRunID)
	assert.Equal(t, []string{"A|B", "C|D"}, f.gotCmd.RowKeys)
	assert.Equal(t, id.String(), r.Data.RunId)
	assert.Equal(t, int32(1), r.Data.Inserted)
	require.Len(t, r.Data.Rows, 2)
	assert.Equal(t, int64(9), r.Data.Rows[0].ProductSysId)
	assert.Equal(t, "P9", r.Data.Rows[0].ProductCode)
	assert.Equal(t, "SKIPPED", r.Data.Rows[1].Result)
}
