package tools

import (
	"context"

	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/domain"
)

type generalKnowledgeIn struct {
	Topic string `json:"topic" jsonschema:"the general topic you will explain from your own knowledge, in a few words"`
}

// GeneralKnowledgeNote is the tool result; the agent turns it into a warning block under the answer.
type GeneralKnowledgeNote struct {
	Topic string `json:"topic"`
	Rules string `json:"rules"`
}

const generalKnowledgeRules = "Allowed: knowledge that is the same in any company. First say that the company's " +
	"documents and tools have nothing on it and whom to ask. Then explain generally. Never name Avito hosts, " +
	"clusters, internal commands, settings or owners: say they come from the mentor or the team docs. " +
	"The user sees a warning that this is general knowledge and may be wrong."

// GeneralKnowledge lets the model answer a general technical question it found nothing about; calling it is
// what makes the server mark the answer as unverified.
func GeneralKnowledge() (Tool, error) {
	return Define("general_knowledge",
		"Call before explaining something from your own general knowledge (what SSH is, how git rebase works) "+
			"because the company's documents and tools have nothing on it. Never for Avito-specific facts.",
		allRoles, allThreads,
		func(_ context.Context, _ domain.Principal, in generalKnowledgeIn) (GeneralKnowledgeNote, error) {
			return GeneralKnowledgeNote{Topic: in.Topic, Rules: generalKnowledgeRules}, nil
		})
}
