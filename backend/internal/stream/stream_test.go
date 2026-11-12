package stream

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/testredis"
)

func TestMain(m *testing.M) { os.Exit(testredis.Main(m)) }

func newBus(t *testing.T) *RedisBus {
	t.Helper()
	c := redis.NewClient(&redis.Options{Addr: testredis.Addr(t)})
	t.Cleanup(func() { _ = c.Close() })
	return NewRedisBus(c)
}

func next(t *testing.T, ch <-chan Event) Event {
	t.Helper()
	select {
	case e, ok := <-ch:
		if !ok {
			t.Fatal("channel closed")
		}
		return e
	case <-time.After(5 * time.Second):
		t.Fatal("no event within 5s")
	}
	return Event{}
}

func TestSubscriberGetsEventsOfItsThreadInOrder(t *testing.T) {
	bus := newBus(t)
	ctx := context.Background()
	events, stop, err := bus.Subscribe(ctx, "thread-a")
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer stop()

	for _, e := range []Event{
		{Type: "status", Data: map[string]any{"text": "ищу в документах"}},
		{Type: "delta", Data: map[string]any{"text": "Привет"}},
	} {
		if err := bus.Publish(ctx, "thread-a", e); err != nil {
			t.Fatalf("Publish: %v", err)
		}
	}
	if err := bus.Publish(ctx, "thread-b", Event{Type: "delta", Data: map[string]any{"text": "чужое"}}); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if err := bus.Publish(ctx, "thread-a", Event{Type: "done", Data: map[string]any{"message_id": "m1"}}); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	var got []string
	for range 3 {
		got = append(got, next(t, events).Type)
	}
	if got[0] != "status" || got[1] != "delta" || got[2] != "done" {
		t.Fatalf("events = %v", got)
	}
}

func TestStopClosesTheChannel(t *testing.T) {
	bus := newBus(t)
	events, stop, err := bus.Subscribe(context.Background(), "thread-c")
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	stop()

	select {
	case _, ok := <-events:
		if ok {
			t.Fatal("event after stop")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("channel not closed after stop")
	}
}

func TestPublishFailsWithoutRedis(t *testing.T) {
	c := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1", DialTimeout: 200 * time.Millisecond})
	defer c.Close()
	if err := NewRedisBus(c).Publish(context.Background(), "x", Event{Type: "delta"}); err == nil {
		t.Fatal("publish to a closed port succeeded")
	}
}
