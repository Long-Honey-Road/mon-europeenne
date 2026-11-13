// Package mockconn dials the corporate services (the mocks today, real services later) with the same rules:
// a deadline on every call and one retry of reads when the service is unavailable.
package mockconn

import (
	"context"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// DefaultTimeout bounds a call that has no deadline of its own.
const DefaultTimeout = 3 * time.Second

// serviceConfig retries reads on UNAVAILABLE; SubmitRequest is not idempotent and is never retried.
const serviceConfig = `{"methodConfig": [{
  "name": [
    {"service": "mocks.authroles.v1.AuthRolesService"},
    {"service": "mocks.people.v1.DirectoryService"},
    {"service": "mocks.corprequests.v1.CorpRequestsService", "method": "GetRequest"},
    {"service": "mocks.corprequests.v1.CorpRequestsService", "method": "ListRequests"},
    {"service": "mocks.corprequests.v1.CorpRequestsService", "method": "ListUpdates"}
  ],
  "retryPolicy": {
    "maxAttempts": 3,
    "initialBackoff": "0.1s",
    "maxBackoff": "0.5s",
    "backoffMultiplier": 2,
    "retryableStatusCodes": ["UNAVAILABLE"]
  }
}]}`

// Dial connects lazily: an unreachable service fails calls, not startup.
func Dial(target string, timeout time.Duration, opts ...grpc.DialOption) (*grpc.ClientConn, error) {
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	base := []grpc.DialOption{
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultServiceConfig(serviceConfig),
		grpc.WithUnaryInterceptor(deadline(timeout)),
	}
	return grpc.NewClient(target, append(base, opts...)...)
}

func deadline(timeout time.Duration) grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoke grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		if _, ok := ctx.Deadline(); !ok {
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(ctx, timeout)
			defer cancel()
		}
		return invoke(ctx, method, req, reply, cc, opts...)
	}
}
