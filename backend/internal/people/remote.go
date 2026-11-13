package people

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"google.golang.org/grpc"

	peoplev1 "github.com/Long-Honey-Road/mon-europeenne/backend/gen/mocks/people/v1"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/domain"
)

// Directory is the people directory as buddy reads it: Memory from seed, or Remote over the people service.
type Directory interface {
	People(ctx context.Context) ([]domain.Person, error)
	Person(ctx context.Context, id string) (domain.Person, error)
	Team(ctx context.Context, id string) (domain.Team, error)
	Relations(ctx context.Context, userID string) (Relations, error)
}

// Remote reads the whole directory from the people service and keeps it for ttl. When the service fails, it
// serves the last good copy, so a directory outage does not break answers.
type Remote struct {
	c   peoplev1.DirectoryServiceClient
	ttl time.Duration
	now func() time.Time

	mu   sync.Mutex
	snap *Memory
	at   time.Time
}

func NewRemote(conn grpc.ClientConnInterface, ttl time.Duration) *Remote {
	return &Remote{c: peoplev1.NewDirectoryServiceClient(conn), ttl: ttl, now: time.Now}
}

func (r *Remote) current(ctx context.Context) (*Memory, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.snap != nil && r.now().Sub(r.at) < r.ttl {
		return r.snap, nil
	}
	snap, err := r.load(ctx)
	if err != nil {
		if r.snap == nil {
			return nil, fmt.Errorf("people service: %w", err)
		}
		slog.Warn("people service unavailable, serving the last copy", "err", err, "age", r.now().Sub(r.at))
		// Wait a full ttl before the next attempt, so an outage does not slow every call.
		r.at = r.now()
		return r.snap, nil
	}
	r.snap, r.at = snap, r.now()
	return snap, nil
}

func (r *Remote) load(ctx context.Context) (*Memory, error) {
	teamsResp, err := r.c.ListTeams(ctx, &peoplev1.ListTeamsRequest{})
	if err != nil {
		return nil, err
	}
	peopleResp, err := r.c.ListPeople(ctx, &peoplev1.ListPeopleRequest{})
	if err != nil {
		return nil, err
	}
	teams := make([]domain.Team, 0, len(teamsResp.GetTeams()))
	for _, t := range teamsResp.GetTeams() {
		teams = append(teams, domain.Team{ID: t.GetId(), Name: t.GetName(), Unit: t.GetUnit(), Cluster: t.GetCluster(), MattermostChannel: t.GetMattermostChannel()})
	}
	persons := make([]domain.Person, 0, len(peopleResp.GetPeople()))
	relations := map[string]Relations{}
	for _, p := range peopleResp.GetPeople() {
		persons = append(persons, domain.Person{
			ID: p.GetId(), UserID: p.GetUserId(), Name: p.GetName(), RoleTitle: p.GetRoleTitle(), TeamID: p.GetTeamId(),
			Responsibilities: p.GetResponsibilities(), AskMeAbout: p.GetAskMeAbout(), Contact: p.GetContact(),
		})
		if p.GetUserId() == "" {
			continue
		}
		rel, err := r.c.GetRelations(ctx, &peoplev1.GetRelationsRequest{UserId: p.GetUserId()})
		if err != nil {
			return nil, err
		}
		g := rel.GetRelations()
		relations[p.GetUserId()] = Relations{MentorID: g.GetMentorId(), ManagerID: g.GetManagerId(), HRBPID: g.GetHrBpId()}
	}
	return NewMemory(teams, persons, relations), nil
}

func (r *Remote) People(ctx context.Context) ([]domain.Person, error) {
	m, err := r.current(ctx)
	if err != nil {
		return nil, err
	}
	return m.People(ctx)
}

func (r *Remote) Person(ctx context.Context, id string) (domain.Person, error) {
	m, err := r.current(ctx)
	if err != nil {
		return domain.Person{}, err
	}
	return m.Person(ctx, id)
}

func (r *Remote) Team(ctx context.Context, id string) (domain.Team, error) {
	m, err := r.current(ctx)
	if err != nil {
		return domain.Team{}, err
	}
	return m.Team(ctx, id)
}

func (r *Remote) Relations(ctx context.Context, userID string) (Relations, error) {
	m, err := r.current(ctx)
	if err != nil {
		return Relations{}, err
	}
	return m.Relations(ctx, userID)
}
