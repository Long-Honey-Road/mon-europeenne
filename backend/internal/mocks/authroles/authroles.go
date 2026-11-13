// Package authroles is the auth-roles mock: test accounts with roles and attributes from seed.
package authroles

import (
	"context"
	"crypto/subtle"
	"fmt"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	authrolesv1 "github.com/Long-Honey-Road/mon-europeenne/backend/gen/mocks/authroles/v1"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/auth"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/domain"
)

// Service keeps test passwords in plain text: they are published in the README, there is nothing to protect.
type Service struct {
	authrolesv1.UnimplementedAuthRolesServiceServer
	byLogin map[string]auth.Credential
	byID    map[string]domain.User
}

func New(creds []auth.Credential) (*Service, error) {
	s := &Service{byLogin: map[string]auth.Credential{}, byID: map[string]domain.User{}}
	for _, c := range creds {
		if _, dup := s.byLogin[c.User.Login]; dup || c.User.Login == "" || c.User.ID == "" {
			return nil, fmt.Errorf("account %q: empty or duplicate login %q", c.User.ID, c.User.Login)
		}
		s.byLogin[c.User.Login] = c
		s.byID[c.User.ID] = c.User
	}
	return s, nil
}

func (s *Service) Authenticate(_ context.Context, req *authrolesv1.AuthenticateRequest) (*authrolesv1.AuthenticateResponse, error) {
	c, ok := s.byLogin[req.GetLogin()]
	if subtle.ConstantTimeCompare([]byte(c.Password), []byte(req.GetPassword())) != 1 || !ok || c.Password == "" {
		return nil, status.Error(codes.Unauthenticated, "invalid login or password")
	}
	return &authrolesv1.AuthenticateResponse{Account: toAccount(c.User)}, nil
}

func (s *Service) GetAccount(_ context.Context, req *authrolesv1.GetAccountRequest) (*authrolesv1.GetAccountResponse, error) {
	u, ok := s.byID[req.GetUserId()]
	if !ok {
		return nil, status.Errorf(codes.NotFound, "account %q not found", req.GetUserId())
	}
	return &authrolesv1.GetAccountResponse{Account: toAccount(u)}, nil
}

func toAccount(u domain.User) *authrolesv1.Account {
	a := &authrolesv1.Account{
		UserId: u.ID, Login: u.Login, DisplayName: u.DisplayName, Role: u.Role.String(), TeamId: u.TeamID,
		ManagerId: u.ManagerID, MentorId: u.MentorID, Repos: u.Repos, JiraProjects: u.JiraProjects,
		Location: u.Location,
	}
	if u.Direction != 0 {
		a.Direction = u.Direction.String()
	}
	if !u.StartDate.IsZero() {
		a.StartDate = u.StartDate.Format(time.DateOnly)
	}
	return a
}
