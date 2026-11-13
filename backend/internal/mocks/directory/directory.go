// Package directory is the people mock: synthetic teams, people and relations from seed.
package directory

import (
	"context"
	"errors"
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	peoplev1 "github.com/Long-Honey-Road/mon-europeenne/backend/gen/mocks/people/v1"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/domain"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/people"
)

type Service struct {
	peoplev1.UnimplementedDirectoryServiceServer
	dir   *people.Memory
	teams []domain.Team
}

func New(dir *people.Memory, teams []domain.Team) *Service {
	return &Service{dir: dir, teams: teams}
}

func (s *Service) ListPeople(ctx context.Context, req *peoplev1.ListPeopleRequest) (*peoplev1.ListPeopleResponse, error) {
	all, err := s.dir.People(ctx)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	words := strings.Fields(strings.ToLower(req.GetQ()))
	out := &peoplev1.ListPeopleResponse{}
	for _, p := range all {
		if req.GetTeamId() != "" && p.TeamID != req.GetTeamId() {
			continue
		}
		if !matches(p, words) {
			continue
		}
		out.People = append(out.People, toPerson(p))
	}
	return out, nil
}

// matches is true when every word occurs in the name, role, responsibilities or topics.
func matches(p domain.Person, words []string) bool {
	text := strings.ToLower(strings.Join(append(append([]string{p.Name, p.RoleTitle}, p.Responsibilities...), p.AskMeAbout...), " "))
	for _, w := range words {
		if !strings.Contains(text, w) {
			return false
		}
	}
	return true
}

func (s *Service) GetPerson(ctx context.Context, req *peoplev1.GetPersonRequest) (*peoplev1.GetPersonResponse, error) {
	p, err := s.dir.Person(ctx, req.GetPersonId())
	if errors.Is(err, people.ErrNotFound) {
		return nil, status.Errorf(codes.NotFound, "person %q not found", req.GetPersonId())
	}
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	return &peoplev1.GetPersonResponse{Person: toPerson(p)}, nil
}

func (s *Service) ListTeams(context.Context, *peoplev1.ListTeamsRequest) (*peoplev1.ListTeamsResponse, error) {
	out := &peoplev1.ListTeamsResponse{}
	for _, t := range s.teams {
		out.Teams = append(out.Teams, &peoplev1.Team{
			Id: t.ID, Name: t.Name, Unit: t.Unit, Cluster: t.Cluster, MattermostChannel: t.MattermostChannel,
		})
	}
	return out, nil
}

func (s *Service) GetRelations(ctx context.Context, req *peoplev1.GetRelationsRequest) (*peoplev1.GetRelationsResponse, error) {
	r, err := s.dir.Relations(ctx, req.GetUserId())
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	return &peoplev1.GetRelationsResponse{Relations: &peoplev1.Relations{
		MentorId: r.MentorID, ManagerId: r.ManagerID, HrBpId: r.HRBPID,
	}}, nil
}

func toPerson(p domain.Person) *peoplev1.Person {
	return &peoplev1.Person{
		Id: p.ID, UserId: p.UserID, Name: p.Name, RoleTitle: p.RoleTitle, TeamId: p.TeamID,
		Responsibilities: p.Responsibilities, AskMeAbout: p.AskMeAbout, Contact: p.Contact,
	}
}
