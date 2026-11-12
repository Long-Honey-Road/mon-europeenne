package store

import (
	"context"
	"testing"
)

func TestOpenConnectsToMigratedDatabase(t *testing.T) {
	dsn := freshDB(t)
	if err := MigrateUp(dsn); err != nil {
		t.Fatalf("MigrateUp: %v", err)
	}

	pool, err := Open(context.Background(), dsn)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer pool.Close()

	var n int
	if err := pool.QueryRow(context.Background(), "SELECT count(*) FROM buddy.roles").Scan(&n); err != nil {
		t.Fatalf("query: %v", err)
	}
	if n == 0 {
		t.Fatal("buddy.roles is empty")
	}
}

func TestOpenFailsOnUnreachableDatabase(t *testing.T) {
	_, err := Open(context.Background(), "postgres://buddy:buddy@127.0.0.1:1/buddy?sslmode=disable&connect_timeout=1")
	if err == nil {
		t.Fatal("Open succeeded against a closed port")
	}
}

func TestOpenFailsOnMalformedDSN(t *testing.T) {
	if _, err := Open(context.Background(), "::not a dsn::"); err == nil {
		t.Fatal("Open succeeded with malformed DSN")
	}
}
