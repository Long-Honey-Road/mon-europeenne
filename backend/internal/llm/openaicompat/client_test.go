package openaicompat

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/llm"
)

// fakeServer answers /chat/completions with status and body and keeps the last request body.
type fakeServer struct {
	*httptest.Server
	lastBody map[string]any
}

func newServer(t *testing.T, status int, body string) *fakeServer {
	t.Helper()
	s := &fakeServer{}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("path = %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("Authorization = %q", got)
		}
		raw, _ := io.ReadAll(r.Body)
		s.lastBody = map[string]any{}
		_ = json.Unmarshal(raw, &s.lastBody)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(s.Close)
	return s
}

func newClient(s *fakeServer, effort string) *Client {
	return New(Config{
		BaseURL:         s.URL + "/v1",
		Model:           "qwen3.6:35b-a3b",
		APIKey:          "test-key",
		ReasoningEffort: effort,
	})
}

const textReply = `{"id":"1","object":"chat.completion","created":1,"model":"qwen3.6:35b-a3b",
"choices":[{"index":0,"finish_reason":"stop","message":{"role":"assistant","content":"Париж","reasoning":"..."}}],
"usage":{"prompt_tokens":19,"completion_tokens":4,"total_tokens":23}}`

const toolReply = `{"id":"2","object":"chat.completion","created":1,"model":"qwen3.6:35b-a3b",
"choices":[{"index":0,"finish_reason":"tool_calls","message":{"role":"assistant","content":"",
"tool_calls":[{"id":"call_79","index":0,"type":"function","function":{"name":"search_docs","arguments":"{\"query\":\"dev\",\"limit\":5}"}}]}}],
"usage":{"prompt_tokens":40,"completion_tokens":32,"total_tokens":72}}`

var searchTool = llm.Tool{
	Name:        "search_docs",
	Description: "Search team documents",
	Parameters:  json.RawMessage(`{"type":"object","properties":{"query":{"type":"string"}},"required":["query"]}`),
}

func TestChatReturnsTextAndUsage(t *testing.T) {
	s := newServer(t, 200, textReply)

	resp, err := newClient(s, "").Chat(context.Background(), llm.Request{
		Messages: []llm.Message{{Role: llm.RoleUser, Content: "Столица Франции?"}},
	})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}

	if resp.Message.Role != llm.RoleAssistant || resp.Message.Content != "Париж" || len(resp.Message.ToolCalls) != 0 {
		t.Errorf("message = %+v", resp.Message)
	}
	if resp.Usage != (llm.Usage{PromptTokens: 19, CompletionTokens: 4}) {
		t.Errorf("usage = %+v", resp.Usage)
	}
	if s.lastBody["model"] != "qwen3.6:35b-a3b" {
		t.Errorf("model = %v", s.lastBody["model"])
	}
	for _, key := range []string{"reasoning_effort", "response_format", "tools"} {
		if _, ok := s.lastBody[key]; ok {
			t.Errorf("%s sent but not configured: %v", key, s.lastBody[key])
		}
	}
}

func TestChatSendsConversationWithToolTurns(t *testing.T) {
	s := newServer(t, 200, textReply)

	_, err := newClient(s, "none").Chat(context.Background(), llm.Request{
		Messages: []llm.Message{
			{Role: llm.RoleSystem, Content: "be brief"},
			{Role: llm.RoleUser, Content: "how to run dev?"},
			{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{
				{ID: "call_1", Name: "search_docs", Arguments: json.RawMessage(`{"query":"dev"}`)},
			}},
			{Role: llm.RoleTool, ToolCallID: "call_1", Content: `{"chunks":[]}`},
		},
		Tools: []llm.Tool{searchTool},
	})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}

	if s.lastBody["reasoning_effort"] != "none" {
		t.Errorf("reasoning_effort = %v", s.lastBody["reasoning_effort"])
	}
	msgs := s.lastBody["messages"].([]any)
	if len(msgs) != 4 {
		t.Fatalf("messages = %v", msgs)
	}
	wantRoles := []string{"system", "user", "assistant", "tool"}
	for i, m := range msgs {
		if role := m.(map[string]any)["role"]; role != wantRoles[i] {
			t.Errorf("messages[%d].role = %v, want %s", i, role, wantRoles[i])
		}
	}
	call := msgs[2].(map[string]any)["tool_calls"].([]any)[0].(map[string]any)
	fn := call["function"].(map[string]any)
	if call["id"] != "call_1" || fn["name"] != "search_docs" || fn["arguments"] != `{"query":"dev"}` {
		t.Errorf("assistant tool call = %v", call)
	}
	if msgs[3].(map[string]any)["tool_call_id"] != "call_1" {
		t.Errorf("tool message = %v", msgs[3])
	}
	tool := s.lastBody["tools"].([]any)[0].(map[string]any)["function"].(map[string]any)
	params := tool["parameters"].(map[string]any)
	if tool["name"] != "search_docs" || tool["description"] != "Search team documents" || params["type"] != "object" {
		t.Errorf("tool = %v", tool)
	}
}

