package tools

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"testing"

	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/domain"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/people"
)

// Synthetic team: Messenger and Infra share a cluster, Ads sits in another one.
func testDirectory() *people.Memory {
	return people.NewMemory(
		[]domain.Team{
			{ID: "t-msg", Name: "Messenger", Cluster: "communications"},
			{ID: "t-infra", Name: "Infra", Cluster: "communications"},
			{ID: "t-ads", Name: "Ads", Cluster: "monetization"},
		},
		[]domain.Person{
			{ID: "p-intern", UserID: "u-intern", Name: "Стажёр", TeamID: "t-msg", AskMeAbout: []string{"деплой на staging"}},
			{ID: "p-anna", Name: "Аня", RoleTitle: "Backend, наставник", TeamID: "t-msg", Contact: "@anna",
				AskMeAbout: []string{"сборка и локальный запуск сервиса", "ревью кода"}},
			{ID: "p-dima", Name: "Дима", RoleTitle: "Руководитель юнита", TeamID: "t-msg", Contact: "@dima",
				Responsibilities: []string{"приоритеты задач"}},
			{ID: "p-eva", Name: "Ева", RoleTitle: "HR BP", TeamID: "t-msg", Contact: "@eva"},
			{ID: "p-vika", Name: "Вика", RoleTitle: "Инфраструктура", TeamID: "t-infra", Contact: "@vika",
				AskMeAbout: []string{"деплой на staging", "доступы к kubernetes"}},
			{ID: "p-gena", Name: "Гена", RoleTitle: "Релизы", TeamID: "t-ads", Contact: "@gena",
				AskMeAbout: []string{"деплой и релизы", "ревью кода"}},
		},
		map[string]people.Relations{
			"u-intern": {MentorID: "p-anna", ManagerID: "p-dima", HRBPID: "p-eva"},
			"u-orphan": {MentorID: "p-anna"},
		},
	)
}

func askWho(t *testing.T, dir PeopleDirectory, p domain.Principal, args string) whoToAskOut {
	t.Helper()
	tool, err := WhoToAsk(dir)
	if err != nil {
		t.Fatalf("WhoToAsk: %v", err)
	}
	r := newRegistry(t, tool)
	raw, err := r.Call(context.Background(), p, domain.ThreadGeneral, "who_to_ask", json.RawMessage(args))
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	var out whoToAskOut
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("decode %s: %v", raw, err)
	}
	return out
}

func ids(hints []PersonHint) []string {
	var out []string
	for _, h := range hints {
		out = append(out, h.PersonID)
	}
	return out
}

func TestWhoToAskRanksByTopicMatch(t *testing.T) {
	out := askWho(t, testDirectory(), intern, `{"topic":"деплой на staging"}`)

	if got := ids(out.People); !slices.Equal(got, []string{"p-vika", "p-gena"}) {
		t.Fatalf("people = %v, want Vika (two words) then Gena (one)", got)
	}
	if out.People[0].Reason != "деплой на staging" {
		t.Errorf("reason = %q", out.People[0].Reason)
	}
}

func TestWhoToAskMatchesWordForms(t *testing.T) {
	out := askWho(t, testDirectory(), intern, `{"topic":"не работает сборка сервиса"}`)

	if got := ids(out.People); len(got) == 0 || got[0] != "p-anna" {
		t.Fatalf("people = %v, want Anna first", got)
	}
}

func TestWhoToAskPrefersOwnTeamOnTie(t *testing.T) {
	out := askWho(t, testDirectory(), intern, `{"topic":"ревью"}`)

	if got := ids(out.People); !slices.Equal(got, []string{"p-anna", "p-gena"}) {
		t.Fatalf("people = %v, want own-team Anna before Gena", got)
	}
	if out.People[0].OtherTeam || !out.People[1].OtherTeam {
		t.Errorf("other_team flags = %v, %v", out.People[0].OtherTeam, out.People[1].OtherTeam)
	}
}

func TestWhoToAskShowsContactOnlyInsideCluster(t *testing.T) {
	out := askWho(t, testDirectory(), intern, `{"topic":"деплой на staging"}`)

	contacts := map[string]string{}
	for _, h := range out.People {
		contacts[h.PersonID] = h.Contact
	}
	if contacts["p-vika"] != "@vika" {
		t.Errorf("same-cluster contact = %q, want @vika", contacts["p-vika"])
	}
	if contacts["p-gena"] != "" {
		t.Errorf("other-cluster contact leaked: %q", contacts["p-gena"])
	}
}

func TestWhoToAskNeverSuggestsTheCaller(t *testing.T) {
	out := askWho(t, testDirectory(), intern, `{"topic":"деплой на staging"}`)

	if slices.Contains(ids(out.People), "p-intern") {
		t.Fatal("caller suggested to themselves")
	}
}

func TestWhoToAskFiltersByTeam(t *testing.T) {
	out := askWho(t, testDirectory(), intern, `{"topic":"деплой","team_id":"t-ads"}`)

	if got := ids(out.People); !slices.Equal(got, []string{"p-gena"}) {
		t.Fatalf("people = %v, want only Ads", got)
	}
}

