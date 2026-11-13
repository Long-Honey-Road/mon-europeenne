package jobs

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// ErrTurnRunning means the thread already waits for an answer.
var ErrTurnRunning = errors.New("a turn is already running in this thread")

// taskMargin is time after the turn for saving the answer and publishing events.
const taskMargin = 30 * time.Second

// TaskTimeout is the asynq deadline of a chat turn: waiting for a model, the turn itself and saving.
func TaskTimeout(queue, turn time.Duration) time.Duration {
	return queue + turn + taskMargin
}

// releaseScript deletes the lock only for its own job, so a late worker cannot free a newer turn.
var releaseScript = redis.NewScript(`
if redis.call("GET", KEYS[1]) == ARGV[1] then
	return redis.call("DEL", KEYS[1])
end
return 0`)

// ActiveTurns keeps one running turn per thread in Redis; the lock expires if a worker dies mid-turn.
type ActiveTurns struct {
	rdb redis.UniversalClient
	ttl time.Duration
}

func NewActiveTurns(rdb redis.UniversalClient, ttl time.Duration) *ActiveTurns {
	return &ActiveTurns{rdb: rdb, ttl: ttl}
}

func activeKey(threadID string) string { return "buddy:turn:" + threadID }

// Reserve takes the thread for a new turn and returns the turn's job id.
func (a *ActiveTurns) Reserve(ctx context.Context, threadID string) (string, error) {
	jobID := uuid.NewString()
	ok, err := a.rdb.SetNX(ctx, activeKey(threadID), jobID, a.ttl).Result()
	if err != nil {
		return "", fmt.Errorf("reserve turn: %w", err)
	}
	if !ok {
		return "", ErrTurnRunning
	}
	return jobID, nil
}

// Release frees the thread if jobID still holds it.
func (a *ActiveTurns) Release(ctx context.Context, threadID, jobID string) error {
	if err := releaseScript.Run(ctx, a.rdb, []string{activeKey(threadID)}, jobID).Err(); err != nil {
		return fmt.Errorf("release turn: %w", err)
	}
	return nil
}
