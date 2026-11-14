package main

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/config"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/mocks"
	"github.com/Long-Honey-Road/mon-europeenne/backend/seed"
)

// approverTick is how often the mock approvers look at due requests.
const approverTick = time.Second

// runMocks serves the mock corporate services until SIGINT or SIGTERM.
func runMocks(_ []string, stdout io.Writer) error {
	cfg, err := config.Load(os.LookupEnv)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return serveMocks(ctx, cfg, stdout)
}

func serveMocks(ctx context.Context, cfg config.Config, stdout io.Writer) error {
	data, err := seed.Load(seed.Files)
	if err != nil {
		return err
	}
	var opts []mocks.Option
	if cfg.Mocks.StateFile != "" {
		opts = append(opts, mocks.WithStateFile(cfg.Mocks.StateFile))
	}
	srv, err := mocks.New(data, mocks.Chaos{FailRate: cfg.Mocks.FailRate, MaxLatency: cfg.Mocks.MaxLatency}, opts...)
	if err != nil {
		return err
	}
	var lc net.ListenConfig
	grpcLn, err := lc.Listen(ctx, "tcp", cfg.Mocks.GRPCAddr)
	if err != nil {
		return fmt.Errorf("listen %s: %w", cfg.Mocks.GRPCAddr, err)
	}
	httpLn, err := lc.Listen(ctx, "tcp", cfg.Mocks.HTTPAddr)
	if err != nil {
		_ = grpcLn.Close()
		return fmt.Errorf("listen %s: %w", cfg.Mocks.HTTPAddr, err)
	}
	fmt.Fprintf(stdout, "mocks listening: gRPC %s, REST %s, fail rate %v, max latency %v\n",
		grpcLn.Addr(), httpLn.Addr(), cfg.Mocks.FailRate, cfg.Mocks.MaxLatency)
	return srv.Run(ctx, grpcLn, httpLn, approverTick)
}
