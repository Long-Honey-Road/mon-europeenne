// Package mocks serves the imitations of corporate services that buddy cannot reach: auth-roles, people,
// corp-requests, documents and team-repo. One process, gRPC for buddy, REST through grpc-gateway for people checking by hand.
package mocks

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"

	authrolesv1 "github.com/Long-Honey-Road/mon-europeenne/backend/gen/mocks/authroles/v1"
	corprequestsv1 "github.com/Long-Honey-Road/mon-europeenne/backend/gen/mocks/corprequests/v1"
	documentsv1 "github.com/Long-Honey-Road/mon-europeenne/backend/gen/mocks/documents/v1"
	peoplev1 "github.com/Long-Honey-Road/mon-europeenne/backend/gen/mocks/people/v1"
	teamrepov1 "github.com/Long-Honey-Road/mon-europeenne/backend/gen/mocks/teamrepo/v1"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/mocks/authroles"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/mocks/corprequests"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/mocks/directory"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/mocks/documents"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/mocks/teamrepo"
	"github.com/Long-Honey-Road/mon-europeenne/backend/seed"
)

// FailHeader forces a failure of one call: "500" (INTERNAL), "503" (UNAVAILABLE) or "timeout".
const FailHeader = "x-mock-fail"

// Chaos makes calls fail or slow down, as real services do.
type Chaos struct {
	// FailRate is the share of calls that fail with UNAVAILABLE.
	FailRate float64
	// MaxLatency adds a random delay up to this value to every call.
	MaxLatency time.Duration
}

type Server struct {
	GRPC     *grpc.Server
	Auth     *authroles.Service
	Requests *corprequests.Service
}

type options struct{ stateFile string }

// Option tunes the mock server.
type Option func(*options)

// WithStateFile keeps interns hired through auth-roles in a JSON file, so they survive a restart.
func WithStateFile(path string) Option {
	return func(o *options) { o.stateFile = path }
}

// New registers all mock services on one gRPC server with chaos applied to every call.
func New(data seed.Data, chaos Chaos, opts ...Option) (*Server, error) {
	var o options
	for _, opt := range opts {
		opt(&o)
	}
	// auth-roles and people share the directory: a hired intern appears in both at once.
	dir := data.Directory()
	auth, err := authroles.New(data.Credentials(), dir, o.stateFile)
	if err != nil {
		return nil, err
	}
	reqs := corprequests.New(corprequests.OutcomesFromSeed(data), nil)
	g := grpc.NewServer(grpc.UnaryInterceptor(chaos.intercept))
	authrolesv1.RegisterAuthRolesServiceServer(g, auth)
	peoplev1.RegisterDirectoryServiceServer(g, directory.New(dir, data.Teams()))
	corprequestsv1.RegisterCorpRequestsServiceServer(g, reqs)
	documentsv1.RegisterDocumentsServiceServer(g, documents.New(data.Docs))
	teamrepov1.RegisterTeamRepoServiceServer(g, teamrepo.New(data.Repos))
	return &Server{GRPC: g, Auth: auth, Requests: reqs}, nil
}

func (c Chaos) intercept(ctx context.Context, req any, _ *grpc.UnaryServerInfo, next grpc.UnaryHandler) (any, error) {
	if md, ok := metadata.FromIncomingContext(ctx); ok {
		if v := md.Get(FailHeader); len(v) > 0 {
			switch strings.TrimSpace(v[0]) {
			case "500":
				return nil, status.Error(codes.Internal, "mock failure")
			case "503":
				return nil, status.Error(codes.Unavailable, "mock failure")
			case "timeout":
				<-ctx.Done()
				return nil, status.FromContextError(ctx.Err()).Err()
			}
		}
	}
	if c.MaxLatency > 0 {
		select {
		case <-time.After(rand.N(c.MaxLatency)): //nolint:gosec // jitter, not security
		case <-ctx.Done():
			return nil, status.FromContextError(ctx.Err()).Err()
		}
	}
	if c.FailRate > 0 && rand.Float64() < c.FailRate { //nolint:gosec // jitter, not security
		return nil, status.Error(codes.Unavailable, "mock failure")
	}
	return next(ctx, req)
}

// Gateway serves the REST projection by calling the gRPC server at grpcAddr, so chaos applies to it too.
func Gateway(ctx context.Context, grpcAddr string) (http.Handler, func(), error) {
	conn, err := grpc.NewClient(grpcAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, nil, err
	}
	mux := runtime.NewServeMux(
		// Same JSON as the buddy API: snake_case field names, empty fields present.
		runtime.WithMarshalerOption(runtime.MIMEWildcard, &runtime.JSONPb{
			MarshalOptions:   protojson.MarshalOptions{UseProtoNames: true, EmitUnpopulated: true},
			UnmarshalOptions: protojson.UnmarshalOptions{DiscardUnknown: true},
		}),
		runtime.WithIncomingHeaderMatcher(func(key string) (string, bool) {
			if strings.EqualFold(key, FailHeader) {
				return FailHeader, true
			}
			return runtime.DefaultHeaderMatcher(key)
		}),
	)
	for _, reg := range []func(context.Context, *runtime.ServeMux, *grpc.ClientConn) error{
		authrolesv1.RegisterAuthRolesServiceHandler,
		peoplev1.RegisterDirectoryServiceHandler,
		corprequestsv1.RegisterCorpRequestsServiceHandler,
		documentsv1.RegisterDocumentsServiceHandler,
		teamrepov1.RegisterTeamRepoServiceHandler,
	} {
		if err := reg(ctx, mux, conn); err != nil {
			_ = conn.Close()
			return nil, nil, err
		}
	}
	return mux, func() { _ = conn.Close() }, nil
}

// Run serves gRPC and REST until ctx is done; approvers decide every tick.
func (s *Server) Run(ctx context.Context, grpcLn, httpLn net.Listener, tick time.Duration) error {
	gw, closeGW, err := Gateway(ctx, grpcLn.Addr().String())
	if err != nil {
		return fmt.Errorf("gateway: %w", err)
	}
	defer closeGW()
	httpSrv := &http.Server{Handler: gw, ReadHeaderTimeout: 5 * time.Second}

	errs := make(chan error, 2)
	go func() { errs <- s.GRPC.Serve(grpcLn) }()
	go func() { errs <- httpSrv.Serve(httpLn) }()
	go s.Requests.Run(ctx, tick)

	select {
	case err = <-errs:
	case <-ctx.Done():
	}
	shutdown, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	_ = httpSrv.Shutdown(shutdown)
	s.GRPC.GracefulStop()
	if errors.Is(err, http.ErrServerClosed) || errors.Is(err, grpc.ErrServerStopped) {
		return nil
	}
	return err
}
