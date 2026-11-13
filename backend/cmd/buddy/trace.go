package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/agent"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/config"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/domain"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/store"
	"github.com/Long-Honey-Road/mon-europeenne/backend/seed"
)

var traceZone = time.FixedZone("MSK", 3*60*60)

// runTrace prints the latest agent turns with their tool calls, oldest of them first.
func runTrace(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("trace", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	n := fs.Int("n", 5, "how many turns")
	login := fs.String("as", "", "only turns of this test account")
	thread := fs.String("thread", "", "only turns of this thread id")
	withPrompt := fs.Bool("prompt", false, "print the system prompt too")
	asJSON := fs.Bool("json", false, "print raw JSON, one trace per line")
	if err := fs.Parse(args); err != nil {
		return err
	}
	cfg, err := config.Load(os.LookupEnv)
	if err != nil {
		return err
	}
	if err := cfg.RequireDatabase(); err != nil {
		return err
	}
	filter := store.TraceFilter{ThreadID: *thread, Limit: *n}
	if *login != "" {
		data, err := seed.Load(seed.Files)
		if err != nil {
			return err
		}
		p, err := principalFor(data, *login)
		if err != nil {
			return err
		}
		filter.UserID = p.UserID
	}
	ctx := context.Background()
	pool, err := store.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	traces, err := store.New(pool).Traces(ctx, filter)
	if err != nil {
		return err
	}
	for i := len(traces) - 1; i >= 0; i-- {
		if *asJSON {
			raw, err := json.Marshal(traces[i])
			if err != nil {
				return err
			}
			fmt.Fprintln(stdout, string(raw))
			continue
		}
		printTrace(stdout, traces[i], *withPrompt)
	}
	return nil
}

func printTrace(w io.Writer, t domain.TurnTrace, withPrompt bool) {
	fmt.Fprintf(w, "=== %s · %s · %s · %s · %.1fs · thread %s\n", t.CreatedAt.In(traceZone).Format("2006-01-02 15:04:05"),
		t.UserID, t.ThreadType, t.Mode, t.Duration.Seconds(), t.ThreadID)
	fmt.Fprintf(w, "Q: %s\n", oneLine(t.Question))
	if withPrompt {
		fmt.Fprintf(w, "--- system prompt\n%s\n---\n", t.SystemPrompt)
	}
	var steps []agent.TraceStep
	_ = json.Unmarshal(t.Steps, &steps)
	for i, s := range steps {
		fmt.Fprintf(w, "step %d: %dms, %d→%d tokens", i+1, s.DurationMS, s.PromptTokens, s.CompletionTokens)
		if s.ReasoningEffort != "" {
			fmt.Fprintf(w, ", effort %s", s.ReasoningEffort)
		}
		fmt.Fprintln(w)
		if s.Text != "" && len(s.Tools) > 0 {
			fmt.Fprintf(w, "  withdrawn text: %s\n", oneLine(s.Text))
		}
		for _, tool := range s.Tools {
			fmt.Fprintf(w, "  → %s %s [%s, %dms]\n", tool.Name, compactJSON(tool.Args), tool.Outcome, tool.DurationMS)
			fmt.Fprintf(w, "    ← %s\n", tool.Result)
		}
	}
	if t.RawText != t.FinalText {
		fmt.Fprintf(w, "raw:   %s\n", oneLine(t.RawText))
	}
	fmt.Fprintf(w, "final: %s\n", oneLine(t.FinalText))
	if len(t.Violations) > 0 {
		fmt.Fprintf(w, "violations: %s\n", strings.Join(t.Violations, "; "))
	}
	if t.Error != "" {
		fmt.Fprintf(w, "error: %s\n", t.Error)
	}
	fmt.Fprintln(w)
}

// compactJSON undoes the spacing Postgres adds to jsonb.
func compactJSON(raw []byte) string {
	var b bytes.Buffer
	if json.Compact(&b, raw) != nil {
		return string(raw)
	}
	return b.String()
}

func oneLine(s string) string { return strings.Join(strings.Fields(s), " ") }
