package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/domain"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/tools"
)

var (
	intern  = domain.Principal{UserID: "u-intern", Role: domain.RoleIntern, TeamID: "t-1"}
	anyRole = []domain.Role{domain.RoleIntern, domain.RoleMentor, domain.RoleManager}
)

type echoIn struct {
	Text string `json:"text" jsonschema:"what to echo"`
}

type echoOut struct {
	Text   string `json:"text"`
	UserID string `json:"user_id"`
}

func registry(t *testing.T) *tools.Registry {
	t.Helper()
	echo, err := tools.Define("echo", "Echo the text back.", anyRole, []domain.ThreadType{domain.ThreadGeneral},
		func(_ context.Context, p domain.Principal, in echoIn) (echoOut, error) {
			switch in.Text {
			case "missing":
				return echoOut{}, fmt.Errorf("%w: item", tools.ErrNotFound)
			case "boom":
				return echoOut{}, errors.New("dial tcp 10.0.0.7: connection refused")
			}
			return echoOut{Text: in.Text, UserID: p.UserID}, nil
		})
	if err != nil {
		t.Fatal(err)
	}
	planOnly, err := tools.Define("plan_only", "Only in plan threads.", anyRole, []domain.ThreadType{domain.ThreadPlan},
		func(context.Context, domain.Principal, echoIn) (echoOut, error) { return echoOut{}, nil })
	if err != nil {
		t.Fatal(err)
	}
	mentorOnly, err := tools.Define("mentor_only", "Only for mentors.", []domain.Role{domain.RoleMentor},
		[]domain.ThreadType{domain.ThreadGeneral},
		func(context.Context, domain.Principal, echoIn) (echoOut, error) { return echoOut{}, nil })
	if err != nil {
		t.Fatal(err)
	}
	reg, err := tools.NewRegistry(echo, planOnly, mentorOnly)
	if err != nil {
		t.Fatal(err)
	}
	return reg
}

func connect(t *testing.T, srv *mcp.Server) *mcp.ClientSession {
	t.Helper()
	ctx := context.Background()
	serverT, clientT := mcp.NewInMemoryTransports()
	ss, err := srv.Connect(ctx, serverT, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ss.Close() })
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "0"}, nil).Connect(ctx, clientT, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cs.Close() })
	return cs
}

func TestListsOnlyAllowedTools(t *testing.T) {
	cs := connect(t, New(registry(t), intern, domain.ThreadGeneral, "test"))

	res, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Tools) != 1 || res.Tools[0].Name != "echo" {
		t.Fatalf("tools = %+v, want only echo", res.Tools)
	}
	schema, _ := json.Marshal(res.Tools[0].InputSchema)
	if !strings.Contains(string(schema), `"text"`) || !strings.Contains(string(schema), `"object"`) {
		t.Fatalf("schema = %s", schema)
	}
	if init := cs.InitializeResult(); init.ServerInfo.Name != "buddy" || init.Instructions == "" {
		t.Fatalf("initialize = %+v", init)
	}
}

func TestCallRunsAsPrincipal(t *testing.T) {
	cs := connect(t, New(registry(t), intern, domain.ThreadGeneral, "test"))

	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "echo", Arguments: map[string]any{"text": "hi"}})
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("unexpected error result: %+v", res.Content[0])
	}
	var out echoOut
	if err := json.Unmarshal([]byte(res.Content[0].(*mcp.TextContent).Text), &out); err != nil {
		t.Fatal(err)
	}
	if out != (echoOut{Text: "hi", UserID: "u-intern"}) {
		t.Fatalf("out = %+v", out)
	}
	if res.StructuredContent == nil {
		t.Fatal("structured content is empty")
	}
}

func TestCallErrors(t *testing.T) {
	cs := connect(t, New(registry(t), intern, domain.ThreadGeneral, "test"))
	tests := []struct {
		name string
		args any
		code string
	}{
		{"wrong type", map[string]any{"text": 7}, tools.CodeInvalidArgs},
		{"no args", nil, tools.CodeInvalidArgs},
		{"not found", map[string]any{"text": "missing"}, tools.CodeNotFound},
		{"handler failure", map[string]any{"text": "boom"}, tools.CodeFailed},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "echo", Arguments: tt.args})
			if err != nil {
				t.Fatal(err)
			}
			if !res.IsError {
				t.Fatal("want an error result")
			}
			text := res.Content[0].(*mcp.TextContent).Text
			var body map[string]string
			if err := json.Unmarshal([]byte(text), &body); err != nil {
				t.Fatal(err)
			}
			if body["error"] != tt.code {
				t.Fatalf("error = %q, want %q (%s)", body["error"], tt.code, text)
			}
			if strings.Contains(text, "10.0.0.7") {
				t.Fatalf("handler error leaked: %s", text)
			}
		})
	}
}

func TestToolOutsideThreadIsRefusedByRegistry(t *testing.T) {
	cs := connect(t, New(registry(t), intern, domain.ThreadGeneral, "test"))

	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "plan_only", Arguments: map[string]any{}})
	if err != nil {
		t.Fatal(err)
	}
	if code := errorCode(t, res); code != tools.CodeNotInThread {
		t.Fatalf("error = %q, want %q", code, tools.CodeNotInThread)
	}
}
