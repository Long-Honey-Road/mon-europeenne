package httpapi

import (
	"context"
	"errors"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	buddyv1 "github.com/Long-Honey-Road/mon-europeenne/backend/gen/buddy/v1"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/auth"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/domain"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/people"
)

type authService struct {
	buddyv1.UnimplementedAuthServiceServer
	deps Deps
}

func (s *authService) Login(ctx context.Context, req *buddyv1.LoginRequest) (*buddyv1.LoginResponse, error) {
	user, err := s.deps.Accounts.Authenticate(ctx, req.GetLogin(), req.GetPassword())
	if errors.Is(err, auth.ErrInvalidCredentials) {
		return nil, status.Error(codes.Unauthenticated, "invalid login or password")
	}
	if err != nil {
		return nil, status.Error(codes.Unavailable, "sign-in service is unavailable, try again")
	}
	return &buddyv1.LoginResponse{Token: s.deps.Tokens.Issue(user.ID), User: userProto(user)}, nil
}

type profileService struct {
	buddyv1.UnimplementedProfileServiceServer
	deps Deps
}

func (s *profileService) GetMe(ctx context.Context, _ *buddyv1.GetMeRequest) (*buddyv1.GetMeResponse, error) {
	p, ok := auth.PrincipalFrom(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "authentication required")
	}
	user, err := s.deps.Accounts.User(ctx, p.UserID)
	if err != nil {
		return nil, status.Error(codes.Unauthenticated, "authentication required")
	}

	resp := &buddyv1.GetMeResponse{User: userProto(user), Relations: &buddyv1.Relations{}}
	team, err := s.deps.Directory.Team(ctx, user.TeamID)
	switch {
	case err == nil:
		resp.Team = &buddyv1.Team{
			Id: team.ID, Name: team.Name, Unit: team.Unit, Cluster: team.Cluster, MattermostChannel: team.MattermostChannel,
		}
	case !errors.Is(err, people.ErrNotFound):
		return nil, status.Error(codes.Unavailable, "directory unavailable")
	}

	rel, err := s.deps.Directory.Relations(ctx, user.ID)
	if err != nil {
		return nil, status.Error(codes.Unavailable, "directory unavailable")
	}
	for _, link := range []struct {
		id  string
		dst **buddyv1.PersonRef
	}{{rel.MentorID, &resp.Relations.Mentor}, {rel.ManagerID, &resp.Relations.Manager}, {rel.HRBPID, &resp.Relations.HrBp}} {
		if link.id == "" {
			continue
		}
		person, err := s.deps.Directory.Person(ctx, link.id)
		if errors.Is(err, people.ErrNotFound) {
			continue
		}
		if err != nil {
			return nil, status.Error(codes.Unavailable, "directory unavailable")
		}
		*link.dst = &buddyv1.PersonRef{Id: person.ID, Name: person.Name, RoleTitle: person.RoleTitle}
	}
	return resp, nil
}

var roles = map[domain.Role]buddyv1.Role{
	domain.RoleIntern:  buddyv1.Role_ROLE_INTERN,
	domain.RoleMentor:  buddyv1.Role_ROLE_MENTOR,
	domain.RoleManager: buddyv1.Role_ROLE_MANAGER,
	domain.RoleHRBP:    buddyv1.Role_ROLE_HR_BP,
	domain.RoleHRAdmin: buddyv1.Role_ROLE_HR_ADMIN,
}

var directions = map[domain.Direction]buddyv1.Direction{
	domain.DirectionBackend:  buddyv1.Direction_DIRECTION_BACKEND,
	domain.DirectionFrontend: buddyv1.Direction_DIRECTION_FRONTEND,
	domain.DirectionQA:       buddyv1.Direction_DIRECTION_QA,
}

func userProto(u domain.User) *buddyv1.User {
	out := &buddyv1.User{
		Id: u.ID, Login: u.Login, DisplayName: u.DisplayName,
		Role: roles[u.Role], Direction: directions[u.Direction], TeamId: u.TeamID,
		Repos: u.Repos, JiraProjects: u.JiraProjects, Location: u.Location,
	}
	if !u.StartDate.IsZero() {
		out.StartDate = u.StartDate.Format(time.DateOnly)
	}
	return out
}
