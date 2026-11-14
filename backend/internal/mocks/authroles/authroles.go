// Package authroles is the auth-roles mock: accounts with roles and attributes from seed, plus interns that
// managers hire through CreateAccount.
package authroles

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	authrolesv1 "github.com/Long-Honey-Road/mon-europeenne/backend/gen/mocks/authroles/v1"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/auth"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/domain"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/people"
)

var loginRe = regexp.MustCompile(`^[a-z][a-z0-9._-]{2,31}$`)

// No look-alikes (0/O, 1/l/I), so a password read aloud or copied by hand survives.
const passwordAlphabet = "abcdefghijkmnpqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789"

// Service keeps seed passwords in plain text: they are published in the README. Passwords of hired interns are
// generated, shown once and kept only as bcrypt hashes.
type Service struct {
	authrolesv1.UnimplementedAuthRolesServiceServer

	dir   *people.Memory
	state string
	now   func() time.Time

	mu      sync.RWMutex
	byLogin map[string]auth.Credential
	hashes  map[string][]byte
	byID    map[string]domain.User
	hired   []hire
}

// hire is one created account as the state file keeps it.
type hire struct {
	User      domain.User      `json:"user"`
	Hash      []byte           `json:"password_hash"`
	Person    domain.Person    `json:"person"`
	Relations people.Relations `json:"relations"`
}

// New serves the seed accounts; with a state file it also restores interns hired before a restart and saves
// every new one there.
func New(creds []auth.Credential, dir *people.Memory, stateFile string) (*Service, error) {
	s := &Service{
		dir: dir, state: stateFile, now: time.Now,
		byLogin: map[string]auth.Credential{}, hashes: map[string][]byte{}, byID: map[string]domain.User{},
	}
	for _, c := range creds {
		if _, dup := s.byLogin[c.User.Login]; dup || c.User.Login == "" || c.User.ID == "" {
			return nil, fmt.Errorf("account %q: empty or duplicate login %q", c.User.ID, c.User.Login)
		}
		s.byLogin[c.User.Login] = c
		s.byID[c.User.ID] = c.User
	}
	if err := s.restore(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Service) Authenticate(_ context.Context, req *authrolesv1.AuthenticateRequest) (*authrolesv1.AuthenticateResponse, error) {
	s.mu.RLock()
	c, ok := s.byLogin[req.GetLogin()]
	hash := s.hashes[req.GetLogin()]
	s.mu.RUnlock()
	if hash != nil {
		if bcrypt.CompareHashAndPassword(hash, []byte(req.GetPassword())) != nil {
			return nil, status.Error(codes.Unauthenticated, "invalid login or password")
		}
		return &authrolesv1.AuthenticateResponse{Account: toAccount(c.User)}, nil
	}
	if subtle.ConstantTimeCompare([]byte(c.Password), []byte(req.GetPassword())) != 1 || !ok || c.Password == "" {
		return nil, status.Error(codes.Unauthenticated, "invalid login or password")
	}
	return &authrolesv1.AuthenticateResponse{Account: toAccount(c.User)}, nil
}

func (s *Service) GetAccount(_ context.Context, req *authrolesv1.GetAccountRequest) (*authrolesv1.GetAccountResponse, error) {
	s.mu.RLock()
	u, ok := s.byID[req.GetUserId()]
	s.mu.RUnlock()
	if !ok {
		return nil, status.Errorf(codes.NotFound, "account %q not found", req.GetUserId())
	}
	return &authrolesv1.GetAccountResponse{Account: toAccount(u)}, nil
}

func (s *Service) CreateAccount(ctx context.Context, req *authrolesv1.CreateAccountRequest) (*authrolesv1.CreateAccountResponse, error) {
	s.mu.RLock()
	requester, ok := s.byID[req.GetRequesterId()]
	s.mu.RUnlock()
	if !ok || requester.Role != domain.RoleManager || requester.TeamID == "" {
		return nil, status.Error(codes.PermissionDenied, "only a team manager may hire interns")
	}
	h, err := s.newHire(ctx, requester, req)
	if err != nil {
		return nil, err
	}
	password, err := newPassword()
	if err != nil {
		return nil, status.Error(codes.Internal, "password generation failed")
	}
	if h.Hash, err = bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost); err != nil {
		return nil, status.Error(codes.Internal, "password hashing failed")
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if _, taken := s.byLogin[h.User.Login]; taken {
		return nil, status.Errorf(codes.AlreadyExists, "login %q is taken", h.User.Login)
	}
	if _, taken := s.byID[h.User.ID]; taken {
		return nil, status.Errorf(codes.AlreadyExists, "login %q is taken", h.User.Login)
	}
	if err := s.dir.Add(h.Person, h.Relations); err != nil {
		return nil, status.Errorf(codes.AlreadyExists, "login %q is taken", h.User.Login)
	}
	s.keep(h)
	if err := s.save(); err != nil {
		return nil, status.Error(codes.Internal, "account saved in memory only: "+err.Error())
	}
	return &authrolesv1.CreateAccountResponse{Account: toAccount(h.User), TemporaryPassword: password}, nil
}

// newHire checks the request against the requester's team and builds the account, directory entry and relations.
func (s *Service) newHire(ctx context.Context, requester domain.User, req *authrolesv1.CreateAccountRequest) (hire, error) {
	name := strings.TrimSpace(req.GetDisplayName())
	login := strings.TrimSpace(req.GetLogin())
	switch {
	case name == "":
		return hire{}, status.Error(codes.InvalidArgument, "display_name is required")
	case !loginRe.MatchString(login):
		return hire{}, status.Error(codes.InvalidArgument, "login: 3 to 32 characters, lowercase latin letters, digits, '.', '_' or '-', starting with a letter")
	}
	direction, err := domain.ParseDirection(req.GetDirection())
	if err != nil {
		return hire{}, status.Error(codes.InvalidArgument, "direction must be backend, frontend or qa")
	}
	mentor, err := s.dir.Person(ctx, req.GetMentorId())
	if err != nil || mentor.TeamID != requester.TeamID {
		return hire{}, status.Error(codes.InvalidArgument, "mentor must be a person of the manager's team")
	}
	start := s.now()
	if d := strings.TrimSpace(req.GetStartDate()); d != "" {
		if start, err = time.Parse(time.DateOnly, d); err != nil {
			return hire{}, status.Error(codes.InvalidArgument, "start_date must be YYYY-MM-DD")
		}
	}

	manager, _ := s.dir.PersonOfUser(requester.ID)
	managerRel, _ := s.dir.Relations(ctx, requester.ID)
	user := domain.User{
		ID: "u-" + login, Login: login, DisplayName: name, Role: domain.RoleIntern, Direction: direction,
		TeamID: requester.TeamID, ManagerID: manager.ID, MentorID: mentor.ID,
		Repos: requester.Repos, JiraProjects: requester.JiraProjects, Location: requester.Location,
		StartDate: time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, time.UTC),
	}
	return hire{
		User: user,
		Person: domain.Person{
			ID: "p-" + login, UserID: user.ID, Name: name, RoleTitle: "Стажёр, " + direction.String(),
			TeamID: requester.TeamID,
		},
		Relations: people.Relations{MentorID: mentor.ID, ManagerID: manager.ID, HRBPID: managerRel.HRBPID},
	}, nil
}

