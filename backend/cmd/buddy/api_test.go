package main

import (
	"bytes"
	"context"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/config"
)

func TestAPIRequiresSessionSecret(t *testing.T) {
	t.Setenv("SESSION_SECRET", "short")
	var stdout, stderr bytes.Buffer

	if code := run([]string{"api"}, &stdout, &stderr); code != exitError {
		t.Fatalf("exit %d, want %d", code, exitError)
	}
	if !strings.Contains(stderr.String(), "SESSION_SECRET") {
		t.Errorf("stderr = %q", stderr.String())
	}
}

func TestAPIReportsBadListenAddress(t *testing.T) {
	t.Setenv("SESSION_SECRET", "test-session-secret-0123456789")
	t.Setenv("HTTP_ADDR", "not-an-address")
	var stdout, stderr bytes.Buffer

	if code := run([]string{"api"}, &stdout, &stderr); code != exitError {
		t.Fatalf("exit %d, want %d", code, exitError)
	}
}

func TestServeAnswersUntilCancelled(t *testing.T) {
	h, err := buildAPI(context.Background(), config.Config{SessionSecret: "test-session-secret-0123456789"})
	if err != nil {
		t.Fatalf("buildAPI: %v", err)
	}
	var lc net.ListenConfig
	ln, err := lc.Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- serve(ctx, ln, h) }()

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://"+ln.Addr().String()+"/healthz", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /healthz: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d", resp.StatusCode)
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("serve returned %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("serve did not stop after cancel")
	}
}
