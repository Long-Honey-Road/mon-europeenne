package people_test

import (
	"errors"
	"testing"
	"time"

	authrolesv1 "github.com/Long-Honey-Road/mon-europeenne/backend/gen/mocks/authroles/v1"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/mocks"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/mocks/mockstest"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/people"
)

func TestRemote(t *testing.T) {
	conn, srv := mockstest.Start(t, mocks.Chaos{})
	var dir people.Directory = people.NewRemote(conn, time.Hour)
	ctx := t.Context()

	all, err := dir.People(ctx)
	if err != nil || len(all) < 8 {
		t.Fatalf("People = %d, %v", len(all), err)
	}
	if p, err := dir.Person(ctx, "p-liza"); err != nil || p.UserID != "u-liza" {
		t.Errorf("Person = %+v, %v", p, err)
	}
	if _, err := dir.Person(ctx, "p-ghost"); !errors.Is(err, people.ErrNotFound) {
		t.Errorf("unknown person: %v", err)
	}
	if tm, err := dir.Team(ctx, "t-messenger"); err != nil || tm.Name == "" {
		t.Errorf("Team = %+v, %v", tm, err)
	}
	rel, err := dir.Relations(ctx, "u-liza")
	if err != nil || rel.MentorID != "p-anna" {
		t.Errorf("Relations = %+v, %v", rel, err)
	}

	// The service goes down: the copy is served, even after ttl.
	srv.GRPC.Stop()
	stale := people.NewRemote(conn, 0)
	if _, err := stale.People(ctx); err == nil {
		t.Error("cold client with the service down: no error")
	}
	if _, err := dir.Team(ctx, "t-people"); err != nil {
		t.Errorf("Team during outage: %v", err)
	}
}

func TestRemoteServesLastCopyAfterTTL(t *testing.T) {
	conn, srv := mockstest.Start(t, mocks.Chaos{})
	dir := people.NewRemote(conn, time.Nanosecond)
	ctx := t.Context()
	if _, err := dir.People(ctx); err != nil {
		t.Fatal(err)
	}
	srv.GRPC.Stop()
	time.Sleep(time.Millisecond)
	for range 2 {
		if p, err := dir.Person(ctx, "p-liza"); err != nil || p.Name == "" {
			t.Errorf("Person after outage = %+v, %v", p, err)
		}
	}
	if _, err := dir.Relations(ctx, "u-liza"); err != nil {
		t.Errorf("Relations after outage: %v", err)
	}
}

func TestRemoteColdErrors(t *testing.T) {
	conn, srv := mockstest.Start(t, mocks.Chaos{})
	srv.GRPC.Stop()
	dir := people.NewRemote(conn, time.Hour)
	ctx := t.Context()
	if _, err := dir.Person(ctx, "p-liza"); err == nil {
		t.Error("Person: no error")
	}
	if _, err := dir.Team(ctx, "t-messenger"); err == nil {
		t.Error("Team: no error")
	}
	if _, err := dir.Relations(ctx, "u-liza"); err == nil {
		t.Error("Relations: no error")
	}
}

func TestRemoteForgetSeesNewPeople(t *testing.T) {
	conn, srv := mockstest.Start(t, mocks.Chaos{})
	r := people.NewRemote(conn, time.Hour)
	ctx := t.Context()
	before, err := r.People(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := srv.Auth.CreateAccount(ctx, &authrolesv1.CreateAccountRequest{
		RequesterId: "u-dmitry", DisplayName: "Вера Новикова", Login: "vnovikova", Direction: "backend", MentorId: "p-anna",
	}); err != nil {
		t.Fatal(err)
	}
	if cached, _ := r.People(ctx); len(cached) != len(before) {
		t.Errorf("cache changed before Forget: %d → %d", len(before), len(cached))
	}
	r.Forget()
	if fresh, _ := r.People(ctx); len(fresh) != len(before)+1 {
		t.Errorf("after Forget: %d people, want %d", len(fresh), len(before)+1)
	}
}
