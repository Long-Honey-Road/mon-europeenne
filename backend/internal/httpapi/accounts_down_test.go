package httpapi

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/domain"
)

// downAccounts is the auth-roles service during an outage.
type downAccounts struct{}

func (downAccounts) Authenticate(context.Context, string, string) (domain.User, error) {
	return domain.User{}, errors.New("auth-roles: unavailable")
}

func (downAccounts) User(context.Context, string) (domain.User, error) {
	return domain.User{}, errors.New("auth-roles: unavailable")
}

func TestAccountServiceOutageIsNotASignOut(t *testing.T) {
	d := deps(t)
	token := d.Tokens.Issue("u-liza")
	d.Accounts = downAccounts{}
	s := newServerWith(t, d)

	if code, body := call(t, s, http.MethodPost, "/v1/public/auth/login", "", `{"login":"intern","password":"intern-demo"}`); code != http.StatusServiceUnavailable {
		t.Errorf("login during outage = %d %v, want 503", code, body)
	}
	if code, body := call(t, s, http.MethodGet, "/v1/private/me", token, ""); code != http.StatusServiceUnavailable {
		t.Errorf("private call during outage = %d %v, want 503, not 401", code, body)
	}
}
