// Package corprequests is the corp-requests mock: the company's request system for access requests. Approvers
// decide on their own schedule (demo outcomes from seed); state lives in memory of the mocks process.
package corprequests

import (
	"cmp"
	"context"
	"slices"
	"strconv"
	"sync"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	corprequestsv1 "github.com/Long-Honey-Road/mon-europeenne/backend/gen/mocks/corprequests/v1"
	"github.com/Long-Honey-Road/mon-europeenne/backend/seed"
)

// defaultDelay applies to types without a demo outcome.
const defaultDelay = 30 * time.Second

// Outcome is how the approvers decide on a request type.
type Outcome struct {
	Approve bool
	After   time.Duration
	Reason  string
}

// OutcomesFromSeed reads the demo decisions of each request type.
func OutcomesFromSeed(d seed.Data) map[string]Outcome {
	out := map[string]Outcome{}
	for _, t := range d.Requests.Types {
		out[t.Key] = Outcome{
			Approve: t.Demo.Outcome != "reject",
			After:   time.Duration(t.Demo.AfterSeconds) * time.Second,
			Reason:  t.Demo.Reason,
		}
	}
	return out
}

type Service struct {
	corprequestsv1.UnimplementedCorpRequestsServiceServer

	outcomes map[string]Outcome
	now      func() time.Time

	mu      sync.Mutex
	seq     int
	version int64
	byID    map[string]*corprequestsv1.Request
	due     map[string]time.Time
}

func New(outcomes map[string]Outcome, now func() time.Time) *Service {
	if now == nil {
		now = time.Now
	}
	return &Service{outcomes: outcomes, now: now, byID: map[string]*corprequestsv1.Request{}, due: map[string]time.Time{}}
}

func (s *Service) outcome(typeKey string) Outcome {
	o, ok := s.outcomes[typeKey]
	if !ok {
		return Outcome{Approve: true, After: defaultDelay}
	}
	if o.After <= 0 {
		o.After = defaultDelay
	}
	return o
}

func (s *Service) SubmitRequest(_ context.Context, req *corprequestsv1.SubmitRequestRequest) (*corprequestsv1.SubmitRequestResponse, error) {
	switch {
	case req.GetTypeKey() == "":
		return nil, status.Error(codes.InvalidArgument, "type_key is required")
	case req.GetRequesterId() == "":
		return nil, status.Error(codes.InvalidArgument, "requester_id is required")
	case len(req.GetApprovers()) == 0:
		return nil, status.Error(codes.InvalidArgument, "approvers are required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now().UTC()
	s.seq++
	r := &corprequestsv1.Request{
		Id: "REQ-" + strconv.Itoa(s.seq), TypeKey: req.GetTypeKey(), RequesterId: req.GetRequesterId(),
		Fields: req.GetFields(), Status: corprequestsv1.Status_STATUS_PENDING, Approvers: req.GetApprovers(),
		History: []*corprequestsv1.Change{{At: timestamppb.New(now), Status: corprequestsv1.Status_STATUS_PENDING, Note: "заявка создана"}},
	}
	s.touch(r, now)
	s.byID[r.GetId()] = r
	s.due[r.GetId()] = now.Add(s.outcome(r.GetTypeKey()).After)
	return &corprequestsv1.SubmitRequestResponse{Request: clone(r)}, nil
}

func (s *Service) GetRequest(_ context.Context, req *corprequestsv1.GetRequestRequest) (*corprequestsv1.GetRequestResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.byID[req.GetRequestId()]
	if !ok {
		return nil, status.Errorf(codes.NotFound, "request %q not found", req.GetRequestId())
	}
	return &corprequestsv1.GetRequestResponse{Request: clone(r)}, nil
}

func (s *Service) ListRequests(_ context.Context, req *corprequestsv1.ListRequestsRequest) (*corprequestsv1.ListRequestsResponse, error) {
	if req.GetRequesterId() == "" {
		return nil, status.Error(codes.InvalidArgument, "requester_id is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	out := &corprequestsv1.ListRequestsResponse{}
	for _, r := range s.byID {
		if r.GetRequesterId() == req.GetRequesterId() {
			out.Requests = append(out.Requests, clone(r))
		}
	}
	// Ids grow with submission, so the longest-then-largest id is the newest.
	slices.SortFunc(out.Requests, func(a, b *corprequestsv1.Request) int {
		return -cmp.Or(cmp.Compare(len(a.GetId()), len(b.GetId())), cmp.Compare(a.GetId(), b.GetId()))
	})
	return out, nil
}

func (s *Service) ListUpdates(_ context.Context, req *corprequestsv1.ListUpdatesRequest) (*corprequestsv1.ListUpdatesResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := &corprequestsv1.ListUpdatesResponse{Cursor: max(req.GetAfter(), 0)}
	for _, r := range s.byID {
		if r.GetVersion() > req.GetAfter() {
			out.Requests = append(out.Requests, clone(r))
		}
	}
	slices.SortFunc(out.Requests, func(a, b *corprequestsv1.Request) int { return cmp.Compare(a.GetVersion(), b.GetVersion()) })
	if n := len(out.Requests); n > 0 {
		out.Cursor = out.Requests[n-1].GetVersion()
	}
	return out, nil
}

// Tick lets approvers decide on every request that is due at now.
func (s *Service) Tick(now time.Time) {
	now = now.UTC()
	s.mu.Lock()
	defer s.mu.Unlock()
	ids := make([]string, 0, len(s.due))
	for id, at := range s.due {
		if !at.After(now) {
			ids = append(ids, id)
		}
	}
	slices.Sort(ids)
	for _, id := range ids {
		delete(s.due, id)
		s.decide(s.byID[id], now)
	}
}

// Run ticks until ctx is done.
func (s *Service) Run(ctx context.Context, every time.Duration) {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.Tick(s.now())
		}
	}
}

func (s *Service) decide(r *corprequestsv1.Request, now time.Time) {
	o := s.outcome(r.GetTypeKey())
	by := r.GetApprovers()[r.GetStep()].GetName()
	change := &corprequestsv1.Change{At: timestamppb.New(now), By: by}
	switch {
	case !o.Approve:
		r.Status, r.Reason = corprequestsv1.Status_STATUS_REJECTED, o.Reason
		change.Status, change.Note = corprequestsv1.Status_STATUS_REJECTED, o.Reason
	case int(r.GetStep())+1 < len(r.GetApprovers()):
		r.Step++
		change.Status, change.Note = corprequestsv1.Status_STATUS_PENDING, "согласовано, дальше: "+r.GetApprovers()[r.GetStep()].GetRole()
		s.due[r.GetId()] = now.Add(o.After)
	default:
		r.Status = corprequestsv1.Status_STATUS_APPROVED
		change.Status, change.Note = corprequestsv1.Status_STATUS_APPROVED, "доступ выдан"
	}
	r.History = append(r.History, change)
	s.touch(r, now)
}

func (s *Service) touch(r *corprequestsv1.Request, now time.Time) {
	s.version++
	r.Version, r.UpdatedAt = s.version, timestamppb.New(now)
}

func clone(r *corprequestsv1.Request) *corprequestsv1.Request {
	return proto.CloneOf(r)
}
