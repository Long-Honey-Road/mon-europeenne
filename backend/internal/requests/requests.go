// Package requests helps prepare access requests to the corporate service. It is a removable module behind
// FEATURE_REQUESTS: deleting this package, store/requests.go and its registration removes the feature.
package requests

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/google/jsonschema-go/jsonschema"

	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/domain"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/people"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/tools"
	"github.com/Long-Honey-Road/mon-europeenne/backend/seed"
)

type Approval struct {
	Resolver string `json:"resolver"`
	Object   string `json:"object,omitempty"`
}

type Type struct {
	Key           string          `json:"key"`
	Title         string          `json:"title"`
	Category      string          `json:"category"`
	FieldsSchema  json.RawMessage `json:"fields_schema"`
	ApprovalChain []Approval      `json:"approval_chain"`
	SLAHours      int             `json:"sla_hours,omitempty"`
}

// Object is something access is requested to; its owner approves.
type Object struct {
	Kind, Key, Title, OwnerID string
}

// Catalog is what the corporate service tells about request types; the agent knows nothing beyond it.
type Catalog struct {
	Types   []Type
	Objects []Object
	// RoleApprovers maps devops and hr_admin to the person who approves for that role.
	RoleApprovers map[string]string
}

type Approver struct {
	Role     string `json:"role"`
	PersonID string `json:"person_id,omitempty"`
	Name     string `json:"name,omitempty"`
}

// Store reads the mirror of submitted requests; Request also returns the author's team.
type Store interface {
	Request(ctx context.Context, externalID string) (domain.Request, string, error)
	RequestsOf(ctx context.Context, userID string) ([]domain.Request, error)
}

func CatalogFromSeed(d seed.Data) Catalog {
	cat := Catalog{RoleApprovers: map[string]string{}}
	for _, t := range d.Requests.Types {
		typ := Type{Key: t.Key, Title: t.Title, Category: t.Category, FieldsSchema: t.FieldsSchema, SLAHours: t.SLAHours}
		for _, a := range t.ApprovalChain {
			typ.ApprovalChain = append(typ.ApprovalChain, Approval{Resolver: a.Resolver, Object: a.Object})
		}
		cat.Types = append(cat.Types, typ)
	}
	for _, o := range d.Requests.Objects {
		cat.Objects = append(cat.Objects, Object{Kind: o.Kind, Key: o.Key, Title: o.Title, OwnerID: o.OwnerID})
	}
	for _, p := range d.People.People {
		for _, role := range p.ApproverFor {
			cat.RoleApprovers[role] = p.ID
		}
	}
	return cat
}

var (
	typeThreads    = []domain.ThreadType{domain.ThreadGeneral, domain.ThreadRequest}
	prepareThreads = []domain.ThreadType{domain.ThreadGeneral, domain.ThreadPlan, domain.ThreadRequest}
	allRoles       = []domain.Role{domain.RoleIntern, domain.RoleMentor, domain.RoleManager, domain.RoleHRBP, domain.RoleHRAdmin}
)

