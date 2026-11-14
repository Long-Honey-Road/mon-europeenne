package auth_test

import (
	"errors"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/auth"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/domain"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/mocks"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/mocks/mockstest"
)

func TestRemote(t *testing.T) {
	conn, srv := mockstest.Start(t, mocks.Chaos{})
	r := auth.NewRemote(conn)
	ctx := t.Context()

	u, err := r.Authenticate(ctx, "intern", "intern-demo")
	if err != nil || u.ID != "u-liza" || u.Role != domain.RoleIntern || u.Direction != domain.DirectionBackend || u.StartDate.IsZero() {
		t.Fatalf("Authenticate = %+v, %v", u, err)
	}
	if _, err := r.Authenticate(ctx, "intern", "wrong"); !errors.Is(err, auth.ErrInvalidCredentials) {
		t.Errorf("wrong password: %v", err)
	}
	if u, err := r.User(ctx, "u-anna"); err != nil || u.Role != domain.RoleMentor {
		t.Errorf("User(u-anna) = %+v, %v", u, err)
	}
	if _, err := r.User(ctx, "u-ghost"); !errors.Is(err, auth.ErrUnknownUser) {
		t.Errorf("unknown user: %v", err)
	}

	// The service goes down: known accounts still resolve, unknown ones and sign-in fail without ErrInvalidCredentials.
	srv.GRPC.Stop()
	if u, err := r.User(ctx, "u-liza"); err != nil || u.Login != "intern" {
		t.Errorf("User during outage = %+v, %v", u, err)
	}
	if _, err := r.User(ctx, "u-dmitry"); err == nil || errors.Is(err, auth.ErrUnknownUser) {
		t.Errorf("unseen user during outage: %v", err)
	}
	if _, err := r.Authenticate(ctx, "intern", "intern-demo"); err == nil || errors.Is(err, auth.ErrInvalidCredentials) {
		t.Errorf("sign-in during outage: %v", err)
	}
}

func TestRemoteHire(t *testing.T) {
	conn, _ := mockstest.Start(t, mocks.Chaos{})
	r := auth.NewRemote(conn)
	ctx := t.Context()

	u, pw, err := r.Hire(ctx, "u-dmitry", auth.Hire{
		DisplayName: "Вера Новикова", Login: "vnovikova", Direction: domain.DirectionQA, MentorID: "p-anna",
	})
	if err != nil || u.Role != domain.RoleIntern || u.Direction != domain.DirectionQA || u.TeamID != "t-messenger" || len(pw) != 12 {
		t.Fatalf("Hire = %+v, %q, %v", u, pw, err)
	}
	if signed, err := r.Authenticate(ctx, "vnovikova", pw); err != nil || signed.ID != u.ID {
		t.Errorf("sign-in of the hired intern = %+v, %v", signed, err)
	}
	_, _, err = r.Hire(ctx, "u-liza", auth.Hire{DisplayName: "X", Login: "xxx", Direction: domain.DirectionQA, MentorID: "p-anna"})
	if status.Code(err) != codes.PermissionDenied {
		t.Errorf("intern hiring: %v, want PERMISSION_DENIED as is", err)
	}
}
