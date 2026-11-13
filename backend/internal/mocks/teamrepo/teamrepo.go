// Package teamrepo is the team-repo mock: one synthetic repository per team and its process from CONTRIBUTING.md.
package teamrepo

import (
	"context"
	"errors"
	"io/fs"
	"path"
	"sort"
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	teamrepov1 "github.com/Long-Honey-Road/mon-europeenne/backend/gen/mocks/teamrepo/v1"
	"github.com/Long-Honey-Road/mon-europeenne/backend/seed"
)

const (
	processFile = "CONTRIBUTING.md"
	maxDepth    = 3
)

// sectionKeys maps CONTRIBUTING.md headings to process sections.
var sectionKeys = map[string]string{
	"definition of ready": "dor",
	"code style":          "code_style",
	"pull requests":       "pr",
	"code review":         "review",
	"running and testing": "run",
}

type Service struct {
	teamrepov1.UnimplementedTeamRepoServiceServer
	repos map[string]seed.Repo
}

func New(repos map[string]seed.Repo) *Service {
	return &Service{repos: repos}
}

func (s *Service) repo(team string) (seed.Repo, error) {
	r, ok := s.repos[team]
	if !ok {
		return seed.Repo{}, status.Errorf(codes.NotFound, "team %q has no repository", team)
	}
	return r, nil
}

// clean turns a user path into an fs path: no leading slash or "./", never outside the repository.
func clean(p string) (string, error) {
	p = strings.TrimPrefix(path.Clean("/"+strings.TrimSpace(p)), "/")
	if p == "" {
		p = "."
	}
	if !fs.ValidPath(p) {
		return "", status.Errorf(codes.InvalidArgument, "bad path %q", p)
	}
	return p, nil
}

func (s *Service) GetTree(_ context.Context, req *teamrepov1.GetTreeRequest) (*teamrepov1.GetTreeResponse, error) {
	r, err := s.repo(req.GetTeamId())
	if err != nil {
		return nil, err
	}
	root, err := clean(req.GetPath())
	if err != nil {
		return nil, err
	}
	if info, err := fs.Stat(r.FS, root); err != nil || !info.IsDir() {
		return nil, status.Errorf(codes.NotFound, "no directory %q", req.GetPath())
	}
	depth := min(max(int(req.GetDepth()), 1), maxDepth)
	out := &teamrepov1.GetTreeResponse{Repo: r.Name}
	base := strings.Count(root, "/")
	if root == "." {
		base = -1
	}
	err = fs.WalkDir(r.FS, root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || p == root {
			return err
		}
		if strings.Count(p, "/")-base > depth {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		out.Entries = append(out.Entries, &teamrepov1.Entry{Path: p, Dir: d.IsDir()})
		return nil
	})
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	sort.Slice(out.Entries, func(i, j int) bool { return out.Entries[i].GetPath() < out.Entries[j].GetPath() })
	return out, nil
}

func (s *Service) GetFile(_ context.Context, req *teamrepov1.GetFileRequest) (*teamrepov1.GetFileResponse, error) {
	r, err := s.repo(req.GetTeamId())
	if err != nil {
		return nil, err
	}
	p, err := clean(req.GetPath())
	if err != nil {
		return nil, err
	}
	raw, err := fs.ReadFile(r.FS, p)
	var pathErr *fs.PathError
	switch {
	case err == nil:
		return &teamrepov1.GetFileResponse{Repo: r.Name, Path: p, Text: string(raw)}, nil
	case errors.Is(err, fs.ErrNotExist):
		return nil, status.Errorf(codes.NotFound, "no file %q", req.GetPath())
	case errors.As(err, &pathErr) || p == ".":
		return nil, status.Errorf(codes.InvalidArgument, "%q is a directory", req.GetPath())
	default:
		return nil, status.Error(codes.Internal, err.Error())
	}
}

func (s *Service) GetProcess(_ context.Context, req *teamrepov1.GetProcessRequest) (*teamrepov1.GetProcessResponse, error) {
	r, err := s.repo(req.GetTeamId())
	if err != nil {
		return nil, err
	}
	raw, err := fs.ReadFile(r.FS, processFile)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "no %s in %s", processFile, r.Name)
	}
	out := &teamrepov1.GetProcessResponse{Repo: r.Name}
	for _, part := range strings.Split("\n"+string(raw), "\n## ")[1:] {
		title, body, _ := strings.Cut(part, "\n")
		key, ok := sectionKeys[strings.ToLower(strings.TrimSpace(title))]
		if !ok {
			continue
		}
		out.Sections = append(out.Sections, &teamrepov1.ProcessSection{
			Key: key, Title: strings.TrimSpace(title), Markdown: strings.TrimSpace(body), File: processFile,
		})
	}
	return out, nil
}
