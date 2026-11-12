// Package corpsim imitates the company's request system until a real mock or service is plugged in: state in
// Redis, so the API submits and the worker advances the same requests; decisions come from the demo seed.
package corpsim

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/requests"
	"github.com/Long-Honey-Road/mon-europeenne/backend/seed"
)

// defaultDelay applies to types without a demo outcome.
const defaultDelay = 30 * time.Second

// Outcome is how the simulated approver decides on a request type.
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

type Sim struct {
	rdb      *redis.Client
	outcomes map[string]Outcome
	prefix   string
	now      func() time.Time
}

// New keeps its keys under prefix ("corpsim" if empty).
func New(rdb *redis.Client, outcomes map[string]Outcome, prefix string) *Sim {
	if prefix == "" {
		prefix = "corpsim"
	}
	return &Sim{rdb: rdb, outcomes: outcomes, prefix: prefix, now: time.Now}
}

func (s *Sim) seqKey() string             { return s.prefix + ":seq" }
func (s *Sim) dueKey() string             { return s.prefix + ":due" }
func (s *Sim) ticketKey(id string) string { return s.prefix + ":ticket:" + id }

func (s *Sim) outcome(typeKey string) Outcome {
	o, ok := s.outcomes[typeKey]
	if !ok {
		return Outcome{Approve: true, After: defaultDelay}
	}
	if o.After <= 0 {
		o.After = defaultDelay
	}
	return o
}

func (s *Sim) Submit(ctx context.Context, sub requests.Submission) (requests.Ticket, error) {
	n, err := s.rdb.Incr(ctx, s.seqKey()).Result()
	if err != nil {
		return requests.Ticket{}, err
	}
	now := s.now().UTC()
	t := requests.Ticket{
		ID: "REQ-" + strconv.FormatInt(n, 10), TypeKey: sub.TypeKey, RequesterID: sub.RequesterID, Fields: sub.Fields,
		Status: requests.StatusPending, Approvers: sub.Approvers, UpdatedAt: now,
		History: []requests.Change{{At: now, Status: requests.StatusPending, Note: "заявка создана"}},
	}
	if t.Approvers == nil {
		t.Approvers = []requests.Approver{}
	}
	if err := s.save(ctx, t); err != nil {
		return requests.Ticket{}, err
	}
	if err := s.schedule(ctx, t.ID, now.Add(s.outcome(t.TypeKey).After)); err != nil {
		return requests.Ticket{}, err
	}
	return t, nil
}

// Advance decides on every due request; a request taken by another worker is skipped.
func (s *Sim) Advance(ctx context.Context, now time.Time) ([]requests.Ticket, error) {
	now = now.UTC()
	ids, err := s.rdb.ZRangeArgs(ctx, redis.ZRangeArgs{
		Key: s.dueKey(), Start: "-inf", Stop: strconv.FormatInt(now.UnixMilli(), 10), ByScore: true,
	}).Result()
	if err != nil {
		return nil, err
	}
	var changed []requests.Ticket
	for _, id := range ids {
		taken, err := s.rdb.ZRem(ctx, s.dueKey(), id).Result()
		if err != nil {
			return changed, err
		}
		if taken == 0 {
			continue
		}
		t, err := s.load(ctx, id)
		if err != nil {
			return changed, err
		}
		if err := s.decide(ctx, &t, now); err != nil {
			return changed, err
		}
		changed = append(changed, t)
	}
	return changed, nil
}

func (s *Sim) decide(ctx context.Context, t *requests.Ticket, now time.Time) error {
	o := s.outcome(t.TypeKey)
	by := t.Approver().Name
	switch {
	case !o.Approve:
		t.Status, t.Reason = requests.StatusRejected, o.Reason
		t.History = append(t.History, requests.Change{At: now, Status: requests.StatusRejected, By: by, Note: o.Reason})
	case t.Step+1 < len(t.Approvers):
		t.Step++
		t.History = append(t.History, requests.Change{At: now, Status: requests.StatusPending, By: by,
			Note: "согласовано, дальше: " + t.Approver().Role})
		if err := s.schedule(ctx, t.ID, now.Add(o.After)); err != nil {
			return err
		}
	default:
		t.Status = requests.StatusApproved
		t.History = append(t.History, requests.Change{At: now, Status: requests.StatusApproved, By: by, Note: "доступ выдан"})
	}
	t.UpdatedAt = now
	return s.save(ctx, *t)
}

func (s *Sim) schedule(ctx context.Context, id string, at time.Time) error {
	return s.rdb.ZAdd(ctx, s.dueKey(), redis.Z{Score: float64(at.UnixMilli()), Member: id}).Err()
}

func (s *Sim) save(ctx context.Context, t requests.Ticket) error {
	raw, err := json.Marshal(t)
	if err != nil {
		return err
	}
	return s.rdb.Set(ctx, s.ticketKey(t.ID), raw, 0).Err()
}

func (s *Sim) load(ctx context.Context, id string) (requests.Ticket, error) {
	raw, err := s.rdb.Get(ctx, s.ticketKey(id)).Bytes()
	if errors.Is(err, redis.Nil) {
		return requests.Ticket{}, fmt.Errorf("corpsim: request %s lost", id)
	}
	if err != nil {
		return requests.Ticket{}, err
	}
	var t requests.Ticket
	return t, json.Unmarshal(raw, &t)
}
