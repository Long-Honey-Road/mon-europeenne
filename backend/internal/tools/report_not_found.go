package tools

import (
	"context"

	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/domain"
)

// NotFoundReport is what the agent shows when the materials have no answer.
type NotFoundReport struct {
	Topic      string           `json:"topic"`
	Escalation []EscalationStep `json:"escalation"`
}

type notFoundIn struct {
	Topic string `json:"topic" jsonschema:"what was asked and not found, in a few words"`
}

// ReportNotFound lets the model say honestly that nothing was found; the answer gets the escalation chain.
func ReportNotFound(dir PeopleDirectory) (Tool, error) {
	return Define("report_not_found",
		"Call when the available documents and tools do not answer the question. Shows whom to ask instead.",
		allRoles, allThreads,
		func(ctx context.Context, p domain.Principal, in notFoundIn) (NotFoundReport, error) {
			chain, err := escalationChain(ctx, dir, p)
			if err != nil {
				return NotFoundReport{}, err
			}
			return NotFoundReport{Topic: in.Topic, Escalation: chain}, nil
		})
}
