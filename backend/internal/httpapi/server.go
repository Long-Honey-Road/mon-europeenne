// Package httpapi serves the public HTTP API: grpc-gateway for proto services plus plain handlers.
package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/encoding/protojson"

	buddyv1 "github.com/Long-Honey-Road/mon-europeenne/backend/gen/buddy/v1"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/auth"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/domain"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/people"
)

const (
	publicPrefix  = "/v1/public/"
	privatePrefix = "/v1/private/"
)

// Directory is what the API needs from the people directory.
type Directory interface {
	Person(ctx context.Context, id string) (domain.Person, error)
	Team(ctx context.Context, id string) (domain.Team, error)
	Relations(ctx context.Context, userID string) (people.Relations, error)
}

type Deps struct {
	Accounts  *auth.Accounts
	Tokens    *auth.Tokens
	Directory Directory
}

// New routes /v1/public/ without a session, /v1/private/ only with one, and 404s everything else under /v1/.
func New(ctx context.Context, d Deps) (http.Handler, error) {
	gw := runtime.NewServeMux(runtime.WithMarshalerOption(runtime.MIMEWildcard, &runtime.HTTPBodyMarshaler{
		Marshaler: &runtime.JSONPb{
			MarshalOptions:   protojson.MarshalOptions{UseProtoNames: true, EmitUnpopulated: true},
			UnmarshalOptions: protojson.UnmarshalOptions{DiscardUnknown: true},
		},
	}))
	if err := buddyv1.RegisterAuthServiceHandlerServer(ctx, gw, &authService{deps: d}); err != nil {
		return nil, fmt.Errorf("register auth service: %w", err)
	}
	if err := buddyv1.RegisterProfileServiceHandlerServer(ctx, gw, &profileService{deps: d}); err != nil {
		return nil, fmt.Errorf("register profile service: %w", err)
	}

	mux := http.NewServeMux()
	mux.Handle(publicPrefix, gw)
	mux.Handle(privatePrefix, requireSession(d, gw))
	mux.HandleFunc("GET /healthz", ok)
	mux.HandleFunc("GET /readyz", ok)
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		writeStatus(w, http.StatusNotFound, codes.NotFound, "not found")
	})
	return mux, nil
}

// requireSession turns a valid bearer token into a principal in the request context.
func requireSession(d Deps, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token, found := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		if !found || token == "" {
			writeStatus(w, http.StatusUnauthorized, codes.Unauthenticated, "authentication required")
			return
		}
		userID, err := d.Tokens.Verify(token)
		if err != nil {
			writeStatus(w, http.StatusUnauthorized, codes.Unauthenticated, "authentication required")
			return
		}
		user, err := d.Accounts.User(userID)
		if err != nil {
			writeStatus(w, http.StatusUnauthorized, codes.Unauthenticated, "authentication required")
			return
		}
		p := domain.Principal{UserID: user.ID, Role: user.Role, TeamID: user.TeamID}
		next.ServeHTTP(w, r.WithContext(auth.WithPrincipal(r.Context(), p)))
	})
}

// writeStatus answers in the same google.rpc.Status shape as the gateway.
func writeStatus(w http.ResponseWriter, httpCode int, code codes.Code, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(httpCode)
	_ = json.NewEncoder(w).Encode(map[string]any{"code": int(code), "message": msg, "details": []any{}})
}

func ok(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte("ok"))
}
