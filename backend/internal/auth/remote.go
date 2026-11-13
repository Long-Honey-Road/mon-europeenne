package auth

import (
	"context"
	"fmt"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	authrolesv1 "github.com/Long-Honey-Road/mon-europeenne/backend/gen/mocks/authroles/v1"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/domain"
)

// Remote checks credentials and reads accounts from the auth-roles service. Accounts it has seen are kept, so a
// signed-in user keeps working while the service is down; sign-in itself needs the service.
type Remote struct {
	c authrolesv1.AuthRolesServiceClient

	mu   sync.Mutex
	seen map[string]domain.User
}

func NewRemote(conn grpc.ClientConnInterface) *Remote {
	return &Remote{c: authrolesv1.NewAuthRolesServiceClient(conn), seen: map[string]domain.User{}}
}

func (r *Remote) Authenticate(ctx context.Context, login, password string) (domain.User, error) {
	resp, err := r.c.Authenticate(ctx, &authrolesv1.AuthenticateRequest{Login: login, Password: password})
	if status.Code(err) == codes.Unauthenticated {
		return domain.User{}, ErrInvalidCredentials
	}
	if err != nil {
		return domain.User{}, fmt.Errorf("auth-roles: %w", err)
	}
	return r.keep(resp.GetAccount())
}

func (r *Remote) User(ctx context.Context, id string) (domain.User, error) {
	resp, err := r.c.GetAccount(ctx, &authrolesv1.GetAccountRequest{UserId: id})
	if status.Code(err) == codes.NotFound {
		return domain.User{}, fmt.Errorf("%w: %q", ErrUnknownUser, id)
	}
	if err != nil {
		r.mu.Lock()
		u, ok := r.seen[id]
		r.mu.Unlock()
		if ok {
			return u, nil
		}
		return domain.User{}, fmt.Errorf("auth-roles: %w", err)
	}
	return r.keep(resp.GetAccount())
}

func (r *Remote) keep(a *authrolesv1.Account) (domain.User, error) {
	u, err := userOf(a)
	if err != nil {
		return domain.User{}, err
	}
	r.mu.Lock()
	r.seen[u.ID] = u
	r.mu.Unlock()
	return u, nil
}

func userOf(a *authrolesv1.Account) (domain.User, error) {
	role, err := domain.ParseRole(a.GetRole())
	if err != nil {
		return domain.User{}, fmt.Errorf("account %q: role: %w", a.GetUserId(), err)
	}
	u := domain.User{
		ID: a.GetUserId(), Login: a.GetLogin(), DisplayName: a.GetDisplayName(), Role: role, TeamID: a.GetTeamId(),
		ManagerID: a.GetManagerId(), MentorID: a.GetMentorId(), Repos: a.GetRepos(), JiraProjects: a.GetJiraProjects(),
		Location: a.GetLocation(),
	}
	if a.GetDirection() != "" {
		if u.Direction, err = domain.ParseDirection(a.GetDirection()); err != nil {
			return domain.User{}, fmt.Errorf("account %q: direction: %w", a.GetUserId(), err)
		}
	}
	if a.GetStartDate() != "" {
		if u.StartDate, err = time.Parse(time.DateOnly, a.GetStartDate()); err != nil {
			return domain.User{}, fmt.Errorf("account %q: start date: %w", a.GetUserId(), err)
		}
	}
	return u, nil
}
