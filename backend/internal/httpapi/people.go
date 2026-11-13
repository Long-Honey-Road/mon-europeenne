package httpapi

import (
	"context"
	"errors"
	"slices"
	"strings"
	"unicode/utf8"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	buddyv1 "github.com/Long-Honey-Road/mon-europeenne/backend/gen/buddy/v1"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/domain"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/people"
)

// stemLen keeps Russian word forms matching: "деплой", "деплоем" and "деплоя" share the first five letters.
const stemLen = 5

type peopleService struct {
	buddyv1.UnimplementedPeopleServiceServer
	deps Deps
}

func (s *peopleService) ListPeople(ctx context.Context, req *buddyv1.ListPeopleRequest) (*buddyv1.ListPeopleResponse, error) {
	p, err := principal(ctx)
	if err != nil {
		return nil, err
	}
	all, err := s.deps.Directory.People(ctx)
	if err != nil {
		return nil, status.Error(codes.Unavailable, "directory unavailable")
	}
	words := stems(req.GetQ())
	var found []domain.Person
	for _, person := range all {
		if req.GetTeamId() != "" && person.TeamID != req.GetTeamId() {
			continue
		}
		if matches(person, words) {
			found = append(found, person)
		}
	}
	slices.SortStableFunc(found, func(a, b domain.Person) int {
		if (a.TeamID == p.TeamID) != (b.TeamID == p.TeamID) {
			if a.TeamID == p.TeamID {
				return -1
			}
			return 1
		}
		return strings.Compare(a.Name, b.Name)
	})
	view, err := s.viewer(ctx, p)
	if err != nil {
		return nil, err
	}
	resp := &buddyv1.ListPeopleResponse{People: make([]*buddyv1.Person, 0, len(found))}
	for _, person := range found {
		pp, err := view.person(ctx, person)
		if err != nil {
			return nil, err
		}
		resp.People = append(resp.People, pp)
	}
	return resp, nil
}

func (s *peopleService) GetPerson(ctx context.Context, req *buddyv1.GetPersonRequest) (*buddyv1.GetPersonResponse, error) {
	p, err := principal(ctx)
	if err != nil {
		return nil, err
	}
	person, err := s.deps.Directory.Person(ctx, req.GetPersonId())
	if errors.Is(err, people.ErrNotFound) {
		return nil, status.Error(codes.NotFound, "person not found")
	}
	if err != nil {
		return nil, status.Error(codes.Unavailable, "directory unavailable")
	}
	view, err := s.viewer(ctx, p)
	if err != nil {
		return nil, err
	}
	pp, err := view.person(ctx, person)
	if err != nil {
		return nil, err
	}
	return &buddyv1.GetPersonResponse{Person: pp}, nil
}

// viewer applies the caller's visibility: contacts only within the caller's cluster, as in who_to_ask.
type viewer struct {
	dir      Directory
	team     string
	cluster  string
	clusters map[string]string
}

func (s *peopleService) viewer(ctx context.Context, p domain.Principal) (*viewer, error) {
	v := &viewer{dir: s.deps.Directory, team: p.TeamID, clusters: map[string]string{}}
	c, err := v.clusterOf(ctx, p.TeamID)
	if err != nil {
		return nil, err
	}
	v.cluster = c
	return v, nil
}

func (v *viewer) clusterOf(ctx context.Context, teamID string) (string, error) {
	if c, ok := v.clusters[teamID]; ok {
		return c, nil
	}
	team, err := v.dir.Team(ctx, teamID)
	if err != nil && !errors.Is(err, people.ErrNotFound) {
		return "", status.Error(codes.Unavailable, "directory unavailable")
	}
	v.clusters[teamID] = team.Cluster
	return team.Cluster, nil
}

func (v *viewer) person(ctx context.Context, p domain.Person) (*buddyv1.Person, error) {
	out := &buddyv1.Person{
		Id: p.ID, Name: p.Name, RoleTitle: p.RoleTitle, TeamId: p.TeamID,
		Responsibilities: orEmpty(p.Responsibilities), AskMeAbout: orEmpty(p.AskMeAbout), OtherTeam: p.TeamID != v.team,
	}
	c, err := v.clusterOf(ctx, p.TeamID)
	if err != nil {
		return nil, err
	}
	if v.cluster != "" && c == v.cluster {
		out.Contact = p.Contact
	}
	return out, nil
}

func stems(q string) []string {
	var out []string
	for _, w := range strings.Fields(strings.ToLower(q)) {
		if utf8.RuneCountInString(w) > stemLen {
			w = string([]rune(w)[:stemLen])
		}
		out = append(out, w)
	}
	return out
}

// matches is true when every query word appears in the person's name, role, responsibilities or topics.
func matches(p domain.Person, words []string) bool {
	text := strings.ToLower(strings.Join(append(append([]string{p.Name, p.RoleTitle}, p.Responsibilities...), p.AskMeAbout...), " "))
	for _, w := range words {
		if !strings.Contains(text, w) {
			return false
		}
	}
	return true
}