func TestChatParsesToolCalls(t *testing.T) {
	s := newServer(t, 200, toolReply)

	resp, err := newClient(s, "").Chat(context.Background(), llm.Request{
		Messages: []llm.Message{{Role: llm.RoleUser, Content: "how to run dev?"}},
		Tools:    []llm.Tool{searchTool},
	})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}

	calls := resp.Message.ToolCalls
	if len(calls) != 1 {
		t.Fatalf("tool calls = %+v", calls)
	}
	if calls[0].ID != "call_79" || calls[0].Name != "search_docs" || string(calls[0].Arguments) != `{"query":"dev","limit":5}` {
		t.Errorf("tool call = %+v (args %s)", calls[0], calls[0].Arguments)
	}
}

func TestChatSendsJSONSchemaFormat(t *testing.T) {
	s := newServer(t, 200, textReply)
	schema := json.RawMessage(`{"type":"object","properties":{"answer":{"type":"string"}},"required":["answer"]}`)

	_, err := newClient(s, "").Chat(context.Background(), llm.Request{
		Messages: []llm.Message{{Role: llm.RoleUser, Content: "hi"}},
		Format:   &llm.Format{Name: "reply", Schema: schema},
	})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}

	rf := s.lastBody["response_format"].(map[string]any)
	js := rf["json_schema"].(map[string]any)
	if rf["type"] != "json_schema" || js["name"] != "reply" || js["strict"] != true {
		t.Errorf("response_format = %v", rf)
	}
	if js["schema"].(map[string]any)["type"] != "object" {
		t.Errorf("schema = %v", js["schema"])
	}
}

func TestChatRejectsMalformedReplies(t *testing.T) {
	cases := map[string]string{
		"no choices": `{"id":"3","object":"chat.completion","created":1,"model":"m","choices":[]}`,
		"tool arguments not json": `{"id":"4","object":"chat.completion","created":1,"model":"m",
"choices":[{"index":0,"finish_reason":"tool_calls","message":{"role":"assistant",
"tool_calls":[{"id":"c","type":"function","function":{"name":"search_docs","arguments":"{query: dev"}}]}}]}`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			s := newServer(t, 200, body)
			_, err := newClient(s, "").Chat(context.Background(), llm.Request{
				Messages: []llm.Message{{Role: llm.RoleUser, Content: "hi"}},
			})
			if !errors.Is(err, llm.ErrBadResponse) {
				t.Fatalf("error = %v, want ErrBadResponse", err)
			}
		})
	}
}

func TestChatMapsProviderFailuresToUnavailable(t *testing.T) {
	for _, status := range []int{429, 500, 503} {
		s := newServer(t, status, `{"error":{"message":"busy","type":"server_error"}}`)
		_, err := newClient(s, "").Chat(context.Background(), llm.Request{
			Messages: []llm.Message{{Role: llm.RoleUser, Content: "hi"}},
		})
		if !errors.Is(err, llm.ErrUnavailable) {
			t.Errorf("status %d: error = %v, want ErrUnavailable", status, err)
		}
	}
}

func TestChatKeepsClientErrorsDistinct(t *testing.T) {
	s := newServer(t, 400, `{"error":{"message":"model not found","type":"invalid_request_error"}}`)

	_, err := newClient(s, "").Chat(context.Background(), llm.Request{
		Messages: []llm.Message{{Role: llm.RoleUser, Content: "hi"}},
	})
	if err == nil || errors.Is(err, llm.ErrUnavailable) {
		t.Fatalf("error = %v, want a non-retryable error", err)
	}
}

func TestChatMapsTimeoutAndRefusedConnection(t *testing.T) {
	slow := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(2 * time.Second):
		}
	}))
	t.Cleanup(slow.Close)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := New(Config{BaseURL: slow.URL, Model: "m"}).Chat(ctx, llm.Request{
		Messages: []llm.Message{{Role: llm.RoleUser, Content: "hi"}},
	})
	if !errors.Is(err, llm.ErrUnavailable) || !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("timeout error = %v, want ErrUnavailable wrapping DeadlineExceeded", err)
	}

	_, err = New(Config{BaseURL: "http://127.0.0.1:1", Model: "m"}).Chat(context.Background(), llm.Request{
		Messages: []llm.Message{{Role: llm.RoleUser, Content: "hi"}},
	})
	if !errors.Is(err, llm.ErrUnavailable) {
		t.Errorf("refused connection error = %v, want ErrUnavailable", err)
	}
}

func TestChatRejectsInvalidToolSchemaBeforeCalling(t *testing.T) {
	s := newServer(t, 200, textReply)

	_, err := newClient(s, "").Chat(context.Background(), llm.Request{
		Messages: []llm.Message{{Role: llm.RoleUser, Content: "hi"}},
		Tools:    []llm.Tool{{Name: "broken", Parameters: json.RawMessage(`{not json`)}},
	})
	if err == nil {
		t.Fatal("Chat accepted a tool with invalid JSON Schema")
	}
	if s.lastBody != nil {
		t.Error("request reached the provider")
	}
}

func TestChatRejectsUnknownRole(t *testing.T) {
	s := newServer(t, 200, textReply)

	_, err := newClient(s, "").Chat(context.Background(), llm.Request{
		Messages: []llm.Message{{Role: "narrator", Content: "hi"}},
	})
	if err == nil {
		t.Fatal("Chat accepted an unknown role")
	}
}
