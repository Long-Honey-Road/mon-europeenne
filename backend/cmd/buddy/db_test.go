package main

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/testdb"
)

func TestMain(m *testing.M) {
	os.Exit(testdb.Main(m))
}

func TestMigrateUpThenDown(t *testing.T) {
	t.Setenv("DATABASE_URL", testdb.Fresh(t))

	for _, direction := range []string{"up", "down"} {
		var stdout, stderr bytes.Buffer
		if code := run([]string{"migrate", direction}, &stdout, &stderr); code != exitOK {
			t.Fatalf("migrate %s: exit %d, stderr %q", direction, code, stderr.String())
		}
		if !strings.Contains(stdout.String(), "migrated "+direction) {
			t.Errorf("migrate %s stdout = %q", direction, stdout.String())
		}
	}
}

func TestMigrateRejectsBadDirection(t *testing.T) {
	for _, args := range [][]string{{"migrate"}, {"migrate", "sideways"}, {"migrate", "up", "extra"}} {
		var stdout, stderr bytes.Buffer
		if code := run(args, &stdout, &stderr); code != exitError {
			t.Errorf("%v: exit %d, want %d", args, code, exitError)
		}
		if !strings.Contains(stderr.String(), "want up or down") {
			t.Errorf("%v: stderr = %q", args, stderr.String())
		}
	}
}

func TestSeedConnectsToMigratedDatabase(t *testing.T) {
	t.Setenv("DATABASE_URL", testdb.Fresh(t))
	var stdout, stderr bytes.Buffer
	if code := run([]string{"migrate", "up"}, &stdout, &stderr); code != exitOK {
		t.Fatalf("migrate up: exit %d, stderr %q", code, stderr.String())
	}

	stdout.Reset()
	if code := run([]string{"seed"}, &stdout, &stderr); code != exitOK {
		t.Fatalf("seed: exit %d, stderr %q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "nothing to seed yet") {
		t.Errorf("seed stdout = %q", stdout.String())
	}
}

func TestDatabaseCommandsRequireDatabaseURL(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	for _, args := range [][]string{{"migrate", "up"}, {"seed"}} {
		var stdout, stderr bytes.Buffer
		if code := run(args, &stdout, &stderr); code != exitError {
			t.Errorf("%v: exit %d, want %d", args, code, exitError)
		}
		if !strings.Contains(stderr.String(), "DATABASE_URL") {
			t.Errorf("%v: stderr = %q", args, stderr.String())
		}
	}
}

func TestDatabaseCommandsReportInvalidConfig(t *testing.T) {
	t.Setenv("LLM_PROVIDER", "gpt")
	var stdout, stderr bytes.Buffer
	if code := run([]string{"seed"}, &stdout, &stderr); code != exitError {
		t.Fatalf("exit %d, want %d", code, exitError)
	}
	if !strings.Contains(stderr.String(), "LLM_PROVIDER") {
		t.Errorf("stderr = %q", stderr.String())
	}
}

func TestSeedFailsWhenDatabaseIsUnreachable(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://buddy:buddy@127.0.0.1:1/buddy?sslmode=disable&connect_timeout=1")
	var stdout, stderr bytes.Buffer
	if code := run([]string{"seed"}, &stdout, &stderr); code != exitError {
		t.Fatalf("exit %d, want %d", code, exitError)
	}
}
