package httpapi

import (
	"context"
	"net/http"
	"sync"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/auth"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/domain"
)

// fakeHiring stands in for the auth-roles CreateAccount call.
type fakeHiring struct {
	mu    sync.Mutex
	calls []struct {
		requester string
		hire      auth.Hire
	}
	err error
}

func (f *fakeHiring) Hire(_ context.Context, requesterID string, h auth.Hire) (domain.User, string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, struct {
		requester string
		hire      auth.Hire
	}{requesterID, h})
	if f.err != nil {
		return domain.User{}, "", f.err
	}
	return domain.User{
		ID: "u-" + h.Login, Login: h.Login, DisplayName: h.DisplayName, Role: domain.RoleIntern,
		Direction: h.Direction, TeamID: "t-messenger",
	}, "Abcdefgh2345", nil
}

// fakeUsers records the users buddy makes sure it has.
type fakeUsers struct {
	mu   sync.Mutex
	seen []domain.User
	err  error
}

func (f *fakeUsers) EnsureUser(_ context.Context, u domain.User) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.seen = append(f.seen, u)
	return f.err
}

func (f *fakeUsers) ids() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []string
	for _, u := range f.seen {
		out = append(out, u.ID)
	}
	return out
}

// forgetful counts directory cache drops.
type forgetful struct {
	Directory
	mu      sync.Mutex
	dropped int
}

func (f *forgetful) Forget() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.dropped++
}

const internBody = `{"display_name":"Вера Новикова","login":"vnovikova","direction":"DIRECTION_BACKEND","mentor_id":"p-anna","start_date":"2026-11-16"}`

func TestManagerAddsIntern(t *testing.T) {
	hiring, users := &fakeHiring{}, &fakeUsers{}
	d := deps(t)
	dir := &forgetful{Directory: d.Directory}
	d.Hiring, d.Users, d.Directory = hiring, users, dir
	s := newServerWith(t, d)

	code, body := call(t, s, http.MethodPost, "/v1/private/team/interns", login(t, s, "manager", "manager-demo"), internBody)
	if code != http.StatusOK {
		t.Fatalf("add intern: %d %v", code, body)
	}
	if body["login"] != "vnovikova" || body["temporary_password"] != "Abcdefgh2345" {
		t.Errorf("response = %v", body)
	}
	if u, _ := body["user"].(map[string]any); u["role"] != "ROLE_INTERN" || u["display_name"] != "Вера Новикова" {
		t.Errorf("user = %v", body["user"])
	}
	if len(hiring.calls) != 1 {
		t.Fatalf("hiring calls = %d", len(hiring.calls))
	}
	c := hiring.calls[0]
	if c.requester != "u-dmitry" || c.hire.Login != "vnovikova" || c.hire.Direction != domain.DirectionBackend ||
		c.hire.MentorID != "p-anna" || c.hire.StartDate != "2026-11-16" {
		t.Errorf("hire call = %+v", c)
	}
	if ids := users.ids(); len(ids) != 2 || ids[1] != "u-vnovikova" {
		t.Errorf("users ensured = %v, want the manager at sign-in, then the intern", ids)
	}
	if dir.dropped != 1 {
		t.Errorf("directory cache dropped %d times, want 1", dir.dropped)
	}
}

func TestOnlyManagersAddInterns(t *testing.T) {
	hiring := &fakeHiring{}
	d := deps(t)
	d.Hiring = hiring
	s := newServerWith(t, d)
	for _, who := range [][2]string{{"intern", "intern-demo"}, {"mentor", "mentor-demo"}} {
		code, _ := call(t, s, http.MethodPost, "/v1/private/team/interns", login(t, s, who[0], who[1]), internBody)
		if code != http.StatusForbidden {
			t.Errorf("%s: %d, want 403", who[0], code)
		}
	}
	if len(hiring.calls) != 0 {
		t.Errorf("the account system was called %d times", len(hiring.calls))
	}
	code, _ := call(t, s, http.MethodPost, "/v1/private/team/interns", "", internBody)
	if code != http.StatusUnauthorized {
		t.Errorf("no session: %d, want 401", code)
	}
}

func TestAddInternPassesRefusalsThrough(t *testing.T) {
	cases := []struct {
		err  error
		want int
	}{
		{status.Error(codes.AlreadyExists, `login "vnovikova" is taken`), http.StatusConflict},
		{status.Error(codes.InvalidArgument, "mentor must be a person of the manager's team"), http.StatusBadRequest},
		{status.Error(codes.PermissionDenied, "only a team manager may hire interns"), http.StatusForbidden},
		{status.Error(codes.Unavailable, "down"), http.StatusServiceUnavailable},
		{context.DeadlineExceeded, http.StatusServiceUnavailable},
	}
	for _, tc := range cases {
		d := deps(t)
		d.Hiring = &fakeHiring{err: tc.err}
		s := newServerWith(t, d)
		code, body := call(t, s, http.MethodPost, "/v1/private/team/interns", login(t, s, "manager", "manager-demo"), internBody)
		if code != tc.want {
			t.Errorf("%v: %d %v, want %d", tc.err, code, body, tc.want)
		}
	}
}

func TestAddInternChecksTheRequestFirst(t *testing.T) {
	hiring := &fakeHiring{}
	d := deps(t)
	d.Hiring = hiring
	s := newServerWith(t, d)
	token := login(t, s, "manager", "manager-demo")
	for _, b := range []string{
		`{"display_name":"","login":"vnovikova","direction":"DIRECTION_BACKEND","mentor_id":"p-anna"}`,
		`{"display_name":"Вера","login":"vnovikova","direction":"DIRECTION_UNSPECIFIED","mentor_id":"p-anna"}`,
		`{"display_name":"Вера","login":"vnovikova","direction":"DIRECTION_BACKEND","mentor_id":""}`,
	} {
		if code, _ := call(t, s, http.MethodPost, "/v1/private/team/interns", token, b); code != http.StatusBadRequest {
			t.Errorf("%s: %d, want 400", b, code)
		}
	}
	if len(hiring.calls) != 0 {
		t.Errorf("invalid requests reached the account system: %d", len(hiring.calls))
	}
}

func TestAddInternWithoutAccountSystem(t *testing.T) {
	s := newServer(t)
	code, _ := call(t, s, http.MethodPost, "/v1/private/team/interns", login(t, s, "manager", "manager-demo"), internBody)
	if code != http.StatusNotImplemented {
		t.Errorf("no account system: %d, want 501", code)
	}
}

func TestSignInMakesSureBuddyKnowsTheUser(t *testing.T) {
	users := &fakeUsers{}
	d := deps(t)
	d.Users = users
	s := newServerWith(t, d)
	login(t, s, "intern", "intern-demo")
	if ids := users.ids(); len(ids) != 1 || ids[0] != "u-liza" {
		t.Errorf("users ensured = %v", ids)
	}

	users.err = context.DeadlineExceeded
	code, _ := call(t, s, http.MethodPost, "/v1/public/auth/login", "", `{"login":"intern","password":"intern-demo"}`)
	if code != http.StatusServiceUnavailable {
		t.Errorf("sign-in with the store down: %d, want 503", code)
	}
}
