// Package mocks serves the imitations of corporate services that buddy cannot reach: auth-roles, people and
// corp-requests. One process, gRPC for buddy, REST through grpc-gateway for people checking by hand.
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

	authrolesv1 "github.com/Long-Honey-Road/mon-europeenne/backend/gen/mocks/authroles/v1"
	corprequestsv1 "github.com/Long-Honey-Road/mon-europeenne/backend/gen/mocks/corprequests/v1"
	peoplev1 "github.com/Long-Honey-Road/mon-europeenne/backend/gen/mocks/people/v1"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/mocks/authroles"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/mocks/corprequests"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/mocks/directory"
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
	Requests *corprequests.Service
}

// New registers all mock services on one gRPC server with chaos applied to every call.
func New(data seed.Data, chaos Chaos) (*Server, error) {
	auth, err := authroles.New(data.Credentials())
	if err != nil {
		return nil, err
	}
	reqs := corprequests.New(corprequests.OutcomesFromSeed(data), nil)
	g := grpc.NewServer(grpc.UnaryInterceptor(chaos.intercept))
	authrolesv1.RegisterAuthRolesServiceServer(g, auth)
	peoplev1.RegisterDirectoryServiceServer(g, directory.New(data.Directory(), data.Teams()))
	corprequestsv1.RegisterCorpRequestsServiceServer(g, reqs)
	return &Server{GRPC: g, Requests: reqs}, nil
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
	mux := runtime.NewServeMux(runtime.WithIncomingHeaderMatcher(func(key string) (string, bool) {
		if strings.EqualFold(key, FailHeader) {
			return FailHeader, true
		}
		return runtime.DefaultHeaderMatcher(key)
	}))
	for _, reg := range []func(context.Context, *runtime.ServeMux, *grpc.ClientConn) error{
		authrolesv1.RegisterAuthRolesServiceHandler,
		peoplev1.RegisterDirectoryServiceHandler,
		corprequestsv1.RegisterCorpRequestsServiceHandler,
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
