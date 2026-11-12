// Package fake is a scripted llm.Client for tests and the LLM_PROVIDER=fake mode.
package fake

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/llm"
)

var (
	ErrScriptExhausted   = errors.New("fake llm: script exhausted")
	ErrUnexpectedRequest = errors.New("fake llm: unexpected request")
)

// Step is one scripted model turn.
type Step struct {
	reply  llm.Response
	err    error
	expect func(llm.Request) error
}

// Expect adds a check on the request this step answers.
func (s Step) Expect(check func(llm.Request) error) Step {
	s.expect = check
	return s
}

// Client answers calls with its steps in order and records every request.
type Client struct {
	mu    sync.Mutex
	steps []Step
	calls []llm.Request
}

func New(steps ...Step) *Client {
	return &Client{steps: steps}
}

func (c *Client) Chat(ctx context.Context, req llm.Request) (llm.Response, error) {
	if err := ctx.Err(); err != nil {
		return llm.Response{}, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	if len(c.steps) == 0 {
		return llm.Response{}, ErrScriptExhausted
	}
	step := c.steps[0]
	c.steps = c.steps[1:]
	c.calls = append(c.calls, req)

	if step.expect != nil {
		if err := step.expect(req); err != nil {
			return llm.Response{}, fmt.Errorf("%w: call %d: %w", ErrUnexpectedRequest, len(c.calls), err)
		}
	}
	return step.reply, step.err
}

func (c *Client) Calls() []llm.Request {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]llm.Request(nil), c.calls...)
}

func (c *Client) Remaining() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.steps)
}

func Text(content string) Step {
	return assistant(llm.Message{Content: content})
}

// JSON replies with v marshalled into the message content, as a model does under llm.Format.
func JSON(v any) Step {
	return assistant(llm.Message{Content: string(mustMarshal(v))})
}

// ToolCall replies with one call to the named tool.
func ToolCall(name string, args any) Step {
	return assistant(llm.Message{ToolCalls: []llm.ToolCall{{
		ID:        "call_" + name,
		Name:      name,
		Arguments: mustMarshal(args),
	}}})
}

func Fail(err error) Step {
	return Step{err: err}
}

func assistant(msg llm.Message) Step {
	msg.Role = llm.RoleAssistant
	return Step{reply: llm.Response{Message: msg}}
}

// mustMarshal panics: a bad script is a bug in the test, not a runtime condition.
func mustMarshal(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		panic(fmt.Sprintf("fake llm: marshal %T: %v", v, err))
	}
	return b
}

// WithText adds text before the tool calls, as models sometimes "think aloud" before calling a tool.
func (s Step) WithText(text string) Step {
	s.reply.Message.Content = text
	return s
}

// ChatStream answers like Chat and replays the text word by word through onDelta.
func (c *Client) ChatStream(ctx context.Context, req llm.Request, onDelta func(string)) (llm.Response, error) {
	resp, err := c.Chat(ctx, req)
	if err != nil {
		return resp, err
	}
	text := resp.Message.Content
	for text != "" {
		i := strings.IndexByte(text, ' ')
		if i < 0 {
			i = len(text) - 1
		}
		onDelta(text[:i+1])
		text = text[i+1:]
	}
	return resp, nil
}
