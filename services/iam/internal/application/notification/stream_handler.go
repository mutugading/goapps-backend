package notification

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/iam/internal/domain/notification"
	notifinfra "github.com/mutugading/goapps-backend/services/iam/internal/infrastructure/notification"
)

// StreamEvent is an event delivered through the stream.
type StreamEvent struct {
	EventID      string                     // sortable resume cursor
	Notification *notification.Notification // nil when IsHeartbeat
	IsHeartbeat  bool
}

// StreamHandler subscribes to a recipient's notification stream and emits
// catchup events from the DB followed by realtime events from the broadcaster.
//
// The handler is consumed by the gRPC layer which forwards each StreamEvent
// to the gRPC server-streaming response and (in turn) to the BFF SSE bridge.
type StreamHandler struct {
	repo            notification.Repository
	broadcaster     *notifinfra.Broadcaster
	heartbeatPeriod time.Duration
}

// NewStreamHandler constructs the handler. Pass heartbeatPeriod=0 to disable
// heartbeats (default 30s when zero is passed via the public ctor).
func NewStreamHandler(repo notification.Repository, b *notifinfra.Broadcaster, heartbeatPeriod time.Duration) *StreamHandler {
	if heartbeatPeriod <= 0 {
		heartbeatPeriod = 30 * time.Second
	}
	return &StreamHandler{repo: repo, broadcaster: b, heartbeatPeriod: heartbeatPeriod}
}

// Handle drives the stream lifecycle. The emit callback is invoked for each
// event that should be sent to the client; if it returns an error the stream
// terminates. Returns when ctx is canceled or emit fails.
//
// `since` is an optional cursor matching a previous EventID; pass empty string
// to skip catchup and only receive realtime events.
func (h *StreamHandler) Handle(
	ctx context.Context,
	recipient uuid.UUID,
	since string,
	emit func(StreamEvent) error,
) error {
	if recipient == uuid.Nil {
		return notification.ErrEmptyRecipient
	}

	// Subscribe FIRST so events published while we run catchup aren't lost.
	ch, unsub := h.broadcaster.Subscribe(recipient)
	defer unsub()

	// Catchup from DB if `since` is provided.
	if since != "" {
		after, err := decodeCursor(since)
		if err == nil { // bad cursor → skip catchup, start realtime
			if cerr := h.replay(ctx, recipient, after, emit); cerr != nil {
				return cerr
			}
		}
	}

	// Heartbeat ticker.
	tick := time.NewTicker(h.heartbeatPeriod)
	defer tick.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case n, ok := <-ch:
			if !ok {
				return nil // unsubscribed
			}
			if err := emit(StreamEvent{EventID: encodeCursor(n.CreatedAt(), n.ID()), Notification: n}); err != nil {
				return fmt.Errorf("emit notification: %w", err)
			}
		case t := <-tick.C:
			if err := emit(StreamEvent{EventID: encodeCursor(t, uuid.Nil), IsHeartbeat: true}); err != nil {
				return fmt.Errorf("emit heartbeat: %w", err)
			}
		}
	}
}

// replay walks DB rows newer than `after` in ascending order and emits each.
// Page size 100 — chunked to bound memory.
func (h *StreamHandler) replay(ctx context.Context, recipient uuid.UUID, after time.Time, emit func(StreamEvent) error) error {
	page := 1
	const pageSize = 100
	for {
		items, _, err := h.repo.ListByRecipient(ctx, recipient, notification.ListFilter{
			Page:     page,
			PageSize: pageSize,
			After:    &after,
			SortDesc: false, // ascending — oldest first so cursor is monotonic
		})
		if err != nil {
			return fmt.Errorf("catchup query: %w", err)
		}
		for _, n := range items {
			if err := emit(StreamEvent{EventID: encodeCursor(n.CreatedAt(), n.ID()), Notification: n}); err != nil {
				return err
			}
		}
		if len(items) < pageSize {
			return nil
		}
		page++
	}
}

// encodeCursor produces a sortable string cursor: "RFC3339Nano|uuid".
// Heartbeats use uuid.Nil so they're distinguishable but still sortable.
func encodeCursor(t time.Time, id uuid.UUID) string {
	return t.UTC().Format(time.RFC3339Nano) + "|" + id.String()
}

// decodeCursor parses a cursor produced by encodeCursor and returns the
// embedded timestamp.
func decodeCursor(s string) (time.Time, error) {
	for i, c := range s {
		if c == '|' {
			return time.Parse(time.RFC3339Nano, s[:i])
		}
	}
	return time.Time{}, fmt.Errorf("invalid cursor: %q", s)
}
