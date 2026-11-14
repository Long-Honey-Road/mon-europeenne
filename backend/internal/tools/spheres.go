package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/domain"
)

const (
	maxPinTitle = 60
	maxPinText  = 600
)

// PinStore keeps the user's pins.
type PinStore interface {
	CreatePin(ctx context.Context, p domain.Pin) (domain.Pin, error)
}

// UIEvent is what a tool asks the server to send to the user's screen (design §6.5); the agent publishes it to
// the thread after the turn.
type UIEvent struct {
	Type string         `json:"type"`
	Data map[string]any `json:"data"`
}

type sphereIn struct {
	Kind string `json:"kind,omitempty" jsonschema:"sphere kind: plan_item (default)"`
	ID   string `json:"id" jsonschema:"the plan item id, e.g. w1-access; the sphere in focus is in the prompt"`
}

type pinNoteIn struct {
	sphereIn
	Title string `json:"title" jsonschema:"short title, up to 60 characters"`
	Text  string `json:"text" jsonschema:"what to keep, in markdown, up to 600 characters"`
}

type pinNoteOut struct {
	PinID   string  `json:"pin_id"`
	Sphere  string  `json:"sphere"`
	UIEvent UIEvent `json:"ui_event"`
}

type highlightIn struct {
	Kind string   `json:"kind,omitempty" jsonschema:"sphere kind: plan_item (default)"`
	IDs  []string `json:"ids" jsonschema:"plan item ids to light up on the map"`
	Note string   `json:"note,omitempty" jsonschema:"a short sticker shown next to them"`
}

type highlightOut struct {
	Highlighted []string `json:"highlighted"`
	UIEvent     UIEvent  `json:"ui_event"`
}

type focusIn struct {
	sphereIn
	Reason string `json:"reason,omitempty" jsonschema:"why going there helps, a few words"`
}

type focusOut struct {
	Suggested string  `json:"suggested"`
	UIEvent   UIEvent `json:"ui_event"`
}

// ownItems is the set of the user's plan item ids with their titles.
func ownItems(ctx context.Context, plans PlanStore, userID string) (map[string]string, error) {
	entries, err := plans.Plan(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make(map[string]string, len(entries))
	for _, e := range entries {
		out[e.Item.Key] = e.Item.Title
	}
	return out, nil
}

func checkKind(kind string) error {
	if kind != "" && kind != domain.FocusPlanItem.String() {
		return fmt.Errorf("%w: kind %q, only plan_item", ErrInvalidArgs, kind)
	}
	return nil
}

func cutRunes(s string, n int) string {
	s = strings.TrimSpace(s)
	if r := []rune(s); len(r) > n {
		return string(r[:n])
	}
	return s
}

// PinNote pins a note to one of the user's spheres. Only on the user's request: the prompt says so and the
// description repeats it, the model reads both.
func PinNote(plans PlanStore, pins PinStore) (Tool, error) {
	return Define("pin_note",
		"Pin a short note to a sphere (a plan item) ONLY when the user asked to pin, save or keep something in this "+
			"message («закрепи», «запомни на пункте»). Never on your own initiative.",
		allRoles, []domain.ThreadType{domain.ThreadPlan},
		func(ctx context.Context, p domain.Principal, in pinNoteIn) (pinNoteOut, error) {
			if err := checkKind(in.Kind); err != nil {
				return pinNoteOut{}, err
			}
			items, err := ownItems(ctx, plans, p.UserID)
			if err != nil {
				return pinNoteOut{}, err
			}
			if _, ok := items[in.ID]; !ok {
				return pinNoteOut{}, ErrNotFound
			}
			title, text := cutRunes(in.Title, maxPinTitle), strings.TrimSpace(in.Text)
			if title == "" || text == "" {
				return pinNoteOut{}, fmt.Errorf("%w: title and text are required", ErrInvalidArgs)
			}
			if len([]rune(text)) > maxPinText {
				return pinNoteOut{}, fmt.Errorf("%w: text is %d characters, keep it under %d", ErrInvalidArgs, len([]rune(text)), maxPinText)
			}
			pin, err := pins.CreatePin(ctx, domain.Pin{UserID: p.UserID, Focus: domain.Focus{Kind: domain.FocusPlanItem, ID: in.ID}, Title: title, Text: text})
			if err != nil {
				return pinNoteOut{}, err
			}
			return pinNoteOut{PinID: pin.ID, Sphere: in.ID, UIEvent: UIEvent{Type: "pinned", Data: map[string]any{
				"pin_id": pin.ID, "kind": domain.FocusPlanItem.String(), "id": in.ID, "title": title, "text": text,
			}}}, nil
		})
}

// HighlightSpheres lights plan items up on the map with a sticker.
func HighlightSpheres(plans PlanStore) (Tool, error) {
	return Define("highlight_spheres",
		"Light up plan items on the user's map with an optional short sticker, e.g. what is blocked or what to do next.",
		allRoles, allThreads,
		func(ctx context.Context, p domain.Principal, in highlightIn) (highlightOut, error) {
			if err := checkKind(in.Kind); err != nil {
				return highlightOut{}, err
			}
			items, err := ownItems(ctx, plans, p.UserID)
			if err != nil {
				return highlightOut{}, err
			}
			ids := []string{}
			for _, id := range in.IDs {
				if _, ok := items[id]; ok {
					ids = append(ids, id)
				}
			}
			if len(ids) == 0 {
				return highlightOut{}, ErrNotFound
			}
			note := cutRunes(in.Note, maxPinTitle)
			return highlightOut{Highlighted: ids, UIEvent: UIEvent{Type: "highlight", Data: map[string]any{
				"kind": domain.FocusPlanItem.String(), "ids": ids, "note": note,
			}}}, nil
		})
}

// FocusSphere suggests going to a sphere; the focus changes only when the user clicks.
func FocusSphere(plans PlanStore) (Tool, error) {
	return Define("focus_sphere",
		"Suggest the user open one plan item to talk about it there; the user decides with a click.",
		allRoles, allThreads,
		func(ctx context.Context, p domain.Principal, in focusIn) (focusOut, error) {
			if err := checkKind(in.Kind); err != nil {
				return focusOut{}, err
			}
			items, err := ownItems(ctx, plans, p.UserID)
			if err != nil {
				return focusOut{}, err
			}
			title, ok := items[in.ID]
			if !ok {
				return focusOut{}, ErrNotFound
			}
			return focusOut{Suggested: in.ID, UIEvent: UIEvent{Type: "focus_suggest", Data: map[string]any{
				"kind": domain.FocusPlanItem.String(), "id": in.ID, "title": title, "reason": cutRunes(in.Reason, 120),
			}}}, nil
		})
}
