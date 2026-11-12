package openaicompat

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/llm"
)

// streamServer answers with the given SSE chunks, one data line each, then [DONE].
func streamServer(t *testing.T, chunks []string) (*httptest.Server, *map[string]any) {
	t.Helper()
	var body map[string]any
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &body)
		w.Header().Set("Content-Type", "text/event-stream")
		for _, c := range chunks {
			fmt.Fprintf(w, "data: %s\n\n", c)
			w.(http.Flusher).Flush()
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	t.Cleanup(s.Close)
	return s, &body
}

func chunk(delta, finish string) string {
	f := "null"
	if finish != "" {
		f = `"` + finish + `"`
	}
	return `{"id":"c","object":"chat.completion.chunk","created":1,"model":"m","choices":[{"index":0,"delta":` + delta + `,"finish_reason":` + f + `}]}`
}

const usageChunk = `{"id":"c","object":"chat.completion.chunk","created":1,"model":"m","choices":[],"usage":{"prompt_tokens":19,"completion_tokens":4,"total_tokens":23}}`

func ask() llm.Request {
	return llm.Request{Messages: []llm.Message{{Role: llm.RoleUser, Content: "hi"}}}
}

func TestChatStreamEmitsTextDeltas(t *testing.T) {
	s, body := streamServer(t, []string{
		chunk(`{"role":"assistant","content":"Па"}`, ""),
		chunk(`{"content":"риж"}`, ""),
		chunk(`{}`, "stop"),
		usageChunk,
	})
	c := New(Config{BaseURL: s.URL, Model: "m"})
	var deltas []string

	resp, err := c.ChatStream(context.Background(), ask(), func(d string) { deltas = append(deltas, d) })
	if err != nil {
		t.Fatalf("ChatStream: %v", err)
	}

	if !slices.Equal(deltas, []string{"Па", "риж"}) {
		t.Errorf("deltas = %q", deltas)
	}
	if resp.Message.Role != llm.RoleAssistant || resp.Message.Content != "Париж" || len(resp.Message.ToolCalls) != 0 {
		t.Errorf("message = %+v", resp.Message)
	}
	if resp.Usage != (llm.Usage{PromptTokens: 19, CompletionTokens: 4}) {
		t.Errorf("usage = %+v", resp.Usage)
	}
	opts, _ := (*body)["stream_options"].(map[string]any)
	if (*body)["stream"] != true || opts["include_usage"] != true {
		t.Errorf("request stream=%v stream_options=%v", (*body)["stream"], (*body)["stream_options"])
	}
}

func TestChatStreamAssemblesToolCalls(t *testing.T) {
	s, _ := streamServer(t, []string{
		chunk(`{"role":"assistant","tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"search_docs","arguments":"{\"query\":"}}]}`, ""),
		chunk(`{"tool_calls":[{"index":0,"function":{"arguments":"\"dev\"}"}}]}`, ""),
		chunk(`{}`, "tool_calls"),
	})
	c := New(Config{BaseURL: s.URL, Model: "m"})
	deltas := 0

	resp, err := c.ChatStream(context.Background(), ask(), func(string) { deltas++ })
	if err != nil {
		t.Fatalf("ChatStream: %v", err)
	}

	if deltas != 0 {
		t.Errorf("%d text deltas for a pure tool call", deltas)
	}
	calls := resp.Message.ToolCalls
	if len(calls) != 1 || calls[0].ID != "call_1" || calls[0].Name != "search_docs" || string(calls[0].Arguments) != `{"query":"dev"}` {
		t.Fatalf("tool calls = %+v", calls)
	}
}

func TestChatStreamRejectsBrokenToolArguments(t *testing.T) {
	s, _ := streamServer(t, []string{
		chunk(`{"role":"assistant","tool_calls":[{"index":0,"id":"c","type":"function","function":{"name":"x","arguments":"{oops"}}]}`, ""),
		chunk(`{}`, "tool_calls"),
	})
	_, err := New(Config{BaseURL: s.URL, Model: "m"}).ChatStream(context.Background(), ask(), func(string) {})
	if !errors.Is(err, llm.ErrBadResponse) {
		t.Fatalf("error = %v, want ErrBadResponse", err)
	}
}

func TestChatStreamRejectsEmptyStream(t *testing.T) {
	s, _ := streamServer(t, nil)
	_, err := New(Config{BaseURL: s.URL, Model: "m"}).ChatStream(context.Background(), ask(), func(string) {})
	if !errors.Is(err, llm.ErrBadResponse) {
		t.Fatalf("error = %v, want ErrBadResponse", err)
	}
}

func TestChatStreamMapsProviderFailure(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = io.WriteString(w, `{"error":{"message":"busy"}}`)
	}))
	t.Cleanup(s.Close)

	_, err := New(Config{BaseURL: s.URL, Model: "m"}).ChatStream(context.Background(), ask(), func(string) {})
	if !errors.Is(err, llm.ErrUnavailable) {
		t.Fatalf("error = %v, want ErrUnavailable", err)
	}
}

func TestChatStreamTimesOutMidStream(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprintf(w, "data: %s\n\n", chunk(`{"role":"assistant","content":"Па"}`, ""))
		w.(http.Flusher).Flush()
		select {
		case <-r.Context().Done():
		case <-time.After(2 * time.Second):
		}
	}))
	t.Cleanup(s.Close)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, err := New(Config{BaseURL: s.URL, Model: "m"}).ChatStream(ctx, ask(), func(string) {})
	if !errors.Is(err, llm.ErrUnavailable) {
		t.Fatalf("error = %v, want ErrUnavailable", err)
	}
}

func TestChatStreamRejectsInvalidRequest(t *testing.T) {
	_, err := New(Config{BaseURL: "http://127.0.0.1:1", Model: "m"}).ChatStream(context.Background(),
		llm.Request{Messages: []llm.Message{{Role: "narrator"}}}, func(string) {})
	if err == nil || !strings.Contains(err.Error(), "role") {
		t.Fatalf("error = %v, want unknown role", err)
	}
}
