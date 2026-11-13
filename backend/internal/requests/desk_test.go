package requests

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/structpb"

	buddyv1 "github.com/Long-Honey-Road/mon-europeenne/backend/gen/buddy/v1"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/auth"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/domain"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/stream"
	"github.com/Long-Honey-Road/mon-europeenne/backend/seed"
)

var errDown = errors.New("down")

// fakeService approves or rejects on Advance, by type: types with "sentry" are rejected.
type fakeService struct {
	tickets map[string]Ticket
	order   []string
	failOn  string
}

func (f *fakeService) Submit(_ context.Context, s Submission) (Ticket, error) {
	if f.failOn == "submit" {
		return Ticket{}, errDown
	}
	id := fmt.Sprintf("REQ-%d", len(f.order)+1)
	t := Ticket{ID: id, TypeKey: s.TypeKey, RequesterID: s.RequesterID, Fields: s.Fields, Status: StatusPending,
		Approvers: s.Approvers, History: []Change{{Status: StatusPending}}, UpdatedAt: time.Unix(int64(len(f.order)), 0)}
	f.tickets[id] = t
	f.order = append(f.order, id)
	return t, nil
}

func (f *fakeService) Advance(_ context.Context, now time.Time) ([]Ticket, error) {
	var out []Ticket
	for _, id := range f.order {
		t := f.tickets[id]
		if t.Status != StatusPending {
			continue
		}
		if strings.Contains(t.TypeKey, "sentry") {
			t.Status = StatusRejected
			t.History = append(t.History, Change{Status: StatusRejected, Note: "слишком общее"})
		} else {
			t.Status = StatusApproved
			t.History = append(t.History, Change{Status: StatusApproved})
		}
		t.UpdatedAt = now
		f.tickets[id] = t
		out = append(out, t)
	}
	if f.failOn == "advance" {
		return out, errDown
	}
	return out, nil
}

type memMirror struct {
	byID    map[string]domain.Request
	failOn  string
	ordered []string
}

func (m *memMirror) SaveRequest(_ context.Context, r domain.Request) error {
	if m.failOn == "save" {
		return errDown
	}
	if _, ok := m.byID[r.ExternalID]; !ok {
		m.ordered = append(m.ordered, r.ExternalID)
	}
	m.byID[r.ExternalID] = r
	return nil
}

func (m *memMirror) Request(_ context.Context, id string) (domain.Request, string, error) {
	r, ok := m.byID[id]
	if !ok {
		return domain.Request{}, "", domain.ErrNotFound
	}
	return r, "t-messenger", nil
}

func (m *memMirror) RequestsOf(_ context.Context, userID string) ([]domain.Request, error) {
	if m.failOn == "list" {
		return nil, errDown
	}
	var out []domain.Request
	for i := len(m.ordered) - 1; i >= 0; i-- {
		if r := m.byID[m.ordered[i]]; r.UserID == userID {
			out = append(out, r)
		}
	}
	return out, nil
}

type memThreads struct{}

func (memThreads) Thread(_ context.Context, userID, id string) (domain.Thread, error) {
	if id == "th-liza" && userID == "u-liza" {
		return domain.Thread{ID: id, UserID: userID}, nil
	}
	return domain.Thread{}, domain.ErrNotFound
}

type recorder struct{ events []stream.Event }

func (r *recorder) Publish(_ context.Context, threadID string, e stream.Event) error {
	if threadID != "th-liza" {
		return errDown
	}
	r.events = append(r.events, e)
	return nil
}

type deskFixture struct {
	desk   *Desk
	svc    *fakeService
	mirror *memMirror
	pub    *recorder
}

func newDeskFixture(t *testing.T) deskFixture {
	t.Helper()
	d, err := seed.Load(seed.Files)
	if err != nil {
		t.Fatal(err)
	}
	f := deskFixture{svc: &fakeService{tickets: map[string]Ticket{}}, mirror: &memMirror{byID: map[string]domain.Request{}}, pub: &recorder{}}
	f.desk = NewDesk(CatalogFromSeed(d), f.svc, f.mirror, d.Directory(), memThreads{}, f.pub)
	return f
}

var jira = map[string]any{"project_key": "MSG", "level": "read", "justification": "первая задача в MSG"}

func TestSubmitSendsMirrorsAndNotifiesTheThread(t *testing.T) {
	f := newDeskFixture(t)

	v, err := f.desk.Submit(context.Background(), liza, "th-liza", "access.jira_project", jira)
	if err != nil {
		t.Fatal(err)
	}
	if v.ID != "REQ-1" || v.Status != StatusPending || v.Approver.PersonID != "p-dmitry" || v.Title == "" {
		t.Fatalf("view = %+v", v)
	}
	if v.NextStep != "ждёт согласования: "+v.Approver.Name {
		t.Fatalf("next step = %q", v.NextStep)
	}
	if r := f.mirror.byID["REQ-1"]; r.UserID != "u-liza" || r.ThreadID != "th-liza" {
		t.Fatalf("mirror = %+v", r)
	}
	if len(f.pub.events) != 1 || f.pub.events[0].Type != EventRequestStatus {
		t.Fatalf("events = %+v", f.pub.events)
	}
	if got := f.svc.tickets["REQ-1"].Approvers; len(got) != 1 || got[0].Role != "object_owner" {
		t.Fatalf("the service gets the resolved chain: %+v", got)
	}
}

