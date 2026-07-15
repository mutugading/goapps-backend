package chat

import (
	"github.com/google/uuid"

	chatinfra "github.com/mutugading/goapps-backend/services/iam/internal/infrastructure/chat"
)

// StreamHandler subscribes a user to real-time chat events via the broadcaster.
type StreamHandler struct {
	broadcaster *chatinfra.Broadcaster
}

// NewStreamHandler constructs the handler.
func NewStreamHandler(broadcaster *chatinfra.Broadcaster) *StreamHandler {
	return &StreamHandler{broadcaster: broadcaster}
}

// Subscribe returns an event channel and unsub func for the given user.
func (h *StreamHandler) Subscribe(userID uuid.UUID) (<-chan *chatinfra.Event, func()) {
	return h.broadcaster.Subscribe(userID)
}
