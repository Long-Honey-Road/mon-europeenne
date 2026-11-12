package fake

import (
	"context"

	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/llm"
)

// Echo answers every request with the same text: the LLM_PROVIDER=fake mode for CI and UI work without a model.
type Echo struct{ text string }

func NewEcho(text string) *Echo { return &Echo{text: text} }

func (e *Echo) Chat(ctx context.Context, req llm.Request) (llm.Response, error) {
	return e.ChatStream(ctx, req, func(string) {})
}

func (e *Echo) ChatStream(ctx context.Context, _ llm.Request, onDelta func(string)) (llm.Response, error) {
	c := New(Text(e.text))
	return c.ChatStream(ctx, llm.Request{}, onDelta)
}
