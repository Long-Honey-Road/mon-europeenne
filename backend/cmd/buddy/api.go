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

	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/auth"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/config"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/httpapi"
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

	h, err := buildAPI(ctx, cfg)
	if err != nil {
		return err
	}
	var lc net.ListenConfig
	ln, err := lc.Listen(ctx, "tcp", cfg.HTTPAddr)
	if err != nil {
		return fmt.Errorf("listen %s: %w", cfg.HTTPAddr, err)
	}
	fmt.Fprintf(stdout, "api listening on %s\n", ln.Addr())
	return serve(ctx, ln, h)
}

// buildAPI wires the API from seed accounts and the seed directory until the mocks take over (A-02).
func buildAPI(ctx context.Context, cfg config.Config) (http.Handler, error) {
	tokens, err := auth.NewTokens(cfg.SessionSecret, sessionTTL)
	if err != nil {
		return nil, fmt.Errorf("SESSION_SECRET: %w", err)
	}
	data, err := seed.Load(seed.Files)
	if err != nil {
		return nil, err
	}
	accounts, err := auth.NewAccounts(data.Credentials())
	if err != nil {
		return nil, err
	}
	return httpapi.New(ctx, httpapi.Deps{Accounts: accounts, Tokens: tokens, Directory: data.Directory()})
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
