package requests

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/domain"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/plan"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/stream"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/tools"
)

// EventRequestStatus is the SSE event sent to the request's thread when it is created or changes.
const EventRequestStatus = "request_status"

// InvalidError lists what is wrong with the fields of a submitted request.
type InvalidError struct {
	Missing  []string
	Problems []string
}

func (e *InvalidError) Error() string {
	var parts []string
	if len(e.Missing) > 0 {
		parts = append(parts, "missing: "+strings.Join(e.Missing, ", "))
	}
	parts = append(parts, e.Problems...)
	return "invalid request fields: " + strings.Join(parts, "; ")
}

// Mirror is buddy's copy of what the service said; ThreadID links a request to the thread it came from.
type Mirror interface {
	SaveRequest(ctx context.Context, r domain.Request) error
	Store
}

type Threads interface {
	Thread(ctx context.Context, userID, id string) (domain.Thread, error)
}

type Publisher interface {
	Publish(ctx context.Context, threadID string, e stream.Event) error
}

// Desk sends requests the user confirmed, keeps the mirror in sync and tells the thread about changes.
type Desk struct {
	cat     Catalog
	svc     Service
	mirror  Mirror
	dir     tools.PeopleDirectory
	threads Threads
	pub     Publisher
}

func NewDesk(cat Catalog, svc Service, mirror Mirror, dir tools.PeopleDirectory, threads Threads, pub Publisher) *Desk {
	return &Desk{cat: cat, svc: svc, mirror: mirror, dir: dir, threads: threads, pub: pub}
}

func (d *Desk) Catalog() Catalog { return d.cat }

// View is a request as the user sees it.
type View struct {
	ID        string
	TypeKey   string
	Title     string
	Status    string
	Approver  Approver
	NextStep  string
	Reason    string
	Fields    map[string]any
	History   []Change
	ThreadID  string
	UpdatedAt time.Time
}

// Submit checks the fields like prepare_request does, then sends the request on the user's behalf.
func (d *Desk) Submit(ctx context.Context, p domain.Principal, threadID, typeKey string, fields map[string]any) (View, error) {
	typ, ok := d.cat.TypeByKey(typeKey)
	if !ok {
		return View{}, fmt.Errorf("%w: request type %q", domain.ErrNotFound, typeKey)
	}
	if fields == nil {
		fields = map[string]any{}
	}
	if threadID != "" {
		if _, err := d.threads.Thread(ctx, p.UserID, threadID); err != nil {
			return View{}, err
		}
	}
	missing, problems, err := check(typ.FieldsSchema, fields)
	if err != nil {
		return View{}, err
	}
	if len(missing) > 0 || len(problems) > 0 {
		return View{}, &InvalidError{Missing: missing, Problems: problems}
	}
	approvers, err := ResolveApprovers(ctx, d.cat, d.dir, p.UserID, typ, fields)
	if err != nil {
		return View{}, err
	}
	tk, err := d.svc.Submit(ctx, Submission{TypeKey: typ.Key, RequesterID: p.UserID, Fields: fields, Approvers: approvers})
	if err != nil {
		return View{}, err
	}
	r, err := mirrorOf(tk, threadID)
	if err != nil {
		return View{}, err
	}
	if err := d.mirror.SaveRequest(ctx, r); err != nil {
		return View{}, err
	}
	v := d.view(r)
	d.publish(ctx, v)
	return v, nil
}

// Sync pulls changed requests from the service, updates the mirror and notifies threads; returns how many changed.
func (d *Desk) Sync(ctx context.Context) (int, error) {
	changed, err := d.svc.Updates(ctx)
	n := 0
	for _, tk := range changed {
		threadID := ""
		if old, _, err := d.mirror.Request(ctx, tk.ID); err == nil {
			threadID = old.ThreadID
		}
		r, merr := mirrorOf(tk, threadID)
		if merr == nil {
			merr = d.mirror.SaveRequest(ctx, r)
		}
		if merr != nil {
			slog.Error("mirror request", "request", tk.ID, "err", merr)
			continue
		}
		n++
		d.publish(ctx, d.view(r))
	}
	return n, err
}

// Get shows a request to its author and to a manager of the author's team, like get_request_status.
func (d *Desk) Get(ctx context.Context, p domain.Principal, id string) (View, error) {
	r, team, err := d.mirror.Request(ctx, id)
	if err != nil {
		return View{}, err
	}
	own := r.UserID == p.UserID
	teamManager := p.Role == domain.RoleManager && team != "" && team == p.TeamID
	if !own && !teamManager {
		return View{}, tools.ErrForbidden
	}
	return d.view(r), nil
}

