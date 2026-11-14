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

// TypeSummarize rewrites the summary of a sphere, or of the whole thread when the focus is empty.
const TypeSummarize = "summarize"

type Summary struct {
	ThreadID  string `json:"thread_id"`
	FocusKind int16  `json:"focus_kind,omitempty"`
	FocusID   string `json:"focus_id,omitempty"`
}
