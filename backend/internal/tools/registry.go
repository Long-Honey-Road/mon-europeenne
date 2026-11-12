// Package tools holds the agent's tools and the registry that enforces who may call them where.
package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sort"

	"github.com/google/jsonschema-go/jsonschema"

	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/domain"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/llm"
)

var (
	ErrUnknownTool = errors.New("unknown tool")
	ErrForbidden   = errors.New("forbidden")
	ErrNotInThread = errors.New("tool not available in this thread")
	ErrInvalidArgs = errors.New("invalid tool arguments")
)

// Tool is a named action with its argument schema and the roles and thread types allowed to use it.
type Tool struct {
	Name        string
	Description string
	Roles       []domain.Role
	Threads     []domain.ThreadType
	// Schema is the JSON Schema of the arguments, inferred from the Go input type.
	Schema *jsonschema.Schema

	resolved *jsonschema.Resolved
	call     func(context.Context, domain.Principal, json.RawMessage) (any, error)
}

// Define builds a tool from a typed handler; the argument schema comes from In's json and jsonschema tags.
func Define[In, Out any](name, description string, roles []domain.Role, threads []domain.ThreadType,
	fn func(context.Context, domain.Principal, In) (Out, error),
) (Tool, error) {
	if len(roles) == 0 || len(threads) == 0 {
		return Tool{}, fmt.Errorf("tool %q: roles and thread types must be explicit", name)
	}
	schema, err := jsonschema.For[In](nil)
	if err != nil {
		return Tool{}, fmt.Errorf("tool %q schema: %w", name, err)
	}
	if schema.Type != "object" {
		return Tool{}, fmt.Errorf("tool %q: arguments must be an object, got %q", name, schema.Type)
	}
	resolved, err := schema.Resolve(nil)
	if err != nil {
		return Tool{}, fmt.Errorf("tool %q resolve schema: %w", name, err)
	}

	return Tool{
		Name:        name,
		Description: description,
		Roles:       roles,
		Threads:     threads,
		Schema:      schema,
		resolved:    resolved,
		call: func(ctx context.Context, p domain.Principal, raw json.RawMessage) (any, error) {
			var in In
			if err := json.Unmarshal(raw, &in); err != nil {
				return nil, fmt.Errorf("%w: %w", ErrInvalidArgs, err)
			}
			return fn(ctx, p, in)
		},
	}, nil
}

// Registry is the single place that decides whether a tool call may run.
type Registry struct {
	tools map[string]Tool
	names []string
}

func NewRegistry(tools ...Tool) (*Registry, error) {
	r := &Registry{tools: make(map[string]Tool, len(tools))}
	for _, t := range tools {
		if _, dup := r.tools[t.Name]; dup {
			return nil, fmt.Errorf("tool %q registered twice", t.Name)
		}
		r.tools[t.Name] = t
		r.names = append(r.names, t.Name)
	}
	sort.Strings(r.names)
	return r, nil
}

// Available lists the tools the principal may call in a thread of this type, as the model sees them.
func (r *Registry) Available(p domain.Principal, thread domain.ThreadType) []llm.Tool {
	var out []llm.Tool
	for _, name := range r.names {
		t := r.tools[name]
		if !t.allows(p, thread) {
			continue
		}
		params, err := json.Marshal(t.Schema)
		if err != nil {
			continue // schemas are inferred from Go types and always marshal
		}
		out = append(out, llm.Tool{Name: t.Name, Description: t.Description, Parameters: params})
	}
	return out
}

// Call checks rights and arguments, then runs the tool and returns its JSON result.
func (r *Registry) Call(ctx context.Context, p domain.Principal, thread domain.ThreadType, name string,
	args json.RawMessage,
) (json.RawMessage, error) {
	t, ok := r.tools[name]
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrUnknownTool, name)
	}
	if !slices.Contains(t.Roles, p.Role) {
		return nil, fmt.Errorf("%w: role %s may not call %s", ErrForbidden, p.Role, name)
	}
	if !slices.Contains(t.Threads, thread) {
		return nil, fmt.Errorf("%w: %s in a %s thread", ErrNotInThread, name, thread)
	}

	var instance any
	if err := json.Unmarshal(args, &instance); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidArgs, err)
	}
	if err := t.resolved.Validate(instance); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidArgs, err)
	}

	out, err := t.call(ctx, p, args)
	if err != nil {
		return nil, err
	}
	return json.Marshal(out)
}

func (t Tool) allows(p domain.Principal, thread domain.ThreadType) bool {
	return slices.Contains(t.Roles, p.Role) && slices.Contains(t.Threads, thread)
}
