package main

import (
	"context"
	"errors"
	"time"

	"github.com/hibiken/asynq"

	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/config"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/knowledge"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/store"
	"github.com/Long-Honey-Road/mon-europeenne/backend/seed"
)

const (
	taskSyncSources = "sync_sources"
	syncEvery       = "@every 10m"
	syncTimeout     = 2 * time.Minute
)

// newKnowledge indexes the team wiki (the documents service, or seed without MOCKS_ADDR), buddy's guide, the Playbook
// and the intern checklist.
func newKnowledge(cfg config.Config, data seed.Data, src sources, st *store.Store) *knowledge.Service {
	var list []knowledge.Source
	if src.conn != nil {
		list = append(list, knowledge.NewMockDocs(src.conn))
	} else {
		list = append(list, knowledge.SeedDocs(data.Docs))
	}
	list = append(list, knowledge.Guide(data.Guide))
	if cfg.ChecklistSheet != "" {
		list = append(list, knowledge.Checklist{SheetID: knowledge.SheetID(cfg.ChecklistSheet)})
	}
	if cfg.PlaybookRepo != "" {
		list = append(list, knowledge.Playbook{Repo: cfg.PlaybookRepo, Ref: cfg.PlaybookRef})
	}
	return knowledge.New(st, list...)
}

// handleKnowledgeJobs syncs sources; a failing source is logged and keeps its copy, so the job never fails.
func handleKnowledgeJobs(mux *asynq.ServeMux, k *knowledge.Service) {
	mux.HandleFunc(taskSyncSources, func(ctx context.Context, _ *asynq.Task) error {
		_ = k.Sync(ctx)
		return nil
	})
}

func syncTask() *asynq.Task { return asynq.NewTask(taskSyncSources, nil) }

var syncOpts = []asynq.Option{asynq.Queue("system"), asynq.MaxRetry(0), asynq.Timeout(syncTimeout), asynq.Unique(time.Minute)}

// newScheduler queues periodic system work: source sync always, request updates with FEATURE_REQUESTS.
func newScheduler(cfg config.Config) (*asynq.Scheduler, error) {
	s := asynq.NewScheduler(asynq.RedisClientOpt{Addr: cfg.RedisAddr}, nil)
	if _, err := s.Register(syncEvery, syncTask(), syncOpts...); err != nil {
		return nil, err
	}
	if cfg.FeatureRequests {
		if err := registerRequestTasks(s); err != nil {
			return nil, err
		}
	}
	return s, nil
}

// enqueueSync indexes sources right after start instead of waiting for the first tick.
func enqueueSync(redisAddr string) error {
	c := asynq.NewClient(asynq.RedisClientOpt{Addr: redisAddr})
	defer c.Close()
	_, err := c.Enqueue(syncTask(), syncOpts...)
	if errors.Is(err, asynq.ErrDuplicateTask) {
		return nil
	}
	return err
}
