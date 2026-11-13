package auth_test

import (
	"errors"
	"testing"

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
