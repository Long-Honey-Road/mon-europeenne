package httpapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/domain"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/jobs"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/store"
)

// memStore is an in-memory Threads; the Postgres version is tested in internal/store.
type memStore struct {
	mu       sync.Mutex
	seq      int
	threads  []domain.Thread
	messages map[string][]domain.Message
	ratings  []string
}

func newMemStore() *memStore { return &memStore{messages: map[string][]domain.Message{}} }

func (m *memStore) id() string {
	m.seq++
	return fmt.Sprintf("00000000-0000-0000-0000-%012d", m.seq)
}

func (m *memStore) CreateThread(_ context.Context, userID string, typ domain.ThreadType, title string, pin domain.ThreadPin) (domain.Thread, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	t := domain.Thread{ID: m.id(), UserID: userID, Type: typ, Title: title, Pin: pin, CreatedAt: time.Now()}
	m.threads = append(m.threads, t)
	return t, nil
}

func (m *memStore) Thread(_ context.Context, userID, id string) (domain.Thread, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, t := range m.threads {
		if t.ID == id && t.UserID == userID {
			return t, nil
		}
	}
	return domain.Thread{}, store.ErrNotFound
}

func (m *memStore) Threads(_ context.Context, userID string) ([]domain.Thread, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []domain.Thread
	for i := len(m.threads) - 1; i >= 0; i-- {
		if m.threads[i].UserID == userID {
			out = append(out, m.threads[i])
		}
	}
	return out, nil
}

func (m *memStore) AddMessage(_ context.Context, msg domain.Message) (domain.Message, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	msg.ID, msg.CreatedAt = m.id(), time.Now()
	m.messages[msg.ThreadID] = append(m.messages[msg.ThreadID], msg)
	return msg, nil
}

func (m *memStore) Messages(_ context.Context, threadID string, limit int) ([]domain.Message, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	all := m.messages[threadID]
	if len(all) > limit {
		all = all[len(all)-limit:]
	}
	return append([]domain.Message(nil), all...), nil
}

func (m *memStore) RateMessage(_ context.Context, userID, messageID, value, note string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, t := range m.threads {
		if t.UserID != userID {
			continue
		}
		for _, msg := range m.messages[t.ID] {
			if msg.ID == messageID && msg.Role == domain.MessageAssistant {
				m.ratings = append(m.ratings, messageID+":"+value+":"+note)
				return nil
			}
		}
	}
	return store.ErrNotFound
}

type memJobs struct {
	mu    sync.Mutex
	turns []jobs.ChatTurn
	fail  bool
}

func (j *memJobs) EnqueueChatTurn(_ context.Context, t jobs.ChatTurn) (string, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.fail {
		return "", errors.New("redis down")
	}
	j.turns = append(j.turns, t)
	return fmt.Sprintf("job-%d", len(j.turns)), nil
}

func threadServer(t *testing.T) (*memStore, *memJobs, func(method, path, user, body string) (int, map[string]any)) {
	t.Helper()
	st, jb := newMemStore(), &memJobs{}
	d := deps(t)
	d.Threads, d.Jobs = st, jb
	s := newServerWith(t, d)
	tokens := map[string]string{}
	return st, jb, func(method, path, user, body string) (int, map[string]any) {
		if _, ok := tokens[user]; !ok {
			tokens[user] = login(t, s, user, user+"-demo")
		}
		return call(t, s, method, path, tokens[user], body)
	}
}

func TestCreateAndListThreads(t *testing.T) {
	_, _, do := threadServer(t)

	code, body := do(http.MethodPost, "/v1/private/threads", "intern", `{"type":"THREAD_TYPE_GENERAL","title":"Вопросы"}`)
	if code != http.StatusOK {
		t.Fatalf("create: %d %v", code, body)
	}
	th := body["thread"].(map[string]any)
	if th["type"] != "THREAD_TYPE_GENERAL" || th["title"] != "Вопросы" || th["created_at"] == "" {
		t.Errorf("thread = %v", th)
	}

	code, body = do(http.MethodGet, "/v1/private/threads", "intern", "")
	list := body["threads"].([]any)
	if code != http.StatusOK || len(list) != 1 || list[0].(map[string]any)["id"] != th["id"] {
		t.Fatalf("list: %d %v", code, body)
	}
	if _, body = do(http.MethodGet, "/v1/private/threads", "manager", ""); len(body["threads"].([]any)) != 0 {
		t.Errorf("manager sees intern's threads: %v", body)
	}
}

