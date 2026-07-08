package grpc

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/costroute"
)

// TestRouteHeadToProto_Aggregates verifies the read-time level/RM aggregates are
// carried into the proto CostRouteHead.
func TestRouteHeadToProto_Aggregates(t *testing.T) {
	got := routeHeadToProto(&costroute.Head{
		HeadID:        7,
		ProductSysID:  42,
		ProductCode:   "PRD-1",
		RoutingStatus: costroute.StatusDraft,
		Version:       3,
		LevelCount:    4,
		RmCount:       9,
	})
	require.NotNil(t, got)
	assert.Equal(t, int64(7), got.GetHeadId())
	assert.Equal(t, int32(4), got.GetLevelCount())
	assert.Equal(t, int32(9), got.GetRmCount())
}

// TestRouteGraphToProto_RmPositionAndGroupName verifies RM node position and the
// joined group name reach the proto.
func TestRouteGraphToProto_RmPositionAndGroupName(t *testing.T) {
	g := &costroute.Graph{
		Head: &costroute.Head{HeadID: 1, LevelCount: 1, RmCount: 1},
		Seqs: []*costroute.Seq{
			{
				SeqID: 10, RouteLevel: 1, RouteSeq: 1,
				Rms: []*costroute.Rm{
					{
						RmID: 100, SeqID: 10, RmType: costroute.RmTypeGroup,
						RmGroupCode: "BLUE-1", RmGroupName: "Blue Pigments",
						PositionX: 123.5, PositionY: -48.25,
					},
				},
			},
		},
	}
	got := routeGraphToProto(g)
	require.NotNil(t, got)
	require.Len(t, got.GetSeqs(), 1)
	require.Len(t, got.GetSeqs()[0].GetRms(), 1)
	rm := got.GetSeqs()[0].GetRms()[0]
	assert.Equal(t, "Blue Pigments", rm.GetRmGroupName())
	assert.InDelta(t, 123.5, rm.GetPositionX(), 1e-9)
	assert.InDelta(t, -48.25, rm.GetPositionY(), 1e-9)
}

// TestRouteGraphFromProto_RmPositionRoundTrip verifies the inbound mapper reads
// RM position back into the domain graph (rm_group_name is read-only/denorm and
// intentionally not written back).
func TestRouteGraphFromProto_RmPositionRoundTrip(t *testing.T) {
	in := routeGraphToProto(&costroute.Graph{
		Head: &costroute.Head{HeadID: 1},
		Seqs: []*costroute.Seq{
			{
				SeqID: 10, RouteLevel: 1, RouteSeq: 1,
				Rms: []*costroute.Rm{
					{RmID: 100, SeqID: 10, RmType: costroute.RmTypeProduct, RmProductSysID: 5, PositionX: 12, PositionY: 34},
				},
			},
		},
	})
	out := routeGraphFromProto(in)
	require.NotNil(t, out)
	require.Len(t, out.Seqs, 1)
	require.Len(t, out.Seqs[0].Rms, 1)
	rm := out.Seqs[0].Rms[0]
	assert.InDelta(t, 12.0, rm.PositionX, 1e-9)
	assert.InDelta(t, 34.0, rm.PositionY, 1e-9)
}
