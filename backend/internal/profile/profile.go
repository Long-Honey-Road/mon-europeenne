// Package profile builds the compact caller profile the agent sees in its prompt.
package profile

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/domain"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/people"
)

type Directory interface {
	Person(ctx context.Context, id string) (domain.Person, error)
	Team(ctx context.Context, id string) (domain.Team, error)
	Relations(ctx context.Context, userID string) (people.Relations, error)
}

type compact struct {
	Name         string   `json:"name,omitempty"`
	Role         string   `json:"role,omitempty"`
	Direction    string   `json:"direction,omitempty"`
	Team         string   `json:"team,omitempty"`
	Unit         string   `json:"unit,omitempty"`
	Cluster      string   `json:"cluster,omitempty"`
	Mentor       string   `json:"mentor,omitempty"`
	Manager      string   `json:"manager,omitempty"`
	HRBP         string   `json:"hr_bp,omitempty"`
	Repos        []string `json:"repos,omitempty"`
	JiraProjects []string `json:"jira_projects,omitempty"`
	StartDate    string   `json:"start_date,omitempty"`
}

// Build returns the profile as compact JSON; empty fields are left out to save tokens.
func Build(ctx context.Context, u domain.User, dir Directory) (string, error) {
	p := compact{Name: u.DisplayName, Repos: u.Repos, JiraProjects: u.JiraProjects}
	if u.Role != 0 {
		p.Role = u.Role.String()
	}
	if u.Direction != 0 {
		p.Direction = u.Direction.String()
	}
	if !u.StartDate.IsZero() {
		p.StartDate = u.StartDate.Format(time.DateOnly)
	}
	if u.TeamID != "" {
		team, err := dir.Team(ctx, u.TeamID)
		if err != nil && !errors.Is(err, people.ErrNotFound) {
			return "", err
		}
		p.Team, p.Unit, p.Cluster = team.Name, team.Unit, team.Cluster
	}
	rel, err := dir.Relations(ctx, u.ID)
	if err != nil {
		return "", err
	}
	for _, link := range []struct {
		id  string
		dst *string
	}{{rel.MentorID, &p.Mentor}, {rel.ManagerID, &p.Manager}, {rel.HRBPID, &p.HRBP}} {
		if link.id == "" {
			continue
		}
		person, err := dir.Person(ctx, link.id)
		if err != nil && !errors.Is(err, people.ErrNotFound) {
			return "", err
		}
		*link.dst = person.Name
	}
	raw, err := json.Marshal(p)
	return string(raw), err
}
