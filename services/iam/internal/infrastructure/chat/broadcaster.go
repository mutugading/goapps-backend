// Package chat provides in-memory pub/sub fan-out and Redis-backed presence
// tracking for the chat stream.
//
// Each authenticated user can have multiple subscribers (open SSE streams in
// different tabs). Publish fans out to all of a recipient's in-memory subscribers.
//
// When a Redis client is provided (via NewRedisBroadcaster), events are also
// published to a Redis pub/sub channel so that other IAM pods pick them up.
// This allows HPA to scale IAM to multiple replicas without losing SSE events.
// Same-pod events are deduplicated via the originating pod ID embedded in the
// Redis message, so subscribers receive each event exactly once.
package chat

import (
	"context"
	"fmt"
	"os"
	"sync"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog/log"
	"google.golang.org/protobuf/proto"

	iamv1 "github.com/mutugading/goapps-backend/gen/iam/v1"
)

const chatChannelPrefix = "iam:chat:"

// Event is a chat event fanned out to subscribers. Response carries the
// typed proto response for zero-copy forwarding to gRPC streams.
type Event struct {
	EventID  string
	UserID   uuid.UUID
	Response *iamv1.StreamChatEventsResponse
}

// Broadcaster is the in-memory event bus.
//
// For multi-replica IAM deployments, construct it with NewRedisBroadcaster so
// that events published on one pod are received by all pods via Redis pub/sub.
// For single-replica deployments, NewBroadcaster (in-memory only) is sufficient.
type Broadcaster struct {
	mu        sync.RWMutex
	subs      map[uuid.UUID]map[chan *Event]struct{}
	rdb       *redis.Client // nil → in-memory only
	selfPodID string
}

// NewBroadcaster returns a fresh in-memory-only Broadcaster.
func NewBroadcaster() *Broadcaster {
	return &Broadcaster{
		subs: make(map[uuid.UUID]map[chan *Event]struct{}),
	}
}

// NewRedisBroadcaster returns a Broadcaster backed by Redis pub/sub for
// cross-pod fan-out. Pass the same *redis.Client used for the session cache.
// If rdb is nil, falls back to in-memory behavior.
func NewRedisBroadcaster(rdb *redis.Client) *Broadcaster {
	return &Broadcaster{
		subs:      make(map[uuid.UUID]map[chan *Event]struct{}),
		rdb:       rdb,
		selfPodID: resolvePodID(),
	}
}

// resolvePodID returns the K8s pod name (os.Hostname) or a random UUID fallback.
func resolvePodID() string {
	h, err := os.Hostname()
	if err != nil || h == "" {
		return uuid.New().String()
	}
	return h
}

// Subscribe registers a new subscriber for recipient.
// Returns the receive channel and an unsubscribe function the caller must invoke
// (typically via defer) when the stream ends.
//
// If a Redis client is configured, a background goroutine bridges events from
// other pods into the returned channel. Events from this pod are excluded to
// prevent duplicates (they arrive via the in-memory path already).
func (b *Broadcaster) Subscribe(recipient uuid.UUID) (<-chan *Event, func()) {
	const bufferSize = 16
	ch := make(chan *Event, bufferSize)

	b.mu.Lock()
	if _, ok := b.subs[recipient]; !ok {
		b.subs[recipient] = make(map[chan *Event]struct{})
	}
	b.subs[recipient][ch] = struct{}{}
	b.mu.Unlock()

	unsubMem := b.makeUnsubFunc(recipient, ch)

	if b.rdb == nil {
		return ch, unsubMem
	}

	cancelRedis := b.bridgeRedis(recipient, ch)
	return ch, func() {
		cancelRedis()
		unsubMem()
	}
}

// makeUnsubFunc returns a function that removes ch from the in-memory subscriber
// set for recipient and closes the channel.
func (b *Broadcaster) makeUnsubFunc(recipient uuid.UUID, ch chan *Event) func() {
	return func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		set, ok := b.subs[recipient]
		if !ok {
			return
		}
		delete(set, ch)
		if len(set) == 0 {
			delete(b.subs, recipient)
		}
		close(ch)
	}
}

// Publish fans out an event to all in-memory subscribers of its recipient,
// then publishes to the Redis channel so other pods can forward it to their
// in-memory subscribers. Non-blocking: slow consumers drop the event.
func (b *Broadcaster) Publish(e *Event) {
	if e == nil {
		return
	}
	b.publishInMemory(e)
	if b.rdb != nil {
		b.publishRedis(e)
	}
}

