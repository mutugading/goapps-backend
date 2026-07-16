// Package chat provides in-memory + Redis Streams fan-out for chat events and
// Redis TTL-based presence tracking.
//
// Each authenticated user can have multiple subscribers (open SSE streams in
// different tabs). Publish fans out to all of a recipient's in-memory
// subscribers, then writes to a per-user Redis Stream so other IAM pods
// (and reconnecting clients) can replay missed events.
//
// Redis Streams (XADD/XREAD) replace the previous Pub/Sub approach:
// events are durable (trimmed to last 500 per user), replayable via
// lastEventId, and support multi-pod consumer reads without consumer groups
// (each pod reads independently using its own cursor).
package chat

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog/log"
	"google.golang.org/protobuf/proto"

	iamv1 "github.com/mutugading/goapps-backend/gen/iam/v1"
)

const (
	chatStreamPrefix = "iam:chatstream:"
	streamMaxLen     = 500
	streamBlockTime  = 5 * time.Second
)

// Event is a chat event fanned out to subscribers.
type Event struct {
	EventID  string
	UserID   uuid.UUID
	Response *iamv1.StreamChatEventsResponse
}

// Broadcaster fans out chat events via in-memory channels and Redis Streams.
type Broadcaster struct {
	mu        sync.RWMutex
	subs      map[uuid.UUID]map[chan *Event]struct{}
	rdb       *redis.Client
	selfPodID string
}

// NewBroadcaster returns an in-memory-only Broadcaster.
func NewBroadcaster() *Broadcaster {
	return &Broadcaster{
		subs: make(map[uuid.UUID]map[chan *Event]struct{}),
	}
}

// NewRedisBroadcaster returns a Broadcaster backed by Redis Streams.
func NewRedisBroadcaster(rdb *redis.Client) *Broadcaster {
	return &Broadcaster{
		subs:      make(map[uuid.UUID]map[chan *Event]struct{}),
		rdb:       rdb,
		selfPodID: resolvePodID(),
	}
}

func resolvePodID() string {
	h, err := os.Hostname()
	if err != nil || h == "" {
		return uuid.New().String()
	}
	return h
}

// Subscribe registers a subscriber for a user and returns the event channel
// and an unsubscribe function.
func (b *Broadcaster) Subscribe(recipient uuid.UUID) (<-chan *Event, func()) {
	const bufferSize = 32
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

	cancelRedis := b.bridgeRedisStream(recipient, ch)
	return ch, func() {
		cancelRedis()
		unsubMem()
	}
}

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

// Publish fans out an event in-memory and writes to Redis Stream.
func (b *Broadcaster) Publish(e *Event) {
	if e == nil {
		return
	}
	b.publishInMemory(e)
	if b.rdb != nil {
		b.publishToStream(e)
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

// SubscriberCount returns the number of in-memory subscribers for a user.
func (b *Broadcaster) SubscriberCount(recipient uuid.UUID) int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.subs[recipient])
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
		}
	}
}

func (b *Broadcaster) publishToStream(e *Event) {
	protoBytes, err := proto.Marshal(e.Response)
	if err != nil {
		log.Warn().Err(err).Msg("broadcaster: marshal proto for stream")
		return
	}
	streamKey := chatStreamPrefix + e.UserID.String()
	err = b.rdb.XAdd(context.Background(), &redis.XAddArgs{
		Stream: streamKey,
		MaxLen: streamMaxLen,
		Approx: true,
		Values: map[string]any{
			"eid":  e.EventID,
			"pod":  b.selfPodID,
			"data": base64.StdEncoding.EncodeToString(protoBytes),
		},
	}).Err()
	if err != nil {
		log.Warn().Err(err).Msg("broadcaster: XADD to Redis Stream")
	}
}

// bridgeRedisStream reads from a per-user Redis Stream and forwards events
// from other pods into ch. Starts reading from "$" (new events only).
func (b *Broadcaster) bridgeRedisStream(recipient uuid.UUID, ch chan *Event) context.CancelFunc {
	ctx, cancel := context.WithCancel(context.Background())
	streamKey := chatStreamPrefix + recipient.String()

	go func() {
		lastID := "$"
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}
			streams, err := b.rdb.XRead(ctx, &redis.XReadArgs{
				Streams: []string{streamKey, lastID},
				Block:   streamBlockTime,
				Count:   50,
			}).Result()
			if err != nil {
				if err == redis.Nil || ctx.Err() != nil {
					continue
				}
				log.Warn().Err(err).Msg("broadcaster: XREAD error")
				time.Sleep(time.Second)
				continue
			}
			for _, stream := range streams {
				for _, msg := range stream.Messages {
					lastID = msg.ID
					b.handleStreamMessage(msg, recipient, ch)
				}
			}
		}
	}()

	return cancel
}

func (b *Broadcaster) handleStreamMessage(msg redis.XMessage, userID uuid.UUID, ch chan *Event) {
	podID, _ := msg.Values["pod"].(string)
	if podID == b.selfPodID {
		return
	}
	eventID, _ := msg.Values["eid"].(string)
	dataB64, _ := msg.Values["data"].(string)
	protoBytes, err := base64.StdEncoding.DecodeString(dataB64)
	if err != nil {
		log.Warn().Err(err).Msg("broadcaster: decode stream data")
		return
	}
	resp := &iamv1.StreamChatEventsResponse{}
	if err := proto.Unmarshal(protoBytes, resp); err != nil {
		log.Warn().Err(err).Msg("broadcaster: unmarshal stream proto")
		return
	}
	resp.EventId = eventID
	evt := &Event{EventID: eventID, UserID: userID, Response: resp}
	select {
	case ch <- evt:
	default:
	}
}
