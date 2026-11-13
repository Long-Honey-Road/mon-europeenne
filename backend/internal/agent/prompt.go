package agent

import (
	"embed"
	"fmt"
	"strings"

	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/domain"
)

//go:embed prompts/*.md
var prompts embed.FS

var threadPrompts = map[domain.ThreadType]string{
	domain.ThreadGeneral: "thread_general.md",
	domain.ThreadPlan:    "thread_plan.md",
	domain.ThreadRequest: "thread_request.md",
	domain.ThreadTask:    "thread_task.md",
}

var modePrompts = map[Mode]string{
	ModeBrief:    "mode_brief.md",
	ModeDetailed: "mode_detailed.md",
}

// systemPrompt joins base, thread, mode and citation rules, then the turn's own context.
func systemPrompt(t Turn) (string, error) {
	threadFile, ok := threadPrompts[t.Thread]
	if !ok {
		return "", fmt.Errorf("no prompt for thread type %s", t.Thread)
	}
	modeFile, ok := modePrompts[t.Mode]
	if !ok {
		return "", fmt.Errorf("no prompt for mode %q", t.Mode)
	}

	var b strings.Builder
	for _, name := range []string{"base.md", threadFile, modeFile, "citations.md"} {
		text, err := prompts.ReadFile("prompts/" + name)
		if err != nil {
			return "", err
		}
		b.Write(text)
		b.WriteString("\n")
	}
	for _, part := range []struct{ title, text string }{
		{"## Today", t.Today},
		{"## Profile", t.Profile},
		{"## Pinned context", t.Pinned},
		{"## Earlier in this thread", t.Summary},
	} {
		if part.text != "" {
			fmt.Fprintf(&b, "%s\n\n%s\n\n", part.title, part.text)
		}
	}
	return strings.TrimSpace(b.String()), nil
}
