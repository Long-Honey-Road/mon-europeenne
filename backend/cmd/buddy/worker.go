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
	registry, err := agentTools(data)
	if err != nil {
		return nil, nil, nil, err
	}
	pool, err := store.Open(context.Background(), cfg.DatabaseURL)
	if err != nil {
		return nil, nil, nil, err
	}
	rdb := redis.NewClient(&redis.Options{Addr: cfg.RedisAddr})
	cleanup := func() {
		_ = rdb.Close()
		pool.Close()
	}

	h := jobs.NewHandler(jobs.Deps{
		Store: store.New(pool), Publisher: stream.NewRedisBus(rdb),
		Agent: agent.New(model, registry, agent.Config{}), Users: seedUsers(data), Directory: data.Directory(),
	})
	return jobs.NewServer(cfg.RedisAddr, workerConcurrency), jobs.Mux(h), cleanup, nil
}