func TestSubmitRefusals(t *testing.T) {
	f := newDeskFixture(t)
	ctx := context.Background()

	_, err := f.desk.Submit(ctx, liza, "", "access.nope", jira)
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("unknown type: %v", err)
	}
	_, err = f.desk.Submit(ctx, liza, "th-someone", "access.jira_project", jira)
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("someone else's thread: %v", err)
	}
	_, err = f.desk.Submit(ctx, liza, "", "access.jira_project", map[string]any{"project_key": "MSG", "level": "admin"})
	ie, ok := IsInvalid(err)
	if !ok || !slices.Equal(ie.Missing, []string{"justification"}) || len(ie.Problems) == 0 {
		t.Fatalf("invalid fields: %v", err)
	}
	if !strings.Contains(ie.Error(), "missing: justification") {
		t.Fatalf("message = %q", ie.Error())
	}
	_, err = f.desk.Submit(ctx, liza, "", "access.vpn", nil)
	if _, ok := IsInvalid(err); !ok {
		t.Fatalf("nil fields: %v", err)
	}
	f.svc.failOn = "submit"
	if _, err := f.desk.Submit(ctx, liza, "", "access.vpn", map[string]any{"justification": "работа из дома"}); !errors.Is(err, errDown) {
		t.Fatalf("service down: %v", err)
	}
	if len(f.mirror.byID) != 0 {
		t.Fatal("nothing must be mirrored on refusal")
	}
}

