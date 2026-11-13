// Package pool spreads agent turns over several model endpoints, each with its own concurrency limit.
package pool

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"sync"
	"time"

	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/llm"
)

// ErrConfig means the endpoint list cannot form a pool.
var ErrConfig = errors.New("invalid llm pool config")

// Endpoint is one model source: a client and how many turns it serves at once.
type Endpoint struct {
	Name   string
	Client llm.Client
	// MaxConcurrency is the number of turns served at once; zero means one.
	MaxConcurrency int
}

// Slots counts busy turns per endpoint. The in-process MemorySlots fits one worker;
// a Redis semaphore with the same methods lets several workers share the limits.
type Slots interface {
	TryAcquire(key string, limit int) bool
	Release(key string)
}

type Config struct {
	// Cooldown keeps a failed endpoint away from new turns while another one is healthy; 30 s if zero.
	Cooldown time.Duration
	// Poll is how often a waiting turn re-checks slots, for slots freed outside this process; 1 s if zero.
	Poll time.Duration
	// Slots counts busy turns; in-process if nil.
	Slots Slots
}

// Pool hands out one endpoint per turn and is itself an llm.Client for single calls.
type Pool struct {
	endpoints []Endpoint
	cfg       Config

	mu        sync.Mutex
	freed     chan struct{}
	downUntil []time.Time
}

func New(endpoints []Endpoint, cfg Config) (*Pool, error) {
	if len(endpoints) == 0 {
		return nil, fmt.Errorf("%w: no endpoints", ErrConfig)
	}
	if cfg.Cooldown == 0 {
		cfg.Cooldown = 30 * time.Second
	}
	if cfg.Poll == 0 {
		cfg.Poll = time.Second
	}
	if cfg.Slots == nil {
		cfg.Slots = NewMemorySlots()
	}
	eps := make([]Endpoint, len(endpoints))
	names := map[string]bool{}
	for i, ep := range endpoints {
		if ep.Name == "" {
			ep.Name = fmt.Sprintf("llm-%d", i+1)
		}
		switch {
		case ep.Client == nil:
			return nil, fmt.Errorf("%w: endpoint %q has no client", ErrConfig, ep.Name)
		case ep.MaxConcurrency < 0:
			return nil, fmt.Errorf("%w: endpoint %q max concurrency %d", ErrConfig, ep.Name, ep.MaxConcurrency)
		case names[ep.Name]:
			return nil, fmt.Errorf("%w: endpoint name %q repeats", ErrConfig, ep.Name)
		}
		if ep.MaxConcurrency == 0 {
			ep.MaxConcurrency = 1
		}
		names[ep.Name] = true
		eps[i] = ep
	}
	return &Pool{endpoints: eps, cfg: cfg, freed: make(chan struct{}), downUntil: make([]time.Time, len(eps))}, nil
}

// Endpoints lists the endpoints with defaults applied, e.g. to probe each one.
func (p *Pool) Endpoints() []Endpoint {
	return append([]Endpoint(nil), p.endpoints...)
}

// Acquire waits for a free endpoint and holds it for a whole turn; onWait is called once if the turn has to queue.
// The returned client fails over to another endpoint only before the turn's first answer; release is idempotent.
func (p *Pool) Acquire(ctx context.Context, onWait func()) (llm.Client, func(), error) {
	i, err := p.acquire(ctx, nil, onWait)
	if err != nil {
		return nil, nil, err
	}
	l := &lease{pool: p, current: i, tried: map[int]bool{i: true}}
	return l, l.release, nil
}

func (p *Pool) Chat(ctx context.Context, req llm.Request) (llm.Response, error) {
	c, release, err := p.Acquire(ctx, nil)
	if err != nil {
		return llm.Response{}, err
	}
	defer release()
	return c.Chat(ctx, req)
}

func (p *Pool) ChatStream(ctx context.Context, req llm.Request, onDelta func(string)) (llm.Response, error) {
	c, release, err := p.Acquire(ctx, nil)
	if err != nil {
		return llm.Response{}, err
	}
	defer release()
	return c.ChatStream(ctx, req, onDelta)
}

// acquire takes a slot on the first free endpoint not in skip, preferring healthy ones, and waits if all are busy.
func (p *Pool) acquire(ctx context.Context, skip map[int]bool, onWait func()) (int, error) {
	waited := false
	for {
		if err := ctx.Err(); err != nil {
			return -1, err
		}
		p.mu.Lock()
		freed := p.freed
		p.mu.Unlock()
		if i, ok := p.tryAcquire(skip); ok {
			return i, nil
		}
		if !waited && onWait != nil {
			onWait()
		}
		waited = true
		timer := time.NewTimer(p.cfg.Poll)
		select {
		case <-ctx.Done():
			timer.Stop()
			return -1, ctx.Err()
		case <-freed:
		case <-timer.C:
		}
		timer.Stop()
	}
}

