// Package openaicompat talks to any OpenAI-compatible chat API: Ollama, vLLM, OpenRouter, Qwen Cloud.
package openaicompat

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/shared"

	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/llm"
)

type Config struct {
	BaseURL string
	Model   string
	APIKey  string
	// ReasoningEffort is sent as reasoning_effort when set; "none" turns Qwen thinking off.
	ReasoningEffort string
	// MaxRetries is the SDK retry budget for 429 and 5xx; zero means a single attempt.
	MaxRetries int
}

type Client struct {
	api    openai.Client
	model  string
	effort string
}

func New(cfg Config) *Client {
	return &Client{
		api: openai.NewClient(
			option.WithBaseURL(cfg.BaseURL),
			option.WithAPIKey(cfg.APIKey),
			option.WithMaxRetries(cfg.MaxRetries),
		),
		model:  cfg.Model,
		effort: cfg.ReasoningEffort,
	}
}

func (c *Client) Chat(ctx context.Context, req llm.Request) (llm.Response, error) {
	params, err := c.params(req)
	if err != nil {
		return llm.Response{}, err
	}

	completion, err := c.api.Chat.Completions.New(ctx, params)
	if err != nil {
		return llm.Response{}, classify(err)
	}
	if len(completion.Choices) == 0 {
		return llm.Response{}, fmt.Errorf("%w: no choices", llm.ErrBadResponse)
	}

	return response(completion.Choices[0].Message, completion.Usage)
}

func response(msg openai.ChatCompletionMessage, usage openai.CompletionUsage) (llm.Response, error) {
	out := llm.Message{Role: llm.RoleAssistant, Content: msg.Content}
	for _, tc := range msg.ToolCalls {
		args := json.RawMessage(tc.Function.Arguments)
		if !json.Valid(args) {
			return llm.Response{}, fmt.Errorf("%w: tool %q arguments are not JSON", llm.ErrBadResponse, tc.Function.Name)
		}
		out.ToolCalls = append(out.ToolCalls, llm.ToolCall{ID: tc.ID, Name: tc.Function.Name, Arguments: args})
	}
	return llm.Response{
		Message: out,
		Usage: llm.Usage{
			PromptTokens:     int(usage.PromptTokens),
			CompletionTokens: int(usage.CompletionTokens),
		},
	}, nil
}

func (c *Client) params(req llm.Request) (openai.ChatCompletionNewParams, error) {
	p := openai.ChatCompletionNewParams{Model: c.model}
	if c.effort != "" {
		p.ReasoningEffort = shared.ReasoningEffort(c.effort)
	}

	for _, m := range req.Messages {
		msg, err := message(m)
		if err != nil {
			return p, err
		}
		p.Messages = append(p.Messages, msg)
	}

	for _, t := range req.Tools {
		var schema shared.FunctionParameters
		if err := json.Unmarshal(t.Parameters, &schema); err != nil {
			return p, fmt.Errorf("tool %q parameters: %w", t.Name, err)
		}
		p.Tools = append(p.Tools, openai.ChatCompletionFunctionTool(shared.FunctionDefinitionParam{
			Name:        t.Name,
			Description: openai.String(t.Description),
			Parameters:  schema,
		}))
	}

	if req.Format != nil {
		p.ResponseFormat = openai.ChatCompletionNewParamsResponseFormatUnion{
			OfJSONSchema: &shared.ResponseFormatJSONSchemaParam{
				JSONSchema: shared.ResponseFormatJSONSchemaJSONSchemaParam{
					Name:   req.Format.Name,
					Strict: openai.Bool(true),
					Schema: req.Format.Schema,
				},
			},
		}
	}
	return p, nil
}

func message(m llm.Message) (openai.ChatCompletionMessageParamUnion, error) {
	switch m.Role {
	case llm.RoleSystem:
		return openai.SystemMessage(m.Content), nil
	case llm.RoleUser:
		return openai.UserMessage(m.Content), nil
	case llm.RoleTool:
		return openai.ToolMessage(m.Content, m.ToolCallID), nil
	case llm.RoleAssistant:
		a := openai.ChatCompletionAssistantMessageParam{}
		if m.Content != "" {
			a.Content.OfString = openai.String(m.Content)
		}
		for _, tc := range m.ToolCalls {
			a.ToolCalls = append(a.ToolCalls, openai.ChatCompletionMessageToolCallUnionParam{
				OfFunction: &openai.ChatCompletionMessageFunctionToolCallParam{
					ID: tc.ID,
					Function: openai.ChatCompletionMessageFunctionToolCallFunctionParam{
						Name:      tc.Name,
						Arguments: string(tc.Arguments),
					},
				},
			})
		}
		return openai.ChatCompletionMessageParamUnion{OfAssistant: &a}, nil
	default:
		return openai.ChatCompletionMessageParamUnion{}, fmt.Errorf("unknown message role %q", m.Role)
	}
}

// classify marks failures worth retrying or escalating as llm.ErrUnavailable.
func classify(err error) error {
	var apiErr *openai.Error
	if errors.As(err, &apiErr) {
		if apiErr.StatusCode == http.StatusTooManyRequests || apiErr.StatusCode >= http.StatusInternalServerError {
			return fmt.Errorf("%w: %w", llm.ErrUnavailable, err)
		}
		return fmt.Errorf("llm request rejected: %w", err)
	}
	return fmt.Errorf("%w: %w", llm.ErrUnavailable, err)
}

// ChatStream streams the answer; text deltas go to onDelta, tool calls arrive assembled in the response.
func (c *Client) ChatStream(ctx context.Context, req llm.Request, onDelta func(string)) (llm.Response, error) {
	params, err := c.params(req)
	if err != nil {
		return llm.Response{}, err
	}
	params.StreamOptions = openai.ChatCompletionStreamOptionsParam{IncludeUsage: openai.Bool(true)}

	stream := c.api.Chat.Completions.NewStreaming(ctx, params)
	defer stream.Close()
	var acc openai.ChatCompletionAccumulator
	for stream.Next() {
		chunk := stream.Current()
		acc.AddChunk(chunk)
		if len(chunk.Choices) > 0 && chunk.Choices[0].Delta.Content != "" {
			onDelta(chunk.Choices[0].Delta.Content)
		}
	}
	if err := stream.Err(); err != nil {
		return llm.Response{}, classify(err)
	}
	if len(acc.Choices) == 0 {
		return llm.Response{}, fmt.Errorf("%w: empty stream", llm.ErrBadResponse)
	}
	return response(acc.Choices[0].Message, acc.Usage)
}