func (d *Desk) List(ctx context.Context, p domain.Principal) ([]View, error) {
	all, err := d.mirror.RequestsOf(ctx, p.UserID)
	if err != nil {
		return nil, err
	}
	out := make([]View, 0, len(all))
	for _, r := range all {
		out = append(out, d.view(r))
	}
	return out, nil
}

// Blockers for "today": access not requested yet (interns only), requests waiting and rejected ones.
func (d *Desk) Blockers(ctx context.Context, p domain.Principal) ([]plan.Blocker, error) {
	views, err := d.List(ctx, p)
	if err != nil {
		return nil, err
	}
	covered := map[string]bool{}
	var out []plan.Blocker
	for _, v := range views {
		if covered[v.TypeKey] {
			continue // an older request of the same type
		}
		switch v.Status {
		case StatusPending:
			covered[v.TypeKey] = true
			out = append(out, plan.Blocker{Kind: "request", ID: v.ID, Title: v.Title, Reason: v.NextStep})
		case StatusApproved:
			covered[v.TypeKey] = true
		case StatusRejected:
			covered[v.TypeKey] = true
			out = append(out, plan.Blocker{Kind: "request", ID: v.ID, Title: v.Title, Reason: v.NextStep})
		}
	}
	if p.Role == domain.RoleIntern {
		var missing []string
		for _, t := range d.cat.Types {
			if t.Category == "access" && !covered[t.Key] {
				missing = append(missing, t.Title)
			}
		}
		if len(missing) > 0 {
			out = append([]plan.Blocker{{
				Kind: "access_missing", Title: fmt.Sprintf("Не запрошены доступы: %d", len(missing)),
				Reason: strings.Join(missing, ", "),
			}}, out...)
		}
	}
	return out, nil
}

func (d *Desk) publish(ctx context.Context, v View) {
	if v.ThreadID == "" || d.pub == nil {
		return
	}
	e := stream.Event{Type: EventRequestStatus, Data: map[string]any{
		"request_id": v.ID, "type_key": v.TypeKey, "title": v.Title, "status": v.Status,
		"approver": v.Approver, "next_step": v.NextStep, "reason": v.Reason,
	}}
	if err := d.pub.Publish(ctx, v.ThreadID, e); err != nil {
		slog.Warn("publish request status", "request", v.ID, "err", err)
	}
}

func mirrorOf(tk Ticket, threadID string) (domain.Request, error) {
	fields, err := json.Marshal(tk.Fields)
	if err != nil {
		return domain.Request{}, err
	}
	approver, err := json.Marshal(tk.Approver())
	if err != nil {
		return domain.Request{}, err
	}
	history, err := json.Marshal(tk.History)
	if err != nil {
		return domain.Request{}, err
	}
	return domain.Request{
		UserID: tk.RequesterID, ThreadID: threadID, TypeKey: tk.TypeKey, ExternalID: tk.ID, Status: tk.Status,
		Fields: fields, Approver: approver, History: history, UpdatedAt: tk.UpdatedAt,
	}, nil
}

func (d *Desk) view(r domain.Request) View {
	v := View{ID: r.ExternalID, TypeKey: r.TypeKey, Status: r.Status, ThreadID: r.ThreadID, UpdatedAt: r.UpdatedAt,
		Fields: map[string]any{}, History: []Change{}}
	if typ, ok := d.cat.TypeByKey(r.TypeKey); ok {
		v.Title = typ.Title
	}
	_ = json.Unmarshal(r.Approver, &v.Approver)
	_ = json.Unmarshal(r.Fields, &v.Fields)
	_ = json.Unmarshal(r.History, &v.History)
	for i := len(v.History) - 1; i >= 0; i-- {
		if v.History[i].Status == StatusRejected {
			v.Reason = v.History[i].Note
			break
		}
	}
	switch v.Status {
	case StatusPending:
		who := v.Approver.Name
		if who == "" {
			who = v.Approver.Role
		}
		v.NextStep = "ждёт согласования: " + who
	case StatusApproved:
		v.NextStep = "доступ выдан"
	case StatusRejected:
		v.NextStep = "отклонена: поправь по причине и отправь заново"
	}
	return v
}

// IsInvalid reports whether err is about the request's fields.
func IsInvalid(err error) (*InvalidError, bool) {
	var ie *InvalidError
	ok := errors.As(err, &ie)
	return ie, ok
}