func TestWhoToAskAlwaysReturnsEscalationChain(t *testing.T) {
	out := askWho(t, testDirectory(), intern, `{"topic":"бенефиты и ДМС"}`)

	if len(out.People) != 0 {
		t.Fatalf("people = %v, want none for an unknown topic", ids(out.People))
	}
	want := []EscalationStep{
		{Level: 1, Role: "mentor", Title: "наставник", PersonID: "p-anna", Name: "Аня"},
		{Level: 2, Role: "manager", Title: "руководитель", PersonID: "p-dima", Name: "Дима"},
		{Level: 3, Role: "hr_bp", Title: "HR BP", PersonID: "p-eva", Name: "Ева"},
		{Level: 4, Role: "open_sources", Title: "база знаний и каналы команды"},
	}
	if !slices.Equal(out.Escalation, want) {
		t.Fatalf("escalation = %+v", out.Escalation)
	}
}

func TestWhoToAskSkipsMissingRelations(t *testing.T) {
	orphan := domain.Principal{UserID: "u-orphan", Role: domain.RoleIntern, TeamID: "t-msg"}
	out := askWho(t, testDirectory(), orphan, `{"topic":"ревью"}`)

	want := []EscalationStep{
		{Level: 1, Role: "mentor", Title: "наставник", PersonID: "p-anna", Name: "Аня"},
		{Level: 2, Role: "open_sources", Title: "база знаний и каналы команды"},
	}
	if !slices.Equal(out.Escalation, want) {
		t.Fatalf("escalation = %+v", out.Escalation)
	}
}

func TestWhoToAskIsOpenToEveryRoleInEveryThread(t *testing.T) {
	tool, err := WhoToAsk(testDirectory())
	if err != nil {
		t.Fatalf("WhoToAsk: %v", err)
	}
	r := newRegistry(t, tool)
	for _, role := range []domain.Role{domain.RoleIntern, domain.RoleMentor, domain.RoleManager, domain.RoleHRBP, domain.RoleHRAdmin} {
		for _, thread := range []domain.ThreadType{domain.ThreadGeneral, domain.ThreadPlan, domain.ThreadRequest, domain.ThreadTask} {
			if len(r.Available(domain.Principal{Role: role}, thread)) != 1 {
				t.Errorf("who_to_ask hidden from %s in %s thread", role, thread)
			}
		}
	}
}

var errDirectoryDown = errors.New("directory down")

// failingDirectory breaks one directory method and delegates the rest.
type failingDirectory struct {
	PeopleDirectory
	broken string
}

func (d failingDirectory) People(ctx context.Context) ([]domain.Person, error) {
	if d.broken == "people" {
		return nil, errDirectoryDown
	}
	return d.PeopleDirectory.People(ctx)
}

func (d failingDirectory) Person(ctx context.Context, id string) (domain.Person, error) {
	if d.broken == "person" {
		return domain.Person{}, errDirectoryDown
	}
	return d.PeopleDirectory.Person(ctx, id)
}

func (d failingDirectory) Team(ctx context.Context, id string) (domain.Team, error) {
	if d.broken == "team" {
		return domain.Team{}, errDirectoryDown
	}
	return d.PeopleDirectory.Team(ctx, id)
}

func (d failingDirectory) Relations(ctx context.Context, userID string) (people.Relations, error) {
	if d.broken == "relations" {
		return people.Relations{}, errDirectoryDown
	}
	return d.PeopleDirectory.Relations(ctx, userID)
}

func TestWhoToAskReportsDirectoryFailure(t *testing.T) {
	for _, broken := range []string{"people", "person", "team", "relations"} {
		t.Run(broken, func(t *testing.T) {
			tool, err := WhoToAsk(failingDirectory{PeopleDirectory: testDirectory(), broken: broken})
			if err != nil {
				t.Fatalf("WhoToAsk: %v", err)
			}
			r := newRegistry(t, tool)

			_, err = r.Call(context.Background(), intern, domain.ThreadGeneral, "who_to_ask", json.RawMessage(`{"topic":"ревью"}`))
			if !errors.Is(err, errDirectoryDown) {
				t.Fatalf("error = %v, want errDirectoryDown", err)
			}
		})
	}
}

func TestWhoToAskSkipsRelationToUnknownPerson(t *testing.T) {
	dir := people.NewMemory(nil, nil, map[string]people.Relations{"u-intern": {MentorID: "p-gone"}})
	out := askWho(t, dir, intern, `{"topic":"ревью"}`)

	want := []EscalationStep{{Level: 1, Role: "open_sources", Title: "база знаний и каналы команды"}}
	if !slices.Equal(out.Escalation, want) {
		t.Fatalf("escalation = %+v", out.Escalation)
	}
}

func TestWhoToAskAcceptsTeamNameInsteadOfID(t *testing.T) {
	for _, team := range []string{"t-ads", "Ads", "ads"} {
		out := askWho(t, testDirectory(), intern, `{"topic":"деплой","team_id":"`+team+`"}`)
		if got := ids(out.People); !slices.Equal(got, []string{"p-gena"}) {
			t.Errorf("team %q: people = %v, want only Ads", team, got)
		}
	}
}

func TestWhoToAskMatchesRoleTitles(t *testing.T) {
	out := askWho(t, testDirectory(), intern, `{"topic":"инфраструктура"}`)
	if got := ids(out.People); !slices.Equal(got, []string{"p-vika"}) || out.People[0].Reason != "Инфраструктура" {
		t.Fatalf("people = %v (%+v)", got, out.People)
	}
}
