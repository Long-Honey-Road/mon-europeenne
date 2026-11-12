package profile

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/domain"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/people"
)

func dir() *people.Memory {
	return people.NewMemory(
		[]domain.Team{{ID: "t-messenger", Name: "Messenger", Unit: "Messenger", Cluster: "Коммуникации"}},
		[]domain.Person{{ID: "p-anna", Name: "Анна"}, {ID: "p-dmitry", Name: "Дмитрий"}},
		map[string]people.Relations{"u-liza": {MentorID: "p-anna", ManagerID: "p-dmitry"}},
	)
}

func TestBuildCompactProfile(t *testing.T) {
	u := domain.User{
		ID: "u-liza", DisplayName: "Лиза", Role: domain.RoleIntern, Direction: domain.DirectionBackend,
		TeamID: "t-messenger", Repos: []string{"messenger-api"}, StartDate: time.Date(2026, 11, 9, 0, 0, 0, 0, time.UTC),
	}

	raw, err := Build(context.Background(), u, dir())
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal([]byte(raw), &got); err != nil {
		t.Fatalf("decode %s: %v", raw, err)
	}
	want := map[string]any{
		"name": "Лиза", "role": "intern", "direction": "backend", "team": "Messenger", "unit": "Messenger",
		"cluster": "Коммуникации", "mentor": "Анна", "manager": "Дмитрий", "repos": []any{"messenger-api"},
		"start_date": "2026-11-09",
	}
	gotJSON, _ := json.Marshal(got)
	wantJSON, _ := json.Marshal(want)
	if string(gotJSON) != string(wantJSON) {
		t.Fatalf("profile = %s\nwant      %s", gotJSON, wantJSON)
	}
}

type brokenDir struct{ *people.Memory }

func (brokenDir) Relations(context.Context, string) (people.Relations, error) {
	return people.Relations{}, errors.New("down")
}

func TestBuildReportsDirectoryFailure(t *testing.T) {
	if _, err := Build(context.Background(), domain.User{ID: "u-liza"}, brokenDir{dir()}); err == nil {
		t.Fatal("directory failure swallowed")
	}
}
