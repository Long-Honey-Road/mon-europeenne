package people

import (
	"context"
	"errors"
	"testing"

	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/domain"
)

func directory() *Memory {
	return NewMemory(
		[]domain.Team{{ID: "t-msg", Name: "Messenger", Cluster: "communications"}},
		[]domain.Person{
			{ID: "p-anna", UserID: "u-anna", Name: "Аня", TeamID: "t-msg"},
			{ID: "p-dima", UserID: "u-dima", Name: "Дима", TeamID: "t-msg"},
		},
		map[string]Relations{"u-intern": {MentorID: "p-anna", ManagerID: "p-dima"}},
	)
}

func TestPeopleReturnsEveryone(t *testing.T) {
	got, err := directory().People(context.Background())
	if err != nil || len(got) != 2 {
		t.Fatalf("People() = %v, %v", got, err)
	}
}

func TestPersonByID(t *testing.T) {
	d := directory()
	p, err := d.Person(context.Background(), "p-dima")
	if err != nil || p.Name != "Дима" {
		t.Fatalf("Person(p-dima) = %+v, %v", p, err)
	}
	if _, err := d.Person(context.Background(), "p-nobody"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Person(p-nobody) error = %v, want ErrNotFound", err)
	}
}

func TestTeamByID(t *testing.T) {
	d := directory()
	team, err := d.Team(context.Background(), "t-msg")
	if err != nil || team.Cluster != "communications" {
		t.Fatalf("Team(t-msg) = %+v, %v", team, err)
	}
	if _, err := d.Team(context.Background(), "t-none"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Team(t-none) error = %v, want ErrNotFound", err)
	}
}

func TestRelationsOfUser(t *testing.T) {
	d := directory()
	rel, err := d.Relations(context.Background(), "u-intern")
	if err != nil || rel.MentorID != "p-anna" || rel.HRBPID != "" {
		t.Fatalf("Relations(u-intern) = %+v, %v", rel, err)
	}
	if rel, err := d.Relations(context.Background(), "u-anna"); err != nil || rel != (Relations{}) {
		t.Fatalf("Relations(u-anna) = %+v, %v; want empty", rel, err)
	}
}

func TestPeopleIsACopy(t *testing.T) {
	d := directory()
	got, _ := d.People(context.Background())
	got[0].Name = "changed"

	again, _ := d.People(context.Background())
	if again[0].Name == "changed" {
		t.Fatal("People() exposes internal state")
	}
}

func TestMemoryAdd(t *testing.T) {
	m := NewMemory([]domain.Team{{ID: "t1"}}, []domain.Person{{ID: "p-a", UserID: "u-a", TeamID: "t1"}}, nil)
	if err := m.Add(domain.Person{ID: "p-b", UserID: "u-b", TeamID: "t1"}, Relations{MentorID: "p-a"}); err != nil {
		t.Fatal(err)
	}
	if err := m.Add(domain.Person{ID: "p-b"}, Relations{}); err == nil {
		t.Error("duplicate id accepted")
	}
	if p, ok := m.PersonOfUser("u-b"); !ok || p.ID != "p-b" {
		t.Errorf("PersonOfUser(u-b) = %+v, %v", p, ok)
	}
	if _, ok := m.PersonOfUser("u-ghost"); ok {
		t.Error("PersonOfUser found a ghost")
	}
	if rel, _ := m.Relations(t.Context(), "u-b"); rel.MentorID != "p-a" {
		t.Errorf("relations = %+v", rel)
	}
	if all, _ := m.People(t.Context()); len(all) != 2 {
		t.Errorf("people = %d, want 2", len(all))
	}
}
