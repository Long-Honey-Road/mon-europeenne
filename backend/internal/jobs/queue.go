package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/hibiken/asynq"
	"github.com/redis/go-redis/v9"
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
