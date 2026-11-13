package tools

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/domain"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/people"
)

func listTeam(t *testing.T, dir PeopleDirectory, args string) (teamMembersOut, error) {
	t.Helper()
	tool, err := TeamMembers(dir)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := newRegistry(t, tool).Call(context.Background(), intern, domain.ThreadGeneral, "team_members", json.RawMessage(args))
	if err != nil {
		return teamMembersOut{}, err
	}
	var out teamMembersOut
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	return out, nil
}

func names(ms []TeamMember) []string {
	var out []string
	for _, m := range ms {
		out = append(out, m.Name+"/"+m.Relation)
	}
	return out
}

func TestTeamMembersOfOwnTeamWithRelations(t *testing.T) {
	out, err := listTeam(t, testDirectory(), `{}`)
	if err != nil {
		t.Fatal(err)
	}
	if out.Team.Name != "Messenger" || len(out.Members) != 3 {
		t.Fatalf("out = %+v", out)
	}
	if got := names(out.Members); got[0] != "Аня/наставник" || got[1] != "Дима/руководитель" || got[2] != "Ева/HR BP" {
		t.Fatalf("members = %v", got)
	}
	if out.Members[0].Contact != "@anna" || out.Members[0].AskMeAbout == nil || out.Members[2].Responsibilities == nil {
		t.Fatalf("member = %+v", out.Members[0])
	}
}

func TestTeamMembersOfOtherTeams(t *testing.T) {
	infra, err := listTeam(t, testDirectory(), `{"team_id":"infra"}`)
	if err != nil || len(infra.Members) != 1 || infra.Members[0].Name != "Вика" || !infra.Members[0].OtherTeam ||
		infra.Members[0].Relation != "" || infra.Members[0].Contact != "@vika" {
		t.Fatalf("infra = %+v %v", infra, err)
	}
	ads, err := listTeam(t, testDirectory(), `{"team_id":"t-ads"}`)
	if err != nil || len(ads.Members) != 1 || ads.Members[0].Contact != "" {
		t.Fatalf("other cluster hides contacts: %+v %v", ads, err)
	}
	if _, err := listTeam(t, testDirectory(), `{"team_id":"nope"}`); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown team: %v", err)
	}
}

func TestTeamMembersIncludeRelationsFromOtherTeams(t *testing.T) {
	dir := people.NewMemory(
		[]domain.Team{{ID: "t-msg", Name: "Messenger", Cluster: "c"}, {ID: "t-hr", Name: "HR", Cluster: "people"}},
		[]domain.Person{
			{ID: "p-intern", UserID: "u-intern", Name: "Стажёр", TeamID: "t-msg"},
			{ID: "p-hr", Name: "Лена", RoleTitle: "HR BP", TeamID: "t-hr", Contact: "@lena"},
			{ID: "p-other-hr", Name: "Зоя", TeamID: "t-hr"},
		},
		map[string]people.Relations{"u-intern": {HRBPID: "p-hr"}},
	)
	out, err := listTeam(t, dir, `{}`)
	if err != nil || len(out.Members) != 1 || out.Members[0].Name != "Лена" || !out.Members[0].OtherTeam || out.Members[0].Contact != "" {
		t.Fatalf("out = %+v %v", out, err)
	}
}

func TestTeamMembersReportsDirectoryFailure(t *testing.T) {
	for _, broken := range []string{"people", "team", "relations"} {
		t.Run(broken, func(t *testing.T) {
			_, err := listTeam(t, failingDirectory{PeopleDirectory: testDirectory(), broken: broken}, `{}`)
			if !errors.Is(err, errDirectoryDown) {
				t.Fatalf("error = %v", err)
			}
		})
	}
}
