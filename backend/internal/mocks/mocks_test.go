package mocks_test

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/structpb"

	authrolesv1 "github.com/Long-Honey-Road/mon-europeenne/backend/gen/mocks/authroles/v1"
	corprequestsv1 "github.com/Long-Honey-Road/mon-europeenne/backend/gen/mocks/corprequests/v1"
	documentsv1 "github.com/Long-Honey-Road/mon-europeenne/backend/gen/mocks/documents/v1"
	peoplev1 "github.com/Long-Honey-Road/mon-europeenne/backend/gen/mocks/people/v1"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/mocks"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/mocks/mockstest"
	"github.com/Long-Honey-Road/mon-europeenne/backend/seed"
)

func wantCode(t *testing.T, err error, want codes.Code) {
	t.Helper()
	if got := status.Code(err); got != want {
		t.Fatalf("code = %v (%v), want %v", got, err, want)
	}
}

func TestAuthRoles(t *testing.T) {
	conn, _ := mockstest.Start(t, mocks.Chaos{})
	c := authrolesv1.NewAuthRolesServiceClient(conn)
	ctx := t.Context()

	resp, err := c.Authenticate(ctx, &authrolesv1.AuthenticateRequest{Login: "intern", Password: "intern-demo"})
	if err != nil {
		t.Fatal(err)
	}
	a := resp.GetAccount()
	if a.GetUserId() != "u-liza" || a.GetRole() != "intern" || a.GetTeamId() != "t-messenger" || a.GetStartDate() == "" {
		t.Errorf("account = %+v", a)
	}
	_, err = c.Authenticate(ctx, &authrolesv1.AuthenticateRequest{Login: "intern", Password: "wrong"})
	wantCode(t, err, codes.Unauthenticated)
	_, err = c.Authenticate(ctx, &authrolesv1.AuthenticateRequest{Login: "ghost", Password: "intern-demo"})
	wantCode(t, err, codes.Unauthenticated)

	got, err := c.GetAccount(ctx, &authrolesv1.GetAccountRequest{UserId: "u-anna"})
	if err != nil || got.GetAccount().GetRole() != "mentor" {
		t.Errorf("GetAccount(u-anna) = %+v, %v", got, err)
	}
	_, err = c.GetAccount(ctx, &authrolesv1.GetAccountRequest{UserId: "u-ghost"})
	wantCode(t, err, codes.NotFound)
}

func TestDirectory(t *testing.T) {
	conn, _ := mockstest.Start(t, mocks.Chaos{})
	c := peoplev1.NewDirectoryServiceClient(conn)
	ctx := t.Context()

	all, err := c.ListPeople(ctx, &peoplev1.ListPeopleRequest{})
	if err != nil || len(all.GetPeople()) < 8 {
		t.Fatalf("ListPeople() = %d people, %v", len(all.GetPeople()), err)
	}
	team, _ := c.ListPeople(ctx, &peoplev1.ListPeopleRequest{TeamId: "t-people"})
	for _, p := range team.GetPeople() {
		if p.GetTeamId() != "t-people" {
			t.Errorf("team filter let %s of %s through", p.GetId(), p.GetTeamId())
		}
	}
	none, _ := c.ListPeople(ctx, &peoplev1.ListPeopleRequest{Q: "нет-такого-слова"})
	if len(none.GetPeople()) != 0 {
		t.Errorf("q filter = %d people, want 0", len(none.GetPeople()))
	}

	p, err := c.GetPerson(ctx, &peoplev1.GetPersonRequest{PersonId: "p-liza"})
	if err != nil || p.GetPerson().GetUserId() != "u-liza" {
		t.Errorf("GetPerson(p-liza) = %+v, %v", p, err)
	}
	if q, _ := c.ListPeople(ctx, &peoplev1.ListPeopleRequest{Q: p.GetPerson().GetName()}); len(q.GetPeople()) == 0 {
		t.Errorf("q by name %q found nobody", p.GetPerson().GetName())
	}
	_, err = c.GetPerson(ctx, &peoplev1.GetPersonRequest{PersonId: "p-ghost"})
	wantCode(t, err, codes.NotFound)

	teams, err := c.ListTeams(ctx, &peoplev1.ListTeamsRequest{})
	if err != nil || len(teams.GetTeams()) != 4 {
		t.Errorf("ListTeams() = %d teams, %v", len(teams.GetTeams()), err)
	}
	rel, err := c.GetRelations(ctx, &peoplev1.GetRelationsRequest{UserId: "u-liza"})
	if err != nil || rel.GetRelations().GetMentorId() == "" || rel.GetRelations().GetManagerId() == "" {
		t.Errorf("GetRelations(u-liza) = %+v, %v", rel, err)
	}
	empty, err := c.GetRelations(ctx, &peoplev1.GetRelationsRequest{UserId: "u-ghost"})
	if err != nil || empty.GetRelations().GetMentorId() != "" {
		t.Errorf("GetRelations(u-ghost) = %+v, %v", empty, err)
	}
}

