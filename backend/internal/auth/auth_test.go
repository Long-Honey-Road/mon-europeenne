package auth

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/domain"
)

var liza = domain.User{ID: "u-liza", Login: "intern", DisplayName: "Лиза", Role: domain.RoleIntern, TeamID: "t-messenger"}

func accounts(t *testing.T) *Accounts {
	t.Helper()
	a, err := NewAccounts([]Credential{{User: liza, Password: "intern-demo"}})
	if err != nil {
		t.Fatalf("NewAccounts: %v", err)
	}
	return a
}

func TestAuthenticateAcceptsRightPassword(t *testing.T) {
	u, err := accounts(t).Authenticate("intern", "intern-demo")
	if err != nil || u.ID != "u-liza" {
		t.Fatalf("Authenticate = %+v, %v", u, err)
	}
}

func TestAuthenticateHidesWhichPartIsWrong(t *testing.T) {
	a := accounts(t)
	for _, c := range [][2]string{{"intern", "wrong"}, {"nobody", "intern-demo"}, {"", ""}} {
		if _, err := a.Authenticate(c[0], c[1]); !errors.Is(err, ErrInvalidCredentials) {
			t.Errorf("Authenticate(%q, %q) error = %v, want ErrInvalidCredentials", c[0], c[1], err)
		}
	}
}

func TestUserByID(t *testing.T) {
	a := accounts(t)
	if u, err := a.User("u-liza"); err != nil || u.Login != "intern" {
		t.Fatalf("User(u-liza) = %+v, %v", u, err)
	}
	if _, err := a.User("u-ghost"); !errors.Is(err, ErrUnknownUser) {
		t.Fatalf("User(u-ghost) error = %v, want ErrUnknownUser", err)
	}
}

func TestNewAccountsRejectsBadCredentials(t *testing.T) {
	if _, err := NewAccounts([]Credential{{User: liza, Password: ""}}); err == nil {
		t.Error("accepted an empty password")
	}
	dup := []Credential{{User: liza, Password: "a"}, {User: domain.User{ID: "u-2", Login: "intern"}, Password: "b"}}
	if _, err := NewAccounts(dup); err == nil {
		t.Error("accepted a duplicate login")
	}
}

func clock(t time.Time) func() time.Time { return func() time.Time { return t } }

func TestTokenRoundTrip(t *testing.T) {
	tok, err := NewTokens("0123456789abcdef-secret", time.Hour)
	if err != nil {
		t.Fatalf("NewTokens: %v", err)
	}
	token := tok.Issue("u-liza")

	id, err := tok.Verify(token)
	if err != nil || id != "u-liza" {
		t.Fatalf("Verify = %q, %v", id, err)
	}
}

func TestTokenRejectsTamperingAndExpiry(t *testing.T) {
	now := time.Date(2026, 11, 12, 10, 0, 0, 0, time.UTC)
	tok, _ := NewTokens("0123456789abcdef-secret", time.Hour)
	tok.now = clock(now)
	token := tok.Issue("u-liza")
	other, _ := NewTokens("another-secret-of-16+", time.Hour)

	cases := map[string]func() (string, error){
		"malformed":       func() (string, error) { return tok.Verify("not-a-token") },
		"bad base64":      func() (string, error) { return tok.Verify("!!!.???") },
		"other secret":    func() (string, error) { return other.Verify(token) },
		"payload swapped": func() (string, error) { return tok.Verify(swapPayload(t, tok, token)) },
		"expired": func() (string, error) {
			tok.now = clock(now.Add(2 * time.Hour))
			defer func() { tok.now = clock(now) }()
			return tok.Verify(token)
		},
	}
	for name, verify := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := verify(); !errors.Is(err, ErrInvalidToken) {
				t.Fatalf("error = %v, want ErrInvalidToken", err)
			}
		})
	}
}

// swapPayload keeps the signature of token but puts another user's payload in front of it.
func swapPayload(t *testing.T, tok *Tokens, token string) string {
	t.Helper()
	forged := tok.Issue("u-dmitry")
	return strings.SplitN(forged, ".", 2)[0] + "." + strings.SplitN(token, ".", 2)[1]
}

func TestNewTokensNeedsStrongSecret(t *testing.T) {
	if _, err := NewTokens("short", time.Hour); err == nil {
		t.Fatal("accepted a 5-byte secret")
	}
}

func TestPrincipalTravelsInContext(t *testing.T) {
	p := domain.Principal{UserID: "u-liza", Role: domain.RoleIntern, TeamID: "t-messenger"}
	ctx := WithPrincipal(context.Background(), p)

	got, ok := PrincipalFrom(ctx)
	if !ok || got != p {
		t.Fatalf("PrincipalFrom = %+v, %v", got, ok)
	}
	if _, ok := PrincipalFrom(context.Background()); ok {
		t.Fatal("empty context has a principal")
	}
}
