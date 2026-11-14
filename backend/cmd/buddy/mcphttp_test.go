package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/tools"
)

type bearerTransport struct{ token string }

func (b bearerTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("Authorization", "Bearer "+b.token)
	return http.DefaultTransport.RoundTrip(r)
}

func mcpOverHTTP(t *testing.T, url, token string) *mcp.ClientSession {
	t.Helper()
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "0"}, nil).Connect(context.Background(),
		&mcp.StreamableClientTransport{Endpoint: url, HTTPClient: &http.Client{Transport: bearerTransport{token}}}, nil)
	if err != nil {
		t.Fatalf("connect %s: %v", url, err)
	}
	t.Cleanup(func() { _ = cs.Close() })
	return cs
}

func listed(t *testing.T, cs *mcp.ClientSession) []string {
	t.Helper()
	res, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, tool := range res.Tools {
		names = append(names, tool.Name)
	}
	return names
}

func TestAPIServesAgentToolsOverMCP(t *testing.T) {
	cfg := stackConfig(t)
	h, cleanup, err := buildAPI(context.Background(), cfg)
	if err != nil {
		t.Fatalf("buildAPI: %v", err)
	}
	t.Cleanup(cleanup)
	// Cleanups run last first: MCP sessions close before the server waits for their streams.
	api := httptest.NewServer(h)
	t.Cleanup(api.Close)
	endpoint := api.URL + "/v1/private/mcp"

	if code, _ := request(t, api.URL, http.MethodPost, "/v1/private/mcp", "", "{}"); code != http.StatusUnauthorized {
		t.Fatalf("no token: status %d, want 401", code)
	}
	_, login := request(t, api.URL, http.MethodPost, "/v1/public/auth/login", "", `{"login":"intern","password":"intern-demo"}`)
	token := login["token"].(string)

	general := mcpOverHTTP(t, endpoint, token)
	names := listed(t, general)
	for _, want := range []string{"who_to_ask", "search_docs", "get_plan"} {
		if !slices.Contains(names, want) {
			t.Fatalf("general tools %v miss %s", names, want)
		}
	}
	// Same set as the worker's agent: plan updates only in a plan thread, request tools only with FEATURE_REQUESTS.
	if slices.Contains(names, "update_item_status") || slices.Contains(names, "list_my_requests") {
		t.Fatalf("general tools %v", names)
	}

	res, err := general.CallTool(context.Background(), &mcp.CallToolParams{Name: "get_plan", Arguments: map[string]any{}})
	if err != nil || res.IsError {
		t.Fatalf("get_plan: %v %+v", err, res)
	}
	if text := res.Content[0].(*mcp.TextContent).Text; !strings.Contains(text, `"items"`) {
		t.Fatalf("get_plan = %s", text)
	}

	res, err = general.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "update_item_status", Arguments: map[string]any{"item_id": "x", "status": "done"},
	})
	if err != nil {
		t.Fatal(err)
	}
	var refusal map[string]string
	_ = json.Unmarshal([]byte(res.Content[0].(*mcp.TextContent).Text), &refusal)
	if !res.IsError || refusal["error"] != tools.CodeNotInThread {
		t.Fatalf("update_item_status in general = %+v", refusal)
	}

	if names := listed(t, mcpOverHTTP(t, endpoint+"?thread=plan", token)); !slices.Contains(names, "update_item_status") {
		t.Fatalf("plan tools %v miss update_item_status", names)
	}
}
