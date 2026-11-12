// Package llm defines the model client used by the agent; adapters live in subpackages.
package llm

import (
	"context"
	"encoding/json"
	"errors"
)

var (
	// ErrUnavailable means the provider failed or timed out; the caller may retry or escalate.
	ErrUnavailable = errors.New("llm unavailable")
	// ErrBadResponse means the provider answered with something we cannot use.
	ErrBadResponse = errors.New("llm returned a malformed response")
)

type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

type Message struct {
	Role    Role
	Content string
	// Set on assistant messages that call tools.
	ToolCalls []ToolCall
	// Set on tool messages: the call this result answers.
	ToolCallID string
}

type ToolCall struct {
	ID        string
	Name      string
	Arguments json.RawMessage
}

type Tool struct {
	Name        string
	Description string
	Parameters  json.RawMessage // JSON Schema
}

// Format asks for a JSON answer matching Schema.
// Local Qwen never calls tools while a format is set, so the agent sends them in separate calls.
type Format struct {
	Name   string
	Schema json.RawMessage
}

type Request struct {
	Messages []Message
	Tools    []Tool
	Format   *Format
}

type Usage struct {
	PromptTokens     int
	CompletionTokens int
}

type Response struct {
	Message Message
	Usage   Usage
}

type Client interface {
	Chat(ctx context.Context, req Request) (Response, error)
}
