package grpc

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	commonv1 "github.com/mutugading/goapps-backend/gen/common/v1"
	iamv1 "github.com/mutugading/goapps-backend/gen/iam/v1"
	appChat "github.com/mutugading/goapps-backend/services/iam/internal/application/chat"
	"github.com/mutugading/goapps-backend/services/iam/internal/domain/chat"
	"github.com/mutugading/goapps-backend/services/iam/internal/infrastructure/postgres"
)

// ChatHandler implements iamv1.ChatServiceServer.
type ChatHandler struct {
	iamv1.UnimplementedChatServiceServer
	createDirect *appChat.CreateDirectHandler
	createGroup  *appChat.CreateGroupHandler
	getConv      *appChat.GetConversationHandler
	listConvs    *appChat.ListConversationsHandler
	leaveConv    *appChat.LeaveConversationHandler
	sendMsg      *appChat.SendMessageHandler
	editMsg      *appChat.EditMessageHandler
	deleteMsg    *appChat.DeleteMessageHandler
	listMsgs     *appChat.ListMessagesHandler
	markRead     *appChat.MarkReadHandler
	setTyping    *appChat.SetTypingHandler
	stream       *appChat.StreamHandler
	userResolver *postgres.ChatUserResolver
}

// NewChatHandler constructs the handler.
func NewChatHandler(
	createDirect *appChat.CreateDirectHandler,
	createGroup *appChat.CreateGroupHandler,
	getConv *appChat.GetConversationHandler,
	listConvs *appChat.ListConversationsHandler,
	leaveConv *appChat.LeaveConversationHandler,
	sendMsg *appChat.SendMessageHandler,
	editMsg *appChat.EditMessageHandler,
	deleteMsg *appChat.DeleteMessageHandler,
	listMsgs *appChat.ListMessagesHandler,
	markRead *appChat.MarkReadHandler,
	setTyping *appChat.SetTypingHandler,
	stream *appChat.StreamHandler,
	userResolver *postgres.ChatUserResolver,
) *ChatHandler {
	return &ChatHandler{
		createDirect: createDirect, createGroup: createGroup,
		getConv: getConv, listConvs: listConvs, leaveConv: leaveConv,
		sendMsg: sendMsg, editMsg: editMsg, deleteMsg: deleteMsg,
		listMsgs: listMsgs, markRead: markRead, setTyping: setTyping,
		stream: stream, userResolver: userResolver,
	}
}

// CreateDirectConversation creates a 1:1 direct conversation.
func (h *ChatHandler) CreateDirectConversation(ctx context.Context, req *iamv1.CreateDirectConversationRequest) (*iamv1.CreateDirectConversationResponse, error) {
	callerID, err := getUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	peerID, err := uuid.Parse(req.GetPeerUserId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid peer_user_id: %v", err)
	}
	conv, err := h.createDirect.Handle(ctx, callerID, peerID)
	if err != nil {
		return nil, mapChatError(err)
	}
	return &iamv1.CreateDirectConversationResponse{Base: chatSuccessBase(), Data: h.convToProto(ctx, conv)}, nil
}

// CreateGroupConversation creates a group conversation.
func (h *ChatHandler) CreateGroupConversation(ctx context.Context, req *iamv1.CreateGroupConversationRequest) (*iamv1.CreateGroupConversationResponse, error) {
	callerID, err := getUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	memberIDs := make([]uuid.UUID, 0, len(req.GetParticipantIds()))
	for _, idStr := range req.GetParticipantIds() {
		uid, parseErr := uuid.Parse(idStr)
		if parseErr != nil {
			return nil, status.Errorf(codes.InvalidArgument, "invalid participant_id: %v", parseErr)
		}
		memberIDs = append(memberIDs, uid)
	}
	conv, err := h.createGroup.Handle(ctx, callerID, req.GetName(), memberIDs)
	if err != nil {
		return nil, mapChatError(err)
	}
	return &iamv1.CreateGroupConversationResponse{Base: chatSuccessBase(), Data: h.convToProto(ctx, conv)}, nil
}

// GetConversation returns a single conversation.
func (h *ChatHandler) GetConversation(ctx context.Context, req *iamv1.GetConversationRequest) (*iamv1.GetConversationResponse, error) {
	callerID, err := getUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	convID, err := uuid.Parse(req.GetConversationId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid conversation_id: %v", err)
	}
	conv, err := h.getConv.Handle(ctx, callerID, convID)
	if err != nil {
		return nil, mapChatError(err)
	}
	return &iamv1.GetConversationResponse{Base: chatSuccessBase(), Data: h.convToProto(ctx, conv)}, nil
}

