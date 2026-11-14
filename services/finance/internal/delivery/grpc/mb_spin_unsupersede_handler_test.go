package grpc

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"

	financev1 "github.com/mutugading/goapps-backend/gen/finance/v1"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbspin"
)

type fakeUnsuperseder struct {
	res *mbspin.UnsupersedeResult
	err error
}

func (f fakeUnsuperseder) Unsupersede(context.Context, uuid.UUID, uuid.UUID, string) (*mbspin.UnsupersedeResult, error) {
	return f.res, f.err
}

func newUnsupersedeTestHandler(t *testing.T, u mbspin.Unsuperseder) *MBSpinHandler {
	t.Helper()
	h, err := NewMBSpinHandler(nil)
	require.NoError(t, err)
	if u != nil {
		h.WithUnsuperseder(u, nil)
	}
	return h
}

func TestUnsupersedeMBSpin_Maps(t *testing.T) {
	spin, err := mbspin.New(uuid.New(), "S", nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, "u")
	require.NoError(t, err)
	h := newUnsupersedeTestHandler(t, fakeUnsuperseder{res: &mbspin.UnsupersedeResult{
		Spin: spin, ParamsReverted: 3, Warnings: []string{"new recipe stays active"},
		Skipped: []mbspin.UnsupersedeSkipped{{CppValueID: 9, ProductSysID: 5, ProductCode: "P5", ParamCode: "MB_SP_DYE", Reason: mbspin.UnsupersedeSkipChanged}},
	}})
	resp, err := h.UnsupersedeMBSpin(context.Background(), &financev1.UnsupersedeMBSpinRequest{MbhId: uuid.NewString(), MbsId: uuid.NewString()})
	require.NoError(t, err)
	require.True(t, resp.Base.IsSuccess, resp.Base.Message)
	assert.Contains(t, resp.Base.Message, "stays active")
	assert.EqualValues(t, 3, resp.ParamsReverted)
	require.Len(t, resp.Skipped, 1)
	assert.Equal(t, "MB_SP_DYE", resp.Skipped[0].ParamCode)
	assert.EqualValues(t, 9, resp.Skipped[0].CppValueId)
	assert.NotNil(t, resp.Data)
}

func TestUnsupersedeMBSpin_ErrorMapping(t *testing.T) {
	cases := map[string]struct {
		err  error
		code string
	}{
		"not found":      {mbspin.ErrNotFound, "404"},
		"conflict":       {fmt.Errorf("x: %w", mbspin.ErrUnsupersedeConflict), "409"},
		"no batch":       {mbspin.ErrUnsupersedeNoBatch, "409"},
		"not superseded": {mbspin.ErrNotSuperseded, "400"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			h := newUnsupersedeTestHandler(t, fakeUnsuperseder{err: tc.err})
			resp, err := h.UnsupersedeMBSpin(context.Background(), &financev1.UnsupersedeMBSpinRequest{MbhId: uuid.NewString(), MbsId: uuid.NewString()})
			require.NoError(t, err)
			assert.False(t, resp.Base.IsSuccess)
			assert.Equal(t, tc.code, resp.Base.StatusCode)
		})
	}
	t.Run("internal", func(t *testing.T) {
		h := newUnsupersedeTestHandler(t, fakeUnsuperseder{err: errors.New("boom")})
		resp, _ := h.UnsupersedeMBSpin(context.Background(), &financev1.UnsupersedeMBSpinRequest{MbhId: uuid.NewString(), MbsId: uuid.NewString()})
		assert.Equal(t, "500", resp.Base.StatusCode)
	})
}

func TestUnsupersedeMBSpin_BadIDAndDisabled(t *testing.T) {
	h := newUnsupersedeTestHandler(t, fakeUnsuperseder{})
	resp, err := h.UnsupersedeMBSpin(context.Background(), &financev1.UnsupersedeMBSpinRequest{MbhId: "x", MbsId: uuid.NewString()})
	require.NoError(t, err)
	assert.Equal(t, "400", resp.Base.StatusCode)

	_, err = newUnsupersedeTestHandler(t, nil).UnsupersedeMBSpin(context.Background(), &financev1.UnsupersedeMBSpinRequest{})
	require.Error(t, err)
	_ = codes.Unimplemented
}
