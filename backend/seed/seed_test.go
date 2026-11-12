package seed

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"testing/fstest"
)

// fixture is the smallest consistent data set; each test breaks one thing.
type fixture struct {
	people   PeopleFile
	requests RequestsFile
	plan     PlanFile
}

func validFixture() fixture {
	return fixture{
		people: PeopleFile{
			Teams: []Team{{ID: "t-msg", Name: "Messenger", Cluster: "communications"}},
			People: []Person{
				{ID: "p-mentor", Name: "Аня", TeamID: "t-msg", AskMeAbout: []string{"ревью"}},
				{ID: "p-manager", Name: "Дима", TeamID: "t-msg"},
				{ID: "p-hradmin", Name: "Зоя", TeamID: "t-msg", ApproverFor: []string{"hr_admin"}},
				{ID: "p-intern", Name: "Лиза", TeamID: "t-msg", Account: &Account{
					UserID: "u-intern", Login: "intern", Password: "pw", Role: "intern", Direction: "backend",
					MentorID: "p-mentor", ManagerID: "p-manager", StartDate: "2026-11-09",
				}},
			},
		},
		requests: RequestsFile{
			Types: []RequestType{
				{
					Key: "access.jira_project", Title: "Доступ к проекту Jira", Category: "access",
					FieldsSchema:  json.RawMessage(`{"type":"object","required":["project_key"],"properties":{"project_key":{"type":"string"}}}`),
					ApprovalChain: []Approval{{Resolver: "object_owner", Object: "jira_project"}},
					SLAHours:      24,
					Demo:          Demo{Outcome: "approve", AfterSeconds: 30},
				},
				{
					Key: "absence.day_off", Title: "Отгул", Category: "absence",
					FieldsSchema:  json.RawMessage(`{"type":"object","properties":{"date":{"type":"string"}}}`),
					ApprovalChain: []Approval{{Resolver: "manager"}, {Resolver: "hr_admin"}},
					Demo:          Demo{Outcome: "reject", AfterSeconds: 30, Reason: "пересекается с релизом"},
				},
			},
			Objects: []Object{{Kind: "jira_project", Key: "MSG", Title: "Messenger", OwnerID: "p-manager"}},
		},
		plan: PlanFile{
			Items: []PlanItem{
				{Key: "w1-team", Title: "Познакомиться с командой", Period: "w1", Map: MapPosition{X: 0, Y: 0}},
				{Key: "w1-access", Title: "Запросить доступы", Period: "w1", Direction: "backend",
					DependsOn: []string{"w1-team"}, Map: MapPosition{X: 1, Y: 0}},
			},
			Statuses: []PlanStatus{{UserID: "u-intern", ItemKey: "w1-team", Status: "done"}},
		},
	}
}

func (f fixture) fs(t *testing.T) fstest.MapFS {
	t.Helper()
	enc := func(v any) *fstest.MapFile {
		b, err := json.Marshal(v)
		if err != nil {
			t.Fatalf("marshal fixture: %v", err)
		}
		return &fstest.MapFile{Data: b}
	}
	return fstest.MapFS{
		"people.json":        enc(f.people),
		"request_types.json": enc(f.requests),
		"plan.json":          enc(f.plan),
	}
}

func TestLoadAcceptsConsistentData(t *testing.T) {
	d, err := Load(validFixture().fs(t))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(d.People.People) != 4 || len(d.Requests.Types) != 2 || len(d.Plan.Items) != 2 {
		t.Fatalf("loaded %d people, %d request types, %d plan items",
			len(d.People.People), len(d.Requests.Types), len(d.Plan.Items))
	}
}