// keep registers a hire in memory; the caller holds the lock.
func (s *Service) keep(h hire) {
	s.byLogin[h.User.Login] = auth.Credential{User: h.User}
	s.hashes[h.User.Login] = h.Hash
	s.byID[h.User.ID] = h.User
	s.hired = append(s.hired, h)
}

func (s *Service) restore() error {
	if s.state == "" {
		return nil
	}
	raw, err := os.ReadFile(s.state)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("auth-roles state: %w", err)
	}
	var saved []hire
	if err := json.Unmarshal(raw, &saved); err != nil {
		return fmt.Errorf("auth-roles state %s: %w", s.state, err)
	}
	for _, h := range saved {
		if _, dup := s.byLogin[h.User.Login]; dup {
			continue
		}
		if err := s.dir.Add(h.Person, h.Relations); err != nil {
			return fmt.Errorf("auth-roles state: %w", err)
		}
		s.keep(h)
	}
	return nil
}

// save writes all hires atomically; the caller holds the lock.
func (s *Service) save() error {
	if s.state == "" {
		return nil
	}
	raw, err := json.MarshalIndent(s.hired, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(s.state), ".auth-roles-*")
	if err != nil {
		return err
	}
	if _, err := tmp.Write(raw); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmp.Name())
		return err
	}
	return os.Rename(tmp.Name(), s.state)
}

func newPassword() (string, error) {
	buf := make([]byte, 12)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	out := make([]byte, len(buf))
	for i, b := range buf {
		out[i] = passwordAlphabet[int(b)%len(passwordAlphabet)]
	}
	return string(out), nil
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