func TestCreateThreadValidatesInput(t *testing.T) {
	_, _, do := threadServer(t)
	for name, body := range map[string]string{
		"no type":          `{"title":"x"}`,
		"unknown pin key":  `{"type":"THREAD_TYPE_GENERAL","pin":{"color":"red"}}`,
		"task without key": `{"type":"THREAD_TYPE_TASK","pin":{"task":{"repo":"messenger-api"}}}`,
	} {
		if code, resp := do(http.MethodPost, "/v1/private/threads", "intern", body); code != http.StatusBadRequest {
			t.Errorf("%s: %d %v, want 400", name, code, resp)
		}
	}
}

func TestPlanThreadIsOnePerUser(t *testing.T) {
	_, _, do := threadServer(t)
	_, first := do(http.MethodPost, "/v1/private/threads", "intern", `{"type":"THREAD_TYPE_PLAN"}`)
	_, second := do(http.MethodPost, "/v1/private/threads", "intern", `{"type":"THREAD_TYPE_PLAN"}`)

	if first["thread"].(map[string]any)["id"] != second["thread"].(map[string]any)["id"] {
		t.Fatalf("two plan threads: %v / %v", first, second)
	}
}

func TestTaskThreadKeepsItsPin(t *testing.T) {
	_, _, do := threadServer(t)
	code, body := do(http.MethodPost, "/v1/private/threads", "intern",
		`{"type":"THREAD_TYPE_TASK","title":"Первая задача","pin":{"task":{"key":"MSG-142","repo":"messenger-api"}}}`)
	if code != http.StatusOK {
		t.Fatalf("create: %d %v", code, body)
	}
	id := body["thread"].(map[string]any)["id"].(string)

	_, got := do(http.MethodGet, "/v1/private/threads/"+id, "intern", "")
	task := got["thread"].(map[string]any)["pin"].(map[string]any)["task"].(map[string]any)
	if task["key"] != "MSG-142" || task["repo"] != "messenger-api" {
		t.Fatalf("pin = %v", got)
	}
}

func TestThreadsOfOthersAreNotFound(t *testing.T) {
	_, jb, do := threadServer(t)
	_, body := do(http.MethodPost, "/v1/private/threads", "intern", `{"type":"THREAD_TYPE_GENERAL"}`)
	id := body["thread"].(map[string]any)["id"].(string)

	for _, req := range [][2]string{
		{http.MethodGet, "/v1/private/threads/" + id},
		{http.MethodGet, "/v1/private/threads/" + id + "/messages"},
		{http.MethodPost, "/v1/private/threads/" + id + "/messages"},
	} {
		if code, resp := do(req[0], req[1], "manager", `{"content":"привет"}`); code != http.StatusNotFound {
			t.Errorf("%s %s as manager: %d %v, want 404", req[0], req[1], code, resp)
		}
	}
	if len(jb.turns) != 0 {
		t.Errorf("turn queued for someone else's thread: %v", jb.turns)
	}
}

func TestSendMessageQueuesATurn(t *testing.T) {
	st, jb, do := threadServer(t)
	_, body := do(http.MethodPost, "/v1/private/threads", "intern", `{"type":"THREAD_TYPE_GENERAL"}`)
	id := body["thread"].(map[string]any)["id"].(string)

	code, resp := do(http.MethodPost, "/v1/private/threads/"+id+"/messages", "intern",
		`{"content":"Как поднять dev?","mode":"ANSWER_MODE_DETAILED"}`)

	if code != http.StatusAccepted || resp["message_id"] == "" || resp["job_id"] != "job-1" {
		t.Fatalf("send: %d %v, want 202", code, resp)
	}
	want := jobs.ChatTurn{ThreadID: id, UserID: "u-liza", MessageID: resp["message_id"].(string), Mode: "detailed"}
	if !slices.Equal(jb.turns, []jobs.ChatTurn{want}) {
		t.Errorf("queued %+v, want %+v", jb.turns, want)
	}
	saved := st.messages[id]
	if len(saved) != 1 || saved[0].Role != domain.MessageUser || saved[0].Content != "Как поднять dev?" {
		t.Errorf("saved = %+v", saved)
	}
}

