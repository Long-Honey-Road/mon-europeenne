package httpapi

import (
	"context"
	"log/slog"
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	buddyv1 "github.com/Long-Honey-Road/mon-europeenne/backend/gen/buddy/v1"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/auth"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/domain"
)

// Hiring creates accounts in the company's account system (the auth-roles mock).
type Hiring interface {
	Hire(ctx context.Context, requesterID string, h auth.Hire) (domain.User, string, error)
}

// Users keeps buddy's own row of every account that signs in.
type Users interface {
	EnsureUser(ctx context.Context, u domain.User) error
}

// forgetter is a directory with a cache that can be dropped after a change.
type forgetter interface{ Forget() }

type teamService struct {
	buddyv1.UnimplementedTeamServiceServer
	deps Deps
}

func (s *teamService) AddIntern(ctx context.Context, req *buddyv1.AddInternRequest) (*buddyv1.AddInternResponse, error) {
	p, err := principal(ctx)
	if err != nil {
		return nil, err
	}
	if p.Role != domain.RoleManager {
		return nil, status.Error(codes.PermissionDenied, "only a team manager may add interns")
	}
	if s.deps.Hiring == nil {
		return nil, status.Error(codes.Unimplemented, "no account system configured")
	}
	direction, ok := directionOf(req.GetDirection())
	switch {
	case strings.TrimSpace(req.GetDisplayName()) == "":
		return nil, status.Error(codes.InvalidArgument, "display_name is required")
	case !ok:
		return nil, status.Error(codes.InvalidArgument, "direction is required")
	case req.GetMentorId() == "":
		return nil, status.Error(codes.InvalidArgument, "mentor_id is required")
	}

	user, password, err := s.deps.Hiring.Hire(ctx, p.UserID, auth.Hire{
		DisplayName: strings.TrimSpace(req.GetDisplayName()), Login: strings.TrimSpace(req.GetLogin()),
		Direction: direction, MentorID: req.GetMentorId(), StartDate: strings.TrimSpace(req.GetStartDate()),
	})
	if err != nil {
		return nil, hiringError(err)
	}
	if f, ok := s.deps.Directory.(forgetter); ok {
		f.Forget()
	}
	// The intern's row is also made at the first sign-in; doing it now only saves that step.
	if s.deps.Users != nil {
		if err := s.deps.Users.EnsureUser(ctx, user); err != nil {
			slog.Warn("ensure hired user", "user", user.ID, "err", err)
		}
	}
	return &buddyv1.AddInternResponse{User: userProto(user), Login: user.Login, TemporaryPassword: password}, nil
}

func directionOf(d buddyv1.Direction) (domain.Direction, bool) {
	for k, v := range directions {
		if v == d {
			return k, true
		}
	}
	return 0, false
}

// hiringError passes the account system's refusals through and hides everything else behind UNAVAILABLE.
func hiringError(err error) error {
	switch c := status.Code(err); c {
	case codes.InvalidArgument, codes.AlreadyExists, codes.PermissionDenied:
		return status.Error(c, status.Convert(err).Message())
	default:
		slog.Warn("hire intern", "err", err)
		return status.Error(codes.Unavailable, "account service is unavailable, try again")
	}
}
