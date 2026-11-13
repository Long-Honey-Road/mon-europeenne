// Package httpapi serves the public HTTP API: grpc-gateway for proto services plus plain handlers.
package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	buddyv1 "github.com/Long-Honey-Road/mon-europeenne/backend/gen/buddy/v1"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/auth"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/domain"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/jobs"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/people"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/plan"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/stream"
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
	People(ctx context.Context) ([]domain.Person, error)
}

// Threads is the storage the thread API needs.
type Threads interface {
	CreateThread(ctx context.Context, userID string, typ domain.ThreadType, title string, pin domain.ThreadPin) (domain.Thread, error)
	Thread(ctx context.Context, userID, id string) (domain.Thread, error)
	Threads(ctx context.Context, userID string) ([]domain.Thread, error)
	AddMessage(ctx context.Context, m domain.Message) (domain.Message, error)
	Messages(ctx context.Context, threadID string, limit int) ([]domain.Message, error)
	RateMessage(ctx context.Context, userID, messageID, value, note string) error
}

// Jobs queues background work.
type Jobs interface {
	EnqueueChatTurn(ctx context.Context, t jobs.ChatTurn) (jobID string, err error)
}

type Deps struct {
	Accounts  *auth.Accounts
	Tokens    *auth.Tokens
	Directory Directory
	Threads   Threads
	Plans     Plans
	// Requests is the removable requests module; nil (FEATURE_REQUESTS off) answers UNIMPLEMENTED.
	Requests buddyv1.RequestServiceServer
	// Blockers add to the "today" blockers beyond the plan, e.g. access requests.
	Blockers []BlockerSource
	Jobs     Jobs
	Bus      Bus
	// Heartbeat is how often SSE sends a comment so proxies keep the stream open; 15 s if zero.
	Heartbeat time.Duration
	// Now is the clock for "today"; time.Now if nil.
	Now func() time.Time
}

func (d Deps) now() time.Time {
	if d.Now != nil {
		return d.Now()
	}
	return time.Now()
}

// BlockerSource tells what else blocks the user today.
type BlockerSource interface {
	Blockers(ctx context.Context, p domain.Principal) ([]plan.Blocker, error)
}

// Bus delivers live turn events of one thread.
type Bus interface {
	Subscribe(ctx context.Context, threadID string) (<-chan stream.Event, func(), error)
}

// New routes /v1/public/ without a session, /v1/private/ only with one, and 404s everything else under /v1/.
func New(ctx context.Context, d Deps) (http.Handler, error) {
	gw := runtime.NewServeMux(
		runtime.WithMarshalerOption(runtime.MIMEWildcard, &runtime.HTTPBodyMarshaler{
			Marshaler: &runtime.JSONPb{
				MarshalOptions:   protojson.MarshalOptions{UseProtoNames: true, EmitUnpopulated: true},
				UnmarshalOptions: protojson.UnmarshalOptions{DiscardUnknown: true},
			},
		}),
		runtime.WithForwardResponseOption(acceptedForQueuedWork),
	)
	if err := buddyv1.RegisterAuthServiceHandlerServer(ctx, gw, &authService{deps: d}); err != nil {
		return nil, fmt.Errorf("register auth service: %w", err)
	}
	if err := buddyv1.RegisterProfileServiceHandlerServer(ctx, gw, &profileService{deps: d}); err != nil {
		return nil, fmt.Errorf("register profile service: %w", err)
	}
	if err := buddyv1.RegisterThreadServiceHandlerServer(ctx, gw, &threadService{deps: d}); err != nil {
		return nil, fmt.Errorf("register thread service: %w", err)
	}
	if err := buddyv1.RegisterPlanServiceHandlerServer(ctx, gw, &planService{deps: d}); err != nil {
		return nil, fmt.Errorf("register plan service: %w", err)
	}
	if err := buddyv1.RegisterPeopleServiceHandlerServer(ctx, gw, &peopleService{deps: d}); err != nil {
		return nil, fmt.Errorf("register people service: %w", err)
	}
	var requests buddyv1.RequestServiceServer = buddyv1.UnimplementedRequestServiceServer{}
	if d.Requests != nil {
		requests = d.Requests
	}
	if err := buddyv1.RegisterRequestServiceHandlerServer(ctx, gw, requests); err != nil {
		return nil, fmt.Errorf("register request service: %w", err)
	}

	mux := http.NewServeMux()
	mux.Handle(publicPrefix, gw)
	mux.Handle(privatePrefix, requireSession(d, gw))
	mux.Handle("GET "+privatePrefix+"threads/{id}/events", requireSession(d, streamThread(d)))
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

// acceptedForQueuedWork answers 202 when the work was queued rather than done.
func acceptedForQueuedWork(_ context.Context, w http.ResponseWriter, m proto.Message) error {
	if _, ok := m.(*buddyv1.SendMessageResponse); ok {
		w.WriteHeader(http.StatusAccepted)
	}
	return nil
}
