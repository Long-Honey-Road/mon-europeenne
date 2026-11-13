package jobs

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/testredis"
)

func TestOneActiveTurnPerThread(t *testing.T) {
	rdb := redis.NewClient(&redis.Options{Addr: testredis.Addr(t)})
	defer rdb.Close()
	turns := NewActiveTurns(rdb, time.Minute)
	ctx := context.Background()

	first, err := turns.Reserve(ctx, "t-active")
	if err != nil || first == "" {
		t.Fatalf("Reserve = %q, %v", first, err)
	}
	if _, err := turns.Reserve(ctx, "t-active"); !errors.Is(err, ErrTurnRunning) {
		t.Fatalf("second Reserve: %v, want ErrTurnRunning", err)
	}
	if other, err := turns.Reserve(ctx, "t-other"); err != nil || other == first {
		t.Errorf("another thread: %q, %v", other, err)
	}
	if ttl := rdb.TTL(ctx, activeKey("t-active")).Val(); ttl <= 0 || ttl > time.Minute {
		t.Errorf("ttl = %v, want the lock to expire within a minute", ttl)
	}

	if err := turns.Release(ctx, "t-active", "someone-else"); err != nil {
		t.Fatalf("Release of a stale job: %v", err)
	}
	if _, err := turns.Reserve(ctx, "t-active"); !errors.Is(err, ErrTurnRunning) {
		t.Fatalf("a stale job freed the thread: %v", err)
	}
	if err := turns.Release(ctx, "t-active", first); err != nil {
		t.Fatalf("Release: %v", err)
	}
	if next, err := turns.Reserve(ctx, "t-active"); err != nil || next == first {
		t.Fatalf("Reserve after release = %q, %v", next, err)
	}
}

func TestActiveTurnsWithoutRedis(t *testing.T) {
	rdb := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1", MaxRetries: -1})
	defer rdb.Close()
	turns := NewActiveTurns(rdb, time.Minute)
	if _, err := turns.Reserve(context.Background(), "t1"); err == nil || errors.Is(err, ErrTurnRunning) {
		t.Fatalf("Reserve without Redis: %v", err)
	}
}

func TestTaskTimeoutCoversQueueAndTurn(t *testing.T) {
	if got := TaskTimeout(5*time.Minute, 80*time.Second); got < 5*time.Minute+80*time.Second {
		t.Errorf("TaskTimeout = %v", got)
	}
}
