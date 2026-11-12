package main

import (
	"bytes"
	"context"
	"io"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestMCPServesToolsOverStdio(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	mcpStdin = inR
	t.Cleanup(func() { mcpStdin = nil })

	done := make(chan int, 1)
	var stderr bytes.Buffer
	go func() {
		done <- run([]string{"mcp", "-as", "mentor", "-thread", "general"}, outW, &stderr)
		_ = outW.Close()
	}()

	ctx := context.Background()
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "0"}, nil).
		Connect(ctx, &mcp.IOTransport{Reader: outR, Writer: inW}, nil)
	if err != nil {
		t.Fatal(err)
	}
	list, err := cs.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, tool := range list.Tools {
		names = append(names, tool.Name)
	}
	// Without a database only the directory tools exist.
	if len(names) != 2 || names[0] != "report_not_found" || names[1] != "who_to_ask" {
		t.Fatalf("tools = %v", names)
	}
	res, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "who_to_ask", Arguments: map[string]any{"topic": "доступ к jira"}})
	if err != nil || res.IsError {
		t.Fatalf("call: %v %+v", err, res)
	}
	_ = cs.Close()

	if code := <-done; code != exitOK {
		t.Fatalf("exit %d, stderr %q", code, stderr.String())
	}
}

func TestMCPRejectsBadInput(t *testing.T) {
	for name, args := range map[string][]string{
		"unknown login":  {"mcp", "-as", "ghost"},
		"unknown thread": {"mcp", "-thread", "chat"},
		"bad flag":       {"mcp", "-loud"},
	} {
		t.Run(name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if code := run(args, &stdout, &stderr); code != exitError {
				t.Fatalf("exit %d, want %d", code, exitError)
			}
		})
	}
}