// ListConversations returns paginated conversations.
func (h *ChatHandler) ListConversations(ctx context.Context, req *iamv1.ListConversationsRequest) (*iamv1.ListConversationsResponse, error) {
	callerID, err := getUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	page, pageSize := int(req.GetPage()), int(req.GetPageSize())
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	result, err := h.listConvs.Handle(ctx, callerID, page, pageSize)
	if err != nil {
		return nil, mapChatError(err)
	}
	protos := make([]*iamv1.ConversationProto, 0, len(result.Conversations))
	for _, c := range result.Conversations {
		protos = append(protos, h.convToProto(ctx, c))
	}
	totalPages := int32((result.Total + int64(pageSize) - 1) / int64(pageSize)) //nolint:gosec // bounded by page count
	return &iamv1.ListConversationsResponse{
		Base: chatSuccessBase(),
		Data: protos,
		Pagination: &commonv1.PaginationResponse{
			CurrentPage: int32(page),     //nolint:gosec // bounded by request validation
			PageSize:    int32(pageSize),  //nolint:gosec // bounded by request validation
			TotalItems:  result.Total,
			TotalPages:  totalPages,
		},
	}, nil
}

// LeaveConversation removes the caller from a conversation.
func (h *ChatHandler) LeaveConversation(ctx context.Context, req *iamv1.LeaveConversationRequest) (*iamv1.LeaveConversationResponse, error) {
	callerID, err := getUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	convID, err := uuid.Parse(req.GetConversationId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid conversation_id: %v", err)
	}
	if err := h.leaveConv.Handle(ctx, callerID, convID); err != nil {
		return nil, mapChatError(err)
	}
	return &iamv1.LeaveConversationResponse{Base: chatSuccessBase()}, nil
}

// SendMessage sends a message to a conversation.
func (h *ChatHandler) SendMessage(ctx context.Context, req *iamv1.SendMessageRequest) (*iamv1.SendMessageResponse, error) {
	callerID, err := getUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	convID, err := uuid.Parse(req.GetConversationId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid conversation_id: %v", err)
	}
	replyToID := uuid.Nil
	if req.GetReplyToId() != "" {
		replyToID, err = uuid.Parse(req.GetReplyToId())
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "invalid reply_to_id: %v", err)
		}
	}
	msg, err := h.sendMsg.Handle(ctx, callerID, convID, req.GetBody(), replyToID)
	if err != nil {
		return nil, mapChatError(err)
	}
	return &iamv1.SendMessageResponse{Base: chatSuccessBase(), Data: msgToProto(msg, req.GetBody(), nil)}, nil
}

// EditMessage edits an existing message.
func (h *ChatHandler) EditMessage(ctx context.Context, req *iamv1.EditMessageRequest) (*iamv1.EditMessageResponse, error) {
	callerID, err := getUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	convID, err := uuid.Parse(req.GetConversationId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid conversation_id: %v", err)
	}
	msgID, err := uuid.Parse(req.GetMessageId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid message_id: %v", err)
	}
	msg, err := h.editMsg.Handle(ctx, callerID, convID, msgID, req.GetBody())
	if err != nil {
		return nil, mapChatError(err)
	}
	return &iamv1.EditMessageResponse{Base: chatSuccessBase(), Data: msgToProto(msg, req.GetBody(), nil)}, nil
}

// DeleteMessage soft-deletes a message.
func (h *ChatHandler) DeleteMessage(ctx context.Context, req *iamv1.DeleteMessageRequest) (*iamv1.DeleteMessageResponse, error) {
	callerID, err := getUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	convID, err := uuid.Parse(req.GetConversationId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid conversation_id: %v", err)
	}
	msgID, err := uuid.Parse(req.GetMessageId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid message_id: %v", err)
	}
	if err := h.deleteMsg.Handle(ctx, callerID, convID, msgID); err != nil {
		return nil, mapChatError(err)
	}
	return &iamv1.DeleteMessageResponse{Base: chatSuccessBase()}, nil
}

// ListMessages returns paginated messages for a conversation.
func (h *ChatHandler) ListMessages(ctx context.Context, req *iamv1.ListMessagesRequest) (*iamv1.ListMessagesResponse, error) {
	callerID, err := getUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	convID, err := uuid.Parse(req.GetConversationId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid conversation_id: %v", err)
	}
	pageSize := int(req.GetPageSize())
	if pageSize < 1 {
		pageSize = 30
	}
	result, err := h.listMsgs.Handle(ctx, callerID, convID, pageSize, req.GetBeforeCursor())
	if err != nil {
		return nil, mapChatError(err)
	}
	protos := make([]*iamv1.MessageProto, 0, len(result.Messages))
	for _, dm := range result.Messages {
		protos = append(protos, msgToProto(dm.Message, dm.PlainBody, dm.ReadReceipts()))
	}
	return &iamv1.ListMessagesResponse{
		Base:       chatSuccessBase(),
		Data:       protos,
		NextCursor: result.NextCursor,
		HasMore:    result.HasMore,
	}, nil
}

// MarkConversationRead marks all messages as read.
func (h *ChatHandler) MarkConversationRead(ctx context.Context, req *iamv1.MarkConversationReadRequest) (*iamv1.MarkConversationReadResponse, error) {
	callerID, err := getUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	convID, err := uuid.Parse(req.GetConversationId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid conversation_id: %v", err)
	}
	if err := h.markRead.Handle(ctx, callerID, convID); err != nil {
		return nil, mapChatError(err)
	}
	return &iamv1.MarkConversationReadResponse{Base: chatSuccessBase()}, nil
}

