package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/auth"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/config"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/httpapi"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/jobs"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/mcpserver"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/requests"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/store"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/stream"
	"github.com/Long-Honey-Road/mon-europeenne/backend/seed"
)

const (
	sessionTTL      = 12 * time.Hour
	shutdownTimeout = 10 * time.Second
)

func runAPI(_ []string, stdout io.Writer) error {
	cfg, err := config.Load(os.LookupEnv)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	h, cleanup, err := buildAPI(ctx, cfg)
	if err != nil {
		return err
	}
	defer cleanup()
	var lc net.ListenConfig
	ln, err := lc.Listen(ctx, "tcp", cfg.HTTPAddr)
	if err != nil {
		return fmt.Errorf("listen %s: %w", cfg.HTTPAddr, err)
	}
	fmt.Fprintf(stdout, "api listening on %s\n", ln.Addr())
	return serve(ctx, ln, h)
}

// buildAPI wires the API: accounts and directory from the corporate services (see sources), threads in
// Postgres, jobs and live events in Redis. cleanup closes the connections.
func buildAPI(ctx context.Context, cfg config.Config) (http.Handler, func(), error) {
	tokens, err := auth.NewTokens(cfg.SessionSecret, sessionTTL)
	if err != nil {
		return nil, nil, fmt.Errorf("SESSION_SECRET: %w", err)
	}
	if err := cfg.RequireDatabase(); err != nil {
		return nil, nil, err
	}
	if err := cfg.RequireRedis(); err != nil {
		return nil, nil, err
	}
	data, err := seed.Load(seed.Files)
	if err != nil {
		return nil, nil, err
	}
	src, closeSrc, err := openSources(cfg, data)
	if err != nil {
		return nil, nil, err
	}
	pool, err := store.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		closeSrc()
		return nil, nil, err
	}
	rdb := redis.NewClient(&redis.Options{Addr: cfg.RedisAddr})
	queue := jobs.NewClient(cfg.RedisAddr, jobs.TaskTimeout(cfg.LLM.QueueTimeout, cfg.TurnTimeout))
	cleanup := func() {
		_ = queue.Close()
		_ = rdb.Close()
		pool.Close()
		closeSrc()
	}

	st := store.New(pool)
	deps := httpapi.Deps{
		Accounts: src.accounts, Tokens: tokens, Directory: src.directory,
		Threads: st, Plans: st, Jobs: queue, Bus: stream.NewRedisBus(rdb), Pins: st, Publish: stream.NewRedisBus(rdb), Vault: st,
	}
	if cfg.FeatureRequests {
		desk, err := newDesk(data, src, st, rdb)
		if err != nil {
			cleanup()
			return nil, nil, err
		}
		deps.Requests, deps.Blockers = requests.NewAPI(desk), []httpapi.BlockerSource{desk}
	}
	// The same registry as the worker's agent, so MCP clients get the same tools and rights.
	registry, err := agentTools(data, src.directory, st, newKnowledge(cfg, data, src, st), src.teamRepo(), cfg.FeatureRequests)
	if err != nil {
		cleanup()
		return nil, nil, err
	}
	deps.MCP = mcpserver.NewHTTPHandler(registry, "dev")
	h, err := httpapi.New(ctx, deps)
	if err != nil {
		cleanup()
		return nil, nil, err
	}
	return h, cleanup, nil
}

// serve runs until ctx is cancelled, then drains in-flight requests.
func serve(ctx context.Context, ln net.Listener, h http.Handler) error {
	srv := &http.Server{Handler: h, ReadHeaderTimeout: 5 * time.Second}
	errs := make(chan error, 1)
	go func() { errs <- srv.Serve(ln) }()

	select {
	case err := <-errs:
		return err
	case <-ctx.Done():
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return err
	}
	if err := <-errs; !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
