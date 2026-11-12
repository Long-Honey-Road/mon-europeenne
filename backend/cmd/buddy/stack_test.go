package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/config"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/testdb"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/testredis"
)

// stackConfig is a migrated and seeded database plus Redis, with the fake model.
func stackConfig(t *testing.T) config.Config {
	t.Helper()
	dsn := testdb.Fresh(t)
	t.Setenv("DATABASE_URL", dsn)
	var out, errOut bytes.Buffer
	for _, args := range [][]string{{"migrate", "up"}, {"seed"}} {
		if code := run(args, &out, &errOut); code != exitOK {
			t.Fatalf("%v: %s", args, errOut.String())
		}
	}
	return config.Config{
		LLM:           config.LLM{Provider: config.ProviderFake},
		DatabaseURL:   dsn,
		RedisAddr:     testredis.Addr(t),
		SessionSecret: "test-session-secret-0123456789",
	}
}

func request(t *testing.T, base, method, path, token, body string) (int, map[string]any) {
	t.Helper()
	req, _ := http.NewRequestWithContext(context.Background(), method, base+path, strings.NewReader(body))
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

// The A-04 path end to end: message -> 202 -> worker -> SSE -> done -> saved history.
func TestChatEndToEndWithFakeModel(t *testing.T) {
	cfg := stackConfig(t)
	ctx := context.Background()

	h, cleanAPI, err := buildAPI(ctx, cfg)
	if err != nil {
		t.Fatalf("buildAPI: %v", err)
	}
	defer cleanAPI()
	api := httptest.NewServer(h)
	defer api.Close()

	srv, mux, cleanWorker, err := buildWorker(cfg)
	if err != nil {
		t.Fatalf("buildWorker: %v", err)
	}
	defer cleanWorker()
	if err := srv.Start(mux); err != nil {
		t.Fatalf("worker start: %v", err)
	}
	defer srv.Shutdown()

	_, login := request(t, api.URL, http.MethodPost, "/v1/public/auth/login", "", `{"login":"intern","password":"intern-demo"}`)
	token := login["token"].(string)
	_, created := request(t, api.URL, http.MethodPost, "/v1/private/threads", token, `{"type":"THREAD_TYPE_GENERAL"}`)
	thread := created["thread"].(map[string]any)["id"].(string)

	sseCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(sseCtx, http.MethodGet, api.URL+"/v1/private/threads/"+thread+"/events", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	sse, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("open SSE: %v", err)
	}
	defer sse.Body.Close()
	reader := bufio.NewReader(sse.Body)
	if line, _ := reader.ReadString('\n'); !strings.HasPrefix(line, ": connected") {
		t.Fatalf("first SSE line %q", line)
	}

	code, sent := request(t, api.URL, http.MethodPost, "/v1/private/threads/"+thread+"/messages", token, `{"content":"Как поднять dev?"}`)
	if code != http.StatusAccepted {
		t.Fatalf("send: %d %v", code, sent)
	}

	var seen []string
	var done map[string]any
	for done == nil {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatalf("SSE read: %v (seen %v)", err, seen)
		}
		if name, ok := strings.CutPrefix(strings.TrimSpace(line), "event: "); ok {
			seen = append(seen, name)
			data, _ := reader.ReadString('\n')
			if name == "done" {
				_ = json.Unmarshal([]byte(strings.TrimPrefix(strings.TrimSpace(data), "data: ")), &done)
			}
		}
	}
	if seen[0] != "delta" || done["job_id"] != sent["job_id"] || !strings.Contains(done["text"].(string), "LLM_PROVIDER=fake") {
		t.Fatalf("seen %v, done %v, sent %v", seen, done, sent)
	}

	_, history := request(t, api.URL, http.MethodGet, "/v1/private/threads/"+thread+"/messages", token, "")
	msgs := history["messages"].([]any)
	if len(msgs) != 2 || msgs[1].(map[string]any)["id"] != done["message_id"] {
		t.Fatalf("history = %v", history)
	}
}

func TestWorkerRequiresDatabaseAndRedis(t *testing.T) {
	for name, env := range map[string][2]string{
		"no database": {"", "redis:6379"},
		"no redis":    {"postgres://x", ""},
	} {
		t.Run(name, func(t *testing.T) {
			t.Setenv("DATABASE_URL", env[0])
			t.Setenv("REDIS_ADDR", env[1])
			var stdout, stderr bytes.Buffer
			if code := run([]string{"worker"}, &stdout, &stderr); code != exitError {
				t.Fatalf("exit %d, want %d", code, exitError)
			}
		})
	}
}