func TestLoadRejectsInconsistentData(t *testing.T) {
	cases := map[string]func(*fixture){
		"duplicate person id":     func(f *fixture) { f.people.People[1].ID = "p-mentor" },
		"person in unknown team":  func(f *fixture) { f.people.People[0].TeamID = "t-none" },
		"unknown account role":    func(f *fixture) { f.people.People[3].Account.Role = "boss" },
		"unknown direction":       func(f *fixture) { f.people.People[3].Account.Direction = "ml" },
		"mentor not in directory": func(f *fixture) { f.people.People[3].Account.MentorID = "p-ghost" },
		"bad start date":          func(f *fixture) { f.people.People[3].Account.StartDate = "9 ноября" },
		"duplicate login": func(f *fixture) {
			f.people.People[0].Account = &Account{UserID: "u-2", Login: "intern", Password: "x", Role: "mentor"}
		},
		"account without password":   func(f *fixture) { f.people.People[3].Account.Password = "" },
		"unknown approver role":      func(f *fixture) { f.people.People[2].ApproverFor = []string{"cfo"} },
		"duplicate request type":     func(f *fixture) { f.requests.Types[1].Key = "access.jira_project" },
		"broken fields schema":       func(f *fixture) { f.requests.Types[0].FieldsSchema = json.RawMessage(`{"type":12}`) },
		"unknown resolver":           func(f *fixture) { f.requests.Types[0].ApprovalChain[0].Resolver = "ceo" },
		"owner resolver w/o object":  func(f *fixture) { f.requests.Types[0].ApprovalChain[0].Object = "" },
		"no object of that kind":     func(f *fixture) { f.requests.Objects[0].Kind = "confluence_space" },
		"object owner unknown":       func(f *fixture) { f.requests.Objects[0].OwnerID = "p-ghost" },
		"nobody approves as hr":      func(f *fixture) { f.people.People[2].ApproverFor = nil },
		"empty approval chain":       func(f *fixture) { f.requests.Types[0].ApprovalChain = nil },
		"unknown demo outcome":       func(f *fixture) { f.requests.Types[0].Demo.Outcome = "maybe" },
		"reject without reason":      func(f *fixture) { f.requests.Types[1].Demo.Reason = "" },
		"duplicate plan key":         func(f *fixture) { f.plan.Items[1].Key = "w1-team" },
		"unknown period":             func(f *fixture) { f.plan.Items[0].Period = "w9" },
		"unknown plan direction":     func(f *fixture) { f.plan.Items[1].Direction = "ml" },
		"plan item in unknown team":  func(f *fixture) { f.plan.Items[1].TeamID = "t-none" },
		"dependency on missing item": func(f *fixture) { f.plan.Items[1].DependsOn = []string{"w1-ghost"} },
		"dependency cycle":           func(f *fixture) { f.plan.Items[0].DependsOn = []string{"w1-access"} },
		"status for unknown user":    func(f *fixture) { f.plan.Statuses[0].UserID = "u-ghost" },
		"status for unknown item":    func(f *fixture) { f.plan.Statuses[0].ItemKey = "w1-ghost" },
		"unknown status":             func(f *fixture) { f.plan.Statuses[0].Status = "half-done" },
	}
	for name, breakIt := range cases {
		t.Run(name, func(t *testing.T) {
			f := validFixture()
			breakIt(&f)
			if _, err := Load(f.fs(t)); !errors.Is(err, ErrInvalid) {
				t.Fatalf("Load() error = %v, want ErrInvalid", err)
			}
		})
	}
}

func TestLoadRejectsUnknownFields(t *testing.T) {
	fsys := validFixture().fs(t)
	fsys["plan.json"] = &fstest.MapFile{Data: []byte(`{"items":[],"statuses":[],"owner":"x"}`)}

	if _, err := Load(fsys); !errors.Is(err, ErrInvalid) {
		t.Fatalf("Load() error = %v, want ErrInvalid", err)
	}
}

func TestLoadReportsMissingFile(t *testing.T) {
	fsys := validFixture().fs(t)
	delete(fsys, "request_types.json")

	_, err := Load(fsys)
	if err == nil || !strings.Contains(err.Error(), "request_types.json") {
		t.Fatalf("Load() error = %v, want it to name the missing file", err)
	}
}

func TestDirectoryExposesPeopleAndRelations(t *testing.T) {
	d, err := Load(validFixture().fs(t))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	dir := d.Directory()

	rel, err := dir.Relations(context.Background(), "u-intern")
	if err != nil || rel.MentorID != "p-mentor" || rel.ManagerID != "p-manager" {
		t.Fatalf("Relations(u-intern) = %+v, %v", rel, err)
	}
	p, err := dir.Person(context.Background(), "p-intern")
	if err != nil || p.UserID != "u-intern" {
		t.Fatalf("Person(p-intern) = %+v, %v", p, err)
	}
	team, err := dir.Team(context.Background(), "t-msg")
	if err != nil || team.Cluster != "communications" {
		t.Fatalf("Team(t-msg) = %+v, %v", team, err)
	}
}
