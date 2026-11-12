package httpapi

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	buddyv1 "github.com/Long-Honey-Road/mon-europeenne/backend/gen/buddy/v1"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/domain"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/people"
)

var errDown = errors.New("directory down")

// brokenDirectory fails one method and delegates the rest.
type brokenDirectory struct {
	Directory
	broken string
}

func (d brokenDirectory) Person(ctx context.Context, id string) (domain.Person, error) {
	if d.broken == "person" {
		return domain.Person{}, errDown
	}
	return d.Directory.Person(ctx, id)
}

func (d brokenDirectory) Team(ctx context.Context, id string) (domain.Team, error) {
	switch d.broken {
	case "team":
		return domain.Team{}, errDown
	case "no team":
		return domain.Team{}, people.ErrNotFound
	}
	return d.Directory.Team(ctx, id)
}

func (d brokenDirectory) Relations(ctx context.Context, userID string) (people.Relations, error) {
	if d.broken == "relations" {
		return people.Relations{}, errDown
	}
	return d.Directory.Relations(ctx, userID)
}

func TestMeReportsDirectoryOutage(t *testing.T) {
	for _, broken := range []string{"team", "relations", "person"} {
		t.Run(broken, func(t *testing.T) {
			d := deps(t)
			d.Directory = brokenDirectory{Directory: d.Directory, broken: broken}
			s := newServerWith(t, d)
			token := login(t, s, "intern", "intern-demo")

			if code, body := call(t, s, http.MethodGet, "/v1/private/me", token, ""); code != http.StatusServiceUnavailable {
				t.Fatalf("status %d, body %v; want 503", code, body)
			}
		})
	}
}

func TestMeWithoutTeamInDirectory(t *testing.T) {
	d := deps(t)
	d.Directory = brokenDirectory{Directory: d.Directory, broken: "no team"}
	s := newServerWith(t, d)
	token := login(t, s, "intern", "intern-demo")

	code, body := call(t, s, http.MethodGet, "/v1/private/me", token, "")
	if code != http.StatusOK || body["team"] != nil {
		t.Fatalf("status %d, team %v; want 200 and null team", code, body["team"])
	}
}

func TestGetMeNeedsPrincipalEvenWithoutMiddleware(t *testing.T) {
	svc := &profileService{deps: deps(t)}
	_, err := svc.GetMe(context.Background(), &buddyv1.GetMeRequest{})
	if status.Code(err) != codes.Unauthenticated {
		t.Fatalf("error = %v, want Unauthenticated", err)
	}
}
