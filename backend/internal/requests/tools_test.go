package requests

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/domain"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/tools"
	"github.com/Long-Honey-Road/mon-europeenne/backend/seed"
)

var (
	liza   = domain.Principal{UserID: "u-liza", Role: domain.RoleIntern, TeamID: "t-messenger"}
	dmitry = domain.Principal{UserID: "u-dmitry", Role: domain.RoleManager, TeamID: "t-messenger"}
	anna   = domain.Principal{UserID: "u-anna", Role: domain.RoleMentor, TeamID: "t-messenger"}
)

type memStore struct{ byID map[string]domain.Request }

func (m memStore) Request(_ context.Context, id string) (domain.Request, string, error) {
	r, ok := m.byID[id]
	if !ok {
		return domain.Request{}, "", domain.ErrNotFound
	}
	return r, "t-messenger", nil
}

func (m memStore) RequestsOf(_ context.Context, userID string) ([]domain.Request, error) {
	var out []domain.Request
	for _, id := range []string{"REQ-1", "REQ-2", "REQ-3"} {
		if r := m.byID[id]; r.UserID == userID {
			out = append(out, r)
		}
	}
	return out, nil
}

func fixture(t *testing.T) *tools.Registry {
	t.Helper()
	d, err := seed.Load(seed.Files)
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	cat := CatalogFromSeed(d)
	st := memStore{byID: map[string]domain.Request{
		"REQ-1": {ExternalID: "REQ-1", UserID: "u-liza", TypeKey: "access.jira_project", Status: "submitted",
			Approver: json.RawMessage(`{"person_id":"p-dmitry"}`), History: json.RawMessage(`[]`), UpdatedAt: time.Now()},
		"REQ-2": {ExternalID: "REQ-2", UserID: "u-liza", TypeKey: "access.vpn", Status: "approved", UpdatedAt: time.Now()},
		"REQ-3": {ExternalID: "REQ-3", UserID: "u-dmitry", TypeKey: "access.vpn", Status: "submitted", UpdatedAt: time.Now()},
	}}
	list, err := Tools(cat, st, d.Directory())
	if err != nil {
		t.Fatalf("Tools: %v", err)
	}
	r, err := tools.NewRegistry(list...)
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	return r
}

func callTool(t *testing.T, r *tools.Registry, p domain.Principal, name, args string, out any) error {
	t.Helper()
	raw, err := r.Call(context.Background(), p, domain.ThreadRequest, name, json.RawMessage(args))
	if err != nil {
		return err
	}
	if err := json.Unmarshal(raw, out); err != nil {
		t.Fatalf("decode %s: %v", raw, err)
	}
	return nil
}

type prepared struct {
	Valid   bool     `json:"valid"`
	Missing []string `json:"missing"`
	Errors  []string `json:"errors"`
	Preview struct {
		Title     string         `json:"title"`
		Fields    map[string]any `json:"fields"`
		Approvers []Approver     `json:"approvers"`
	} `json:"preview"`
}

func TestPrepareValidRequestResolvesObjectOwner(t *testing.T) {
	r := fixture(t)
	var out prepared
	err := callTool(t, r, liza, "prepare_request",
		`{"type_key":"access.jira_project","fields":{"project_key":"MSG","level":"write","justification":"Задача MSG-142 из первой недели"}}`, &out)
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	if !out.Valid || len(out.Missing) != 0 || len(out.Errors) != 0 || out.Preview.Title != "Доступ к проекту Jira" {
		t.Fatalf("prepared = %+v", out)
	}
	if want := []Approver{{Role: "object_owner", PersonID: "p-dmitry", Name: "Дмитрий Козлов"}}; !slices.Equal(out.Preview.Approvers, want) {
		t.Errorf("approvers = %+v", out.Preview.Approvers)
	}
}

func TestPrepareListsMissingFields(t *testing.T) {
	r := fixture(t)
	var out prepared
	if err := callTool(t, r, liza, "prepare_request", `{"type_key":"access.jira_project","fields":{"project_key":"MSG"}}`, &out); err != nil {
		t.Fatalf("prepare: %v", err)
	}
	if out.Valid || !slices.Equal(out.Missing, []string{"justification", "level"}) {
		t.Fatalf("prepared = %+v", out)
	}
}

func TestPrepareReportsSchemaViolations(t *testing.T) {
	r := fixture(t)
	var out prepared
	err := callTool(t, r, liza, "prepare_request",
		`{"type_key":"access.jira_project","fields":{"project_key":"MSG","level":"admin","justification":"надо"}}`, &out)
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	if out.Valid || len(out.Missing) != 0 || len(out.Errors) == 0 {
		t.Fatalf("prepared = %+v", out)
	}
}

func TestPrepareResolvesRoleApprover(t *testing.T) {
	r := fixture(t)
	var out prepared
	if err := callTool(t, r, liza, "prepare_request", `{"type_key":"access.vpn","fields":{"justification":"Работа из дома по задаче MSG-142"}}`, &out); err != nil {
		t.Fatalf("prepare: %v", err)
	}
	if !out.Valid || len(out.Preview.Approvers) != 1 || out.Preview.Approvers[0].PersonID != "p-artem" {
		t.Fatalf("prepared = %+v", out)
	}
}