func (p *Pool) tryAcquire(skip map[int]bool) (int, bool) {
	now := time.Now()
	p.mu.Lock()
	var healthy, down []int
	for i := range p.endpoints {
		switch {
		case skip[i]:
		case now.Before(p.downUntil[i]):
			down = append(down, i)
		default:
			healthy = append(healthy, i)
		}
	}
	p.mu.Unlock()

	// A failed endpoint is used only when no healthy one is left to wait for.
	candidates := healthy
	if len(candidates) == 0 {
		candidates = down
	}
	for _, i := range candidates {
		ep := p.endpoints[i]
		if p.cfg.Slots.TryAcquire(ep.Name, ep.MaxConcurrency) {
			return i, true
		}
	}
	return -1, false
}

func (p *Pool) release(i int) {
	p.cfg.Slots.Release(p.endpoints[i].Name)
	p.mu.Lock()
	close(p.freed)
	p.freed = make(chan struct{})
	p.mu.Unlock()
}

func (p *Pool) markDown(i int) {
	p.mu.Lock()
	p.downUntil[i] = time.Now().Add(p.cfg.Cooldown)
	p.mu.Unlock()
}

// lease is one turn's hold on an endpoint.
type lease struct {
	pool *Pool

	mu      sync.Mutex
	current int // -1 once released or lost in a failed failover
	tried   map[int]bool
	// answered is set once the endpoint returned anything: the turn stays on it from then on.
	answered bool
	released bool
}

func (l *lease) Chat(ctx context.Context, req llm.Request) (llm.Response, error) {
	return l.call(ctx, func(c llm.Client, _ func(string)) (llm.Response, error) {
		return c.Chat(ctx, req)
	}, nil)
}

func (l *lease) ChatStream(ctx context.Context, req llm.Request, onDelta func(string)) (llm.Response, error) {
	return l.call(ctx, func(c llm.Client, onDelta func(string)) (llm.Response, error) {
		return c.ChatStream(ctx, req, onDelta)
	}, onDelta)
}

func (l *lease) call(ctx context.Context, do func(llm.Client, func(string)) (llm.Response, error), onDelta func(string)) (llm.Response, error) {
	for {
		l.mu.Lock()
		i := l.current
		l.mu.Unlock()
		if i < 0 {
			return llm.Response{}, fmt.Errorf("%w: turn has no endpoint", llm.ErrUnavailable)
		}

		streamed := false
		resp, err := do(l.pool.endpoints[i].Client, func(d string) {
			streamed = true
			if onDelta != nil {
				onDelta(d)
			}
		})
		l.mu.Lock()
		first := !l.answered
		if err == nil || streamed {
			l.answered = true
		}
		l.mu.Unlock()
		if err == nil || !first || streamed || ctx.Err() != nil || !errors.Is(err, llm.ErrUnavailable) {
			return resp, err
		}

		l.pool.markDown(i)
		if !l.failover(ctx, i) {
			return resp, err
		}
		slog.Warn("llm endpoint failed, turn moved", "from", l.pool.endpoints[i].Name, "err", err)
	}
}

// failover swaps the failed endpoint for one the turn has not tried; false leaves the turn without a better one.
func (l *lease) failover(ctx context.Context, failed int) bool {
	l.mu.Lock()
	if len(l.tried) == len(l.pool.endpoints) || l.current != failed {
		l.mu.Unlock()
		return false
	}
	l.pool.release(failed)
	l.current = -1
	skip := maps.Clone(l.tried)
	l.mu.Unlock()

	next, err := l.pool.acquire(ctx, skip, nil)
	if err != nil {
		return false
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.released {
		l.pool.release(next)
		return false
	}
	l.current = next
	l.tried[next] = true
	return true
}

func (l *lease) release() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.released = true
	if l.current >= 0 {
		l.pool.release(l.current)
		l.current = -1
	}
}

// MemorySlots counts busy turns in this process.
type MemorySlots struct {
	mu   sync.Mutex
	busy map[string]int
}

func NewMemorySlots() *MemorySlots { return &MemorySlots{busy: map[string]int{}} }

func (s *MemorySlots) TryAcquire(key string, limit int) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.busy[key] >= limit {
		return false
	}
	s.busy[key]++
	return true
}

func (s *MemorySlots) Release(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.busy[key] > 0 {
		s.busy[key]--
	}
}