func TestDocuments(t *testing.T) {
	conn, _ := mockstest.Start(t, mocks.Chaos{})
	c := documentsv1.NewDocumentsServiceClient(conn)
	ctx := t.Context()

	all, err := c.ListDocuments(ctx, &documentsv1.ListDocumentsRequest{})
	if err != nil || len(all.GetDocuments()) < 15 {
		t.Fatalf("ListDocuments() = %d, %v", len(all.GetDocuments()), err)
	}
	for _, d := range all.GetDocuments() {
		if !d.GetTestInstruction() {
			t.Errorf("%s is not marked as a training instruction", d.GetId())
		}
	}
	team, _ := c.ListDocuments(ctx, &documentsv1.ListDocumentsRequest{TeamId: "t-notifications"})
	review, _ := c.ListDocuments(ctx, &documentsv1.ListDocumentsRequest{Topic: "code-review"})
	if len(team.GetDocuments()) != 1 || len(review.GetDocuments()) != 2 {
		t.Errorf("filters: team %d, topic %d", len(team.GetDocuments()), len(review.GetDocuments()))
	}
	doc, err := c.GetDocument(ctx, &documentsv1.GetDocumentRequest{DocumentId: "messenger-dev-setup"})
	if err != nil || !strings.Contains(doc.GetMarkdown(), "make dev") || doc.GetInfo().GetTeamId() != "t-messenger" {
		t.Errorf("GetDocument = %+v, %v", doc.GetInfo(), err)
	}
	_, err = c.GetDocument(ctx, &documentsv1.GetDocumentRequest{DocumentId: "ghost"})
	wantCode(t, err, codes.NotFound)
}

func submit(t *testing.T, c corprequestsv1.CorpRequestsServiceClient, typeKey string, approvers ...string) *corprequestsv1.Request {
	t.Helper()
	fields, _ := structpb.NewStruct(map[string]any{"project_key": "MSG", "level": "read"})
	req := &corprequestsv1.SubmitRequestRequest{TypeKey: typeKey, RequesterId: "u-liza", Fields: fields}
	for _, role := range approvers {
		req.Approvers = append(req.Approvers, &corprequestsv1.Approver{Role: role, PersonId: "p-" + role, Name: role})
	}
	resp, err := c.SubmitRequest(t.Context(), req)
	if err != nil {
		t.Fatal(err)
	}
	return resp.GetRequest()
}

func TestCorpRequestsLifecycle(t *testing.T) {
	conn, srv := mockstest.Start(t, mocks.Chaos{})
	c := corprequestsv1.NewCorpRequestsServiceClient(conn)
	ctx := t.Context()

	_, err := c.SubmitRequest(ctx, &corprequestsv1.SubmitRequestRequest{TypeKey: "access.jira_project", RequesterId: "u-liza"})
	wantCode(t, err, codes.InvalidArgument)

	first := submit(t, c, "access.jira_project", "object_owner", "manager")
	if first.GetId() != "REQ-1" || first.GetStatus() != corprequestsv1.Status_STATUS_PENDING || first.GetFields().GetFields()["level"].GetStringValue() != "read" {
		t.Fatalf("submitted = %+v", first)
	}
	rejected := submit(t, c, "access.sentry_project", "object_owner")

	upd, err := c.ListUpdates(ctx, &corprequestsv1.ListUpdatesRequest{})
	if err != nil || len(upd.GetRequests()) != 2 {
		t.Fatalf("ListUpdates(0) = %d, %v", len(upd.GetRequests()), err)
	}
	cursor := upd.GetCursor()
	if again, _ := c.ListUpdates(ctx, &corprequestsv1.ListUpdatesRequest{After: cursor}); len(again.GetRequests()) != 0 {
		t.Errorf("ListUpdates(cursor) = %d, want 0", len(again.GetRequests()))
	}

	// First tick: chain of two moves to the second approver, the sentry request is rejected.
	srv.Requests.Tick(time.Now().Add(time.Hour))
	upd, _ = c.ListUpdates(ctx, &corprequestsv1.ListUpdatesRequest{After: cursor})
	if len(upd.GetRequests()) != 2 {
		t.Fatalf("after tick = %d updates, want 2", len(upd.GetRequests()))
	}
	got, _ := c.GetRequest(ctx, &corprequestsv1.GetRequestRequest{RequestId: first.GetId()})
	if r := got.GetRequest(); r.GetStatus() != corprequestsv1.Status_STATUS_PENDING || r.GetStep() != 1 || len(r.GetHistory()) != 2 {
		t.Errorf("after first approval = %+v", r)
	}
	got, _ = c.GetRequest(ctx, &corprequestsv1.GetRequestRequest{RequestId: rejected.GetId()})
	if r := got.GetRequest(); r.GetStatus() != corprequestsv1.Status_STATUS_REJECTED || r.GetReason() == "" {
		t.Errorf("sentry = %+v, want rejected with a reason", r)
	}

	srv.Requests.Tick(time.Now().Add(2 * time.Hour))
	got, _ = c.GetRequest(ctx, &corprequestsv1.GetRequestRequest{RequestId: first.GetId()})
	if got.GetRequest().GetStatus() != corprequestsv1.Status_STATUS_APPROVED {
		t.Errorf("after second approval = %v", got.GetRequest().GetStatus())
	}

	list, err := c.ListRequests(ctx, &corprequestsv1.ListRequestsRequest{RequesterId: "u-liza"})
	if err != nil || len(list.GetRequests()) != 2 || list.GetRequests()[0].GetId() != rejected.GetId() {
		t.Errorf("ListRequests = %+v, %v; want newest first", list.GetRequests(), err)
	}
	_, err = c.ListRequests(ctx, &corprequestsv1.ListRequestsRequest{})
	wantCode(t, err, codes.InvalidArgument)
	_, err = c.GetRequest(ctx, &corprequestsv1.GetRequestRequest{RequestId: "REQ-404"})
	wantCode(t, err, codes.NotFound)
}