func TestSendMessageValidatesContent(t *testing.T) {
	_, jb, do := threadServer(t)
	_, body := do(http.MethodPost, "/v1/private/threads", "intern", `{"type":"THREAD_TYPE_GENERAL"}`)
	id := body["thread"].(map[string]any)["id"].(string)

	for name, content := range map[string]string{"empty": "  ", "too long": strings.Repeat("я", 4001)} {
		if code, _ := do(http.MethodPost, "/v1/private/threads/"+id+"/messages", "intern", `{"content":"`+content+`"}`); code != http.StatusBadRequest {
			t.Errorf("%s: %d, want 400", name, code)
		}
	}
	if len(jb.turns) != 0 {
		t.Errorf("invalid messages queued: %v", jb.turns)
	}
}

func TestSendMessageWhenQueueIsDown(t *testing.T) {
	_, jb, do := threadServer(t)
	jb.fail = true
	_, body := do(http.MethodPost, "/v1/private/threads", "intern", `{"type":"THREAD_TYPE_GENERAL"}`)
	id := body["thread"].(map[string]any)["id"].(string)

	if code, _ := do(http.MethodPost, "/v1/private/threads/"+id+"/messages", "intern", `{"content":"привет"}`); code != http.StatusServiceUnavailable {
		t.Fatalf("status %d, want 503", code)
	}
}

func TestListMessagesOldestFirstWithBlocks(t *testing.T) {
	st, _, do := threadServer(t)
	_, body := do(http.MethodPost, "/v1/private/threads", "intern", `{"type":"THREAD_TYPE_GENERAL"}`)
	id := body["thread"].(map[string]any)["id"].(string)
	_, _ = st.AddMessage(context.Background(), domain.Message{ThreadID: id, Role: domain.MessageUser, Content: "вопрос"})
	_, _ = st.AddMessage(context.Background(), domain.Message{ThreadID: id, Role: domain.MessageAssistant, Content: "ответ [1]",
		Blocks: []byte(`[{"type":"sources","items":[{"ref":1,"source_id":"chunk-dev-1"}]}]`)})

	code, resp := do(http.MethodGet, "/v1/private/threads/"+id+"/messages?limit=10", "intern", "")

	msgs, _ := resp["messages"].([]any)
	if code != http.StatusOK || len(msgs) != 2 {
		t.Fatalf("list: %d %v", code, resp)
	}
	answer := msgs[1].(map[string]any)
	block := answer["blocks"].([]any)[0].(map[string]any)
	if answer["role"] != "MESSAGE_ROLE_ASSISTANT" || answer["text"] != "ответ [1]" || block["type"] != "sources" {
		t.Errorf("answer = %v", answer)
	}
}

func TestRateMessage(t *testing.T) {
	st, _, do := threadServer(t)
	_, created := do(http.MethodPost, "/v1/private/threads", "intern", `{"type":"THREAD_TYPE_GENERAL"}`)
	thread := created["thread"].(map[string]any)["id"].(string)
	answer, _ := st.AddMessage(context.Background(), domain.Message{ThreadID: thread, Role: domain.MessageAssistant, Content: "!"})

	code, body := do(http.MethodPost, "/v1/private/messages/"+answer.ID+"/feedback", "intern", `{"value":"RATING_UP","note":"помогло"}`)
	if code != http.StatusOK || len(st.ratings) != 1 || st.ratings[0] != answer.ID+":up:помогло" {
		t.Fatalf("code %d %v, ratings %v", code, body, st.ratings)
	}
	for name, tc := range map[string]struct {
		user, body string
		want       int
	}{
		"no value":       {"intern", `{}`, http.StatusBadRequest},
		"long note":      {"intern", `{"value":"RATING_DOWN","note":"` + strings.Repeat("я", maxFeedbackNote+1) + `"}`, http.StatusBadRequest},
		"someone else's": {"manager", `{"value":"RATING_DOWN"}`, http.StatusNotFound},
	} {
		t.Run(name, func(t *testing.T) {
			if code, body := do(http.MethodPost, "/v1/private/messages/"+answer.ID+"/feedback", tc.user, tc.body); code != tc.want {
				t.Fatalf("code %d, want %d: %v", code, tc.want, body)
			}
		})
	}
}
