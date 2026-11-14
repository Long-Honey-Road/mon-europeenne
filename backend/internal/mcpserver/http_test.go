package mcpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/auth"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/domain"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/tools"
)

var mentor = domain.Principal{UserID: "u-mentor", Role: domain.RoleMentor, TeamID: "t-1"}

// withPrincipal stands in for the API session middleware: the header names the principal.
func withPrincipal(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Header.Get("Authorization") {
		case "Bearer intern":
			r = r.WithContext(auth.WithPrincipal(r.Context(), intern))
		case "Bearer mentor":
			r = r.WithContext(auth.WithPrincipal(r.Context(), mentor))
		}
		next.ServeHTTP(w, r)
	})
}

func httpServer(t *testing.T) *httptest.Server {
	t.Helper()
	s := httptest.NewServer(withPrincipal(NewHTTPHandler(registry(t), "test")))
	t.Cleanup(s.Close)
	return s
}

type bearer struct {
	token string
	base  http.RoundTripper
}

func (b bearer) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("Authorization", "Bearer "+b.token)
	return b.base.RoundTrip(r)
}

func connectHTTP(t *testing.T, url, token string) *mcp.ClientSession {
	t.Helper()
	transport := &mcp.StreamableClientTransport{
		Endpoint:   url,
		HTTPClient: &http.Client{Transport: bearer{token: token, base: http.DefaultTransport}},
	}
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "0"}, nil).Connect(context.Background(), transport, nil)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = cs.Close() })
	return cs
}

func toolNames(t *testing.T, cs *mcp.ClientSession) []string {
	t.Helper()
	res, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, tool := range res.Tools {
		names = append(names, tool.Name)
	}
	slices.Sort(names)
	return names
}

func errorCode(t *testing.T, res *mcp.CallToolResult) string {
	t.Helper()
	if !res.IsError {
		t.Fatalf("want an error result, got %+v", res.Content)
	}
	var body map[string]string
	if err := json.Unmarshal([]byte(res.Content[0].(*mcp.TextContent).Text), &body); err != nil {
		t.Fatal(err)
	}
	return body["error"]
}

func TestHTTPServesToolsAsSessionUser(t *testing.T) {
	s := httpServer(t)
	cs := connectHTTP(t, s.URL, "intern")

	if names := toolNames(t, cs); !slices.Equal(names, []string{"echo"}) {
		t.Fatalf("tools = %v, want [echo]", names)
	}
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "echo", Arguments: map[string]any{"text": "hi"}})
	if err != nil || res.IsError {
		t.Fatalf("call: %v %+v", err, res)
	}
	if !strings.Contains(res.Content[0].(*mcp.TextContent).Text, `"user_id":"u-intern"`) {
		t.Fatalf("tool ran as someone else: %s", res.Content[0].(*mcp.TextContent).Text)
	}

	mentorTools := toolNames(t, connectHTTP(t, s.URL, "mentor"))
	if !slices.Equal(mentorTools, []string{"echo", "mentor_only"}) {
		t.Fatalf("mentor tools = %v", mentorTools)
	}
}

func TestHTTPThreadComesFromQuery(t *testing.T) {
	s := httpServer(t)
	cs := connectHTTP(t, s.URL+"?thread=plan", "intern")

	if names := toolNames(t, cs); !slices.Equal(names, []string{"plan_only"}) {
		t.Fatalf("tools = %v, want [plan_only]", names)
	}
}

func TestHTTPRefusesLikeTheAgent(t *testing.T) {
	cs := connectHTTP(t, httpServer(t).URL, "intern")
	tests := []struct{ tool, code string }{
		{"mentor_only", tools.CodeForbidden},
		{"plan_only", tools.CodeNotInThread},
		{"rm_rf", tools.CodeUnknownTool},
	}
	for _, tt := range tests {
		t.Run(tt.tool, func(t *testing.T) {
			res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: tt.tool, Arguments: map[string]any{"text": "x"}})
			if err != nil {
				t.Fatal(err)
			}
			if code := errorCode(t, res); code != tt.code {
				t.Fatalf("error = %q, want %q", code, tt.code)
			}
		})
	}
}

func TestHTTPRejectsBadRequests(t *testing.T) {
	s := httpServer(t)
	tests := []struct {
		name, query, token string
		want               int
	}{
		{"no principal", "", "", http.StatusUnauthorized},
		{"unknown thread", "?thread=chat", "intern", http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"t","version":"0"}}}`
			req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, s.URL+tt.query, strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Accept", "application/json, text/event-stream")
			if tt.token != "" {
				req.Header.Set("Authorization", "Bearer "+tt.token)
			}
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			resp.Body.Close()
			if resp.StatusCode != tt.want {
				t.Fatalf("status %d, want %d", resp.StatusCode, tt.want)
			}
		})
	}
}

func TestHTTPSessionBelongsToItsUser(t *testing.T) {
	s := httpServer(t)
	cs := connectHTTP(t, s.URL, "intern")

	body := `{"jsonrpc":"2.0","id":2,"method":"tools/list"}`
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, s.URL, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("Mcp-Session-Id", cs.ID())
	req.Header.Set("Mcp-Protocol-Version", cs.InitializeResult().ProtocolVersion)
	req.Header.Set("Authorization", "Bearer mentor")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status %d, want 403 for another user's session", resp.StatusCode)
	}
}
