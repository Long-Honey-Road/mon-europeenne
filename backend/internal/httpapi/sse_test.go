package httpapi

import (
	"bufio"
	"context"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/stream"
)

// memBus is an in-memory stream.Bus that reports subscribe and stop.
type memBus struct {
	mu         sync.Mutex
	subs       map[string]chan stream.Event
	subscribed chan string
	stopped    chan string
}

func newMemBus() *memBus {
	return &memBus{subs: map[string]chan stream.Event{}, subscribed: make(chan string, 4), stopped: make(chan string, 4)}
}

func (b *memBus) Subscribe(_ context.Context, threadID string) (<-chan stream.Event, func(), error) {
	b.mu.Lock()
	ch := make(chan stream.Event, 8)
	b.subs[threadID] = ch
	b.mu.Unlock()
	b.subscribed <- threadID
	var once sync.Once
	return ch, func() { once.Do(func() { b.stopped <- threadID }) }, nil
}

func (b *memBus) publish(threadID string, e stream.Event) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.subs[threadID] <- e
}

func sseServer(t *testing.T, heartbeat time.Duration) (bus *memBus, base, intern, manager string) {
	t.Helper()
	bus = newMemBus()
	d := deps(t)
	d.Threads, d.Jobs, d.Bus, d.Heartbeat = newMemStore(), &memJobs{}, bus, heartbeat
	s := newServerWith(t, d)
	return bus, s.URL, login(t, s, "intern", "intern-demo"), login(t, s, "manager", "manager-demo")
}

func openStream(t *testing.T, url, token string) (*http.Response, context.CancelFunc) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		cancel()
		t.Fatalf("GET %s: %v", url, err)
	}
	return resp, cancel
}

func threadID(t *testing.T, bus *memBus) string {
	t.Helper()
	select {
	case id := <-bus.subscribed:
		return id
	case <-time.After(5 * time.Second):
		t.Fatal("no subscription")
	}
	return ""
}

func TestStreamRelaysEventsAsSSE(t *testing.T) {
	bus, base, intern, _ := sseServer(t, time.Hour)
	thread, _ := newThreadID(t, base, intern)
	resp, cancel := openStream(t, base+"/v1/private/threads/"+thread+"/events", intern)
	defer cancel()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK || resp.Header.Get("Content-Type") != "text/event-stream" {
		t.Fatalf("status %d, content type %q", resp.StatusCode, resp.Header.Get("Content-Type"))
	}
	id := threadID(t, bus)
	bus.publish(id, stream.Event{Type: "delta", Data: map[string]any{"job_id": "j1", "text": "При"}})
	bus.publish(id, stream.Event{Type: "done", Data: map[string]any{"job_id": "j1", "message_id": "m2"}})

	got := readEvents(t, resp, 2)
	if got[0] != `event: delta|data: {"job_id":"j1","text":"При"}` || got[1] != `event: done|data: {"job_id":"j1","message_id":"m2"}` {
		t.Fatalf("events = %q", got)
	}
}

func TestStreamSendsHeartbeats(t *testing.T) {
	bus, base, intern, _ := sseServer(t, 30*time.Millisecond)
	id, _ := newThreadID(t, base, intern)
	resp, cancel := openStream(t, base+"/v1/private/threads/"+id+"/events", intern)
	defer cancel()
	defer resp.Body.Close()
	threadID(t, bus)

	r := bufio.NewReader(resp.Body)
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		line, err := r.ReadString('\n')
		if err != nil {
			t.Fatalf("read: %v", err)
		}
		if strings.HasPrefix(line, ": ping") {
			return
		}
	}
	t.Fatal("no heartbeat")
}

func TestStreamStopsWhenClientLeaves(t *testing.T) {
	bus, base, intern, _ := sseServer(t, time.Hour)
	id, _ := newThreadID(t, base, intern)
	resp, cancel := openStream(t, base+"/v1/private/threads/"+id+"/events", intern)
	threadID(t, bus)

	cancel()
	resp.Body.Close()
	select {
	case <-bus.stopped:
	case <-time.After(5 * time.Second):
		t.Fatal("subscription not stopped after the client left")
	}
}

func TestStreamOfSomeoneElsesThreadIsNotFound(t *testing.T) {
	bus, base, intern, manager := sseServer(t, time.Hour)
	id, _ := newThreadID(t, base, intern)

	for name, token := range map[string]string{"other user": manager, "no session": ""} {
		resp, cancel := openStream(t, base+"/v1/private/threads/"+id+"/events", token)
		resp.Body.Close()
		cancel()
		want := http.StatusNotFound
		if token == "" {
			want = http.StatusUnauthorized
		}
		if resp.StatusCode != want {
			t.Errorf("%s: status %d, want %d", name, resp.StatusCode, want)
		}
	}
	select {
	case id := <-bus.subscribed:
		t.Fatalf("subscribed to %s for a refused request", id)
	default:
	}
}

func newThreadID(t *testing.T, base, token string) (string, error) {
	t.Helper()
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, base+"/v1/private/threads",
		strings.NewReader(`{"type":"THREAD_TYPE_GENERAL"}`))
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	sc := bufio.NewScanner(resp.Body)
	sc.Scan()
	body := sc.Text()
	i := strings.Index(body, `"id":"`) + len(`"id":"`)
	return body[i : i+36], nil
}

// readEvents returns n SSE events as "event: x|data: y", skipping comments.
func readEvents(t *testing.T, resp *http.Response, n int) []string {
	t.Helper()
	r := bufio.NewReader(resp.Body)
	var out []string
	var cur []string
	for len(out) < n {
		line, err := r.ReadString('\n')
		if err != nil {
			t.Fatalf("read: %v (got %q)", err, out)
		}
		line = strings.TrimRight(line, "\n")
		switch {
		case strings.HasPrefix(line, ":"):
		case line == "":
			if len(cur) > 0 {
				out = append(out, strings.Join(cur, "|"))
				cur = nil
			}
		default:
			cur = append(cur, line)
		}
	}
	return out
}
