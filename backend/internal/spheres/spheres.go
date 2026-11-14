// Package spheres builds the agent's context around the sphere a message is about (design §6.5): the sphere's
// card, its own memory and a short overview of the other spheres, so the context does not grow with their number.
package spheres

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/domain"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/plan"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/store"
)

const (
	// FocusHistory is how many messages of the sphere itself go into the context.
	FocusHistory = 8
	// OverviewLines limits the other spheres listed in the overview.
	OverviewLines = 8
	// focusSummaryAfter and threadSummaryAfter are how many new messages trigger a new summary.
	focusSummaryAfter  = 6
	threadSummaryAfter = 12
)

type Store interface {
	FocusMessages(ctx context.Context, threadID string, f domain.Focus, before time.Time, limit int) ([]domain.Message, error)
	ThreadFoci(ctx context.Context, threadID string) ([]store.FocusActivity, error)
	FocusSummary(ctx context.Context, threadID string, f domain.Focus) (string, time.Time, error)
	CountAfter(ctx context.Context, threadID string, f domain.Focus, after time.Time) (int, error)
}

type Plans interface {
	Plan(ctx context.Context, userID string) ([]domain.PlanEntry, error)
}

// Vault reads one of the user's vault records.
type Vault interface {
	VaultItem(ctx context.Context, userID, id string) (domain.VaultItem, error)
}

// Pins lists the user's pins of one sphere.
type Pins interface {
	Pins(ctx context.Context, userID string, f domain.Focus) ([]domain.Pin, error)
}

const (
	// CardPins and pinPreview keep the card small: up to 10 pins, each title and the start of its text.
	CardPins   = 10
	pinPreview = 200
)

// Queue schedules summaries in the background; a duplicate request for the same sphere is dropped.
type Queue interface {
	EnqueueSummary(ctx context.Context, threadID string, f domain.Focus) error
}

// Context is what a turn about a sphere adds to the prompt; History replaces the plain thread tail.
type Context struct {
	Card     string
	Memory   string
	Overview string
	History  []domain.Message
}

type Service struct {
	store Store
	plans Plans
	pins  Pins
	vault Vault
	queue Queue
}

func New(st Store, plans Plans, pins Pins, vault Vault, queue Queue) *Service {
	return &Service{store: st, plans: plans, pins: pins, vault: vault, queue: queue}
}

// For builds the context of a message: with a focus the sphere's card, memory and own history; without one only
// the overview. History is nil without a focus, the caller keeps the thread tail then.
func (s *Service) For(ctx context.Context, th domain.Thread, msg domain.Message) (Context, error) {
	entries, err := s.plans.Plan(ctx, th.UserID)
	if err != nil {
		return Context{}, err
	}
	titles := map[string]plan.Item{}
	for _, it := range plan.Build(entries).Items {
		titles[it.Item.Key] = it
	}
	var out Context
	if out.Overview, err = s.overview(ctx, th, msg.Focus, titles); err != nil {
		return Context{}, err
	}
	if msg.Focus.IsZero() {
		return out, nil
	}
	if msg.Focus.Kind == domain.FocusVaultItem {
		v, err := s.vault.VaultItem(ctx, th.UserID, msg.Focus.ID)
		if err != nil {
			return Context{}, err
		}
		out.Card = vaultCard(v)
	} else {
		pins, err := s.pins.Pins(ctx, th.UserID, msg.Focus)
		if err != nil {
			return Context{}, err
		}
		out.Card = card(msg.Focus, titles) + pinLines(pins)
	}
	if out.Memory, _, err = s.store.FocusSummary(ctx, th.ID, msg.Focus); err != nil {
		return Context{}, err
	}
	if out.History, err = s.store.FocusMessages(ctx, th.ID, msg.Focus, msg.CreatedAt, FocusHistory); err != nil {
		return Context{}, err
	}
	return out, nil
}

func card(f domain.Focus, items map[string]plan.Item) string {
	it, ok := items[f.ID]
	if f.Kind != domain.FocusPlanItem || !ok {
		return fmt.Sprintf("%s %s", f.Kind, f.ID)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Plan item %s «%s», period %s, status %s.\n", it.Item.Key, it.Item.Title, it.Item.Period, it.Status)
	if it.Item.Practice != "" {
		fmt.Fprintf(&b, "Practice: %s\n", it.Item.Practice)
	}
	if it.Note != "" {
		fmt.Fprintf(&b, "Note: %s\n", it.Note)
	}
	if len(it.BlockedBy) > 0 {
		names := make([]string, 0, len(it.BlockedBy))
		for _, dep := range it.BlockedBy {
			names = append(names, fmt.Sprintf("%s «%s»", dep, items[dep].Item.Title))
		}
		fmt.Fprintf(&b, "Waits for: %s\n", strings.Join(names, ", "))
	}
	return strings.TrimSpace(b.String())
}

func vaultCard(v domain.VaultItem) string {
	tags := ""
	if len(v.Tags) > 0 {
		tags = ", tags " + strings.Join(v.Tags, ", ")
	}
	return fmt.Sprintf("Vault record %s «%s», version %d%s.\nSummary:\n%s", v.ID, v.Title, v.Version, tags, v.Summary)
}

func pinLines(pins []domain.Pin) string {
	if len(pins) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("\nPinned by the user:")
	for i, p := range pins {
		if i == CardPins {
			break
		}
		fmt.Fprintf(&b, "\n- %s: %s", p.Title, cut(strings.ReplaceAll(p.Text, "\n", " "), pinPreview))
	}
	return b.String()
}

func (s *Service) overview(ctx context.Context, th domain.Thread, current domain.Focus, items map[string]plan.Item) (string, error) {
	foci, err := s.store.ThreadFoci(ctx, th.ID)
	if err != nil {
		return "", err
	}
	var lines []string
	if th.Summary != "" {
		lines = append(lines, th.Summary)
	}
	n := 0
	for _, a := range foci {
		if a.Focus == current || n == OverviewLines {
			continue
		}
		n++
		title := a.Focus.ID
		if it, ok := items[a.Focus.ID]; ok && a.Focus.Kind == domain.FocusPlanItem {
			title = fmt.Sprintf("%s «%s», %s", a.Focus.ID, it.Item.Title, it.Status)
		}
		if a.Focus.Kind == domain.FocusVaultItem {
			if v, err := s.vault.VaultItem(ctx, th.UserID, a.Focus.ID); err == nil {
				title = fmt.Sprintf("record «%s»", v.Title)
			}
		}
		lines = append(lines, fmt.Sprintf("- %s: %d messages, last %s", title, a.Messages, a.Last.Format("02.01 15:04")))
	}
	return strings.Join(lines, "\n"), nil
}

// AfterTurn asks for new summaries when enough has been said since the last ones; failures only log.
func (s *Service) AfterTurn(ctx context.Context, th domain.Thread, f domain.Focus) {
	if !f.IsZero() {
		_, until, err := s.store.FocusSummary(ctx, th.ID, f)
		if err == nil {
			s.maybe(ctx, th.ID, f, until, focusSummaryAfter)
		}
	}
	s.maybe(ctx, th.ID, domain.Focus{}, th.SummaryUntil, threadSummaryAfter)
}

func (s *Service) maybe(ctx context.Context, threadID string, f domain.Focus, until time.Time, after int) {
	n, err := s.store.CountAfter(ctx, threadID, f, until)
	if err == nil && n > after {
		err = s.queue.EnqueueSummary(ctx, threadID, f)
	}
	if err != nil {
		slog.Warn("summary not scheduled", "thread", threadID, "focus", f, "err", err)
	}
}
