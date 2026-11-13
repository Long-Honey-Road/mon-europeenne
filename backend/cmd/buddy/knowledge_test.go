package main

import (
	"context"
	"testing"

	"github.com/hibiken/asynq"

	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/domain"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/knowledge"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/store"
	"github.com/Long-Honey-Road/mon-europeenne/backend/seed"
)

// Spec 003 end to end: the worker indexes the documents service, search respects the team.
func TestSyncSourcesThroughTheDocumentsService(t *testing.T) {
	cfg := stackConfig(t)
	cfg.MocksAddr = startMocks(t)
	ctx := context.Background()

	_, mux, cleanup, err := buildWorker(cfg)
	if err != nil {
		t.Fatalf("buildWorker: %v", err)
	}
	defer cleanup()
	if err := mux.ProcessTask(ctx, asynq.NewTask(taskSyncSources, nil)); err != nil {
		t.Fatalf("sync_sources: %v", err)
	}

	pool, err := store.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	k := knowledge.New(store.New(pool))
	intern := domain.Principal{UserID: "u-liza", Role: domain.RoleIntern, TeamID: "t-messenger"}

	hits, conflicts, err := k.Search(ctx, intern, "Сколько нужно аппрувов на ревью?", 5)
	if err != nil || len(hits) == 0 {
		t.Fatalf("search = %v, %v", hits, err)
	}
	if len(conflicts) != 1 || conflicts[0].Topic != "code-review" {
		t.Errorf("conflicts = %+v, want the code-review pair", conflicts)
	}
	if hits, _, _ := k.Search(ctx, intern, "push-уведомления провайдеры ретраи", 5); len(hits) != 0 {
		for _, h := range hits {
			if h.DocID == "notifications-push" {
				t.Errorf("another team's document leaked: %+v", h)
			}
		}
	}
	if hits, _, _ := k.Search(ctx, intern, "как запустить messenger-api локально", 3); len(hits) == 0 || hits[0].DocID != "messenger-dev-setup" {
		t.Errorf("dev setup search = %+v", hits)
	}
}

func TestKnowledgeSourcesFollowConfig(t *testing.T) {
	data, _ := seed.Load(seed.Files)
	cfg := stackConfig(t)
	cfg.PlaybookRepo, cfg.PlaybookRef = "avito-tech/playbook", "abc"
	if k := newKnowledge(cfg, data, sources{}, nil); k == nil {
		t.Fatal("no service")
	}
	if err := enqueueSync(cfg.RedisAddr); err != nil {
		t.Fatalf("enqueueSync: %v", err)
	}
	if err := enqueueSync(cfg.RedisAddr); err != nil {
		t.Errorf("second enqueueSync within a minute: %v", err)
	}
	cfg.FeatureRequests = false
	sched, err := newScheduler(cfg)
	if err != nil {
		t.Fatal(err)
	}
	sched.Shutdown()
}
