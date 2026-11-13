// Package auth signs in test accounts and issues session tokens.
package auth

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/domain"
)

var (
	ErrInvalidCredentials = errors.New("invalid login or password")
	ErrUnknownUser        = errors.New("unknown user")
	ErrInvalidToken       = errors.New("invalid session token")
)

const minSecretBytes = 16

// Credential is a test account as it comes from seed data.
type Credential struct {
	User     domain.User
	Password string
}

type account struct {
	user domain.User
	hash []byte
}

// Directory checks credentials and looks accounts up: Accounts in memory, or Remote over the auth-roles service.
type Directory interface {
	// Authenticate returns ErrInvalidCredentials for both unknown login and wrong password.
	Authenticate(ctx context.Context, login, password string) (domain.User, error)
	// User returns ErrUnknownUser for an unknown id.
	User(ctx context.Context, id string) (domain.User, error)
}

// Accounts keeps test accounts in memory; the auth-roles mock serves them, and buddy uses it without MOCKS_ADDR.
type Accounts struct {
	byLogin map[string]account
	byID    map[string]domain.User
	// dummy is compared for unknown logins so both failures take the same time.
	dummy []byte
}

func NewAccounts(creds []Credential) (*Accounts, error) {
	a := &Accounts{byLogin: map[string]account{}, byID: map[string]domain.User{}}
	for _, c := range creds {
		if c.Password == "" || c.User.Login == "" || c.User.ID == "" {
			return nil, fmt.Errorf("account %q: id, login and password are required", c.User.ID)
		}
		if _, dup := a.byLogin[c.User.Login]; dup {
			return nil, fmt.Errorf("account %q: duplicate login %q", c.User.ID, c.User.Login)
		}
		hash, err := bcrypt.GenerateFromPassword([]byte(c.Password), bcrypt.DefaultCost)
		if err != nil {
			return nil, fmt.Errorf("account %q: hash password: %w", c.User.ID, err)
		}
		a.byLogin[c.User.Login] = account{user: c.User, hash: hash}
		a.byID[c.User.ID] = c.User
	}
	dummy, err := bcrypt.GenerateFromPassword([]byte("dummy"), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}
	a.dummy = dummy
	return a, nil
}

// Authenticate returns ErrInvalidCredentials for both unknown login and wrong password.
func (a *Accounts) Authenticate(_ context.Context, login, password string) (domain.User, error) {
	acc, ok := a.byLogin[login]
	hash := acc.hash
	if !ok {
		hash = a.dummy
	}
	if err := bcrypt.CompareHashAndPassword(hash, []byte(password)); err != nil || !ok {
		return domain.User{}, ErrInvalidCredentials
	}
	return acc.user, nil
}

func (a *Accounts) User(_ context.Context, id string) (domain.User, error) {
	u, ok := a.byID[id]
	if !ok {
		return domain.User{}, fmt.Errorf("%w: %q", ErrUnknownUser, id)
	}
	return u, nil
}

// Tokens issues stateless session tokens: base64url(payload) + "." + base64url(HMAC-SHA256).
type Tokens struct {
	secret []byte
	ttl    time.Duration
	now    func() time.Time
}

type payload struct {
	Sub string `json:"sub"`
	Exp int64  `json:"exp"`
}

func NewTokens(secret string, ttl time.Duration) (*Tokens, error) {
	if len(secret) < minSecretBytes {
		return nil, fmt.Errorf("session secret must be at least %d bytes", minSecretBytes)
	}
	return &Tokens{secret: []byte(secret), ttl: ttl, now: time.Now}, nil
}

func (t *Tokens) Issue(userID string) string {
	body, _ := json.Marshal(payload{Sub: userID, Exp: t.now().Add(t.ttl).Unix()}) // plain struct, cannot fail
	enc := base64.RawURLEncoding.EncodeToString(body)
	return enc + "." + base64.RawURLEncoding.EncodeToString(t.sign(enc))
}

// Verify returns the user id of a valid, unexpired token.
func (t *Tokens) Verify(token string) (string, error) {
	enc, sig, ok := strings.Cut(token, ".")
	if !ok {
		return "", ErrInvalidToken
	}
	got, err := base64.RawURLEncoding.DecodeString(sig)
	if err != nil || !hmac.Equal(got, t.sign(enc)) {
		return "", ErrInvalidToken
	}
	body, err := base64.RawURLEncoding.DecodeString(enc)
	if err != nil {
		return "", ErrInvalidToken
	}
	var p payload
	if err := json.Unmarshal(body, &p); err != nil || p.Sub == "" {
		return "", ErrInvalidToken
	}
	if t.now().Unix() >= p.Exp {
		return "", fmt.Errorf("%w: expired", ErrInvalidToken)
	}
	return p.Sub, nil
}

func (t *Tokens) sign(data string) []byte {
	m := hmac.New(sha256.New, t.secret)
	m.Write([]byte(data))
	return m.Sum(nil)
}

type principalKey struct{}

func WithPrincipal(ctx context.Context, p domain.Principal) context.Context {
	return context.WithValue(ctx, principalKey{}, p)
}

func PrincipalFrom(ctx context.Context) (domain.Principal, bool) {
	p, ok := ctx.Value(principalKey{}).(domain.Principal)
	return p, ok
}
