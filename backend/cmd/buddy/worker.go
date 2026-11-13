package main

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/hibiken/asynq"
	"github.com/redis/go-redis/v9"

	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/agent"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/config"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/jobs"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/store"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/stream"
	"github.com/Long-Honey-Road/mon-europeenne/backend/seed"
)

// runWorker processes queued agent turns until SIGINT or SIGTERM.
func runWorker(_ []string, stdout io.Writer) error {
	cfg, err := config.Load(os.LookupEnv)
	if err != nil {
		return err
	}
	srv, mux, cleanup, err := buildWorker(cfg)
	if err != nil {
		return err
	}
	defer cleanup()
	if cfg.FeatureRequests {
		sched, err := newRequestsScheduler(cfg.RedisAddr)
		if err != nil {
			return err
		}
		if err := sched.Start(); err != nil {
			return err
		}
		defer sched.Shutdown()
	}
	fmt.Fprintf(stdout, "worker started, model provider %s\n", cfg.LLM.Provider)
	return srv.Run(mux)
}

func buildWorker(cfg config.Config) (*asynq.Server, *asynq.ServeMux, func(), error) {
	if err := cfg.RequireDatabase(); err != nil {
		return nil, nil, nil, err
	}
	if err := cfg.RequireRedis(); err != nil {
		return nil, nil, nil, err
	}
	model, err := newLLM(cfg.LLM)
	if err != nil {
		return nil, nil, nil, err
	}
	data, err := seed.Load(seed.Files)
	if err != nil {
		return nil, nil, nil, err
	}
	pool, err := store.Open(context.Background(), cfg.DatabaseURL)
	if err != nil {
		return nil, nil, nil, err
	}
	st := store.New(pool)
	registry, err := agentTools(data, st, cfg.FeatureRequests)
	if err != nil {
		pool.Close()
		return nil, nil, nil, err
	}
	rdb := redis.NewClient(&redis.Options{Addr: cfg.RedisAddr})
	cleanup := func() {
		_ = rdb.Close()
		pool.Close()
	}

	h := jobs.NewHandler(jobs.Deps{
		Store: st, Publisher: stream.NewRedisBus(rdb),
		Agent: agent.New(model, registry, agent.Config{DetailedEffort: cfg.LLM.DetailedReasoningEffort, Timeout: cfg.TurnTimeout}),
		Users: seedUsers(data), Directory: data.Directory(),
		Models: model, Turns: jobs.NewActiveTurns(rdb, jobs.TaskTimeout(cfg.LLM.QueueTimeout, cfg.TurnTimeout)),
		QueueTimeout: cfg.LLM.QueueTimeout,
	})
	mux := jobs.Mux(h)
	if cfg.FeatureRequests {
		handleRequestJobs(mux, newDesk(data, st, rdb))
	}
	return jobs.NewServer(cfg.RedisAddr, cfg.WorkerConcurrency), mux, cleanup, nil
}
