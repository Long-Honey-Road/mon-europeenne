package requests

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/structpb"

	buddyv1 "github.com/Long-Honey-Road/mon-europeenne/backend/gen/buddy/v1"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/auth"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/domain"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/tools"
)

// API serves RequestService over the gateway.
type API struct {
	buddyv1.UnimplementedRequestServiceServer
	desk *Desk
}

func NewAPI(d *Desk) *API { return &API{desk: d} }

var statuses = map[string]buddyv1.RequestStatus{
	StatusPending:  buddyv1.RequestStatus_REQUEST_STATUS_PENDING,
	StatusApproved: buddyv1.RequestStatus_REQUEST_STATUS_APPROVED,
	StatusRejected: buddyv1.RequestStatus_REQUEST_STATUS_REJECTED,
}

func (a *API) ListRequestTypes(_ context.Context, req *buddyv1.ListRequestTypesRequest) (*buddyv1.ListRequestTypesResponse, error) {
	resp := &buddyv1.ListRequestTypesResponse{Types: []*buddyv1.RequestType{}}
	for _, t := range a.desk.cat.Types {
		if req.GetCategory() != "" && t.Category != req.GetCategory() {
			continue
		}
		schema := &structpb.Struct{}
		if err := schema.UnmarshalJSON(t.FieldsSchema); err != nil {
			return nil, status.Error(codes.Internal, "request type data")
		}
		pt := &buddyv1.RequestType{Key: t.Key, Title: t.Title, Category: t.Category, FieldsSchema: schema,
			SlaHours: int32(t.SLAHours), ApprovalChain: []*buddyv1.ApprovalStep{}}
		for _, s := range t.ApprovalChain {
			pt.ApprovalChain = append(pt.ApprovalChain, &buddyv1.ApprovalStep{Resolver: s.Resolver, Object: s.Object})
		}
		resp.Types = append(resp.Types, pt)
	}
	return resp, nil
}

func (a *API) SubmitRequest(ctx context.Context, req *buddyv1.SubmitRequestRequest) (*buddyv1.SubmitRequestResponse, error) {
	p, ok := auth.PrincipalFrom(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "authentication required")
	}
	if req.GetTypeKey() == "" {
		return nil, status.Error(codes.InvalidArgument, "type_key is required")
	}
	v, err := a.desk.Submit(ctx, p, req.GetThreadId(), req.GetTypeKey(), req.GetFields().AsMap())
	if err != nil {
		return nil, apiError(err)
	}
	return &buddyv1.SubmitRequestResponse{Request: requestProto(v)}, nil
}

func (a *API) ListRequests(ctx context.Context, _ *buddyv1.ListRequestsRequest) (*buddyv1.ListRequestsResponse, error) {
	p, ok := auth.PrincipalFrom(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "authentication required")
	}
	views, err := a.desk.List(ctx, p)
	if err != nil {
		return nil, apiError(err)
	}
	resp := &buddyv1.ListRequestsResponse{Requests: make([]*buddyv1.Request, 0, len(views))}
	for _, v := range views {
		resp.Requests = append(resp.Requests, requestProto(v))
	}
	return resp, nil
}

func (a *API) GetRequest(ctx context.Context, req *buddyv1.GetRequestRequest) (*buddyv1.GetRequestResponse, error) {
	p, ok := auth.PrincipalFrom(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "authentication required")
	}
	v, err := a.desk.Get(ctx, p, req.GetRequestId())
	if err != nil {
		return nil, apiError(err)
	}
	return &buddyv1.GetRequestResponse{Request: requestProto(v)}, nil
}

// apiError hides who owns a request: someone else's request is NOT_FOUND, like a missing one.
func apiError(err error) error {
	if ie, ok := IsInvalid(err); ok {
		return status.Error(codes.InvalidArgument, ie.Error())
	}
	switch {
	case errors.Is(err, domain.ErrNotFound), errors.Is(err, tools.ErrForbidden):
		return status.Error(codes.NotFound, "not found")
	default:
		return status.Error(codes.Unavailable, "request service unavailable")
	}
}

func requestProto(v View) *buddyv1.Request {
	out := &buddyv1.Request{
		Id: v.ID, TypeKey: v.TypeKey, Title: v.Title, Status: statuses[v.Status],
		Approver: &buddyv1.Approver{Role: v.Approver.Role, PersonId: v.Approver.PersonID, Name: v.Approver.Name},
		NextStep: v.NextStep, Reason: v.Reason, ThreadId: v.ThreadID, History: []*buddyv1.StatusChange{},
	}
	if !v.UpdatedAt.IsZero() {
		out.UpdatedAt = v.UpdatedAt.UTC().Format(time.RFC3339)
	}
	if raw, err := json.Marshal(v.Fields); err == nil {
		out.Fields = &structpb.Struct{}
		_ = out.Fields.UnmarshalJSON(raw)
	}
	for _, c := range v.History {
		out.History = append(out.History, &buddyv1.StatusChange{
			At: c.At.UTC().Format(time.RFC3339), Status: statuses[c.Status], By: c.By, Note: c.Note,
		})
	}
	return out
}
