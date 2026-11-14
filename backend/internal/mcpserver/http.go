package mcpserver

import (
	"context"
	"fmt"
	"net/http"
	"time"

	mcpauth "github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/auth"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/domain"
)

// ThreadParam is the query parameter that picks the thread type of an MCP session; general by default.
const ThreadParam = "thread"

const sessionIdle = 30 * time.Minute

// NewHTTPHandler serves the tools over streamable HTTP. The API session middleware must run first and put the
// principal into the request context; each MCP session is bound to that user and the thread type it began with.
func NewHTTPHandler(reg Tools, version string) http.Handler {
	streamable := mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
		p, _ := auth.PrincipalFrom(r.Context())
		thread, _ := threadOf(r)
		return New(reg, p, thread, version)
	}, &mcp.StreamableHTTPOptions{SessionTimeout: sessionIdle})
	// TokenInfo.UserID lets the SDK refuse a session ID presented by another user.
	bound := mcpauth.RequireBearerToken(func(ctx context.Context, _ string, _ *http.Request) (*mcpauth.TokenInfo, error) {
		p, _ := auth.PrincipalFrom(ctx)
		return &mcpauth.TokenInfo{UserID: p.UserID}, nil
	}, &mcpauth.RequireBearerTokenOptions{AllowMissingExpiration: true})(streamable)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := auth.PrincipalFrom(r.Context()); !ok {
			http.Error(w, "authentication required", http.StatusUnauthorized)
			return
		}
		if _, err := threadOf(r); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		bound.ServeHTTP(w, r)
	})
}

func threadOf(r *http.Request) (domain.ThreadType, error) {
	name := r.URL.Query().Get(ThreadParam)
	if name == "" {
		return domain.ThreadGeneral, nil
	}
	thread, err := domain.ParseThreadType(name)
	if err != nil {
		return 0, fmt.Errorf("query parameter %s: %w", ThreadParam, err)
	}
	return thread, nil
}