func TestCorpRequestsRunTicks(t *testing.T) {
	conn, srv := mockstest.Start(t, mocks.Chaos{})
	c := corprequestsv1.NewCorpRequestsServiceClient(conn)
	submit(t, c, "access.unknown_type", "manager")

	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() { srv.Requests.Run(ctx, time.Millisecond); close(done) }()
	cancel()
	<-done
}

func TestChaos(t *testing.T) {
	conn, _ := mockstest.Start(t, mocks.Chaos{})
	c := peoplev1.NewDirectoryServiceClient(conn)
	for header, want := range map[string]codes.Code{"500": codes.Internal, "503": codes.Unavailable} {
		ctx := metadata.AppendToOutgoingContext(t.Context(), mocks.FailHeader, header)
		_, err := c.ListTeams(ctx, &peoplev1.ListTeamsRequest{})
		wantCode(t, err, want)
	}
	ctx, cancel := context.WithTimeout(metadata.AppendToOutgoingContext(t.Context(), mocks.FailHeader, "timeout"), 50*time.Millisecond)
	defer cancel()
	_, err := c.ListTeams(ctx, &peoplev1.ListTeamsRequest{})
	wantCode(t, err, codes.DeadlineExceeded)

	flaky, _ := mockstest.Start(t, mocks.Chaos{FailRate: 1, MaxLatency: time.Millisecond})
	_, err = peoplev1.NewDirectoryServiceClient(flaky).ListTeams(t.Context(), &peoplev1.ListTeamsRequest{})
	wantCode(t, err, codes.Unavailable)
}

func TestGatewayServesRESTWithChaos(t *testing.T) {
	data, err := seed.Load(seed.Files)
	if err != nil {
		t.Fatal(err)
	}
	srv, err := mocks.New(data, mocks.Chaos{})
	if err != nil {
		t.Fatal(err)
	}
	var lc net.ListenConfig
	ln, err := lc.Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = srv.GRPC.Serve(ln) }()
	t.Cleanup(srv.GRPC.Stop)
	gw, closeGW, err := mocks.Gateway(t.Context(), ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(closeGW)
	web := httptest.NewServer(gw)
	t.Cleanup(web.Close)

	get := func(path, fail string) int {
		req, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, web.URL+path, nil)
		if fail != "" {
			req.Header.Set(mocks.FailHeader, fail)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		return resp.StatusCode
	}
	if code := get("/people?team_id=t-messenger", ""); code != http.StatusOK {
		t.Errorf("GET /people = %d", code)
	}
	resp, err := http.Get(web.URL + "/people/p-liza") //nolint:noctx // test
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if !strings.Contains(string(body), `"user_id"`) || !strings.Contains(string(body), `"responsibilities"`) {
		t.Errorf("GET /people/p-liza = %s, want snake_case with empty fields", body)
	}
	if code := get("/auth/accounts/u-ghost", ""); code != http.StatusNotFound {
		t.Errorf("GET unknown account = %d, want 404", code)
	}
	if code := get("/teams", "503"); code != http.StatusServiceUnavailable {
		t.Errorf("GET /teams with fail header = %d, want 503", code)
	}
}

func TestRunStopsWithContext(t *testing.T) {
	data, _ := seed.Load(seed.Files)
	srv, err := mocks.New(data, mocks.Chaos{})
	if err != nil {
		t.Fatal(err)
	}
	var lc net.ListenConfig
	g, _ := lc.Listen(t.Context(), "tcp", "127.0.0.1:0")
	h, _ := lc.Listen(t.Context(), "tcp", "127.0.0.1:0")
	ctx, cancel := context.WithCancel(t.Context())
	errs := make(chan error, 1)
	go func() { errs <- srv.Run(ctx, g, h, time.Millisecond) }()
	time.Sleep(50 * time.Millisecond)
	cancel()
	if err := <-errs; err != nil {
		t.Errorf("Run() = %v", err)
	}
}
