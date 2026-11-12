package tools

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"testing"

	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/domain"
)

type echoIn struct {
	Text  string `json:"text" jsonschema:"text to echo back"`
	Times int    `json:"times,omitempty" jsonschema:"how many times, default 1"`
}

type echoOut struct {
	Echo string `json:"echo"`
}

var (
	intern  = domain.Principal{UserID: "u-intern", Role: domain.RoleIntern, TeamID: "t-msg"}
	manager = domain.Principal{UserID: "u-manager", Role: domain.RoleManager, TeamID: "t-msg"}
	errBoom = errors.New("boom")
)

// echoTool counts handler calls so tests can prove a rejected call never reached it.
func echoTool(t *testing.T, calls *int) Tool {
	t.Helper()
	tool, err := Define("echo", "Echo text back",
		[]domain.Role{domain.RoleIntern},
		[]domain.ThreadType{domain.ThreadGeneral, domain.ThreadTask},
		func(_ context.Context, _ domain.Principal, in echoIn) (echoOut, error) {
			*calls++
			if in.Text == "fail" {
				return echoOut{}, errBoom
			}
			out := in.Text
			for range in.Times - 1 {
				out += in.Text
			}
			return echoOut{Echo: out}, nil
		})
	if err != nil {
		t.Fatalf("Define: %v", err)
	}
	return tool
}

func newRegistry(t *testing.T, tools ...Tool) *Registry {
	t.Helper()
	r, err := NewRegistry(tools...)
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	return r
}

func TestCallRunsHandlerWithDecodedArgs(t *testing.T) {
	calls := 0
	r := newRegistry(t, echoTool(t, &calls))

	out, err := r.Call(context.Background(), intern, domain.ThreadGeneral, "echo", json.RawMessage(`{"text":"ab","times":2}`))
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if string(out) != `{"echo":"abab"}` {
		t.Fatalf("output = %s", out)
	}
	if calls != 1 {
		t.Fatalf("handler calls = %d", calls)
	}
}

func TestCallRejectsBeforeHandler(t *testing.T) {
	cases := []struct {
		name      string
		principal domain.Principal
		thread    domain.ThreadType
		tool      string
		args      string
		want      error
	}{
		{"unknown tool", intern, domain.ThreadGeneral, "drop_db", `{}`, ErrUnknownTool},
		{"role not allowed", manager, domain.ThreadGeneral, "echo", `{"text":"x"}`, ErrForbidden},
		{"thread type not allowed", intern, domain.ThreadPlan, "echo", `{"text":"x"}`, ErrNotInThread},
		{"missing required field", intern, domain.ThreadGeneral, "echo", `{"times":2}`, ErrInvalidArgs},
		{"wrong type", intern, domain.ThreadGeneral, "echo", `{"text":5}`, ErrInvalidArgs},
		{"unknown field", intern, domain.ThreadGeneral, "echo", `{"text":"x","sql":"drop"}`, ErrInvalidArgs},
		{"malformed json", intern, domain.ThreadGeneral, "echo", `{"text":`, ErrInvalidArgs},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			r := newRegistry(t, echoTool(t, &calls))

			_, err := r.Call(context.Background(), tc.principal, tc.thread, tc.tool, json.RawMessage(tc.args))

			if !errors.Is(err, tc.want) {
				t.Fatalf("error = %v, want %v", err, tc.want)
			}
			if calls != 0 {
				t.Fatal("handler ran for a rejected call")
			}
		})
	}
}

func TestCallPassesHandlerErrorThrough(t *testing.T) {
	calls := 0
	r := newRegistry(t, echoTool(t, &calls))

	_, err := r.Call(context.Background(), intern, domain.ThreadGeneral, "echo", json.RawMessage(`{"text":"fail"}`))
	if !errors.Is(err, errBoom) {
		t.Fatalf("error = %v, want errBoom", err)
	}
}

func TestAvailableFiltersByRoleAndThread(t *testing.T) {
	calls := 0
	planOnly, err := Define("plan_note", "Note on a plan item",
		[]domain.Role{domain.RoleIntern, domain.RoleManager},
		[]domain.ThreadType{domain.ThreadPlan},
		func(context.Context, domain.Principal, echoIn) (echoOut, error) { return echoOut{}, nil })
	if err != nil {
		t.Fatalf("Define: %v", err)
	}
	r := newRegistry(t, echoTool(t, &calls), planOnly)

	names := func(p domain.Principal, th domain.ThreadType) []string {
		var out []string
		for _, d := range r.Available(p, th) {
			out = append(out, d.Name)
		}
		return out
	}
	if got := names(intern, domain.ThreadGeneral); !slices.Equal(got, []string{"echo"}) {
		t.Errorf("intern/general = %v", got)
	}
	if got := names(intern, domain.ThreadPlan); !slices.Equal(got, []string{"plan_note"}) {
		t.Errorf("intern/plan = %v", got)
	}
	if got := names(manager, domain.ThreadGeneral); len(got) != 0 {
		t.Errorf("manager/general = %v, want none", got)
	}
}

func TestAvailableDescribesArgumentsForTheModel(t *testing.T) {
	calls := 0
	r := newRegistry(t, echoTool(t, &calls))

	defs := r.Available(intern, domain.ThreadGeneral)
	if len(defs) != 1 || defs[0].Description != "Echo text back" {
		t.Fatalf("definitions = %+v", defs)
	}
	var schema struct {
		Type       string                       `json:"type"`
		Required   []string                     `json:"required"`
		Properties map[string]map[string]string `json:"properties"`
	}
	if err := json.Unmarshal(defs[0].Parameters, &schema); err != nil {
		t.Fatalf("parameters: %v", err)
	}
	if schema.Type != "object" || !slices.Equal(schema.Required, []string{"text"}) {
		t.Errorf("schema = %+v", schema)
	}
	if schema.Properties["text"]["description"] != "text to echo back" {
		t.Errorf("text property = %v", schema.Properties["text"])
	}
}

func TestDefineRequiresExplicitRights(t *testing.T) {
	handler := func(context.Context, domain.Principal, echoIn) (echoOut, error) { return echoOut{}, nil }

	if _, err := Define("x", "d", nil, []domain.ThreadType{domain.ThreadGeneral}, handler); err == nil {
		t.Error("Define accepted a tool without roles")
	}
	if _, err := Define("x", "d", []domain.Role{domain.RoleIntern}, nil, handler); err == nil {
		t.Error("Define accepted a tool without thread types")
	}
}

func TestDefineRejectsArgsThatAreNotObjects(t *testing.T) {
	handler := func(context.Context, domain.Principal, string) (echoOut, error) { return echoOut{}, nil }

	if _, err := Define("x", "d", []domain.Role{domain.RoleIntern}, []domain.ThreadType{domain.ThreadGeneral}, handler); err == nil {
		t.Error("Define accepted a string argument; tools take JSON objects")
	}
}

func TestNewRegistryRejectsDuplicateNames(t *testing.T) {
	calls := 0
	if _, err := NewRegistry(echoTool(t, &calls), echoTool(t, &calls)); err == nil {
		t.Fatal("NewRegistry accepted two tools named echo")
	}
}
