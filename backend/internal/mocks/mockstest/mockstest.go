// Package mockstest runs the mock services in memory for tests of their clients.
package mockstest

import (
	"context"
	"net"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/test/bufconn"

	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/mockconn"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/mocks"
	"github.com/Long-Honey-Road/mon-europeenne/backend/seed"
)

// Start serves the mocks from seed over an in-memory listener and returns a connection made by mockconn.
func Start(t *testing.T, chaos mocks.Chaos) (*grpc.ClientConn, *mocks.Server) {
	t.Helper()
	data, err := seed.Load(seed.Files)
	if err != nil {
		t.Fatal(err)
	}
	srv, err := mocks.New(data, chaos)
	if err != nil {
		t.Fatal(err)
	}
	ln := bufconn.Listen(1 << 20)
	go func() { _ = srv.GRPC.Serve(ln) }()
	t.Cleanup(srv.GRPC.Stop)
	conn, err := mockconn.Dial("passthrough:///mocks", time.Second,
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return ln.DialContext(ctx) }))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn, srv
}
