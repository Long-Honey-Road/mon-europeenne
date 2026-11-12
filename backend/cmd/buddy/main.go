// Command buddy is the single binary of AI-buddy; each subcommand runs one process.
package main

import (
	"errors"
	"fmt"
	"io"
	"os"
)

const (
	exitOK = iota
	exitError
	exitUsage
)

var errNotImplemented = errors.New("not implemented yet")

type command struct {
	name    string
	summary string
	run     func(args []string, stdout io.Writer) error
}

var commands = []command{
	{name: "api", summary: "serve the HTTP API and SSE", run: runAPI},
	{name: "worker", summary: "run background jobs and the scheduler", run: runWorker},
	{name: "mcp", summary: "expose agent tools over MCP (stdio) as a test user", run: runMCP},
	{name: "mocks", summary: "serve mock corporate services", run: notImplemented},
	{name: "migrate", summary: "apply (up) or revert (down) database migrations", run: runMigrate},
	{name: "seed", summary: "load synthetic data and build the index", run: runSeed},
	{name: "ask", summary: "run one agent turn against the configured model", run: runAsk},
	{name: "llm-check", summary: "probe the configured model: text, tool call, JSON, stream", run: runLLMCheck},
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		printUsage(stderr)
		return exitUsage
	}

	name := args[0]
	if name == "help" || name == "-h" || name == "--help" {
		printUsage(stdout)
		return exitOK
	}

	for _, cmd := range commands {
		if cmd.name != name {
			continue
		}
		if err := cmd.run(args[1:], stdout); err != nil {
			fmt.Fprintf(stderr, "buddy %s: %v\n", name, err)
			return exitError
		}
		return exitOK
	}

	fmt.Fprintf(stderr, "buddy: unknown command %q\n\n", name)
	printUsage(stderr)
	return exitUsage
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, "usage: buddy <command> [flags]")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "commands:")
	for _, cmd := range commands {
		fmt.Fprintf(w, "  %-10s %s\n", cmd.name, cmd.summary)
	}
}

func notImplemented([]string, io.Writer) error {
	return errNotImplemented
}
