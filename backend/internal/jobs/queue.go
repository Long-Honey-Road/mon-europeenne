package jobs

import (
	"context"
	"encoding/json"
	"time"

	"github.com/hibiken/asynq"
)

const (
	queueChat   = "chat"
	queueSystem = "system"
	// turnTimeout covers the agent's own 60 s limit plus saving the answer.
	turnTimeout = 90 * time.Second
)

// Client queues background work.
type Client struct{ c *asynq.Client }

func NewClient(redisAddr string) *Client {
	return &Client{c: asynq.NewClient(asynq.RedisClientOpt{Addr: redisAddr})}
}

// EnqueueChatTurn queues one agent turn; a turn is not retried, the user sees the error and can resend.
func (c *Client) EnqueueChatTurn(ctx context.Context, t ChatTurn) (string, error) {
	payload, err := json.Marshal(t)
	if err != nil {
		return "", err
	}
	info, err := c.c.EnqueueContext(ctx, asynq.NewTask(TypeChatTurn, payload),
		asynq.Queue(queueChat), asynq.MaxRetry(0), asynq.Timeout(turnTimeout))
	if err != nil {
		return "", err
	}
	return info.ID, nil
}

func (c *Client) Close() error { return c.c.Close() }

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