// SetTyping sets the typing indicator.
func (h *ChatHandler) SetTyping(ctx context.Context, req *iamv1.SetTypingRequest) (*iamv1.SetTypingResponse, error) {
	callerID, err := getUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	convID, err := uuid.Parse(req.GetConversationId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid conversation_id: %v", err)
	}
	if err := h.setTyping.Handle(ctx, callerID, convID, req.GetIsTyping(), ""); err != nil {
		return nil, mapChatError(err)
	}
	return &iamv1.SetTypingResponse{Base: chatSuccessBase()}, nil
}

// StreamChatEvents opens a server-streaming connection for chat events.
func (h *ChatHandler) StreamChatEvents(_ *iamv1.StreamChatEventsRequest, stream iamv1.ChatService_StreamChatEventsServer) error {
	ctx := stream.Context()
	callerID, err := getUserIDFromContext(ctx)
	if err != nil {
		return err
	}
	ch, unsub := h.stream.Subscribe(callerID)
	defer unsub()
	for {
		select {
		case <-ctx.Done():
			return nil
		case evt, ok := <-ch:
			if !ok {
				return nil
			}
			if sendErr := stream.Send(evt.Response); sendErr != nil {
				return sendErr
			}
		}
	}
}

func (h *ChatHandler) convToProto(ctx context.Context, conv *chat.Conversation) *iamv1.ConversationProto {
	userIDs := make([]uuid.UUID, 0, len(conv.Participants()))
	for _, p := range conv.Participants() {
		userIDs = append(userIDs, p.UserID())
	}
	userMap, _ := h.userResolver.ResolveUsers(ctx, userIDs)

	parts := make([]*iamv1.ParticipantProto, 0, len(conv.Participants()))
	for _, p := range conv.Participants() {
		pp := &iamv1.ParticipantProto{
			UserId:   p.UserID().String(),
			Role:     p.Role().String(),
			JoinedAt: p.JoinedAt().UTC().Format(time.RFC3339Nano),
		}
		if info := userMap[p.UserID()]; info != nil {
			pp.Username = info.Username
			pp.FullName = info.FullName
			pp.AvatarUrl = info.AvatarURL
		}
		parts = append(parts, pp)
	}
	return &iamv1.ConversationProto{
		ConversationId: conv.ID().String(),
		Type:           conv.Type().String(),
		Name:           conv.Name(),
		AvatarUrl:      conv.AvatarURL(),
		Participants:   parts,
		CreatedAt:      conv.CreatedAt().UTC().Format(time.RFC3339Nano),
		UpdatedAt:      conv.UpdatedAt().UTC().Format(time.RFC3339Nano),
	}
}

func msgToProto(msg *chat.Message, plainBody string, receipts []*chat.ReadReceipt) *iamv1.MessageProto {
	receiptProtos := make([]*iamv1.ReadReceiptProto, 0, len(receipts))
	for _, r := range receipts {
		receiptProtos = append(receiptProtos, &iamv1.ReadReceiptProto{
			UserId: r.UserID().String(),
			ReadAt: r.ReadAt().UTC().Format(time.RFC3339Nano),
		})
	}
	replyTo := ""
	if msg.ReplyToID() != uuid.Nil {
		replyTo = msg.ReplyToID().String()
	}
	return &iamv1.MessageProto{
		MessageId:      msg.MessageID().String(),
		ConversationId: msg.ConversationID().String(),
		SenderUserId:   msg.SenderUserID().String(),
		Body:           plainBody,
		IsEdited:       msg.IsEdited(),
		IsDeleted:      msg.IsDeleted(),
		ReplyToId:      replyTo,
		ReadReceipts:   receiptProtos,
		CreatedAt:      msg.CreatedAt().UTC().Format(time.RFC3339Nano),
		UpdatedAt:      msg.UpdatedAt().UTC().Format(time.RFC3339Nano),
	}
}

func chatSuccessBase() *commonv1.BaseResponse {
	return &commonv1.BaseResponse{IsSuccess: true, Message: "success", StatusCode: "200"}
}

func mapChatError(err error) error {
	switch {
	case errors.Is(err, chat.ErrConversationNotFound), errors.Is(err, chat.ErrMessageNotFound):
		return status.Errorf(codes.NotFound, "%v", err)
	case errors.Is(err, chat.ErrNotParticipant), errors.Is(err, chat.ErrNotAuthor), errors.Is(err, chat.ErrNotAdmin):
		return status.Errorf(codes.PermissionDenied, "%v", err)
	case errors.Is(err, chat.ErrDirectConversationFull), errors.Is(err, chat.ErrAlreadyParticipant):
		return status.Errorf(codes.FailedPrecondition, "%v", err)
	default:
		log.Error().Err(err).Msg("chat handler: unhandled error")
		return status.Errorf(codes.Internal, "internal error")
	}
}
