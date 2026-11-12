package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunWithoutArgsPrintsUsageAndFails(t *testing.T) {
	var stdout, stderr bytes.Buffer

	code := run(nil, &stdout, &stderr)

	if code != exitUsage {
		t.Fatalf("exit code = %d, want %d", code, exitUsage)
	}
	if !strings.Contains(stderr.String(), "usage: buddy") {
		t.Fatalf("stderr = %q, want usage", stderr.String())
	}
}

func TestRunHelpListsAllCommands(t *testing.T) {
	var stdout, stderr bytes.Buffer

	code := run([]string{"help"}, &stdout, &stderr)

	if code != exitOK {
		t.Fatalf("exit code = %d, want %d", code, exitOK)
	}
	for _, name := range []string{"api", "worker", "mcp", "mocks", "migrate", "seed"} {
		if !strings.Contains(stdout.String(), name) {
			t.Errorf("help output misses %q: %q", name, stdout.String())
		}
	}
}

func TestRunUnknownCommandFails(t *testing.T) {
	var stdout, stderr bytes.Buffer

	code := run([]string{"deploy"}, &stdout, &stderr)

	if code != exitUsage {
		t.Fatalf("exit code = %d, want %d", code, exitUsage)
	}
	if !strings.Contains(stderr.String(), `unknown command "deploy"`) {
		t.Fatalf("stderr = %q, want unknown command error", stderr.String())
	}
}

func TestRunStubCommandsReportNotImplemented(t *testing.T) {
	for _, name := range []string{"mocks"} {
		t.Run(name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer

			code := run([]string{name}, &stdout, &stderr)

			if code != exitError {
				t.Fatalf("exit code = %d, want %d", code, exitError)
			}
			if !strings.Contains(stderr.String(), "not implemented") {
				t.Fatalf("stderr = %q, want not implemented", stderr.String())
			}
		})
	}
}
