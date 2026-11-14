package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hibiken/asynq"

	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/domain"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/jobs"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/spheres"
)

// handleSphereJobs rewrites sphere and thread summaries in the background (design §6.5).
func handleSphereJobs(mux *asynq.ServeMux, s *spheres.Summarizer) {
	mux.HandleFunc(jobs.TypeSummarize, func(ctx context.Context, t *asynq.Task) error {
		var job jobs.Summary
		if err := json.Unmarshal(t.Payload(), &job); err != nil {
			return fmt.Errorf("%w: %w", asynq.SkipRetry, err)
		}
		return s.Summarize(ctx, job.ThreadID, domain.Focus{Kind: domain.FocusKind(job.FocusKind), ID: job.FocusID})
	})
}
