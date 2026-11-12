package seed

import (
	"testing"
	"time"

	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/domain"
)

func TestCredentialsFromShippedData(t *testing.T) {
	d, err := Load(Files)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	creds := d.Credentials()

	byLogin := map[string]domain.User{}
	for _, c := range creds {
		if c.Password == "" {
			t.Errorf("%s: empty password", c.User.Login)
		}
		byLogin[c.User.Login] = c.User
	}
	if len(byLogin) != 3 {
		t.Fatalf("%d accounts, want intern, mentor, manager", len(byLogin))
	}
	intern := byLogin["intern"]
	want := domain.User{
		ID: "u-liza", Login: "intern", DisplayName: "Лиза Орлова", Role: domain.RoleIntern,
		Direction: domain.DirectionBackend, TeamID: "t-messenger",
		Repos: []string{"messenger-api"}, JiraProjects: []string{"MSG"}, Location: "Москва",
		StartDate: time.Date(2026, 11, 9, 0, 0, 0, 0, time.UTC),
	}
	if intern.ID != want.ID || intern.DisplayName != want.DisplayName || intern.Role != want.Role ||
		intern.Direction != want.Direction || intern.TeamID != want.TeamID || !intern.StartDate.Equal(want.StartDate) ||
		len(intern.Repos) != 1 || intern.Location != want.Location {
		t.Errorf("intern = %+v, want %+v", intern, want)
	}
	if byLogin["manager"].Role != domain.RoleManager || byLogin["manager"].Direction != 0 {
		t.Errorf("manager = %+v", byLogin["manager"])
	}
}

func TestPlanFromShippedData(t *testing.T) {
	d, err := Load(Files)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	items, statuses := d.PlanItems(), d.PlanStatuses()
	if len(items) != 17 || len(statuses) != 3 {
		t.Fatalf("%d items, %d statuses", len(items), len(statuses))
	}
	var channels domain.PlanItem
	for _, it := range items {
		if it.Key == "w1-channels" {
			channels = it
		}
	}
	if channels.Period != domain.PeriodWeek1 || len(channels.DependsOn) != 1 || channels.Map.Group != "team" {
		t.Errorf("w1-channels = %+v", channels)
	}
	if statuses[0].UserID != "u-liza" || statuses[0].ItemID != "w1-team" || statuses[0].Status != domain.ItemDone {
		t.Errorf("first status = %+v", statuses[0])
	}
}
