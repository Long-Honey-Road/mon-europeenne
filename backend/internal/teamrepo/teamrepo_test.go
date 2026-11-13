package teamrepo_test

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/domain"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/mocks"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/mocks/mockstest"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/teamrepo"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/tools"
)

var intern = domain.Principal{UserID: "u-liza", Role: domain.RoleIntern, TeamID: "t-messenger"}

func registry(t *testing.T) (*tools.Registry, func()) {
	t.Helper()
	conn, srv := mockstest.Start(t, mocks.Chaos{})
	repo := teamrepo.New(conn)
	read, err := tools.ReadRepo(repo)
	if err != nil {
		t.Fatal(err)
	}
	proc, err := tools.TeamProcess(repo)
	if err != nil {
		t.Fatal(err)
	}
	reg, err := tools.NewRegistry(read, proc)
	if err != nil {
		t.Fatal(err)
	}
	return reg, srv.GRPC.Stop
}

func call(t *testing.T, reg *tools.Registry, p domain.Principal, thread domain.ThreadType, name, args string) (map[string]any, error) {
	t.Helper()
	raw, err := reg.Call(t.Context(), p, thread, name, json.RawMessage(args))
	if err != nil {
		return nil, err
	}
	var out map[string]any
	return out, json.Unmarshal(raw, &out)
}

// Golden dialogs 1, 4 and 5 of spec 007, against the team-repo mock.
func TestReadRepo(t *testing.T) {
	reg, _ := registry(t)

	root, err := call(t, reg, intern, domain.ThreadTask, "read_repo", `{}`)
	if err != nil || root["repo"] != "messenger-api" || len(root["tree"].([]any)) < 8 {
		t.Fatalf("root = %v, %v", root, err)
	}
	file, err := call(t, reg, intern, domain.ThreadTask, "read_repo", `{"path":"/README.md"}`)
	if err != nil {
		t.Fatal(err)
	}
	src := file["sources"].([]any)[0].(map[string]any)
	if src["title"] != "messenger-api/README.md" || src["kind"] != "repo" || !strings.Contains(src["text"].(string), "make dev") {
		t.Errorf("README fragment = %v", src)
	}
	dir, err := call(t, reg, intern, domain.ThreadTask, "read_repo", `{"path":"internal"}`)
	if err != nil || len(dir["tree"].([]any)) != 5 {
		t.Errorf("internal = %v, %v", dir, err)
	}
	if _, err := call(t, reg, intern, domain.ThreadTask, "read_repo", `{"path":"docs/nothing.md"}`); !errors.Is(err, tools.ErrNotFound) {
		t.Errorf("missing file: %v", err)
	}
	other := domain.Principal{UserID: "u-x", Role: domain.RoleIntern, TeamID: "t-notifications"}
	if _, err := call(t, reg, other, domain.ThreadTask, "read_repo", `{}`); !errors.Is(err, tools.ErrNotFound) {
		t.Errorf("team without a repository: %v", err)
	}
	if _, err := call(t, reg, intern, domain.ThreadGeneral, "read_repo", `{}`); !errors.Is(err, tools.ErrNotInThread) {
		t.Errorf("general thread: %v", err)
	}
}

// Golden dialog 2 of spec 007.
func TestTeamProcess(t *testing.T) {
	reg, stop := registry(t)

	all, err := call(t, reg, intern, domain.ThreadTask, "get_team_process", `{}`)
	if err != nil || len(all["sources"].([]any)) != 5 {
		t.Fatalf("all sections = %v, %v", all, err)
	}
	pr, err := call(t, reg, intern, domain.ThreadTask, "get_team_process", `{"section":"pr"}`)
	if err != nil {
		t.Fatal(err)
	}
	src := pr["sources"].([]any)[0].(map[string]any)
	if src["title"] != "messenger-api/CONTRIBUTING.md / Pull requests" || src["source_id"] != "repo:messenger-api/CONTRIBUTING.md#pr" {
		t.Errorf("pr section = %v", src)
	}
	if _, err := call(t, reg, intern, domain.ThreadTask, "get_team_process", `{"section":"deploy"}`); !errors.Is(err, tools.ErrNotFound) {
		t.Errorf("unknown section: %v", err)
	}
	stop()
	for _, name := range []string{"read_repo", "get_team_process"} {
		if _, err := call(t, reg, intern, domain.ThreadTask, name, `{"path":"README.md"}`); err == nil || errors.Is(err, tools.ErrNotFound) {
			t.Errorf("%s with the service down: %v", name, err)
		}
	}
	if _, err := call(t, reg, intern, domain.ThreadTask, "read_repo", `{}`); err == nil {
		t.Error("tree with the service down: no error")
	}
}
