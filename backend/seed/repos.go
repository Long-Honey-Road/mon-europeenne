package seed

import (
	"fmt"
	"io/fs"
	"path"
)

const reposDir = "_repo"

// Repo is a synthetic team repository; FS is rooted at the repository.
type Repo struct {
	Name string
	FS   fs.FS
}

// repoNames are the repository names shown for each team.
var repoNames = map[string]string{"t-messenger": "messenger-api"}

// loadRepos reads _repo/<team id>/; a missing directory means no repositories.
func loadRepos(fsys fs.FS) (map[string]Repo, error) {
	entries, err := fs.ReadDir(fsys, reposDir)
	if err != nil {
		return map[string]Repo{}, nil //nolint:nilerr // no repositories in this data set
	}
	out := map[string]Repo{}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		sub, err := fs.Sub(fsys, path.Join(reposDir, e.Name()))
		if err != nil {
			return nil, err
		}
		name, ok := repoNames[e.Name()]
		if !ok {
			return nil, fmt.Errorf("%w: repository for %q has no name", ErrInvalid, e.Name())
		}
		out[e.Name()] = Repo{Name: name, FS: sub}
	}
	return out, nil
}
