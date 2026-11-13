package tools

import (
	"context"
	"sort"

	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/domain"
)

type teamMembersIn struct {
	TeamID string `json:"team_id,omitempty" jsonschema:"team id or name; empty means the user's own team"`
}

type TeamMember struct {
	PersonID  string `json:"person_id"`
	Name      string `json:"name"`
	RoleTitle string `json:"role_title"`
	// Relation is who the person is to the user: наставник, руководитель or HR BP.
	Relation         string   `json:"relation,omitempty"`
	Responsibilities []string `json:"responsibilities"`
	AskMeAbout       []string `json:"ask_me_about"`
	Contact          string   `json:"contact,omitempty"`
	OtherTeam        bool     `json:"other_team,omitempty"`
}

type teamInfo struct {
	TeamID  string `json:"team_id"`
	Name    string `json:"name"`
	Unit    string `json:"unit"`
	Channel string `json:"mattermost_channel,omitempty"`
}

type teamMembersOut struct {
	Team    teamInfo     `json:"team"`
	Members []TeamMember `json:"members"`
}

// TeamMembers lists a team: its people with roles and topics, plus, for the user's own team, the mentor, manager
// and HR BP even when they sit in another team.
func TeamMembers(dir PeopleDirectory) (Tool, error) {
	return Define("team_members",
		"Everyone in a team (the user's own by default) with role, what they answer for and who they are to the user. "+
			"Use it to describe the team; use who_to_ask to find a person for a topic.",
		allRoles, allThreads,
		func(ctx context.Context, p domain.Principal, in teamMembersIn) (teamMembersOut, error) {
			everyone, err := dir.People(ctx)
			if err != nil {
				return teamMembersOut{}, err
			}
			clusters := clusterCache{dir: dir}
			want := in.TeamID
			if want == "" {
				want = p.TeamID
			}
			var teamID string
			for _, person := range everyone {
				if ok, err := clusters.teamMatches(ctx, person.TeamID, want); err != nil {
					return teamMembersOut{}, err
				} else if ok {
					teamID = person.TeamID
					break
				}
			}
			if teamID == "" {
				return teamMembersOut{}, ErrNotFound
			}
			relations := map[string]string{}
			if teamID == p.TeamID {
				rel, err := dir.Relations(ctx, p.UserID)
				if err != nil {
					return teamMembersOut{}, err
				}
				for role, id := range map[string]string{"mentor": rel.MentorID, "manager": rel.ManagerID, "hr_bp": rel.HRBPID} {
					if id != "" {
						relations[id] = escalationTitles[role]
					}
				}
			}
			team, err := clusters.team(ctx, teamID)
			if err != nil {
				return teamMembersOut{}, err
			}
			ownCluster, err := clusters.of(ctx, p.TeamID)
			if err != nil {
				return teamMembersOut{}, err
			}

			out := teamMembersOut{
				Team:    teamInfo{TeamID: team.ID, Name: team.Name, Unit: team.Unit, Channel: team.MattermostChannel},
				Members: []TeamMember{},
			}
			for _, person := range everyone {
				if person.UserID != "" && person.UserID == p.UserID {
					continue
				}
				relation := relations[person.ID]
				if person.TeamID != teamID && relation == "" {
					continue
				}
				m := TeamMember{
					PersonID: person.ID, Name: person.Name, RoleTitle: person.RoleTitle, Relation: relation,
					Responsibilities: orEmpty(person.Responsibilities), AskMeAbout: orEmpty(person.AskMeAbout),
					OtherTeam: person.TeamID != p.TeamID,
				}
				cluster, err := clusters.of(ctx, person.TeamID)
				if err != nil {
					return teamMembersOut{}, err
				}
				if ownCluster != "" && cluster == ownCluster {
					m.Contact = person.Contact
				}
				out.Members = append(out.Members, m)
			}
			// People with a relation to the user first, then by name.
			sort.SliceStable(out.Members, func(i, j int) bool {
				a, b := out.Members[i], out.Members[j]
				if (a.Relation != "") != (b.Relation != "") {
					return a.Relation != ""
				}
				return a.Name < b.Name
			})
			return out, nil
		})
}

func orEmpty(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
