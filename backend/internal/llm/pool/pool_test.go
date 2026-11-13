package pool

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/llm"
)

// model is a scriptable endpoint client that counts calls and calls in flight.
type model struct {
	name     string
	answer   func(n int, onDelta func(string)) (llm.Response, error)
	calls    atomic.Int32
	inFlight atomic.Int32
	maxSeen  atomic.Int32
	delay    time.Duration
}

func (m *model) Chat(ctx context.Context, req llm.Request) (llm.Response, error) {
	return m.ChatStream(ctx, req, func(string) {})
}

func (m *model) ChatStream(ctx context.Context, _ llm.Request, onDelta func(string)) (llm.Response, error) {
	n := int(m.calls.Add(1))
	cur := m.inFlight.Add(1)
	defer m.inFlight.Add(-1)
	for {
		seen := m.maxSeen.Load()
		if cur <= seen || m.maxSeen.CompareAndSwap(seen, cur) {
			break
		}
	}
	if m.delay > 0 {
		select {
		case <-time.After(m.delay):
		case <-ctx.Done():
			return llm.Response{}, ctx.Err()
		}
	}
	if m.answer == nil {
		onDelta(m.name)
		return reply(m.name), nil
	}
	return m.answer(n, onDelta)
}

func reply(text string) llm.Response {
	return llm.Response{Message: llm.Message{Role: llm.RoleAssistant, Content: text}}
}

var errDown = fmt.Errorf("%w: connection refused", llm.ErrUnavailable)

func failing(err error) func(int, func(string)) (llm.Response, error) {
	return func(int, func(string)) (llm.Response, error) { return llm.Response{}, err }
}

func newPool(t *testing.T, eps ...Endpoint) *Pool {
	t.Helper()
	p, err := New(eps, Config{Poll: 5 * time.Millisecond})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return p
}

func TestSixTurnsShareOneSlot(t *testing.T) {
	m := &model{name: "pc", delay: 5 * time.Millisecond}
	p := newPool(t, Endpoint{Name: "pc", Client: m, MaxConcurrency: 1})

	var waited atomic.Int32
	var wg sync.WaitGroup
	errs := make(chan error, 6)
	for range 6 {
		wg.Go(func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			client, release, err := p.Acquire(ctx, func() { waited.Add(1) })
			if err != nil {
				errs <- err
				return
			}
			defer release()
			for range 2 {
				if _, err := client.ChatStream(ctx, llm.Request{}, func(string) {}); err != nil {
					errs <- err
					return
				}
			}
		})
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Errorf("turn failed: %v", err)
	}
	if got := m.calls.Load(); got != 12 {
		t.Errorf("calls = %d, want 12", got)
	}
	if got := m.maxSeen.Load(); got != 1 {
		t.Errorf("max calls in flight = %d, want 1", got)
	}
	if got := waited.Load(); got < 1 || got > 5 {
		t.Errorf("turns told to wait = %d, want 1..5", got)
	}
}

func TestTurnKeepsItsEndpoint(t *testing.T) {
	a, b := &model{name: "a"}, &model{name: "b"}
	p := newPool(t, Endpoint{Name: "a", Client: a, MaxConcurrency: 1}, Endpoint{Name: "b", Client: b, MaxConcurrency: 1})

	first, releaseFirst, err := p.Acquire(context.Background(), nil)
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	second, releaseSecond, err := p.Acquire(context.Background(), nil)
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	for range 3 {
		if resp, _ := first.Chat(context.Background(), llm.Request{}); resp.Message.Content != "a" {
			t.Errorf("first turn answered by %q", resp.Message.Content)
		}
		if resp, _ := second.Chat(context.Background(), llm.Request{}); resp.Message.Content != "b" {
			t.Errorf("second turn answered by %q", resp.Message.Content)
		}
	}
	releaseFirst()
	releaseFirst()
	releaseSecond()

	// Both slots are free again: the first endpoint wins.
	c, release, err := p.Acquire(context.Background(), nil)
	if err != nil {
		t.Fatalf("Acquire after release: %v", err)
	}
	defer release()
	if resp, _ := c.Chat(context.Background(), llm.Request{}); resp.Message.Content != "a" {
		t.Errorf("answered by %q, want the first endpoint", resp.Message.Content)
	}
}

