package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/config"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/store"
)

// runFeedback prints testers' ratings with the question and the answer, oldest of them first.
func runFeedback(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("feedback", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	n := fs.Int("n", 20, "how many ratings")
	notes := fs.Bool("notes", false, "only ratings with a comment")
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
	ctx := context.Background()
	pool, err := store.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	list, err := store.New(pool).Feedback(ctx, *n, *notes)
	if err != nil {
		return err
	}
	marks := map[string]string{"up": "👍", "down": "👎"}
	for i := len(list) - 1; i >= 0; i-- {
		f := list[i]
		fmt.Fprintf(stdout, "=== %s · %s · %s · thread %s\n", f.At.In(traceZone).Format("2006-01-02 15:04:05"),
			f.Login, marks[f.Value], f.ThreadID)
		fmt.Fprintf(stdout, "Q: %s\nA: %s\n", oneLine(f.Question), oneLine(f.Answer))
		if f.Note != "" {
			fmt.Fprintf(stdout, "note: %s\n", oneLine(f.Note))
		}
		fmt.Fprintln(stdout)
	}
	return nil
}
