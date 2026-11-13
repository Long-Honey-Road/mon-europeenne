package tools

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/domain"
)

type fakeRepo struct {
	text string
	err  error
}

func (f fakeRepo) Tree(context.Context, string, string, int) (string, []RepoEntry, error) {
	return "r", []RepoEntry{{Path: "a"}}, f.err
}

func (f fakeRepo) File(context.Context, string, string) (string, string, error) {
	return "r", f.text, f.err
}

func (f fakeRepo) Process(context.Context, string) (string, []ProcessSection, error) {
	return "r", nil, f.err
}

func callRepo(t *testing.T, repo TeamRepo, name, args string) (json.RawMessage, error) {
	t.Helper()
	read, _ := ReadRepo(repo)
	proc, _ := TeamProcess(repo)
	reg, err := NewRegistry(read, proc)
	if err != nil {
		t.Fatal(err)
	}
	p := domain.Principal{UserID: "u", Role: domain.RoleIntern, TeamID: "t"}
	return reg.Call(context.Background(), p, domain.ThreadTask, name, json.RawMessage(args))
}

func TestReadRepoCutsLongFiles(t *testing.T) {
	raw, err := callRepo(t, fakeRepo{text: strings.Repeat("я", maxFileRunes+10)}, "read_repo", `{"path":"big.go"}`)
	if err != nil {
		t.Fatal(err)
	}
	var out readRepoOut
	_ = json.Unmarshal(raw, &out)
	if n := len([]rune(out.Sources[0].Text)); n != maxFileRunes || !strings.Contains(out.Note, "cut") {
		t.Errorf("text %d runes, note %q", n, out.Note)
	}
}

func TestRepoToolsPassErrors(t *testing.T) {
	down := fakeRepo{err: errors.New("down")}
	for _, c := range []struct{ name, args string }{{"read_repo", `{"path":"a.go"}`}, {"read_repo", `{}`}, {"get_team_process", `{}`}} {
		if _, err := callRepo(t, down, c.name, c.args); err == nil {
			t.Errorf("%s %s: no error", c.name, c.args)
		}
	}
	if _, err := callRepo(t, fakeRepo{}, "get_team_process", `{}`); !errors.Is(err, ErrNotFound) {
		t.Errorf("no sections: %v", err)
	}
}
