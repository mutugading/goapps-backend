package grpc

import (
	"context"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	commonv1 "github.com/mutugading/goapps-backend/gen/common/v1"
	iamv1 "github.com/mutugading/goapps-backend/gen/iam/v1"
	chatinfra "github.com/mutugading/goapps-backend/services/iam/internal/infrastructure/chat"
)

// PresenceHandler implements iamv1.PresenceServiceServer.
type PresenceHandler struct {
	iamv1.UnimplementedPresenceServiceServer
	presence *chatinfra.PresenceService
}

// NewPresenceHandler constructs the handler.
func NewPresenceHandler(presence *chatinfra.PresenceService) *PresenceHandler {
	return &PresenceHandler{presence: presence}
}

// Heartbeat updates the caller's online presence.
func (h *PresenceHandler) Heartbeat(ctx context.Context, _ *iamv1.HeartbeatRequest) (*iamv1.HeartbeatResponse, error) {
	callerID, err := getUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	if err := h.presence.SetOnline(ctx, callerID); err != nil {
		return nil, status.Errorf(codes.Internal, "heartbeat failed: %v", err)
	}
	return &iamv1.HeartbeatResponse{
		Base: &commonv1.BaseResponse{IsSuccess: true, Message: "success", StatusCode: "200"},
	}, nil
}

// GetOnlineUsers returns which users are currently online.
func (h *PresenceHandler) GetOnlineUsers(ctx context.Context, req *iamv1.GetOnlineUsersRequest) (*iamv1.GetOnlineUsersResponse, error) {
	userIDs := make([]uuid.UUID, 0, len(req.GetUserIds()))
	for _, idStr := range req.GetUserIds() {
		uid, err := uuid.Parse(idStr)
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "invalid user_id %q: %v", idStr, err)
		}
		userIDs = append(userIDs, uid)
	}
	onlineIDs, err := h.presence.GetOnlineUsers(ctx, userIDs)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get online users: %v", err)
	}
	strs := make([]string, len(onlineIDs))
	for i, uid := range onlineIDs {
		strs[i] = uid.String()
	}
	return &iamv1.GetOnlineUsersResponse{
		Base:    &commonv1.BaseResponse{IsSuccess: true, Message: "success", StatusCode: "200"},
		UserIds: strs,
	}, nil
}
