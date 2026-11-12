package httpapi

import (
	"context"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	buddyv1 "github.com/Long-Honey-Road/mon-europeenne/backend/gen/buddy/v1"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/domain"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/plan"
)

// Plans keeps each user's adaptation plan; SetItemStatus returns store.ErrNotFound outside the user's plan.
type Plans interface {
	Plan(ctx context.Context, userID string) ([]domain.PlanEntry, error)
	SetItemStatus(ctx context.Context, userID, key string, status domain.PlanItemStatus, note string) error
}

// companyZone is where onboarding days change: Moscow time.
var companyZone = time.FixedZone("MSK", 3*60*60)

type planService struct {
	buddyv1.UnimplementedPlanServiceServer
	deps Deps
}

func (s *planService) GetPlan(ctx context.Context, _ *buddyv1.GetPlanRequest) (*buddyv1.GetPlanResponse, error) {
	p, err := principal(ctx)
	if err != nil {
		return nil, err
	}
	entries, err := s.deps.Plans.Plan(ctx, p.UserID)
	if err != nil {
		return nil, storeError(err)
	}
	v := plan.Build(entries)
	resp := &buddyv1.GetPlanResponse{Items: make([]*buddyv1.PlanItem, 0, len(v.Items)), Done: int32(v.Done), Total: int32(v.Total)}
	for _, it := range v.Items {
		resp.Items = append(resp.Items, planItemProto(it))
	}
	return resp, nil
}

func (s *planService) UpdatePlanItem(ctx context.Context, req *buddyv1.UpdatePlanItemRequest) (*buddyv1.UpdatePlanItemResponse, error) {
	p, err := principal(ctx)
	if err != nil {
		return nil, err
	}
	st := domain.PlanItemStatus(req.GetStatus())
	if st < domain.ItemTodo || st > domain.ItemDone {
		return nil, status.Error(codes.InvalidArgument, "status must be todo, doing, blocked or done")
	}
	if err := s.deps.Plans.SetItemStatus(ctx, p.UserID, req.GetItemId(), st, req.GetNote()); err != nil {
		return nil, storeError(err)
	}
	entries, err := s.deps.Plans.Plan(ctx, p.UserID)
	if err != nil {
		return nil, storeError(err)
	}
	for _, it := range plan.Build(entries).Items {
		if it.Item.Key == req.GetItemId() {
			return &buddyv1.UpdatePlanItemResponse{Item: planItemProto(it)}, nil
		}
	}
	return nil, status.Error(codes.NotFound, "plan item not found")
}

func (s *planService) GetToday(ctx context.Context, _ *buddyv1.GetTodayRequest) (*buddyv1.GetTodayResponse, error) {
	p, err := principal(ctx)
	if err != nil {
		return nil, err
	}
	user, err := s.deps.Accounts.User(p.UserID)
	if err != nil {
		return nil, status.Error(codes.Unauthenticated, "authentication required")
	}
	entries, err := s.deps.Plans.Plan(ctx, p.UserID)
	if err != nil {
		return nil, storeError(err)
	}
	t := plan.TodayOf(entries, user.StartDate, s.deps.now().In(companyZone))
	resp := &buddyv1.GetTodayResponse{
		Day: int32(t.Day), Week: int32(t.Week), Period: buddyv1.PlanPeriod(t.Period),
		NextSteps: make([]*buddyv1.PlanItem, 0, len(t.Next)), Blockers: make([]*buddyv1.Blocker, 0, len(t.Blocked)),
		Done: int32(t.Done), Total: int32(t.Total),
	}
	for _, it := range t.Next {
		resp.NextSteps = append(resp.NextSteps, planItemProto(it))
	}
	for _, it := range t.Blocked {
		resp.Blockers = append(resp.Blockers, &buddyv1.Blocker{Kind: "plan_item", Id: it.Item.Key, Title: it.Item.Title, Reason: it.Note})
	}
	for _, src := range s.deps.Blockers {
		extra, err := src.Blockers(ctx, p)
		if err != nil {
			return nil, storeError(err)
		}
		for _, b := range extra {
			resp.Blockers = append(resp.Blockers, &buddyv1.Blocker{Kind: b.Kind, Id: b.ID, Title: b.Title, Reason: b.Reason})
		}
	}
	return resp, nil
}

func planItemProto(it plan.Item) *buddyv1.PlanItem {
	return &buddyv1.PlanItem{
		Id: it.Item.Key, Title: it.Item.Title, Practice: it.Item.Practice,
		Period: buddyv1.PlanPeriod(it.Item.Period), Status: buddyv1.PlanItemStatus(it.Status), Note: it.Note,
		DependsOn: orEmpty(it.Item.DependsOn), BlockedBy: it.BlockedBy, Sources: orEmpty(it.Item.Sources),
		Map:    &buddyv1.MapPosition{X: it.Item.Map.X, Y: it.Item.Map.Y, Group: it.Item.Map.Group},
		TeamId: it.Item.TeamID, Direction: directions[it.Item.Direction],
	}
}

func orEmpty(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
