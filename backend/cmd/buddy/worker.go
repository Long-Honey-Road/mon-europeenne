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

const workerConcurrency = 4

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
	sched, err := newScheduler(cfg)
	if err != nil {
		return err
	}
	if err := sched.Start(); err != nil {
		return err
	}
	defer sched.Shutdown()
	if err := enqueueSync(cfg.RedisAddr); err != nil {
		return err
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
	src, closeSrc, err := openSources(cfg, data)
	if err != nil {
		return nil, nil, nil, err
	}
	pool, err := store.Open(context.Background(), cfg.DatabaseURL)
	if err != nil {
		closeSrc()
		return nil, nil, nil, err
	}
	st := store.New(pool)
	rdb := redis.NewClient(&redis.Options{Addr: cfg.RedisAddr})
	cleanup := func() {
		_ = rdb.Close()
		pool.Close()
		closeSrc()
	}
	know := newKnowledge(cfg, data, src, st)
	registry, err := agentTools(data, src.directory, st, know, cfg.FeatureRequests)
	if err != nil {
		cleanup()
		return nil, nil, nil, err
	}

	h := jobs.NewHandler(jobs.Deps{
		Store: st, Publisher: stream.NewRedisBus(rdb),
		Agent: agent.New(model, registry, agent.Config{DetailedEffort: cfg.LLM.DetailedReasoningEffort}), Users: src.accounts, Directory: src.directory,
	})
	mux := jobs.Mux(h)
	handleKnowledgeJobs(mux, know)
	if cfg.FeatureRequests {
		desk, err := newDesk(data, src, st, rdb)
		if err != nil {
			cleanup()
			return nil, nil, nil, err
		}
		handleRequestJobs(mux, desk)
	}
	return jobs.NewServer(cfg.RedisAddr, workerConcurrency), mux, cleanup, nil
}
