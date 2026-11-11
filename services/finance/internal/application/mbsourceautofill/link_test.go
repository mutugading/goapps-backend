package mbsourceautofill_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/application/mbsourceautofill"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/costproductparameter"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbsource"
)

type fakeLinkReader struct {
	p   mbsourceautofill.LinkProduct
	err error
}

func (f fakeLinkReader) GetLinkProduct(context.Context, int64) (mbsourceautofill.LinkProduct, error) {
	return f.p, f.err
}
func (fakeLinkReader) ProductCodes(_ context.Context, ids []int64) (map[int64]string, error) {
	m := map[int64]string{}
	for _, id := range ids {
		m[id] = "POY-1"
	}
	return m, nil
}

type fakeFgExplainer struct{ cands []int64 }

func (f fakeFgExplainer) Explain(context.Context, mbsource.Target) (mbsource.FgExplain, error) {
	return mbsource.FgExplain{Candidates: f.cands}, nil
}

type fakeForce struct {
	calls int
	err   error
}

func (f *fakeForce) ApplyForce(context.Context, mbsourceautofill.ParamSet, mbsourceautofill.Plan, string) error {
	f.calls++
	return f.err
}

func linkSvc(p mbsourceautofill.LinkProduct, res *mbsource.Resolution, fa *fakeForce) *mbsourceautofill.LinkService {
	m := map[int64]mbsource.Resolution{}
	if res != nil {
		m[p.ProductSysID] = *res
	}
	svc := mbsourceautofill.New(&fakeStore{}, fakeResolver{}, okParams{})
	return mbsourceautofill.NewLinkService(fakeLinkReader{p: p}, &targetResolver{byProduct: m}, fakeFgExplainer{cands: []int64{7, 8}}, fa, svc)
}

func TestLink_Explain_Sources(t *testing.T) {
	fg := int64(7)
	sid := uuid.New()
	p := mbsourceautofill.LinkProduct{ProductSysID: 1, TypeCode: "FDY", Shade: " ab ", CurrentSPCode: "x1"}
	tests := []struct {
		name    string
		res     *mbsource.Resolution
		source  string
		matches bool
	}{
		{"fg", &mbsource.Resolution{SPCode: "X1", Source: mbsource.SourceMBSpinFg, SpinID: &sid, FgProductID: &fg}, mbsourceautofill.LinkFgMatch, true},
		{"shade", &mbsource.Resolution{SPCode: "Y", Source: mbsource.SourceMBSpin}, mbsourceautofill.LinkShadeOnly, false},
		{"superba", &mbsource.Resolution{SPCode: "AB", Source: mbsource.SourceSuperbaCostSP}, mbsourceautofill.LinkSuperba, false},
		{"none", nil, mbsourceautofill.LinkNone, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			l, err := linkSvc(p, tc.res, &fakeForce{}).Explain(context.Background(), 1)
			require.NoError(t, err)
			assert.Equal(t, tc.source, l.LinkSource)
			assert.Equal(t, tc.matches, l.ParamMatches)
			assert.Equal(t, "x1", l.CurrentParamValue)
			assert.NotEmpty(t, l.Reason)
			if tc.source == mbsourceautofill.LinkFgMatch {
				assert.Equal(t, int64(7), l.FgProductID)
				assert.Equal(t, "POY-1", l.FgProductCode)
				assert.Equal(t, sid.String(), l.SpinID)
				assert.Equal(t, []int64{7, 8}, l.CandidatePoyIDs)
			}
		})
	}
}

func TestLink_Explain_MBTypeAndNoShade(t *testing.T) {
	l, err := linkSvc(mbsourceautofill.LinkProduct{ProductSysID: 1, TypeCode: "MB", Shade: "A"}, nil, &fakeForce{}).Explain(context.Background(), 1)
	require.NoError(t, err)
	assert.Equal(t, mbsourceautofill.LinkNone, l.LinkSource)
	l, err = linkSvc(mbsourceautofill.LinkProduct{ProductSysID: 1, TypeCode: "FDY"}, nil, &fakeForce{}).Explain(context.Background(), 1)
	require.NoError(t, err)
	assert.Equal(t, mbsourceautofill.LinkNone, l.LinkSource)
}

func TestLink_Relink(t *testing.T) {
	p := mbsourceautofill.LinkProduct{ProductSysID: 1, TypeCode: "FDY", Shade: "A"}
	res := &mbsource.Resolution{SPCode: "S", Source: mbsource.SourceMBSpin}

	fa := &fakeForce{}
	l, err := linkSvc(p, res, fa).Relink(context.Background(), 1, "u")
	require.NoError(t, err)
	assert.Equal(t, 1, fa.calls)
	assert.Equal(t, mbsourceautofill.LinkShadeOnly, l.LinkSource)

	fa = &fakeForce{}
	l, err = linkSvc(p, nil, fa).Relink(context.Background(), 1, "u")
	require.NoError(t, err)
	assert.Equal(t, 0, fa.calls)
	assert.Equal(t, mbsourceautofill.LinkNone, l.LinkSource)

	fa = &fakeForce{}
	_, err = linkSvc(mbsourceautofill.LinkProduct{ProductSysID: 1, TypeCode: "MB", Shade: "A"}, res, fa).Relink(context.Background(), 1, "u")
	require.ErrorIs(t, err, mbsourceautofill.ErrLinkMBType)
	assert.Equal(t, 0, fa.calls)

	fa = &fakeForce{err: costproductparameter.ErrProductLocked}
	_, err = linkSvc(p, res, fa).Relink(context.Background(), 1, "u")
	require.ErrorIs(t, err, costproductparameter.ErrProductLocked)
}
