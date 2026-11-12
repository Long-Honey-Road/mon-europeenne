package fake

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/llm"
)

func ask(content string) llm.Request {
	return llm.Request{Messages: []llm.Message{{Role: llm.RoleUser, Content: content}}}
}

func TestRepliesFollowTheScript(t *testing.T) {
	c := New(
		ToolCall("search_docs", map[string]any{"query": "dev"}),
		Text("done"),
	)
	ctx := context.Background()

	first, err := c.Chat(ctx, ask("how to run dev?"))
	if err != nil {
		t.Fatalf("first call: %v", err)
	}
	calls := first.Message.ToolCalls
	if len(calls) != 1 || calls[0].Name != "search_docs" || string(calls[0].Arguments) != `{"query":"dev"}` {
		t.Fatalf("first reply tool calls = %+v", calls)
	}
	if calls[0].ID == "" || first.Message.Role != llm.RoleAssistant {
		t.Fatalf("tool call needs an id and assistant role: %+v", first.Message)
	}

	second, err := c.Chat(ctx, ask("next"))
	if err != nil || second.Message.Content != "done" {
		t.Fatalf("second reply = %+v, %v", second.Message, err)
	}
	if c.Remaining() != 0 {
		t.Fatalf("Remaining() = %d, want 0", c.Remaining())
	}
}

func TestRecordsEveryRequest(t *testing.T) {
	c := New(Text("a"), Text("b"))
	_, _ = c.Chat(context.Background(), ask("one"))
	_, _ = c.Chat(context.Background(), ask("two"))

	calls := c.Calls()
	if len(calls) != 2 || calls[1].Messages[0].Content != "two" {
		t.Fatalf("Calls() = %+v", calls)
	}
}

func TestFailsWhenScriptIsExhausted(t *testing.T) {
	c := New(Text("only"))
	_, _ = c.Chat(context.Background(), ask("one"))

	if _, err := c.Chat(context.Background(), ask("two")); !errors.Is(err, ErrScriptExhausted) {
		t.Fatalf("error = %v, want ErrScriptExhausted", err)
	}
}

func TestExpectRejectsUnexpectedRequest(t *testing.T) {
	needsTools := func(req llm.Request) error {
		if len(req.Tools) == 0 {
			return errors.New("no tools offered")
		}
		return nil
	}
	c := New(Text("x").Expect(needsTools))

	if _, err := c.Chat(context.Background(), ask("hi")); !errors.Is(err, ErrUnexpectedRequest) {
		t.Fatalf("error = %v, want ErrUnexpectedRequest", err)
	}
}

func TestStepCanReturnProviderError(t *testing.T) {
	c := New(Fail(llm.ErrUnavailable))

	if _, err := c.Chat(context.Background(), ask("hi")); !errors.Is(err, llm.ErrUnavailable) {
		t.Fatalf("error = %v, want ErrUnavailable", err)
	}
}

func TestJSONStepReturnsMarshalledContent(t *testing.T) {
	c := New(JSON(map[string]string{"mode": "brief"}))

	resp, err := c.Chat(context.Background(), ask("hi"))
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	var got map[string]string
	if err := json.Unmarshal([]byte(resp.Message.Content), &got); err != nil || got["mode"] != "brief" {
		t.Fatalf("content = %q, %v", resp.Message.Content, err)
	}
}

func TestHonoursCancelledContext(t *testing.T) {
	c := New(Text("never"))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := c.Chat(ctx, ask("hi")); !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
	if c.Remaining() != 1 {
		t.Fatal("cancelled call consumed a step")
	}
}

func TestUnmarshalableArgumentsPanicAtScriptTime(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("ToolCall with a channel argument did not panic")
		}
	}()
	ToolCall("bad", map[string]any{"ch": make(chan int)})
}
