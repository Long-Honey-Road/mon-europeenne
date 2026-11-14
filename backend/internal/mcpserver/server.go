// Package mcpserver exposes the agent's tools over MCP for one principal and thread type.
package mcpserver

import (
	"context"
	"encoding/json"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/domain"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/llm"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/tools"
)

// Tools is the registry as the server sees it; the same rights checks run as for the agent.
type Tools interface {
	Available(p domain.Principal, thread domain.ThreadType) []llm.Tool
	Call(ctx context.Context, p domain.Principal, thread domain.ThreadType, name string, args json.RawMessage) (json.RawMessage, error)
}

const instructions = "Tools of AI-buddy, the onboarding assistant, acting as one user in one thread type. " +
	"Errors come back as {\"error\": code, \"message\": text} with isError set."

// New lists only the tools the principal may call in this thread type.
func New(reg Tools, p domain.Principal, thread domain.ThreadType, version string) *mcp.Server {
	srv := mcp.NewServer(&mcp.Implementation{Name: "buddy", Title: "AI-buddy tools", Version: version},
		&mcp.ServerOptions{Instructions: instructions})
	call := handler(reg, p, thread)
	for _, t := range reg.Available(p, thread) {
		srv.AddTool(&mcp.Tool{Name: t.Name, Description: t.Description, InputSchema: t.Parameters}, call)
	}
	// Unlisted tools also go to the registry, so the refusal is the agent's (forbidden, not_in_thread), not the SDK's.
	srv.AddReceivingMiddleware(func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			if r, ok := req.(*mcp.CallToolRequest); ok {
				return call(ctx, r)
			}
			return next(ctx, method, req)
		}
	})
	return srv
}

func handler(reg Tools, p domain.Principal, thread domain.ThreadType) mcp.ToolHandler {
	return func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.Params.Arguments
		if len(args) == 0 {
			args = json.RawMessage(`{}`)
		}
		out, err := reg.Call(ctx, p, thread, req.Params.Name, args)
		if err != nil {
			code, msg := tools.Explain(err, req.Params.Name, thread)
			body, _ := json.Marshal(map[string]string{"error": code, "message": msg})
			return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: string(body)}}}, nil
		}
		return &mcp.CallToolResult{
			Content:           []mcp.Content{&mcp.TextContent{Text: string(out)}},
			StructuredContent: out,
		}, nil
	}
}
