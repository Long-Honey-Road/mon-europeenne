// Package seed holds the synthetic data set and loads it with strict validation.
package seed

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"strings"
	"time"

	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/auth"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/domain"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/people"
)

var ErrInvalid = errors.New("invalid seed data")

type Team struct {
	ID                string `json:"id"`
	Name              string `json:"name"`
	Unit              string `json:"unit,omitempty"`
	Cluster           string `json:"cluster"`
	MattermostChannel string `json:"mattermost_channel,omitempty"`
}

type Account struct {
	UserID       string   `json:"user_id"`
	Login        string   `json:"login"`
	Password     string   `json:"password"`
	Role         string   `json:"role"`
	Direction    string   `json:"direction,omitempty"`
	MentorID     string   `json:"mentor_id,omitempty"`
	ManagerID    string   `json:"manager_id,omitempty"`
	HRBPID       string   `json:"hr_bp_id,omitempty"`
	Repos        []string `json:"repos,omitempty"`
	JiraProjects []string `json:"jira_projects,omitempty"`
	Location     string   `json:"location,omitempty"`
	StartDate    string   `json:"start_date,omitempty"`
}

type Person struct {
	ID               string   `json:"id"`
	Name             string   `json:"name"`
	RoleTitle        string   `json:"role_title,omitempty"`
	TeamID           string   `json:"team_id"`
	Responsibilities []string `json:"responsibilities,omitempty"`
	AskMeAbout       []string `json:"ask_me_about,omitempty"`
	Contact          string   `json:"contact,omitempty"`
	ApproverFor      []string `json:"approver_for,omitempty"`
	Account          *Account `json:"account,omitempty"`
}

type PeopleFile struct {
	Teams  []Team   `json:"teams"`
	People []Person `json:"people"`
}

type Approval struct {
	Resolver string `json:"resolver"`
	Object   string `json:"object,omitempty"`
}

type Demo struct {
	Outcome      string `json:"outcome"`
	AfterSeconds int    `json:"after_seconds"`
	Reason       string `json:"reason,omitempty"`
}

type RequestType struct {
	Key            string          `json:"key"`
	Title          string          `json:"title"`
	Category       string          `json:"category"`
	FieldsSchema   json.RawMessage `json:"fields_schema"`
	ApprovalChain  []Approval      `json:"approval_chain"`
	InstructionDoc string          `json:"instruction_doc,omitempty"`
	SLAHours       int             `json:"sla_hours,omitempty"`
	Demo           Demo            `json:"demo"`
}

type Object struct {
	Kind    string `json:"kind"`
	Key     string `json:"key"`
	Title   string `json:"title"`
	OwnerID string `json:"owner_id"`
}

type RequestsFile struct {
	Types   []RequestType `json:"types"`
	Objects []Object      `json:"objects"`
}

type MapPosition struct {
	X     float64 `json:"x"`
	Y     float64 `json:"y"`
	Group string  `json:"group,omitempty"`
}

type PlanItem struct {
	Key       string      `json:"key"`
	Title     string      `json:"title"`
	Practice  string      `json:"practice,omitempty"`
	Period    string      `json:"period"`
	Direction string      `json:"direction,omitempty"`
	TeamID    string      `json:"team_id,omitempty"`
	DependsOn []string    `json:"depends_on,omitempty"`
	Sources   []string    `json:"sources,omitempty"`
	Map       MapPosition `json:"map"`
}

type PlanStatus struct {
	UserID  string `json:"user_id"`
	ItemKey string `json:"item_key"`
	Status  string `json:"status"`
	Note    string `json:"note,omitempty"`
}

type PlanFile struct {
	Items    []PlanItem   `json:"items"`
	Statuses []PlanStatus `json:"statuses"`
}

type Data struct {
	People   PeopleFile
	Requests RequestsFile
	Plan     PlanFile
}

