package chat_test

import (
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"

	iamv1 "github.com/mutugading/goapps-backend/gen/iam/v1"
	chatinfra "github.com/mutugading/goapps-backend/services/iam/internal/infrastructure/chat"
)

func TestBroadcaster_PublishReceive(t *testing.T) {
	t.Parallel()
	b := chatinfra.NewBroadcaster()
	user := uuid.New()
	ch, unsub := b.Subscribe(user)
	defer unsub()

	resp := &iamv1.StreamChatEventsResponse{EventId: "test-event"}
	ev := &chatinfra.Event{
		EventID:  uuid.New().String(),
		UserID:   user,
		Response: resp,
	}
	b.Publish(ev)

	select {
	case got := <-ch:
		assert.Equal(t, ev.EventID, got.EventID)
		assert.Equal(t, ev.UserID, got.UserID)
		assert.Equal(t, "test-event", got.Response.GetEventId())
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timed out waiting for chat event")
	}
}

func TestBroadcaster_MultipleSubscribers(t *testing.T) {
	t.Parallel()
	b := chatinfra.NewBroadcaster()
	user := uuid.New()

	ch1, unsub1 := b.Subscribe(user)
	defer unsub1()
	ch2, unsub2 := b.Subscribe(user)
	defer unsub2()

	assert.Equal(t, 2, b.SubscriberCount(user))

	ev := &chatinfra.Event{
		EventID:  uuid.New().String(),
		UserID:   user,
		Response: &iamv1.StreamChatEventsResponse{EventId: "multi-test"},
	}
	b.Publish(ev)

	var wg sync.WaitGroup
	wg.Add(2)
	for _, ch := range []<-chan *chatinfra.Event{ch1, ch2} {
		go func(ch <-chan *chatinfra.Event) {
			defer wg.Done()
			select {
			case got := <-ch:
				assert.Equal(t, ev.EventID, got.EventID)
			case <-time.After(100 * time.Millisecond):
				t.Error("subscriber didn't receive event")
			}
		}(ch)
	}
	wg.Wait()
}
