// Package stream carries live turn events from the worker to SSE clients over Redis pub/sub.
package stream

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/redis/go-redis/v9"
)

// Event is one SSE event: Type becomes the SSE event name, Data its JSON payload.
type Event struct {
	Type string         `json:"type"`
	Data map[string]any `json:"data"`
}

type RedisBus struct{ c *redis.Client }

func NewRedisBus(c *redis.Client) *RedisBus { return &RedisBus{c: c} }

func channel(threadID string) string { return "thread:" + threadID }

func (b *RedisBus) Publish(ctx context.Context, threadID string, e Event) error {
	raw, err := json.Marshal(e)
	if err != nil {
		return err
	}
	return b.c.Publish(ctx, channel(threadID), raw).Err()
}

// Subscribe returns once Redis confirmed the subscription, so nothing published afterwards is lost.
// stop unsubscribes and closes the channel.
func (b *RedisBus) Subscribe(ctx context.Context, threadID string) (<-chan Event, func(), error) {
	ps := b.c.Subscribe(ctx, channel(threadID))
	if _, err := ps.Receive(ctx); err != nil {
		_ = ps.Close()
		return nil, nil, fmt.Errorf("subscribe %s: %w", threadID, err)
	}
	out := make(chan Event, 64)
	go func() {
		defer close(out)
		for msg := range ps.Channel() {
			var e Event
			if json.Unmarshal([]byte(msg.Payload), &e) == nil {
				out <- e
			}
		}
	}()
	var once sync.Once
	return out, func() { once.Do(func() { _ = ps.Close() }) }, nil
}