func TestFailoverBeforeFirstToken(t *testing.T) {
	a, b := &model{name: "a", answer: failing(errDown)}, &model{name: "b"}
	p := newPool(t, Endpoint{Name: "a", Client: a, MaxConcurrency: 1}, Endpoint{Name: "b", Client: b, MaxConcurrency: 1})

	c, release, err := p.Acquire(context.Background(), nil)
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	var deltas []string
	resp, err := c.ChatStream(context.Background(), llm.Request{}, func(d string) { deltas = append(deltas, d) })
	if err != nil || resp.Message.Content != "b" || len(deltas) != 1 {
		t.Fatalf("ChatStream = %q, %v (deltas %v), want the second endpoint", resp.Message.Content, err, deltas)
	}
	if resp, _ := c.Chat(context.Background(), llm.Request{}); resp.Message.Content != "b" {
		t.Errorf("next call of the turn went to %q", resp.Message.Content)
	}
	release()

	// The failed endpoint is skipped by new turns while the other one is healthy.
	c, release, err = p.Acquire(context.Background(), nil)
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	defer release()
	if resp, _ := c.Chat(context.Background(), llm.Request{}); resp.Message.Content != "b" {
		t.Errorf("new turn went to %q, want the healthy endpoint", resp.Message.Content)
	}
	if got := a.calls.Load(); got != 1 {
		t.Errorf("failed endpoint called %d times, want 1", got)
	}
}

func TestFailoverWaitsForTheOtherEndpoint(t *testing.T) {
	a, b := &model{name: "a", answer: failing(errDown)}, &model{name: "b"}
	p := newPool(t, Endpoint{Name: "a", Client: a, MaxConcurrency: 1}, Endpoint{Name: "b", Client: b, MaxConcurrency: 1})

	c, release, err := p.Acquire(context.Background(), nil)
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	defer release()
	_, releaseBusy, err := p.Acquire(context.Background(), nil)
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	go func() {
		time.Sleep(20 * time.Millisecond)
		releaseBusy()
	}()
	resp, err := c.Chat(context.Background(), llm.Request{})
	if err != nil || resp.Message.Content != "b" {
		t.Fatalf("Chat = %q, %v", resp.Message.Content, err)
	}
}

func TestNoFailoverAfterFirstToken(t *testing.T) {
	a := &model{name: "a", answer: func(_ int, onDelta func(string)) (llm.Response, error) {
		onDelta("Начинаю")
		return llm.Response{}, errDown
	}}
	b := &model{name: "b"}
	p := newPool(t, Endpoint{Name: "a", Client: a, MaxConcurrency: 1}, Endpoint{Name: "b", Client: b, MaxConcurrency: 1})

	c, release, err := p.Acquire(context.Background(), nil)
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	defer release()
	if _, err := c.ChatStream(context.Background(), llm.Request{}, func(string) {}); !errors.Is(err, llm.ErrUnavailable) {
		t.Fatalf("err = %v, want unavailable", err)
	}
	if b.calls.Load() != 0 {
		t.Error("turn switched endpoints after streaming text")
	}
}

func TestNoFailoverMidTurn(t *testing.T) {
	a := &model{name: "a", answer: func(n int, _ func(string)) (llm.Response, error) {
		if n == 1 {
			return reply(""), nil
		}
		return llm.Response{}, errDown
	}}
	b := &model{name: "b"}
	p := newPool(t, Endpoint{Name: "a", Client: a, MaxConcurrency: 1}, Endpoint{Name: "b", Client: b, MaxConcurrency: 1})

	c, release, err := p.Acquire(context.Background(), nil)
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	defer release()
	if _, err := c.Chat(context.Background(), llm.Request{}); err != nil {
		t.Fatalf("first call: %v", err)
	}
	if _, err := c.Chat(context.Background(), llm.Request{}); !errors.Is(err, llm.ErrUnavailable) {
		t.Fatalf("err = %v, want unavailable", err)
	}
	if b.calls.Load() != 0 {
		t.Error("turn switched endpoints in the middle of its tool loop")
	}
}

func TestNoFailoverOnRejectedRequest(t *testing.T) {
	rejected := errors.New("llm request rejected: 400")
	a, b := &model{name: "a", answer: failing(rejected)}, &model{name: "b"}
	p := newPool(t, Endpoint{Name: "a", Client: a, MaxConcurrency: 1}, Endpoint{Name: "b", Client: b, MaxConcurrency: 1})

	c, release, err := p.Acquire(context.Background(), nil)
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	defer release()
	if _, err := c.Chat(context.Background(), llm.Request{}); !errors.Is(err, rejected) {
		t.Fatalf("err = %v", err)
	}
	if b.calls.Load() != 0 {
		t.Error("a rejected request moved to another endpoint")
	}
}

