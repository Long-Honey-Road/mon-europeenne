package jobs

import (
	"context"
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

	client := NewClient(addr)
	defer client.Close()
	jobID, err := client.EnqueueChatTurn(context.Background(), ChatTurn{ThreadID: "t1", UserID: "u-liza", MessageID: msgID, Mode: "brief"})
	if err != nil || jobID == "" {
		t.Fatalf("EnqueueChatTurn = %q, %v", jobID, err)
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
				return
			}
		case <-deadline:
			t.Fatal("no done event within 15s")
		}
	}
}

func TestEnqueueFailsWithoutRedis(t *testing.T) {
	client := NewClient("127.0.0.1:1")
	defer client.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if _, err := client.EnqueueChatTurn(ctx, ChatTurn{ThreadID: "t1"}); err == nil {
		t.Fatal("enqueue without Redis succeeded")
	}
}