func TestPrepareWithUnknownObjectKeepsTheRole(t *testing.T) {
	r := fixture(t)
	var out prepared
	err := callTool(t, r, liza, "prepare_request",
		`{"type_key":"access.jira_project","fields":{"project_key":"XYZ","level":"read","justification":"Посмотреть задачи соседей"}}`, &out)
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	if got := out.Preview.Approvers; len(got) != 1 || got[0].Role != "object_owner" || got[0].PersonID != "" {
		t.Fatalf("approvers = %+v", got)
	}
}

func TestPrepareUnknownType(t *testing.T) {
	r := fixture(t)
	var out prepared
	if err := callTool(t, r, liza, "prepare_request", `{"type_key":"absence.day_off","fields":{}}`, &out); !errors.Is(err, tools.ErrNotFound) {
		t.Fatalf("error = %v, want ErrNotFound", err)
	}
}

func TestManagerResolver(t *testing.T) {
	d, _ := seed.Load(seed.Files)
	cat := Catalog{Types: []Type{{Key: "x.manager", Title: "X", Category: "x",
		FieldsSchema:  json.RawMessage(`{"type":"object"}`),
		ApprovalChain: []Approval{{Resolver: "manager"}}}}}
	list, err := Tools(cat, memStore{}, d.Directory())
	if err != nil {
		t.Fatalf("Tools: %v", err)
	}
	r, _ := tools.NewRegistry(list...)
	var out prepared
	if err := callTool(t, r, liza, "prepare_request", `{"type_key":"x.manager","fields":{}}`, &out); err != nil {
		t.Fatalf("prepare: %v", err)
	}
	if got := out.Preview.Approvers; len(got) != 1 || got[0].PersonID != "p-dmitry" {
		t.Fatalf("approvers = %+v", got)
	}
}

func TestRequestTypesWithCategoryFilter(t *testing.T) {
	r := fixture(t)
	var all, none struct {
		Types []map[string]any `json:"types"`
	}
	if err := callTool(t, r, liza, "get_request_types", `{}`, &all); err != nil || len(all.Types) != 7 {
		t.Fatalf("types = %d, %v", len(all.Types), err)
	}
	if err := callTool(t, r, liza, "get_request_types", `{"category":"absence"}`, &none); err != nil || len(none.Types) != 0 {
		t.Fatalf("absence types = %d, %v", len(none.Types), err)
	}
	if all.Types[0]["fields_schema"] == nil || all.Types[0]["approval_chain"] == nil {
		t.Errorf("type = %v", all.Types[0])
	}
}

func TestRequestStatusRights(t *testing.T) {
	r := fixture(t)
	var out map[string]any
	for name, p := range map[string]domain.Principal{"author": liza, "manager of her team": dmitry} {
		if err := callTool(t, r, p, "get_request_status", `{"request_id":"REQ-1"}`, &out); err != nil || out["status"] != "submitted" {
			t.Errorf("%s: %v, %v", name, out, err)
		}
	}
	if err := callTool(t, r, anna, "get_request_status", `{"request_id":"REQ-1"}`, &out); !errors.Is(err, tools.ErrForbidden) {
		t.Errorf("mentor: error = %v, want ErrForbidden", err)
	}
	if err := callTool(t, r, liza, "get_request_status", `{"request_id":"REQ-000"}`, &out); !errors.Is(err, tools.ErrNotFound) {
		t.Errorf("unknown: error = %v, want ErrNotFound", err)
	}
}

func TestListMyRequests(t *testing.T) {
	r := fixture(t)
	var out struct {
		Requests []struct {
			RequestID string `json:"request_id"`
			Status    string `json:"status"`
		} `json:"requests"`
	}
	if err := callTool(t, r, liza, "list_my_requests", `{}`, &out); err != nil || len(out.Requests) != 2 || out.Requests[0].RequestID != "REQ-1" {
		t.Fatalf("requests = %+v, %v", out, err)
	}
	if err := callTool(t, r, liza, "list_my_requests", `{"status":"approved"}`, &out); err != nil || len(out.Requests) != 1 || out.Requests[0].RequestID != "REQ-2" {
		t.Fatalf("approved = %+v, %v", out, err)
	}
	if err := callTool(t, r, liza, "list_my_requests", `{"category":"absence"}`, &out); err != nil || len(out.Requests) != 0 {
		t.Fatalf("absence = %+v, %v", out, err)
	}
}

func TestToolsLiveInTheirThreads(t *testing.T) {
	r := fixture(t)
	var names []string
	for _, tl := range r.Available(liza, domain.ThreadTask) {
		names = append(names, tl.Name)
	}
	if len(names) != 0 {
		t.Errorf("request tools in a task thread: %v", names)
	}
}
