package jobs

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/llm/fake"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/stream"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/testredis"
)

func TestMain(m *testing.M) { os.Exit(testredis.Main(m)) }

// The whole path: client enqueues, the asynq worker runs the turn, a subscriber gets "done".
func TestQueuedTurnReachesSubscriber(t *testing.T) {
	addr := testredis.Addr(t)
	rdb := redis.NewClient(&redis.Options{Addr: addr})
	defer rdb.Close()
	bus := stream.NewRedisBus(rdb)

	st, msgID := newStore("Привет")
	h, _ := handler(t, st, fake.New(fake.Text("Привет! Я buddy.")))
	h.d.Publisher = bus
	h.d.Turns = NewActiveTurns(rdb, time.Minute)

	srv := NewServer(addr, 2)
	if err := srv.Start(Mux(h)); err != nil {
		t.Fatalf("start worker: %v", err)
	}
	defer srv.Shutdown()

	events, stop, err := bus.Subscribe(context.Background(), "t1")
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer stop()

	client := NewClient(addr, time.Minute)
	defer client.Close()
	jobID, err := client.ReserveChatTurn(context.Background(), "t1")
	if err != nil || jobID == "" {
		t.Fatalf("ReserveChatTurn = %q, %v", jobID, err)
	}
	if _, err := client.ReserveChatTurn(context.Background(), "t1"); !errors.Is(err, ErrTurnRunning) {
		t.Fatalf("second turn in the thread: %v", err)
	}
	if err := client.EnqueueChatTurn(context.Background(), jobID, ChatTurn{ThreadID: "t1", UserID: "u-liza", MessageID: msgID, Mode: "brief"}); err != nil {
		t.Fatalf("EnqueueChatTurn: %v", err)
	}

	deadline := time.After(15 * time.Second)
	for {
		select {
		case e := <-events:
			if e.Data["job_id"] != jobID {
				t.Fatalf("event %s with job %v, want %s", e.Type, e.Data["job_id"], jobID)
			}
			if e.Type == "done" {
				if e.Data["text"] != "Привет! Я buddy." {
					t.Errorf("done = %v", e.Data)
				}
				next, err := client.ReserveChatTurn(context.Background(), "t1")
				if err != nil {
					t.Fatalf("thread still busy after done: %v", err)
				}
				if err := client.ReleaseChatTurn(context.Background(), "t1", next); err != nil {
					t.Errorf("ReleaseChatTurn: %v", err)
				}
				return
			}
		case <-deadline:
			t.Fatal("no done event within 15s")
		}
	}
}

func TestEnqueueFailsWithoutRedis(t *testing.T) {
	client := NewClient("127.0.0.1:1", time.Minute)
	defer client.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := client.EnqueueChatTurn(ctx, "job-1", ChatTurn{ThreadID: "t1"}); err == nil {
		t.Fatal("enqueue without Redis succeeded")
	}
}
