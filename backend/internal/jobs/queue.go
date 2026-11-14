package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/hibiken/asynq"
	"github.com/redis/go-redis/v9"

	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/domain"
)

const (
	queueChat   = "chat"
	queueSystem = "system"
)

// Client queues background work.
type Client struct {
	c       *asynq.Client
	rdb     *redis.Client
	turns   *ActiveTurns
	timeout time.Duration
}

// NewClient queues chat turns with timeout as their deadline (see TaskTimeout); a thread lock lives as long.
func NewClient(redisAddr string, timeout time.Duration) *Client {
	rdb := redis.NewClient(&redis.Options{Addr: redisAddr})
	return &Client{
		c: asynq.NewClient(asynq.RedisClientOpt{Addr: redisAddr}), rdb: rdb,
		turns: NewActiveTurns(rdb, timeout), timeout: timeout,
	}
}

// ReserveChatTurn takes the thread for a new turn and returns its job id; ErrTurnRunning if one is running.
func (c *Client) ReserveChatTurn(ctx context.Context, threadID string) (string, error) {
	return c.turns.Reserve(ctx, threadID)
}

// ReleaseChatTurn frees the thread when the turn could not be queued.
func (c *Client) ReleaseChatTurn(ctx context.Context, threadID, jobID string) error {
	return c.turns.Release(ctx, threadID, jobID)
}

// EnqueueChatTurn queues a reserved turn; a turn is not retried, the user sees the error and can resend.
func (c *Client) EnqueueChatTurn(ctx context.Context, jobID string, t ChatTurn) error {
	payload, err := json.Marshal(t)
	if err != nil {
		return err
	}
	_, err = c.c.EnqueueContext(ctx, asynq.NewTask(TypeChatTurn, payload),
		asynq.TaskID(jobID), asynq.Queue(queueChat), asynq.MaxRetry(0), asynq.Timeout(c.timeout))
	return err
}

// EnqueueSummary asks for a new summary; a second request for the same sphere within a few minutes is dropped.
func (c *Client) EnqueueSummary(ctx context.Context, threadID string, f domain.Focus) error {
	payload, err := json.Marshal(Summary{ThreadID: threadID, FocusKind: int16(f.Kind), FocusID: f.ID})
	if err != nil {
		return err
	}
	_, err = c.c.EnqueueContext(ctx, asynq.NewTask(TypeSummarize, payload),
		asynq.Queue(queueSystem), asynq.MaxRetry(1), asynq.Timeout(2*time.Minute), asynq.Unique(3*time.Minute))
	if errors.Is(err, asynq.ErrDuplicateTask) {
		return nil
	}
	return err
}

func (c *Client) Close() error { return errors.Join(c.c.Close(), c.rdb.Close()) }

// NewServer runs tasks with chat turns ahead of system work.
func NewServer(redisAddr string, concurrency int) *asynq.Server {
	return asynq.NewServer(asynq.RedisClientOpt{Addr: redisAddr}, asynq.Config{
		Concurrency: concurrency,
		Queues:      map[string]int{queueChat: 6, queueSystem: 1},
	})
}

func Mux(h *Handler) *asynq.ServeMux {
	mux := asynq.NewServeMux()
	mux.Handle(TypeChatTurn, h)
	return mux
}
