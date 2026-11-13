package main

import (
	"context"
	"fmt"

	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/config"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/domain"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/knowledge"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/people"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/requests"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/store"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/tools"
	"github.com/Long-Honey-Road/mon-europeenne/backend/seed"
)

// fakeAnswer is what LLM_PROVIDER=fake says to every question; it names the mode so nobody mistakes it for buddy.
const fakeAnswer = "Тестовый режим: модель не подключена (LLM_PROVIDER=fake). Вопрос получен, ответ придёт, когда подключат модель."

// agentTools builds the tool set; without a store (buddy ask without DATABASE_URL) only directory tools exist.
// The requests module is added only when FEATURE_REQUESTS is on.
// repo is nil without the team-repo service: the task thread then has no repository tools.
func agentTools(data seed.Data, dir people.Directory, st *store.Store, search tools.Searcher, repo tools.TeamRepo, requestsOn bool) (*tools.Registry, error) {
	var list []tools.Tool
	add := func(t tools.Tool, err error) error {
		if err != nil {
			return err
		}
		list = append(list, t)
		return nil
	}
	if err := add(tools.WhoToAsk(dir)); err != nil {
		return nil, err
	}
	if err := add(tools.ReportNotFound(dir)); err != nil {
		return nil, err
	}
	if err := add(tools.GeneralKnowledge()); err != nil {
		return nil, err
	}
	if err := add(tools.TeamMembers(dir)); err != nil {
		return nil, err
	}
	if repo != nil {
		if err := add(tools.ReadRepo(repo)); err != nil {
			return nil, err
		}
		if err := add(tools.TeamProcess(repo)); err != nil {
			return nil, err
		}
	}
	if st != nil {
		if err := add(tools.SearchDocs(search)); err != nil {
			return nil, err
		}
		if err := add(tools.GetPlan(st)); err != nil {
			return nil, err
		}
		if err := add(tools.UpdateItemStatus(st)); err != nil {
			return nil, err
		}
		if requestsOn {
			reqTools, err := requests.Tools(requests.CatalogFromSeed(data), st, dir)
			if err != nil {
				return nil, err
			}
			list = append(list, reqTools...)
		}
	}
	return tools.NewRegistry(list...)
}

// localTools builds the tool set for dev commands: with DATABASE_URL the plan and requests tools work too,
// without it only the directory ones.
func localTools(ctx context.Context, cfg config.Config, data seed.Data) (*tools.Registry, func(), error) {
	var st *store.Store
	closeFn := func() {}
	if cfg.DatabaseURL != "" {
		pool, err := store.Open(ctx, cfg.DatabaseURL)
		if err != nil {
			return nil, nil, err
		}
		st, closeFn = store.New(pool), pool.Close
	}
	var search tools.Searcher
	if st != nil {
		search = knowledge.New(st)
	}
	reg, err := agentTools(data, data.Directory(), st, search, nil, cfg.FeatureRequests)
	if err != nil {
		closeFn()
		return nil, nil, err
	}
	return reg, closeFn, nil
}

// seedUsers looks up test users by id without hashing passwords.
type seedUsers seed.Data

func (s seedUsers) User(_ context.Context, id string) (domain.User, error) {
	for _, c := range seed.Data(s).Credentials() {
		if c.User.ID == id {
			return c.User, nil
		}
	}
	return domain.User{}, fmt.Errorf("unknown user %q", id)
}
