package main

import (
	"context"
	"time"

	"github.com/hibiken/asynq"
	"github.com/redis/go-redis/v9"

	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/requests"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/store"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/stream"
	"github.com/Long-Honey-Road/mon-europeenne/backend/seed"
)

// Wiring of the removable requests module (FEATURE_REQUESTS); see design §9.

const (
	taskAdvanceRequests = "advance_requests"
	advanceEvery        = "@every 15s"
)

// newDesk sends requests to the company's request system (the corp-requests mock).
func newDesk(data seed.Data, src sources, st *store.Store, rdb *redis.Client) (*requests.Desk, error) {
	svc, err := src.requestService(rdb)
	if err != nil {
		return nil, err
	}
	return requests.NewDesk(requests.CatalogFromSeed(data), svc, st, src.directory, st, stream.NewRedisBus(rdb)), nil
}

func handleRequestJobs(mux *asynq.ServeMux, desk *requests.Desk) {
	mux.HandleFunc(taskAdvanceRequests, func(ctx context.Context, _ *asynq.Task) error {
		_, err := desk.Sync(ctx)
		return err
	})
}

// registerRequestTasks queues advance_requests; a tick is dropped while the previous one is still queued.
func registerRequestTasks(s *asynq.Scheduler) error {
	_, err := s.Register(advanceEvery, asynq.NewTask(taskAdvanceRequests, nil),
		asynq.Queue("system"), asynq.MaxRetry(0), asynq.Timeout(10*time.Second), asynq.Unique(14*time.Second))
	return err
}
