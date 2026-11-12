// Package testdb runs a throwaway Postgres in Docker for integration tests.
package testdb

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
)

// Image matches deploy/docker-compose.yml.
const Image = "pgvector/pgvector:0.8.7-pg16"

var (
	once     sync.Once
	ctr      *postgres.PostgresContainer
	adminDSN string
	startErr error
	seq      atomic.Int64
)

// Main wraps m.Run and stops the container, if one was started, after all tests.
func Main(m *testing.M) int {
	code := m.Run()
	if ctr != nil {
		if err := testcontainers.TerminateContainer(ctr); err != nil {
			log.Printf("terminate postgres container: %v", err)
		}
	}
	return code
}

// Fresh returns the DSN of a new empty database; the container starts on first use.
func Fresh(t *testing.T) string {
	t.Helper()
	once.Do(start)
	if startErr != nil {
		t.Fatalf("postgres container: %v", startErr)
	}

	ctx := context.Background()
	name := fmt.Sprintf("test_%d", seq.Add(1))
	conn, err := pgx.Connect(ctx, adminDSN)
	if err != nil {
		t.Fatalf("connect admin: %v", err)
	}
	defer conn.Close(ctx)
	if _, err := conn.Exec(ctx, "CREATE DATABASE "+name); err != nil {
		t.Fatalf("create database: %v", err)
	}
	return strings.Replace(adminDSN, "/admin?", "/"+name+"?", 1)
}

// Docker Desktop on macOS sometimes fails to publish the port of a fresh container; retry a few times.
const startAttempts = 3

func start() {
	for attempt := 1; attempt <= startAttempts; attempt++ {
		if startErr = startOnce(); startErr == nil {
			return
		}
		log.Printf("postgres container, attempt %d/%d: %v", attempt, startAttempts, startErr)
	}
}

func startOnce() error {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	c, err := postgres.Run(ctx, Image,
		postgres.WithDatabase("admin"),
		postgres.WithUsername("buddy"),
		postgres.WithPassword("buddy"),
		postgres.BasicWaitStrategies(),
	)
	if err == nil {
		adminDSN, err = c.ConnectionString(ctx, "sslmode=disable")
	}
	if err != nil {
		if c != nil {
			_ = testcontainers.TerminateContainer(c)
		}
		return err
	}
	ctr = c
	return nil
}
