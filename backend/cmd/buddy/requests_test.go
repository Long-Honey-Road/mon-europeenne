package main

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/hibiken/asynq"
)

// S-02 wiring end to end: submit -> mirror -> request_status on SSE -> today blockers -> advance job.
func TestRequestSubmissionEndToEnd(t *testing.T) {
	cfg := stackConfig(t)
	cfg.FeatureRequests = true
	cfg.MocksAddr = startMocks(t)
	ctx := context.Background()

	h, cleanAPI, err := buildAPI(ctx, cfg)
	if err != nil {
		t.Fatalf("buildAPI: %v", err)
	}
	defer cleanAPI()
	api := httptest.NewServer(h)
	defer api.Close()

	_, login := request(t, api.URL, http.MethodPost, "/v1/public/auth/login", "", `{"login":"intern","password":"intern-demo"}`)
	token := login["token"].(string)
	_, created := request(t, api.URL, http.MethodPost, "/v1/private/threads", token, `{"type":"THREAD_TYPE_REQUEST"}`)
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
	_, _ = reader.ReadString('\n')

	code, sub := request(t, api.URL, http.MethodPost, "/v1/private/requests", token,
		`{"thread_id":"`+thread+`","type_key":"access.jira_project","fields":{"project_key":"MSG","level":"read","justification":"первая задача в проекте MSG"}}`)
	if code != http.StatusOK {
		t.Fatalf("submit: %d %v", code, sub)
	}
	r := sub["request"].(map[string]any)
	if !strings.HasPrefix(r["id"].(string), "REQ-") || r["status"] != "REQUEST_STATUS_PENDING" || r["thread_id"] != thread {
		t.Fatalf("request = %v", r)
	}

	var event map[string]any
	for event == nil {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatalf("SSE read: %v", err)
		}
		if strings.TrimSpace(line) == "event: request_status" {
			data, _ := reader.ReadString('\n')
			_ = json.Unmarshal([]byte(strings.TrimPrefix(strings.TrimSpace(data), "data: ")), &event)
		}
	}
	if event["request_id"] != r["id"] || event["status"] != "pending" {
		t.Fatalf("event = %v", event)
	}

	_, today := request(t, api.URL, http.MethodGet, "/v1/private/today", token, "")
	kinds := map[string]bool{}
	for _, b := range today["blockers"].([]any) {
		kinds[b.(map[string]any)["kind"].(string)] = true
	}
	if !kinds["access_missing"] || !kinds["request"] {
		t.Fatalf("today blockers = %v", today["blockers"])
	}
	if code, bad := request(t, api.URL, http.MethodPost, "/v1/private/requests", token, `{"type_key":"access.vpn","fields":{}}`); code != http.StatusBadRequest {
		t.Fatalf("invalid fields: %d %v", code, bad)
	}

	_, mux, cleanWorker, err := buildWorker(cfg)
	if err != nil {
		t.Fatalf("buildWorker: %v", err)
	}
	defer cleanWorker()
	if err := mux.ProcessTask(ctx, asynq.NewTask(taskAdvanceRequests, nil)); err != nil {
		t.Fatalf("advance_requests: %v", err)
	}
	sched, err := newScheduler(cfg)
	if err != nil {
		t.Fatalf("scheduler: %v", err)
	}
	sched.Shutdown()
}