// PublishToConversation fans out the same event to every participant.
func (b *Broadcaster) PublishToConversation(participantIDs []uuid.UUID, eventID string, resp *iamv1.StreamChatEventsResponse) {
	for _, userID := range participantIDs {
		b.Publish(&Event{
			EventID:  fmt.Sprintf("%s-%s", eventID, userID),
			UserID:   userID,
			Response: resp,
		})
	}
}

func (b *Broadcaster) publishInMemory(e *Event) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	set, ok := b.subs[e.UserID]
	if !ok {
		return
	}
	for ch := range set {
		select {
		case ch <- e:
		default:
			// drop — subscriber will resync via DB on reconnect
		}
	}
}

func (b *Broadcaster) publishRedis(e *Event) {
	payload, err := serializeEvent(e, b.selfPodID)
	if err != nil {
		log.Warn().Err(err).Msg("broadcaster: failed to serialize chat event for Redis pub/sub")
		return
	}
	channel := chatChannelPrefix + e.UserID.String()
	if err := b.rdb.Publish(context.Background(), channel, payload).Err(); err != nil {
		log.Warn().Err(err).Msg("broadcaster: failed to publish chat event to Redis")
	}
}

// bridgeRedis subscribes to the Redis pub/sub channel for recipient and
// forwards events from other pods into ch. Returns a cancel function that
// terminates the bridge goroutine.
func (b *Broadcaster) bridgeRedis(recipient uuid.UUID, ch chan *Event) context.CancelFunc {
	ctx, cancel := context.WithCancel(context.Background())
	channel := chatChannelPrefix + recipient.String()
	pubsub := b.rdb.Subscribe(ctx, channel)

	go func() {
		defer func() {
			if err := pubsub.Close(); err != nil {
				log.Warn().Err(err).Msg("broadcaster: Redis pubsub close error")
			}
		}()
		msgCh := pubsub.Channel()
		for {
			select {
			case <-ctx.Done():
				return
			case msg, ok := <-msgCh:
				if !ok {
					return
				}
				b.handleRedisMessage(msg.Payload, ch)
			}
		}
	}()

	return cancel
}

func (b *Broadcaster) handleRedisMessage(payload string, ch chan *Event) {
	e, podID, err := deserializeEvent(payload)
	if err != nil {
		log.Warn().Err(err).Msg("broadcaster: failed to deserialize Redis chat event")
		return
	}
	if podID == b.selfPodID {
		return // already delivered via in-memory fan-out
	}
	select {
	case ch <- e:
	default:
		// drop — subscriber will resync via DB on reconnect
	}
}

// SubscriberCount returns the total number of in-memory subscribers for recipient.
func (b *Broadcaster) SubscriberCount(recipient uuid.UUID) int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.subs[recipient])
}

func serializeEvent(e *Event, podID string) (string, error) {
	protoBytes, err := proto.Marshal(e.Response)
	if err != nil {
		return "", fmt.Errorf("marshal proto event: %w", err)
	}
	envelope := fmt.Sprintf("%s\x00%s\x00%s\x00", podID, e.EventID, e.UserID.String())
	return envelope + string(protoBytes), nil
}

func deserializeEvent(payload string) (*Event, string, error) {
	parts := splitNullSep(payload, 4)
	if parts == nil {
		return nil, "", fmt.Errorf("invalid redis envelope")
	}
	podID, eventID, userIDStr, protoBytes := parts[0], parts[1], parts[2], []byte(parts[3])
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		return nil, "", fmt.Errorf("parse user id: %w", err)
	}
	resp := &iamv1.StreamChatEventsResponse{}
	if err := proto.Unmarshal(protoBytes, resp); err != nil {
		return nil, "", fmt.Errorf("unmarshal proto event: %w", err)
	}
	resp.EventId = eventID
	return &Event{EventID: eventID, UserID: userID, Response: resp}, podID, nil
}

func splitNullSep(s string, n int) []string {
	result := make([]string, 0, n)
	for i := 0; i < n-1; i++ {
		idx := 0
		for idx < len(s) && s[idx] != 0 {
			idx++
		}
		if idx >= len(s) {
			return nil
		}
		result = append(result, s[:idx])
		s = s[idx+1:]
	}
	result = append(result, s)
	return result
}