// Tools returns the module's tools for the registry.
func Tools(cat Catalog, st Store, dir tools.PeopleDirectory) ([]tools.Tool, error) {
	m := module{cat: cat, store: st, dir: dir}
	var out []tools.Tool
	for _, build := range []func() (tools.Tool, error){m.types, m.prepare, m.status, m.list} {
		t, err := build()
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, nil
}

type module struct {
	cat   Catalog
	store Store
	dir   tools.PeopleDirectory
}

func (m module) typeByKey(key string) (Type, bool) { return m.cat.TypeByKey(key) }

func (c Catalog) TypeByKey(key string) (Type, bool) {
	for _, t := range c.Types {
		if t.Key == key {
			return t, true
		}
	}
	return Type{}, false
}

type typesIn struct {
	Category string `json:"category,omitempty" jsonschema:"only this category, e.g. access"`
}

type typesOut struct {
	Types []Type `json:"types"`
}

func (m module) types() (tools.Tool, error) {
	return tools.Define("get_request_types",
		"Request types of the corporate service: fields to fill in (JSON Schema) and who approves.",
		allRoles, typeThreads,
		func(_ context.Context, _ domain.Principal, in typesIn) (typesOut, error) {
			out := typesOut{Types: []Type{}}
			for _, t := range m.cat.Types {
				if in.Category == "" || t.Category == in.Category {
					out.Types = append(out.Types, t)
				}
			}
			return out, nil
		})
}

type prepareIn struct {
	TypeKey string         `json:"type_key" jsonschema:"request type key from get_request_types, e.g. access.jira_project"`
	Fields  map[string]any `json:"fields" jsonschema:"field values collected in the dialog"`
}

type preview struct {
	Title     string         `json:"title"`
	Fields    map[string]any `json:"fields"`
	Approvers []Approver     `json:"approvers"`
}

type prepareOut struct {
	Valid   bool     `json:"valid"`
	Missing []string `json:"missing"`
	Errors  []string `json:"errors"`
	Preview preview  `json:"preview"`
	// Action becomes the "action" block: a form the user confirms and sends; only for a valid request.
	Action *Action `json:"action,omitempty"`
}

// Action is what the frontend needs to show the form and call POST /v1/private/requests.
type Action struct {
	Kind      string         `json:"kind"`
	TypeKey   string         `json:"type_key"`
	Title     string         `json:"title"`
	Fields    map[string]any `json:"fields"`
	Approvers []Approver     `json:"approvers"`
}

// prepare checks the fields and shows who approves; it never sends anything.
func (m module) prepare() (tools.Tool, error) {
	return tools.Define("prepare_request",
		"Check a request and show the user a form to send it: missing fields, invalid values and who will approve it. "+
			"Sends nothing, so call it as soon as the required fields are known, without asking the user first.",
		allRoles, prepareThreads,
		func(ctx context.Context, p domain.Principal, in prepareIn) (prepareOut, error) {
			typ, ok := m.typeByKey(in.TypeKey)
			if !ok {
				return prepareOut{}, fmt.Errorf("%w: request type %q", tools.ErrNotFound, in.TypeKey)
			}
			fields := in.Fields
			if fields == nil {
				fields = map[string]any{}
			}
			missing, problems, err := check(typ.FieldsSchema, fields)
			if err != nil {
				return prepareOut{}, err
			}
			approvers, err := m.approvers(ctx, p, typ, fields)
			if err != nil {
				return prepareOut{}, err
			}
			out := prepareOut{
				Valid: len(missing) == 0 && len(problems) == 0, Missing: missing, Errors: problems,
				Preview: preview{Title: typ.Title, Fields: fields, Approvers: approvers},
			}
			if out.Valid {
				out.Action = &Action{Kind: "request", TypeKey: typ.Key, Title: typ.Title, Fields: fields, Approvers: approvers}
			}
			return out, nil
		})
}

// check splits schema problems into missing required fields and invalid given values.
func check(schemaJSON json.RawMessage, fields map[string]any) (missing, problems []string, err error) {
	var schema jsonschema.Schema
	if err := json.Unmarshal(schemaJSON, &schema); err != nil {
		return nil, nil, fmt.Errorf("request fields schema: %w", err)
	}
	missing = []string{}
	for _, name := range schema.Required {
		if v, ok := fields[name]; !ok || v == nil || v == "" {
			missing = append(missing, name)
		}
	}
	slices.Sort(missing)

	schema.Required = nil
	resolved, err := schema.Resolve(nil)
	if err != nil {
		return nil, nil, fmt.Errorf("request fields schema: %w", err)
	}
	problems = []string{}
	if err := resolved.Validate(fields); err != nil {
		problems = append(problems, err.Error())
	}
	return missing, problems, nil
}

func (m module) approvers(ctx context.Context, p domain.Principal, typ Type, fields map[string]any) ([]Approver, error) {
	return ResolveApprovers(ctx, m.cat, m.dir, p.UserID, typ, fields)
}

// ResolveApprovers names who approves each step of the type's chain for this requester and these fields.
func ResolveApprovers(ctx context.Context, cat Catalog, dir tools.PeopleDirectory, userID string, typ Type,
	fields map[string]any,
) ([]Approver, error) {
	out := []Approver{}
	for _, step := range typ.ApprovalChain {
		var personID string
		switch step.Resolver {
		case "object_owner":
			personID = cat.objectOwner(step.Object, fields)
		case "manager":
			rel, err := dir.Relations(ctx, userID)
			if err != nil {
				return nil, err
			}
			personID = rel.ManagerID
		default:
			personID = cat.RoleApprovers[step.Resolver]
		}
		a := Approver{Role: step.Resolver}
		if personID != "" {
			person, err := dir.Person(ctx, personID)
			if err != nil && !errors.Is(err, people.ErrNotFound) {
				return nil, err
			}
			a.PersonID, a.Name = person.ID, person.Name
		}
		out = append(out, a)
	}
	return out, nil
}

// objectOwner finds the object of the kind whose key is one of the field values ("~messenger-team" matches too).
func (c Catalog) objectOwner(kind string, fields map[string]any) string {
	for _, v := range fields {
		s, ok := v.(string)
		if !ok {
			continue
		}
		s = strings.TrimPrefix(strings.TrimSpace(s), "~")
		for _, o := range c.Objects {
			if o.Kind == kind && strings.EqualFold(o.Key, s) {
				return o.OwnerID
			}
		}
	}
	return ""
}

type statusIn struct {
	RequestID string `json:"request_id" jsonschema:"request id in the corporate service, e.g. REQ-12"`
}

type statusOut struct {
	RequestID string          `json:"request_id"`
	TypeKey   string          `json:"type_key"`
	Status    string          `json:"status"`
	Approver  json.RawMessage `json:"approver"`
	History   json.RawMessage `json:"history"`
}

// status shows a request to its author and to a manager of the author's team.
func (m module) status() (tools.Tool, error) {
	return tools.Define("get_request_status",
		"Status, current approver and history of one request.",
		allRoles, typeThreads,
		func(ctx context.Context, p domain.Principal, in statusIn) (statusOut, error) {
			r, team, err := m.store.Request(ctx, in.RequestID)
			if err != nil {
				return statusOut{}, err
			}
			own := r.UserID == p.UserID
			teamManager := p.Role == domain.RoleManager && team != "" && team == p.TeamID
			if !own && !teamManager {
				return statusOut{}, tools.ErrForbidden
			}
			return statusOut{RequestID: r.ExternalID, TypeKey: r.TypeKey, Status: r.Status, Approver: r.Approver, History: r.History}, nil
		})
}

type listIn struct {
	Category string `json:"category,omitempty" jsonschema:"only this category, e.g. access"`
	Status   string `json:"status,omitempty" jsonschema:"only requests in this status"`
}

type listItem struct {
	RequestID string          `json:"request_id"`
	TypeKey   string          `json:"type_key"`
	Title     string          `json:"title"`
	Status    string          `json:"status"`
	Approver  json.RawMessage `json:"approver,omitempty"`
	UpdatedAt string          `json:"updated_at"`
}

type listOut struct {
	Requests []listItem `json:"requests"`
}

func (m module) list() (tools.Tool, error) {
	return tools.Define("list_my_requests",
		"The user's own requests with their statuses, newest first.",
		allRoles, prepareThreads,
		func(ctx context.Context, p domain.Principal, in listIn) (listOut, error) {
			all, err := m.store.RequestsOf(ctx, p.UserID)
			if err != nil {
				return listOut{}, err
			}
			out := listOut{Requests: []listItem{}}
			for _, r := range all {
				typ, _ := m.typeByKey(r.TypeKey)
				if (in.Category != "" && typ.Category != in.Category) || (in.Status != "" && r.Status != in.Status) {
					continue
				}
				out.Requests = append(out.Requests, listItem{
					RequestID: r.ExternalID, TypeKey: r.TypeKey, Title: typ.Title, Status: r.Status,
					Approver: r.Approver, UpdatedAt: r.UpdatedAt.UTC().Format(time.RFC3339),
				})
			}
			return out, nil
		})
}