// Load reads people.json, request_types.json and plan.json from fsys and checks them as a whole.
func Load(fsys fs.FS) (Data, error) {
	var d Data
	for name, dst := range map[string]any{
		"people.json":        &d.People,
		"request_types.json": &d.Requests,
		"plan.json":          &d.Plan,
	} {
		if err := decodeFile(fsys, name, dst); err != nil {
			return Data{}, err
		}
	}
	if p := validate(d); len(p) > 0 {
		return Data{}, fmt.Errorf("%w:\n  %s", ErrInvalid, strings.Join(p, "\n  "))
	}
	return d, nil
}

func decodeFile(fsys fs.FS, name string, dst any) error {
	raw, err := fs.ReadFile(fsys, name)
	if err != nil {
		return fmt.Errorf("read %s: %w", name, err)
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return fmt.Errorf("%w: %s: %w", ErrInvalid, name, err)
	}
	return nil
}

// Directory serves the seed people as the in-memory directory used by tools.
func (d Data) Directory() *people.Memory {
	teams := d.Teams()
	persons := make([]domain.Person, 0, len(d.People.People))
	relations := map[string]people.Relations{}
	for _, p := range d.People.People {
		person := domain.Person{
			ID: p.ID, Name: p.Name, RoleTitle: p.RoleTitle, TeamID: p.TeamID,
			Responsibilities: p.Responsibilities, AskMeAbout: p.AskMeAbout, Contact: p.Contact,
		}
		if a := p.Account; a != nil {
			person.UserID = a.UserID
			relations[a.UserID] = people.Relations{MentorID: a.MentorID, ManagerID: a.ManagerID, HRBPID: a.HRBPID}
		}
		persons = append(persons, person)
	}
	return people.NewMemory(teams, persons, relations)
}

// Credentials returns the test accounts with their users; Load has already validated roles and dates.
func (d Data) Credentials() []auth.Credential {
	var out []auth.Credential
	for _, p := range d.People.People {
		a := p.Account
		if a == nil {
			continue
		}
		role, _ := domain.ParseRole(a.Role)
		u := domain.User{
			ID: a.UserID, Login: a.Login, DisplayName: p.Name, Role: role, TeamID: p.TeamID,
			ManagerID: a.ManagerID, MentorID: a.MentorID,
			Repos: a.Repos, JiraProjects: a.JiraProjects, Location: a.Location,
		}
		if a.Direction != "" {
			u.Direction, _ = domain.ParseDirection(a.Direction)
		}
		if a.StartDate != "" {
			u.StartDate, _ = time.Parse(time.DateOnly, a.StartDate)
		}
		out = append(out, auth.Credential{User: u, Password: a.Password})
	}
	return out
}

// Teams returns the seed teams as domain values.
func (d Data) Teams() []domain.Team {
	out := make([]domain.Team, 0, len(d.People.Teams))
	for _, t := range d.People.Teams {
		out = append(out, domain.Team{ID: t.ID, Name: t.Name, Unit: t.Unit, Cluster: t.Cluster, MattermostChannel: t.MattermostChannel})
	}
	return out
}

// PlanItems returns the checklist as domain values; Load has already validated periods and directions.
func (d Data) PlanItems() []domain.PlanItem {
	out := make([]domain.PlanItem, 0, len(d.Plan.Items))
	for _, it := range d.Plan.Items {
		period, _ := domain.ParsePlanPeriod(it.Period)
		item := domain.PlanItem{
			Key: it.Key, Title: it.Title, Practice: it.Practice, Period: period, TeamID: it.TeamID,
			DependsOn: it.DependsOn, Sources: it.Sources,
			Map: domain.MapPosition{X: it.Map.X, Y: it.Map.Y, Group: it.Map.Group},
		}
		if it.Direction != "" {
			item.Direction, _ = domain.ParseDirection(it.Direction)
		}
		out = append(out, item)
	}
	return out
}

// PlanStatuses returns the demo progress; ItemID holds the item key.
func (d Data) PlanStatuses() []domain.PlanStatus {
	out := make([]domain.PlanStatus, 0, len(d.Plan.Statuses))
	for _, s := range d.Plan.Statuses {
		status, _ := domain.ParsePlanItemStatus(s.Status)
		out = append(out, domain.PlanStatus{UserID: s.UserID, ItemID: s.ItemKey, Status: status, Note: s.Note})
	}
	return out
}