func TestLastEndpointDownStillServes(t *testing.T) {
	a := &model{name: "a", answer: func(n int, _ func(string)) (llm.Response, error) {
		if n == 1 {
			return llm.Response{}, errDown
		}
		return reply("a"), nil
	}}
	p := newPool(t, Endpoint{Name: "a", Client: a, MaxConcurrency: 1})

	c, release, err := p.Acquire(context.Background(), nil)
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	if _, err := c.Chat(context.Background(), llm.Request{}); !errors.Is(err, llm.ErrUnavailable) {
		t.Fatalf("err = %v, want unavailable", err)
	}
	release()

	resp, err := p.Chat(context.Background(), llm.Request{})
	if err != nil || resp.Message.Content != "a" {
		t.Fatalf("Chat after the only endpoint failed = %q, %v", resp.Message.Content, err)
	}
}

func TestAcquireGivesUpWhenContextEnds(t *testing.T) {
	p := newPool(t, Endpoint{Name: "a", Client: &model{name: "a"}, MaxConcurrency: 1})
	_, release, err := p.Acquire(context.Background(), nil)
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	defer release()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	waited := 0
	if _, _, err := p.Acquire(ctx, func() { waited++ }); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want deadline exceeded", err)
	}
	if waited != 1 {
		t.Errorf("onWait called %d times, want 1", waited)
	}
}

func TestFailoverGivesUpWhenContextEnds(t *testing.T) {
	a := &model{name: "a", answer: failing(errDown)}
	p := newPool(t, Endpoint{Name: "a", Client: a, MaxConcurrency: 1}, Endpoint{Name: "b", Client: &model{name: "b"}, MaxConcurrency: 1})
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()

	c, release, err := p.Acquire(ctx, nil)
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	defer release()
	_, releaseOther, err := p.Acquire(context.Background(), nil)
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	defer releaseOther()
	if _, err := c.Chat(ctx, llm.Request{}); !errors.Is(err, llm.ErrUnavailable) {
		t.Fatalf("err = %v, want the endpoint's own error", err)
	}
	if _, err := c.Chat(context.Background(), llm.Request{}); !errors.Is(err, llm.ErrUnavailable) {
		t.Fatalf("call on a lease without an endpoint: %v", err)
	}
}

func TestPoolAnswersSingleCalls(t *testing.T) {
	p := newPool(t, Endpoint{Name: "a", Client: &model{name: "a"}})
	if resp, err := p.Chat(context.Background(), llm.Request{}); err != nil || resp.Message.Content != "a" {
		t.Errorf("Chat = %q, %v", resp.Message.Content, err)
	}
	var got string
	if _, err := p.ChatStream(context.Background(), llm.Request{}, func(d string) { got += d }); err != nil || got != "a" {
		t.Errorf("ChatStream deltas = %q, %v", got, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := p.Chat(ctx, llm.Request{}); err != nil && !errors.Is(err, context.Canceled) {
		t.Errorf("Chat on a cancelled context: %v", err)
	}
	if names := p.Endpoints(); len(names) != 1 || names[0].Name != "a" || names[0].MaxConcurrency != 1 {
		t.Errorf("Endpoints = %+v", names)
	}
}

func TestNewValidatesEndpoints(t *testing.T) {
	m := &model{name: "a"}
	for name, eps := range map[string][]Endpoint{
		"none":      nil,
		"no client": {{Name: "a"}},
		"duplicate": {{Name: "a", Client: m}, {Name: "a", Client: m}},
		"negative":  {{Name: "a", Client: m, MaxConcurrency: -1}},
	} {
		if _, err := New(eps, Config{}); !errors.Is(err, ErrConfig) {
			t.Errorf("%s: err = %v, want ErrConfig", name, err)
		}
	}
	p, err := New([]Endpoint{{Client: m}, {Client: m}}, Config{})
	if err != nil {
		t.Fatalf("unnamed endpoints: %v", err)
	}
	if eps := p.Endpoints(); eps[0].Name == eps[1].Name {
		t.Errorf("unnamed endpoints share a name: %+v", eps)
	}
}

// countingSlots shows a custom Slots (e.g. a Redis semaphore) is what the pool counts with.
type countingSlots struct {
	mu       sync.Mutex
	acquired int
	inner    Slots
}

func (c *countingSlots) TryAcquire(key string, limit int) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	ok := c.inner.TryAcquire(key, limit)
	if ok {
		c.acquired++
	}
	return ok
}

func (c *countingSlots) Release(key string) { c.inner.Release(key) }

func TestPoolCountsWithGivenSlots(t *testing.T) {
	slots := &countingSlots{inner: NewMemorySlots()}
	p, err := New([]Endpoint{{Name: "a", Client: &model{name: "a"}}}, Config{Slots: slots})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := p.Chat(context.Background(), llm.Request{}); err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if slots.acquired != 1 {
		t.Errorf("slots acquired %d times, want 1", slots.acquired)
	}
}
