// Package jobs runs background work on asynq: agent turns now, scheduler tasks later.
package jobs

// TypeChatTurn is the asynq task that runs one agent turn.
const TypeChatTurn = "chat_turn"

// ChatTurn asks the worker to answer one user message.
type ChatTurn struct {
	ThreadID  string `json:"thread_id"`
	UserID    string `json:"user_id"`
	MessageID string `json:"message_id"`
	Mode      string `json:"mode"`
}