func TestSyncUpdatesMirrorAndThread(t *testing.T) {
	f := newDeskFixture(t)
	ctx := context.Background()
	if _, err := f.desk.Submit(ctx, liza, "th-liza", "access.sentry_project", map[string]any{"project": "messenger-api", "justification": "разбор ошибок первой задачи"}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.desk.Submit(ctx, liza, "", "access.vpn", map[string]any{"justification": "работа из дома"}); err != nil {
		t.Fatal(err)
	}

	n, err := f.desk.Sync(ctx, time.Now())
	if err != nil || n != 2 {
		t.Fatalf("sync: %d %v", n, err)
	}
	sentry, _ := f.desk.Get(ctx, liza, "REQ-1")
	if sentry.Status != StatusRejected || sentry.Reason != "слишком общее" || sentry.ThreadID != "th-liza" {
		t.Fatalf("sentry = %+v", sentry)
	}
	if vpn, _ := f.desk.Get(ctx, liza, "REQ-2"); vpn.Status != StatusApproved || vpn.NextStep != "доступ выдан" {
		t.Fatalf("vpn = %+v", vpn)
	}
	// Submit of REQ-1 and its change go to the thread; REQ-2 has no thread.
	if len(f.pub.events) != 2 || f.pub.events[1].Data["status"] != StatusRejected {
		t.Fatalf("events = %+v", f.pub.events)
	}
}

func TestSyncKeepsGoingAfterMirrorFailure(t *testing.T) {
	f := newDeskFixture(t)
	ctx := context.Background()
	_, _ = f.desk.Submit(ctx, liza, "", "access.vpn", map[string]any{"justification": "работа из дома"})
	f.mirror.failOn = "save"
	if n, err := f.desk.Sync(ctx, time.Now()); n != 0 || err != nil {
		t.Fatalf("sync = %d %v", n, err)
	}
	f.mirror.failOn = ""
	f.svc.failOn = "advance"
	if _, err := f.desk.Sync(ctx, time.Now()); !errors.Is(err, errDown) {
		t.Fatalf("service error must surface: %v", err)
	}
}

func TestGetRights(t *testing.T) {
	f := newDeskFixture(t)
	ctx := context.Background()
	_, _ = f.desk.Submit(ctx, liza, "", "access.vpn", map[string]any{"justification": "работа из дома"})

	if _, err := f.desk.Get(ctx, dmitry, "REQ-1"); err != nil {
		t.Fatalf("team manager: %v", err)
	}
	if _, err := f.desk.Get(ctx, anna, "REQ-1"); err == nil {
		t.Fatal("mentor must not see the intern's request")
	}
	if _, err := f.desk.Get(ctx, liza, "REQ-404"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("missing: %v", err)
	}
}

func TestBlockers(t *testing.T) {
	f := newDeskFixture(t)
	ctx := context.Background()

	b, err := f.desk.Blockers(ctx, liza)
	if err != nil || len(b) != 1 || b[0].Kind != "access_missing" || b[0].Title != "Не запрошены доступы: 7" {
		t.Fatalf("fresh intern: %+v %v", b, err)
	}
	if b, _ := f.desk.Blockers(ctx, dmitry); len(b) != 0 {
		t.Fatalf("managers get no access blocker: %+v", b)
	}

	_, _ = f.desk.Submit(ctx, liza, "", "access.sentry_project", map[string]any{"project": "messenger-api", "justification": "разбор ошибок первой задачи"})
	_, _ = f.desk.Submit(ctx, liza, "", "access.vpn", map[string]any{"justification": "работа из дома"})
	_, _ = f.desk.Sync(ctx, time.Now())
	_, _ = f.desk.Submit(ctx, liza, "", "access.jira_project", jira)

	b, _ = f.desk.Blockers(ctx, liza)
	kinds := map[string]int{}
	for _, x := range b {
		kinds[x.Kind]++
	}
	// Jira pending, Sentry rejected, VPN approved; four types not requested.
	if kinds["request"] != 2 || b[0].Title != "Не запрошены доступы: 4" {
		t.Fatalf("blockers = %+v", b)
	}

	f.mirror.failOn = "list"
	if _, err := f.desk.Blockers(ctx, liza); err == nil {
		t.Fatal("storage error must surface")
	}
}

func TestAPI(t *testing.T) {
	f := newDeskFixture(t)
	api := NewAPI(f.desk)
	ctx := auth.WithPrincipal(context.Background(), liza)

	types, err := api.ListRequestTypes(ctx, &buddyv1.ListRequestTypesRequest{Category: "access"})
	if err != nil || len(types.GetTypes()) != 7 || types.GetTypes()[0].GetFieldsSchema() == nil {
		t.Fatalf("types: %v %v", types, err)
	}
	fields, _ := structpb.NewStruct(jira)
	sub, err := api.SubmitRequest(ctx, &buddyv1.SubmitRequestRequest{ThreadId: "th-liza", TypeKey: "access.jira_project", Fields: fields})
	if err != nil {
		t.Fatal(err)
	}
	r := sub.GetRequest()
	if r.GetId() != "REQ-1" || r.GetStatus() != buddyv1.RequestStatus_REQUEST_STATUS_PENDING ||
		r.GetFields().GetFields()["project_key"].GetStringValue() != "MSG" || len(r.GetHistory()) != 1 {
		t.Fatalf("submitted = %v", r)
	}
	if list, err := api.ListRequests(ctx, &buddyv1.ListRequestsRequest{}); err != nil || len(list.GetRequests()) != 1 {
		t.Fatalf("list: %v %v", list, err)
	}
	if got, err := api.GetRequest(ctx, &buddyv1.GetRequestRequest{RequestId: "REQ-1"}); err != nil || got.GetRequest().GetApprover().GetPersonId() != "p-dmitry" {
		t.Fatalf("get: %v %v", got, err)
	}

	for name, tc := range map[string]struct {
		ctx  context.Context
		call func(context.Context) error
		want codes.Code
	}{
		"no principal": {context.Background(), func(c context.Context) error { _, err := api.ListRequests(c, nil); return err }, codes.Unauthenticated},
		"no type": {ctx, func(c context.Context) error {
			_, err := api.SubmitRequest(c, &buddyv1.SubmitRequestRequest{})
			return err
		}, codes.InvalidArgument},
		"bad fields": {ctx, func(c context.Context) error {
			_, err := api.SubmitRequest(c, &buddyv1.SubmitRequestRequest{TypeKey: "access.vpn"})
			return err
		}, codes.InvalidArgument},
		"someone else's": {auth.WithPrincipal(context.Background(), anna), func(c context.Context) error {
			_, err := api.GetRequest(c, &buddyv1.GetRequestRequest{RequestId: "REQ-1"})
			return err
		}, codes.NotFound},
	} {
		t.Run(name, func(t *testing.T) {
			if got := status.Code(tc.call(tc.ctx)); got != tc.want {
				t.Fatalf("code = %v, want %v", got, tc.want)
			}
		})
	}

	f.mirror.failOn = "list"
	if _, err := api.ListRequests(ctx, nil); status.Code(err) != codes.Unavailable {
		t.Fatalf("storage down: %v", err)
	}
}

func TestPrepareOffersAnActionOnlyWhenValid(t *testing.T) {
	r := fixture(t)
	var out struct {
		Valid  bool    `json:"valid"`
		Action *Action `json:"action"`
	}
	if err := callTool(t, r, liza, "prepare_request", `{"type_key":"access.vpn","fields":{"justification":"работа из дома"}}`, &out); err != nil {
		t.Fatal(err)
	}
	if !out.Valid || out.Action == nil || out.Action.Kind != "request" || out.Action.TypeKey != "access.vpn" || len(out.Action.Approvers) != 1 {
		t.Fatalf("valid = %+v", out)
	}
	out.Action = nil
	if err := callTool(t, r, liza, "prepare_request", `{"type_key":"access.vpn","fields":{}}`, &out); err != nil {
		t.Fatal(err)
	}
	if out.Valid || out.Action != nil {
		t.Fatalf("invalid must have no action: %+v", out)
	}
}
