package grpc

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	financev1 "github.com/mutugading/goapps-backend/gen/finance/v1"
	"github.com/mutugading/goapps-backend/services/finance/internal/application/mbsourceautofill"
	cpp "github.com/mutugading/goapps-backend/services/finance/internal/domain/costproductparameter"
)

type fakeMBLink struct {
	link mbsourceautofill.Link
	err  error
}

func (f fakeMBLink) Explain(context.Context, int64) (mbsourceautofill.Link, error) {
	return f.link, f.err
}
func (f fakeMBLink) Relink(context.Context, int64, string) (mbsourceautofill.Link, error) {
	return f.link, f.err
}

func TestMBLinkHandler_NotConfigured(t *testing.T) {
	h := &CostProductMasterHandler{}
	r, err := h.GetCostProductMbLink(context.Background(), &financev1.GetCostProductMbLinkRequest{ProductSysId: 1})
	assert.NoError(t, err)
	assert.Equal(t, "501", r.GetBase().GetStatusCode())
	r2, err := h.RelinkCostProductMb(context.Background(), &financev1.RelinkCostProductMbRequest{ProductSysId: 1})
	assert.NoError(t, err)
	assert.Equal(t, "501", r2.GetBase().GetStatusCode())
}

func TestMBLinkHandler_MapsLinkAndErrors(t *testing.T) {
	h := &CostProductMasterHandler{}
	h.WithMBLink(fakeMBLink{link: mbsourceautofill.Link{LinkSource: "FG_MATCH", SpinID: "s", CandidatePoyIDs: []int64{3}, ParamMatches: true}})
	r, _ := h.GetCostProductMbLink(context.Background(), &financev1.GetCostProductMbLinkRequest{ProductSysId: 1})
	assert.True(t, r.GetBase().GetIsSuccess())
	assert.Equal(t, "FG_MATCH", r.GetData().GetLinkSource())
	assert.Equal(t, []int64{3}, r.GetData().GetCandidatePoyProductIds())
	assert.True(t, r.GetData().GetParamMatches())

	cases := map[string]error{
		"404": mbsourceautofill.ErrLinkProductNotFound,
		"409": fmt.Errorf("x: %w", cpp.ErrProductLocked),
		"400": mbsourceautofill.ErrLinkMBType,
		"500": fmt.Errorf("boom"),
	}
	for code, e := range cases {
		h.WithMBLink(fakeMBLink{err: e})
		rr, _ := h.RelinkCostProductMb(context.Background(), &financev1.RelinkCostProductMbRequest{ProductSysId: 1})
		assert.False(t, rr.GetBase().GetIsSuccess())
		assert.Equal(t, code, rr.GetBase().GetStatusCode())
	}
}
