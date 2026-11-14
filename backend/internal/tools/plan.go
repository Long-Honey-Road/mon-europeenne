package tools

import (
	"context"
	"fmt"

	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/domain"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/plan"
)

// PlanStore keeps each user's adaptation plan; it returns ErrNotFound for items outside the user's plan.
type PlanStore interface {
	Plan(ctx context.Context, userID string) ([]domain.PlanEntry, error)
	SetItemStatus(ctx context.Context, userID, key string, status domain.PlanItemStatus, note string) error
}

type getPlanIn struct {
	Period string `json:"period,omitempty" jsonschema:"only this period: w1, w2, m1, m2, equator or final"`
	Status string `json:"status,omitempty" jsonschema:"only items in this status: todo, doing, blocked or done"`
}

type PlanItemOut struct {
	ItemID   string `json:"item_id"`
	Title    string `json:"title"`
	Practice string `json:"practice"`
	Period   string `json:"period"`
	Status   string `json:"status"`
	Note     string `json:"note,omitempty"`
	// BlockedBy lists dependencies that are not done yet.
	BlockedBy []string `json:"blocked_by"`
}

type getPlanOut struct {
	Items []PlanItemOut `json:"items"`
	// Done and Total count the whole plan, whatever the filters.
	Done  int `json:"done"`
	Total int `json:"total"`
}

// internOnly: only interns have an adaptation plan; mentors and managers get none rather than a stray copy.
var internOnly = []domain.Role{domain.RoleIntern}

// GetPlan shows the caller's own adaptation plan; there is no way to ask for someone else's.
func GetPlan(plans PlanStore) (Tool, error) {
	return Define("get_plan",
		"The user's adaptation plan: practical items by period with status, note and what blocks each item.",
		internOnly, []domain.ThreadType{domain.ThreadGeneral, domain.ThreadPlan, domain.ThreadTask},
		func(ctx context.Context, p domain.Principal, in getPlanIn) (getPlanOut, error) {
			var period domain.PlanPeriod
			var status domain.PlanItemStatus
			var err error
			if in.Period != "" {
				if period, err = domain.ParsePlanPeriod(in.Period); err != nil {
					return getPlanOut{}, fmt.Errorf("%w: period %w", ErrInvalidArgs, err)
				}
			}
			if in.Status != "" {
				if status, err = domain.ParsePlanItemStatus(in.Status); err != nil {
					return getPlanOut{}, fmt.Errorf("%w: status %w", ErrInvalidArgs, err)
				}
			}
			entries, err := plans.Plan(ctx, p.UserID)
			if err != nil {
				return getPlanOut{}, err
			}
			view := plan.Build(entries)
			out := getPlanOut{Items: []PlanItemOut{}, Done: view.Done, Total: view.Total}
			for _, it := range view.Items {
				if (period != 0 && it.Item.Period != period) || (status != 0 && it.Status != status) {
					continue
				}
				out.Items = append(out.Items, PlanItemOut{
					ItemID: it.Item.Key, Title: it.Item.Title, Practice: it.Item.Practice,
					Period: it.Item.Period.String(), Status: it.Status.String(), Note: it.Note, BlockedBy: it.BlockedBy,
				})
			}
			return out, nil
		})
}

type updateItemIn struct {
	ItemID string `json:"item_id" jsonschema:"plan item id from get_plan, e.g. w1-access"`
	Status string `json:"status" jsonschema:"new status: todo, doing, blocked or done"`
	Note   string `json:"note,omitempty" jsonschema:"short note for the user, e.g. why the item is blocked"`
}

type updateItemOut struct {
	ItemID string `json:"item_id"`
	Status string `json:"status"`
}

// UpdateItemStatus changes the status of one item in the caller's own plan.
func UpdateItemStatus(plans PlanStore) (Tool, error) {
	return Define("update_item_status",
		"Mark an item of the user's adaptation plan as todo, doing, blocked or done.",
		internOnly, []domain.ThreadType{domain.ThreadPlan},
		func(ctx context.Context, p domain.Principal, in updateItemIn) (updateItemOut, error) {
			status, err := domain.ParsePlanItemStatus(in.Status)
			if err != nil {
				return updateItemOut{}, fmt.Errorf("%w: status %w", ErrInvalidArgs, err)
			}
			if err := plans.SetItemStatus(ctx, p.UserID, in.ItemID, status, in.Note); err != nil {
				return updateItemOut{}, err
			}
			return updateItemOut{ItemID: in.ItemID, Status: status.String()}, nil
		})
}
