package httpapi

import (
	"context"
	"encoding/json"
	"regexp"
	"strings"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	buddyv1 "github.com/Long-Honey-Road/mon-europeenne/backend/gen/buddy/v1"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/domain"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/stream"
)

const (
	maxPinTitle = 60
	maxPinText  = 600
)

// Pins is the storage the pins API needs; everything is scoped to the caller.
type Pins interface {
	CreatePin(ctx context.Context, p domain.Pin) (domain.Pin, error)
	Pins(ctx context.Context, userID string, f domain.Focus) ([]domain.Pin, error)
	UpdatePin(ctx context.Context, userID, id, title, text string) (domain.Pin, error)
	DeletePin(ctx context.Context, userID, id string) error
	OwnMessage(ctx context.Context, userID, id string) (domain.Message, error)
}

// Publisher sends live events to a thread, e.g. a pin made by the button.
type Publisher interface {
	Publish(ctx context.Context, threadID string, e stream.Event) error
}

type pinService struct {
	buddyv1.UnimplementedPinServiceServer
	deps Deps
}

var markerStrip = regexp.MustCompile(`\s?\[\d+\]`)

func (s *pinService) PinMessage(ctx context.Context, req *buddyv1.PinMessageRequest) (*buddyv1.PinMessageResponse, error) {
	p, err := principal(ctx)
	if err != nil {
		return nil, err
	}
	msg, err := s.deps.Pins.OwnMessage(ctx, p.UserID, req.GetMessageId())
	if err != nil {
		return nil, storeError(err)
	}
	focus := msg.Focus
	if req.GetFocus().GetKind() != buddyv1.FocusKind_FOCUS_KIND_UNSPECIFIED {
		focus = domain.Focus{Kind: domain.FocusKind(req.GetFocus().GetKind()), ID: req.GetFocus().GetId()}
	}
	if err := s.ownSphere(ctx, p, focus); err != nil {
		return nil, err
	}
	text := strings.TrimSpace(markerStrip.ReplaceAllString(msg.Content, ""))
	title := req.GetTitle()
	if strings.TrimSpace(title) == "" {
		title, _, _ = strings.Cut(text, "\n")
		title = strings.Trim(title, "#*_ ")
	}
	pin, err := s.deps.Pins.CreatePin(ctx, domain.Pin{
		UserID: p.UserID, Focus: focus, Title: cutRunes(title, maxPinTitle), Text: cutRunes(text, maxPinText),
		Sources: sourcesOf(msg.Blocks), MessageID: msg.ID,
	})
	if err != nil {
		return nil, storeError(err)
	}
	if s.deps.Publish != nil {
		_ = s.deps.Publish.Publish(ctx, msg.ThreadID, stream.Event{Type: "pinned", Data: map[string]any{
			"pin_id": pin.ID, "kind": focus.Kind.String(), "id": focus.ID, "title": pin.Title, "text": pin.Text,
		}})
	}
	return &buddyv1.PinMessageResponse{Pin: pinProto(pin)}, nil
}

// ownSphere allows only the user's own plan items for now; the vault comes with its own kind.
func (s *pinService) ownSphere(ctx context.Context, p domain.Principal, f domain.Focus) error {
	if f.Kind != domain.FocusPlanItem || f.ID == "" {
		return status.Error(codes.InvalidArgument, "choose a plan item to pin to")
	}
	entries, err := s.deps.Plans.Plan(ctx, p.UserID)
	if err != nil {
		return storeError(err)
	}
	for _, e := range entries {
		if e.Item.Key == f.ID {
			return nil
		}
	}
	return status.Errorf(codes.NotFound, "no plan item %q in your plan", f.ID)
}

func (s *pinService) ListPins(ctx context.Context, req *buddyv1.ListPinsRequest) (*buddyv1.ListPinsResponse, error) {
	p, err := principal(ctx)
	if err != nil {
		return nil, err
	}
	pins, err := s.deps.Pins.Pins(ctx, p.UserID, domain.Focus{Kind: domain.FocusKind(req.GetKind()), ID: req.GetId()})
	if err != nil {
		return nil, storeError(err)
	}
	out := &buddyv1.ListPinsResponse{Pins: []*buddyv1.Pin{}}
	for _, pin := range pins {
		out.Pins = append(out.Pins, pinProto(pin))
	}
	return out, nil
}

func (s *pinService) UpdatePin(ctx context.Context, req *buddyv1.UpdatePinRequest) (*buddyv1.UpdatePinResponse, error) {
	p, err := principal(ctx)
	if err != nil {
		return nil, err
	}
	title, text := strings.TrimSpace(req.GetTitle()), strings.TrimSpace(req.GetText())
	if title == "" || text == "" || len([]rune(title)) > maxPinTitle || len([]rune(text)) > maxPinText {
		return nil, status.Errorf(codes.InvalidArgument, "title 1..%d and text 1..%d characters", maxPinTitle, maxPinText)
	}
	pin, err := s.deps.Pins.UpdatePin(ctx, p.UserID, req.GetPinId(), title, text)
	if err != nil {
		return nil, storeError(err)
	}
	return &buddyv1.UpdatePinResponse{Pin: pinProto(pin)}, nil
}

func (s *pinService) DeletePin(ctx context.Context, req *buddyv1.DeletePinRequest) (*buddyv1.DeletePinResponse, error) {
	p, err := principal(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.deps.Pins.DeletePin(ctx, p.UserID, req.GetPinId()); err != nil {
		return nil, storeError(err)
	}
	return &buddyv1.DeletePinResponse{}, nil
}

func pinProto(p domain.Pin) *buddyv1.Pin {
	return &buddyv1.Pin{
		Id: p.ID, Focus: &buddyv1.Focus{Kind: focusKinds[p.Focus.Kind], Id: p.Focus.ID}, Title: p.Title, Text: p.Text,
		MessageId: p.MessageID, CreatedAt: p.CreatedAt.UTC().Format(time.RFC3339), UpdatedAt: p.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

// sourcesOf keeps the sources block of an answer with its pin.
func sourcesOf(blocks json.RawMessage) json.RawMessage {
	var list []struct {
		Type  string          `json:"type"`
		Items json.RawMessage `json:"items"`
	}
	if json.Unmarshal(blocks, &list) == nil {
		for _, b := range list {
			if b.Type == "sources" && len(b.Items) > 0 {
				return b.Items
			}
		}
	}
	return json.RawMessage(`[]`)
}

func cutRunes(s string, n int) string {
	s = strings.TrimSpace(s)
	if r := []rune(s); len(r) > n {
		return strings.TrimSpace(string(r[:n-1])) + "…"
	}
	return s
}
