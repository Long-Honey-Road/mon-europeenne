package main

import (
	"context"
	"time"

	"github.com/hibiken/asynq"
	"github.com/redis/go-redis/v9"

	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/requests"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/requests/corpsim"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/store"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/stream"
	"github.com/Long-Honey-Road/mon-europeenne/backend/seed"
)

// Wiring of the removable requests module (FEATURE_REQUESTS); see design §9.

const (
	taskAdvanceRequests = "advance_requests"
	advanceEvery        = "@every 15s"
)

// newDesk sends requests to the simulated company system; a real client would replace corpsim here.
func newDesk(data seed.Data, st *store.Store, rdb *redis.Client) *requests.Desk {
	svc := corpsim.New(rdb, corpsim.OutcomesFromSeed(data), "")
	return requests.NewDesk(requests.CatalogFromSeed(data), svc, st, data.Directory(), st, stream.NewRedisBus(rdb))
}

func handleRequestJobs(mux *asynq.ServeMux, desk *requests.Desk) {
	mux.HandleFunc(taskAdvanceRequests, func(ctx context.Context, _ *asynq.Task) error {
		_, err := desk.Sync(ctx, time.Now())
		return err
	})
}

// newRequestsScheduler queues advance_requests; a tick is dropped while the previous one is still queued.
func newRequestsScheduler(redisAddr string) (*asynq.Scheduler, error) {
	s := asynq.NewScheduler(asynq.RedisClientOpt{Addr: redisAddr}, nil)
	_, err := s.Register(advanceEvery, asynq.NewTask(taskAdvanceRequests, nil),
		asynq.Queue("system"), asynq.MaxRetry(0), asynq.Timeout(10*time.Second), asynq.Unique(14*time.Second))
	return s, err
}
