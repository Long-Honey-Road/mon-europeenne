package tools

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/domain"
)

// ErrIsDir is what a repository answers when a file is asked for a directory path.
var ErrIsDir = errors.New("path is a directory")

// maxFileRunes keeps one file within a reasonable share of the context.
const maxFileRunes = 8000

type RepoEntry struct {
	Path string `json:"path"`
	Dir  bool   `json:"dir,omitempty"`
}

type ProcessSection struct {
	Key, Title, Markdown, File string
}

// TeamRepo is the team's repository host; unknown teams and paths are ErrNotFound.
type TeamRepo interface {
	Tree(ctx context.Context, team, path string, depth int) (repo string, entries []RepoEntry, err error)
	File(ctx context.Context, team, path string) (repo, text string, err error)
	Process(ctx context.Context, team string) (repo string, sections []ProcessSection, err error)
}

// RepoFragment is a citable piece of the repository.
type RepoFragment struct {
	SourceID string `json:"source_id"`
	Title    string `json:"title"`
	Text     string `json:"text"`
	Kind     string `json:"kind"`
}

type readRepoIn struct {
	Path string `json:"path,omitempty" jsonschema:"file or directory in the team repository, e.g. README.md or internal/chat; empty for the root"`
}

type readRepoOut struct {
	Repo    string         `json:"repo"`
	Tree    []RepoEntry    `json:"tree,omitempty"`
	Sources []RepoFragment `json:"sources,omitempty"`
	Note    string         `json:"note,omitempty"`
}

var taskThread = []domain.ThreadType{domain.ThreadTask}

// ReadRepo reads the caller's team repository: a directory gives its tree, a file is a citable fragment.
func ReadRepo(repo TeamRepo) (Tool, error) {
	return Define("read_repo",
		"Read the user's team repository: the tree of a directory (empty path: the root) or a file such as README.md, "+
			"Makefile or a source file. Cite a read file with [ref]. Never write code for the user.",
		allRoles, taskThread,
		func(ctx context.Context, p domain.Principal, in readRepoIn) (readRepoOut, error) {
			path := strings.Trim(strings.TrimSpace(in.Path), "/")
			if path != "" && path != "." {
				name, text, err := repo.File(ctx, p.TeamID, path)
				if err == nil {
					out := readRepoOut{Repo: name, Sources: []RepoFragment{fileFragment(name, path, text)}}
					if len([]rune(text)) > maxFileRunes {
						out.Note = fmt.Sprintf("The file is cut to the first %d characters.", maxFileRunes)
					}
					return out, nil
				}
				if !errors.Is(err, ErrIsDir) {
					return readRepoOut{}, err
				}
			}
			name, entries, err := repo.Tree(ctx, p.TeamID, path, 2)
			if err != nil {
				return readRepoOut{}, err
			}
			return readRepoOut{Repo: name, Tree: entries}, nil
		})
}

func fileFragment(repo, path, text string) RepoFragment {
	if r := []rune(text); len(r) > maxFileRunes {
		text = string(r[:maxFileRunes])
	}
	return RepoFragment{SourceID: "repo:" + repo + "/" + path, Title: repo + "/" + path, Text: text, Kind: "repo"}
}

type teamProcessIn struct {
	Section string `json:"section,omitempty" jsonschema:"one section: dor, code_style, pr, review or run; empty for all"`
}

type teamProcessOut struct {
	Repo    string         `json:"repo"`
	Sources []RepoFragment `json:"sources"`
}

// TeamProcess gives the team's working rules from the repository: definition of ready, code style, pull
// requests, review and running tests.
func TeamProcess(repo TeamRepo) (Tool, error) {
	return Define("get_team_process",
		"The user's team process from its repository: dor (definition of ready), code_style, pr (branches and pull "+
			"requests), review, run (running and testing). Cite sections with [ref].",
		allRoles, taskThread,
		func(ctx context.Context, p domain.Principal, in teamProcessIn) (teamProcessOut, error) {
			name, sections, err := repo.Process(ctx, p.TeamID)
			if err != nil {
				return teamProcessOut{}, err
			}
			out := teamProcessOut{Repo: name, Sources: []RepoFragment{}}
			for _, s := range sections {
				if in.Section != "" && s.Key != in.Section {
					continue
				}
				out.Sources = append(out.Sources, RepoFragment{
					SourceID: "repo:" + name + "/" + s.File + "#" + s.Key, Title: name + "/" + s.File + " / " + s.Title,
					Text: s.Markdown, Kind: "repo",
				})
			}
			if len(out.Sources) == 0 {
				return teamProcessOut{}, ErrNotFound
			}
			return out, nil
		})
}
