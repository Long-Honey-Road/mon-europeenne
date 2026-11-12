package tools

import (
	"context"
	"errors"
	"sort"
	"strings"
	"unicode"

	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/domain"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/people"
)

const (
	maxHints = 3
	// Comparing word prefixes survives Russian case endings ("сборка", "сборки") without a stemmer.
	stemRunes    = 5
	minWordRunes = 3
)

// PeopleDirectory is what who_to_ask needs from the employee directory.
type PeopleDirectory interface {
	People(ctx context.Context) ([]domain.Person, error)
	Person(ctx context.Context, id string) (domain.Person, error)
	Team(ctx context.Context, id string) (domain.Team, error)
	Relations(ctx context.Context, userID string) (people.Relations, error)
}

type whoToAskIn struct {
	Topic  string `json:"topic" jsonschema:"what the person needs help with, as key nouns, e.g. деплой на staging"`
	TeamID string `json:"team_id,omitempty" jsonschema:"only people of this team; omit to search everyone"`
}

type PersonHint struct {
	PersonID  string `json:"person_id"`
	Name      string `json:"name"`
	RoleTitle string `json:"role_title"`
	// Reason is the directory entry that matched the topic.
	Reason string `json:"reason"`
	// Contact is shown only within the caller's cluster.
	Contact   string `json:"contact,omitempty"`
	OtherTeam bool   `json:"other_team,omitempty"`
}

type EscalationStep struct {
	Level    int    `json:"level"`
	Role     string `json:"role"`
	PersonID string `json:"person_id,omitempty"`
	Name     string `json:"name,omitempty"`
}

type whoToAskOut struct {
	People     []PersonHint     `json:"people"`
	Escalation []EscalationStep `json:"escalation"`
}

var allRoles = []domain.Role{
	domain.RoleIntern, domain.RoleMentor, domain.RoleManager, domain.RoleHRBP, domain.RoleHRAdmin,
}

var allThreads = []domain.ThreadType{
	domain.ThreadGeneral, domain.ThreadPlan, domain.ThreadRequest, domain.ThreadTask,
}

// WhoToAsk suggests people for a topic and always returns the escalation chain of the caller.
func WhoToAsk(dir PeopleDirectory) (Tool, error) {
	w := whoToAsk{dir: dir}
	return Define("who_to_ask",
		"Find colleagues who can help with a topic, plus the escalation chain: mentor, manager, HR BP, open sources.",
		allRoles, allThreads, w.run)
}

type whoToAsk struct {
	dir PeopleDirectory
}

type candidate struct {
	hint  PersonHint
	score int
}

func (w whoToAsk) run(ctx context.Context, p domain.Principal, in whoToAskIn) (whoToAskOut, error) {
	everyone, err := w.dir.People(ctx)
	if err != nil {
		return whoToAskOut{}, err
	}
	clusters := clusterCache{dir: w.dir}
	ownCluster, err := clusters.of(ctx, p.TeamID)
	if err != nil {
		return whoToAskOut{}, err
	}

	topic := stems(in.Topic)
	var found []candidate
	for _, person := range everyone {
		if person.UserID != "" && person.UserID == p.UserID {
			continue
		}
		if in.TeamID != "" && person.TeamID != in.TeamID {
			continue
		}
		score, reason := match(topic, person)
		if score == 0 {
			continue
		}
		hint := PersonHint{
			PersonID:  person.ID,
			Name:      person.Name,
			RoleTitle: person.RoleTitle,
			Reason:    reason,
			OtherTeam: person.TeamID != p.TeamID,
		}
		cluster, err := clusters.of(ctx, person.TeamID)
		if err != nil {
			return whoToAskOut{}, err
		}
		if ownCluster != "" && cluster == ownCluster {
			hint.Contact = person.Contact
		}
		found = append(found, candidate{hint: hint, score: score})
	}

	sort.SliceStable(found, func(i, j int) bool {
		a, b := found[i], found[j]
		if a.score != b.score {
			return a.score > b.score
		}
		if a.hint.OtherTeam != b.hint.OtherTeam {
			return !a.hint.OtherTeam
		}
		return a.hint.Name < b.hint.Name
	})

	out := whoToAskOut{People: []PersonHint{}}
	for i := 0; i < len(found) && i < maxHints; i++ {
		out.People = append(out.People, found[i].hint)
	}
	out.Escalation, err = EscalationChain(ctx, w.dir, p)
	return out, err
}

// EscalationChain lists whom the user should ask, in the order agreed at the Q&A: mentor, manager, HR BP, then open sources.
func EscalationChain(ctx context.Context, dir PeopleDirectory, p domain.Principal) ([]EscalationStep, error) {
	rel, err := dir.Relations(ctx, p.UserID)
	if err != nil {
		return nil, err
	}
	var steps []EscalationStep
	for _, link := range []struct{ role, id string }{
		{"mentor", rel.MentorID}, {"manager", rel.ManagerID}, {"hr_bp", rel.HRBPID},
	} {
		if link.id == "" {
			continue
		}
		person, err := dir.Person(ctx, link.id)
		if errors.Is(err, people.ErrNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		steps = append(steps, EscalationStep{Level: len(steps) + 1, Role: link.role, PersonID: person.ID, Name: person.Name})
	}
	return append(steps, EscalationStep{Level: len(steps) + 1, Role: "open_sources"}), nil
}

// match counts distinct topic stems found in the person's entries and returns the best entry.
func match(topic map[string]bool, person domain.Person) (int, string) {
	matched := map[string]bool{}
	best, bestScore := "", 0
	for _, entry := range append(append([]string{}, person.AskMeAbout...), person.Responsibilities...) {
		score := 0
		for s := range stems(entry) {
			if topic[s] {
				matched[s] = true
				score++
			}
		}
		if score > bestScore {
			best, bestScore = entry, score
		}
	}
	return len(matched), best
}

func stems(text string) map[string]bool {
	out := map[string]bool{}
	for _, word := range strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	}) {
		runes := []rune(word)
		if len(runes) < minWordRunes {
			continue
		}
		out[string(runes[:min(len(runes), stemRunes)])] = true
	}
	return out
}

// clusterCache resolves team clusters once per call; unknown teams have no cluster.
type clusterCache struct {
	dir  PeopleDirectory
	seen map[string]string
}

func (c *clusterCache) of(ctx context.Context, teamID string) (string, error) {
	if teamID == "" {
		return "", nil
	}
	if cluster, ok := c.seen[teamID]; ok {
		return cluster, nil
	}
	team, err := c.dir.Team(ctx, teamID)
	if err != nil && !errors.Is(err, people.ErrNotFound) {
		return "", err
	}
	if c.seen == nil {
		c.seen = map[string]string{}
	}
	c.seen[teamID] = team.Cluster
	return team.Cluster, nil
}
