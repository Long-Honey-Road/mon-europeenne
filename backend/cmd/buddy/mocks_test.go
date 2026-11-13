package main

import (
	"bytes"
	"context"
	"errors"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/config"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/mocks"
	"github.com/Long-Honey-Road/mon-europeenne/backend/seed"
)

// startMocks serves the mock services on a free port for stack tests and returns their gRPC address.
func startMocks(t *testing.T) string {
	t.Helper()
	data, err := seed.Load(seed.Files)
	if err != nil {
		t.Fatal(err)
	}
	srv, err := mocks.New(data, mocks.Chaos{})
	if err != nil {
		t.Fatal(err)
	}
	var lc net.ListenConfig
	ln, err := lc.Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = srv.GRPC.Serve(ln) }()
	ctx, cancel := context.WithCancel(context.Background())
	go srv.Requests.Run(ctx, 10*time.Millisecond)
	t.Cleanup(func() { cancel(); srv.GRPC.Stop() })
	return ln.Addr().String()
}

func TestServeMocksUntilCancelled(t *testing.T) {
	cfg := config.Config{Mocks: config.Mocks{GRPCAddr: "127.0.0.1:0", HTTPAddr: "127.0.0.1:0"}}
	ctx, cancel := context.WithCancel(t.Context())
	var out bytes.Buffer
	errs := make(chan error, 1)
	go func() { errs <- serveMocks(ctx, cfg, &out) }()
	time.Sleep(100 * time.Millisecond)
	cancel()
	if err := <-errs; err != nil {
		t.Fatalf("serveMocks: %v", err)
	}
	if !strings.Contains(out.String(), "mocks listening") {
		t.Errorf("output = %q", out.String())
	}
}

func TestServeMocksBadAddress(t *testing.T) {
	for _, m := range []config.Mocks{
		{GRPCAddr: "bad:addr:1", HTTPAddr: "127.0.0.1:0"},
		{GRPCAddr: "127.0.0.1:0", HTTPAddr: "bad:addr:1"},
	} {
		if err := serveMocks(t.Context(), config.Config{Mocks: m}, &bytes.Buffer{}); err == nil {
			t.Errorf("%+v: no error", m)
		}
	}
}

func TestRunMocksRejectsBadConfig(t *testing.T) {
	t.Setenv("MOCK_FAIL_RATE", "often")
	var stdout, stderr bytes.Buffer
	if code := run([]string{"mocks"}, &stdout, &stderr); code != exitError {
		t.Fatalf("exit code = %d, stderr %q", code, stderr.String())
	}
}

func TestRequestsNeedMocks(t *testing.T) {
	data, _ := seed.Load(seed.Files)
	src, closeSrc, err := openSources(config.Config{}, data)
	if err != nil {
		t.Fatal(err)
	}
	defer closeSrc()
	if _, err := src.requestService(nil); !errors.Is(err, errNoMocks) {
		t.Errorf("requestService without mocks = %v", err)
	}
	if _, err := newDesk(data, src, nil, nil); !errors.Is(err, errNoMocks) {
		t.Errorf("newDesk without mocks = %v", err)
	}
}
