package spheres

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/domain"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/llm"
)

const (
	maxSummaryRunes = 1000
	// summaryWindow bounds how many new messages one summary call reads.
	summaryWindow = 60
)

const summaryPrompt = `You keep the memory of a conversation between an intern and buddy, their onboarding mentor.
Write the new summary in Russian, at most 900 characters: what the intern asked or decided, what is done, what is
open and what was promised next. Facts only, no greetings, no advice of your own. Merge the previous summary with
the new messages; drop what no longer matters.`

type SummaryStore interface {
	FocusMessages(ctx context.Context, threadID string, f domain.Focus, before time.Time, limit int) ([]domain.Message, error)
	Messages(ctx context.Context, threadID string, limit int) ([]domain.Message, error)
	FocusSummary(ctx context.Context, threadID string, f domain.Focus) (string, time.Time, error)
	ThreadSummary(ctx context.Context, threadID string) (string, time.Time, error)
	SaveFocusSummary(ctx context.Context, threadID string, f domain.Focus, summary string, until time.Time) error
	SaveThreadSummary(ctx context.Context, threadID, summary string, until time.Time) error
}

type Models interface {
	Acquire(ctx context.Context, onWait func()) (llm.Client, func(), error)
}

// Summarizer rewrites a sphere's or a thread's summary with one model call without tools or thinking.
type Summarizer struct {
	store  SummaryStore
	models Models
}

func NewSummarizer(st SummaryStore, models Models) *Summarizer {
	return &Summarizer{store: st, models: models}
}

// Summarize updates the summary of a sphere, or of the whole thread for a zero focus; with nothing new it does
// nothing. A failure keeps the old summary.
func (s *Summarizer) Summarize(ctx context.Context, threadID string, f domain.Focus) error {
	var (
		old   string
		until time.Time
		msgs  []domain.Message
		err   error
	)
	if f.IsZero() {
		if old, until, err = s.store.ThreadSummary(ctx, threadID); err == nil {
			msgs, err = s.store.Messages(ctx, threadID, summaryWindow)
		}
	} else if old, until, err = s.store.FocusSummary(ctx, threadID, f); err == nil {
		msgs, err = s.store.FocusMessages(ctx, threadID, f, time.Now().Add(time.Minute), summaryWindow)
	}
	if err != nil {
		return err
	}
	var b strings.Builder
	var last time.Time
	for _, m := range msgs {
		if !m.CreatedAt.After(until) {
			continue
		}
		who := "Стажёр"
		if m.Role == domain.MessageAssistant {
			who = "buddy"
		}
		fmt.Fprintf(&b, "%s: %s\n\n", who, m.Content)
		last = m.CreatedAt
	}
	if last.IsZero() {
		return nil
	}
	model, release, err := s.models.Acquire(ctx, nil)
	if err != nil {
		return err
	}
	defer release()
	resp, err := model.Chat(ctx, llm.Request{
		ReasoningEffort: "none",
		Messages: []llm.Message{
			{Role: llm.RoleSystem, Content: summaryPrompt},
			{Role: llm.RoleUser, Content: "Previous summary:\n" + orNone(old) + "\n\nNew messages:\n" + b.String()},
		},
	})
	if err != nil {
		return err
	}
	summary := cut(strings.TrimSpace(resp.Message.Content), maxSummaryRunes)
	if summary == "" {
		return nil
	}
	if f.IsZero() {
		return s.store.SaveThreadSummary(ctx, threadID, summary, last)
	}
	return s.store.SaveFocusSummary(ctx, threadID, f, summary, last)
}

func orNone(s string) string {
	if s == "" {
		return "(none)"
	}
	return s
}

func cut(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n])
	}
	return s
}
