// Package people is the employee directory; Memory serves it from seed data until the mock client lands.
package people

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/domain"
)

var ErrNotFound = errors.New("not found")

// Relations links a user to the people who support them; values are person ids.
type Relations struct {
	MentorID  string
	ManagerID string
	HRBPID    string
}

// Memory is a directory held in memory; the people mock adds hired interns to it.
type Memory struct {
	mu        sync.RWMutex
	people    []domain.Person
	byID      map[string]domain.Person
	teams     map[string]domain.Team
	relations map[string]Relations
}

func NewMemory(teams []domain.Team, people []domain.Person, relations map[string]Relations) *Memory {
	m := &Memory{
		people:    people,
		byID:      make(map[string]domain.Person, len(people)),
		teams:     make(map[string]domain.Team, len(teams)),
		relations: relations,
	}
	for _, p := range people {
		m.byID[p.ID] = p
	}
	for _, t := range teams {
		m.teams[t.ID] = t
	}
	return m
}

func (m *Memory) People(context.Context) ([]domain.Person, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return append([]domain.Person(nil), m.people...), nil
}

func (m *Memory) Person(_ context.Context, id string) (domain.Person, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	p, ok := m.byID[id]
	if !ok {
		return domain.Person{}, fmt.Errorf("person %q: %w", id, ErrNotFound)
	}
	return p, nil
}

func (m *Memory) Team(_ context.Context, id string) (domain.Team, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	t, ok := m.teams[id]
	if !ok {
		return domain.Team{}, fmt.Errorf("team %q: %w", id, ErrNotFound)
	}
	return t, nil
}

// Relations of a user without mentor, manager or HR BP are empty, not an error.
func (m *Memory) Relations(_ context.Context, userID string) (Relations, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.relations[userID], nil
}

// PersonOfUser finds the directory entry of an account.
func (m *Memory) PersonOfUser(userID string) (domain.Person, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, p := range m.people {
		if p.UserID != "" && p.UserID == userID {
			return p, true
		}
	}
	return domain.Person{}, false
}

// Add puts a new person with their relations into the directory; an existing id is an error.
func (m *Memory) Add(p domain.Person, rel Relations) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, dup := m.byID[p.ID]; dup {
		return fmt.Errorf("person %q already exists", p.ID)
	}
	m.people = append(m.people, p)
	m.byID[p.ID] = p
	if p.UserID != "" {
		if m.relations == nil {
			m.relations = map[string]Relations{}
		}
		m.relations[p.UserID] = rel
	}
	return nil
}
