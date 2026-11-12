package fake

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/llm"
)

func TestChatStreamSplitsTextIntoDeltas(t *testing.T) {
	c := New(Text("Запусти make dev [1]."))
	var deltas []string

	resp, err := c.ChatStream(context.Background(), ask("hi"), func(d string) { deltas = append(deltas, d) })
	if err != nil {
		t.Fatalf("ChatStream: %v", err)
	}
	if len(deltas) < 2 || strings.Join(deltas, "") != "Запусти make dev [1]." {
		t.Fatalf("deltas = %q", deltas)
	}
	if resp.Message.Content != "Запусти make dev [1]." {
		t.Fatalf("content = %q", resp.Message.Content)
	}
}

func TestChatStreamToolCallHasNoDeltas(t *testing.T) {
	c := New(ToolCall("search_docs", map[string]string{"query": "dev"}))
	deltas := 0

	resp, err := c.ChatStream(context.Background(), ask("hi"), func(string) { deltas++ })
	if err != nil || deltas != 0 || len(resp.Message.ToolCalls) != 1 {
		t.Fatalf("resp = %+v, deltas = %d, err = %v", resp.Message, deltas, err)
	}
}

func TestToolCallWithTextStreamsTheTextFirst(t *testing.T) {
	c := New(ToolCall("search_docs", map[string]string{"query": "dev"}).WithText("Сейчас поищу"))
	var deltas []string

	resp, err := c.ChatStream(context.Background(), ask("hi"), func(d string) { deltas = append(deltas, d) })
	if err != nil {
		t.Fatalf("ChatStream: %v", err)
	}
	if strings.Join(deltas, "") != "Сейчас поищу" || len(resp.Message.ToolCalls) != 1 {
		t.Fatalf("deltas = %q, message = %+v", deltas, resp.Message)
	}
}

func TestChatStreamSharesScriptWithChat(t *testing.T) {
	c := New(Text("a"), Fail(llm.ErrUnavailable))
	if _, err := c.Chat(context.Background(), ask("1")); err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if _, err := c.ChatStream(context.Background(), ask("2"), func(string) {}); !errors.Is(err, llm.ErrUnavailable) {
		t.Fatalf("ChatStream error = %v, want ErrUnavailable", err)
	}
	if len(c.Calls()) != 2 {
		t.Fatalf("calls = %d", len(c.Calls()))
	}
}
