package main

import (
	"context"
	"flag"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/config"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/domain"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/mcpserver"
	"github.com/Long-Honey-Road/mon-europeenne/backend/seed"
)

// mcpStdin is where MCP requests come from; tests replace it.
var mcpStdin io.ReadCloser = os.Stdin

// runMCP serves the agent's tools over stdio as one test user in one thread type.
func runMCP(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("mcp", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	login := fs.String("as", "intern", "test account login")
	threadName := fs.String("thread", "general", "thread type")
	if err := fs.Parse(args); err != nil {
		return err
	}
	thread, err := domain.ParseThreadType(*threadName)
	if err != nil {
		return err
	}
	cfg, err := config.Load(os.LookupEnv)
	if err != nil {
		return err
	}
	data, err := seed.Load(seed.Files)
	if err != nil {
		return err
	}
	principal, err := principalFor(data, *login)
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	registry, closeTools, err := localTools(ctx, cfg, data)
	if err != nil {
		return err
	}
	defer closeTools()

	srv := mcpserver.New(registry, principal, thread, "dev")
	return srv.Run(ctx, &mcp.IOTransport{Reader: mcpStdin, Writer: nopCloser{stdout}})
}

// nopCloser keeps the caller's stdout open when the session ends.
type nopCloser struct{ io.Writer }

func (nopCloser) Close() error { return nil }
