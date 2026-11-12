// Package testredis runs a throwaway Redis in Docker for integration tests.
package testredis

import (
	"context"
	"log"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	tcredis "github.com/testcontainers/testcontainers-go/modules/redis"
)

// Image matches deploy/docker-compose.yml.
const Image = "redis:7.4.12-alpine"

const startAttempts = 3

var (
	once     sync.Once
	ctr      *tcredis.RedisContainer
	addr     string
	startErr error
)

// Main wraps m.Run and stops the container, if one was started, after all tests.
func Main(m *testing.M) int {
	code := m.Run()
	if ctr != nil {
		if err := testcontainers.TerminateContainer(ctr); err != nil {
			log.Printf("terminate redis container: %v", err)
		}
	}
	return code
}

// Addr returns host:port of the shared test Redis; the container starts on first use.
func Addr(t *testing.T) string {
	t.Helper()
	once.Do(start)
	if startErr != nil {
		t.Fatalf("redis container: %v", startErr)
	}
	return addr
}

// Docker Desktop on macOS sometimes fails to publish the port of a fresh container; retry a few times.
func start() {
	for attempt := 1; attempt <= startAttempts; attempt++ {
		if startErr = startOnce(); startErr == nil {
			return
		}
		log.Printf("redis container, attempt %d/%d: %v", attempt, startAttempts, startErr)
	}
}

func startOnce() error {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	c, err := tcredis.Run(ctx, Image)
	if err != nil {
		if c != nil {
			_ = testcontainers.TerminateContainer(c)
		}
		return err
	}
	uri, err := c.ConnectionString(ctx)
	if err != nil {
		_ = testcontainers.TerminateContainer(c)
		return err
	}
	ctr, addr = c, strings.TrimPrefix(uri, "redis://")
	return nil
}
