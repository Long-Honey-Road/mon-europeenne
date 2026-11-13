// Package corpclient is the requests.Service over the corp-requests contract: the mock today, the company's
// request system when one is reachable.
package corpclient

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/redis/go-redis/v9"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/structpb"

	corprequestsv1 "github.com/Long-Honey-Road/mon-europeenne/backend/gen/mocks/corprequests/v1"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/requests"
)

// Cursor remembers the last change the worker has seen, so a restart does not replay every request.
type Cursor interface {
	Load(ctx context.Context) (int64, error)
	Save(ctx context.Context, v int64) error
}

type Client struct {
	c      corprequestsv1.CorpRequestsServiceClient
	cursor Cursor
}

func New(conn grpc.ClientConnInterface, cursor Cursor) *Client {
	return &Client{c: corprequestsv1.NewCorpRequestsServiceClient(conn), cursor: cursor}
}

func (c *Client) Submit(ctx context.Context, s requests.Submission) (requests.Ticket, error) {
	fields, err := structpb.NewStruct(s.Fields)
	if err != nil {
		return requests.Ticket{}, fmt.Errorf("request fields: %w", err)
	}
	req := &corprequestsv1.SubmitRequestRequest{TypeKey: s.TypeKey, RequesterId: s.RequesterID, Fields: fields}
	for _, a := range s.Approvers {
		req.Approvers = append(req.Approvers, &corprequestsv1.Approver{Role: a.Role, PersonId: a.PersonID, Name: a.Name})
	}
	resp, err := c.c.SubmitRequest(ctx, req)
	if err != nil {
		return requests.Ticket{}, fmt.Errorf("corp-requests: submit: %w", err)
	}
	return ticket(resp.GetRequest()), nil
}

// Updates moves the cursor only after the changes are read, so a failed call is retried on the next tick.
func (c *Client) Updates(ctx context.Context) ([]requests.Ticket, error) {
	after, err := c.cursor.Load(ctx)
	if err != nil {
		return nil, fmt.Errorf("corp-requests cursor: %w", err)
	}
	resp, err := c.c.ListUpdates(ctx, &corprequestsv1.ListUpdatesRequest{After: after})
	if err == nil && resp.GetCursor() < after {
		// The service restarted and lost its history: read everything it has now.
		after = 0
		resp, err = c.c.ListUpdates(ctx, &corprequestsv1.ListUpdatesRequest{})
	}
	if err != nil {
		return nil, fmt.Errorf("corp-requests: updates: %w", err)
	}
	out := make([]requests.Ticket, 0, len(resp.GetRequests()))
	for _, r := range resp.GetRequests() {
		out = append(out, ticket(r))
	}
	if resp.GetCursor() != after {
		if err := c.cursor.Save(ctx, resp.GetCursor()); err != nil {
			return out, fmt.Errorf("corp-requests cursor: %w", err)
		}
	}
	return out, nil
}

var statuses = map[corprequestsv1.Status]string{
	corprequestsv1.Status_STATUS_PENDING:  requests.StatusPending,
	corprequestsv1.Status_STATUS_APPROVED: requests.StatusApproved,
	corprequestsv1.Status_STATUS_REJECTED: requests.StatusRejected,
}

func ticket(r *corprequestsv1.Request) requests.Ticket {
	t := requests.Ticket{
		ID: r.GetId(), TypeKey: r.GetTypeKey(), RequesterID: r.GetRequesterId(), Fields: r.GetFields().AsMap(),
		Status: statuses[r.GetStatus()], Step: int(r.GetStep()), Reason: r.GetReason(),
		Approvers: []requests.Approver{}, UpdatedAt: r.GetUpdatedAt().AsTime(),
	}
	for _, a := range r.GetApprovers() {
		t.Approvers = append(t.Approvers, requests.Approver{Role: a.GetRole(), PersonID: a.GetPersonId(), Name: a.GetName()})
	}
	for _, h := range r.GetHistory() {
		t.History = append(t.History, requests.Change{At: h.GetAt().AsTime(), Status: statuses[h.GetStatus()], By: h.GetBy(), Note: h.GetNote()})
	}
	return t
}

// RedisCursor keeps the cursor under one Redis key.
type RedisCursor struct {
	rdb *redis.Client
	key string
}

func NewRedisCursor(rdb *redis.Client, key string) RedisCursor {
	return RedisCursor{rdb: rdb, key: key}
}

func (r RedisCursor) Load(ctx context.Context) (int64, error) {
	v, err := r.rdb.Get(ctx, r.key).Result()
	if errors.Is(err, redis.Nil) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return strconv.ParseInt(v, 10, 64)
}

func (r RedisCursor) Save(ctx context.Context, v int64) error {
	return r.rdb.Set(ctx, r.key, v, 0).Err()
}
