package httpapi

import (
	"context"
	"io"
	"net/http"
	"testing"

	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/auth"
)

// MCP is mounted under the private prefix, so the session check runs before the handler.
func TestMCPNeedsSession(t *testing.T) {
	d := deps(t)
	d.MCP = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, _ := auth.PrincipalFrom(r.Context())
		_, _ = io.WriteString(w, p.UserID+" "+r.Method)
	})
	s := newServerWith(t, d)

	if code, _ := call(t, s, http.MethodPost, "/v1/private/mcp", "", "{}"); code != http.StatusUnauthorized {
		t.Fatalf("no token: status %d, want 401", code)
	}
	if code, _ := call(t, s, http.MethodPost, "/v1/private/mcp", "forged", "{}"); code != http.StatusUnauthorized {
		t.Fatalf("bad token: status %d, want 401", code)
	}

	token := login(t, s, "intern", "intern-demo")
	for _, method := range []string{http.MethodPost, http.MethodGet, http.MethodDelete} {
		req, _ := http.NewRequestWithContext(context.Background(), method, s.URL+"/v1/private/mcp?thread=plan", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		resp, err := s.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK || string(body) != "u-liza "+method {
			t.Fatalf("%s: status %d, body %q", method, resp.StatusCode, body)
		}
	}
}

func TestMCPAbsentWithoutHandler(t *testing.T) {
	s := newServer(t)
	token := login(t, s, "intern", "intern-demo")

	if code, _ := call(t, s, http.MethodPost, "/v1/private/mcp", token, "{}"); code != http.StatusNotFound {
		t.Fatalf("status %d, want 404", code)
	}
}
