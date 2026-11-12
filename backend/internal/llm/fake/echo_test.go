package fake

import (
	"context"
	"strings"
	"testing"
)

func TestEchoAnswersEveryRequestWithItsText(t *testing.T) {
	e := NewEcho("Тестовый режим.")
	for range 3 {
		var deltas []string
		resp, err := e.ChatStream(context.Background(), ask("что угодно"), func(d string) { deltas = append(deltas, d) })
		if err != nil || resp.Message.Content != "Тестовый режим." || strings.Join(deltas, "") != "Тестовый режим." {
			t.Fatalf("resp = %+v, deltas %q, err %v", resp.Message, deltas, err)
		}
		if len(resp.Message.ToolCalls) != 0 {
			t.Fatal("echo called a tool")
		}
	}
	if resp, err := e.Chat(context.Background(), ask("x")); err != nil || resp.Message.Content != "Тестовый режим." {
		t.Fatalf("Chat = %+v, %v", resp, err)
	}
}

func TestEchoHonoursCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := NewEcho("x").ChatStream(ctx, ask("x"), func(string) {}); err == nil {
		t.Fatal("cancelled context ignored")
	}
}
