package requests

import (
	"context"
	"time"
)

// Request statuses of the corporate service.
const (
	StatusPending  = "pending"
	StatusApproved = "approved"
	StatusRejected = "rejected"
)

// Change is one line of a request's history.
type Change struct {
	At     time.Time `json:"at"`
	Status string    `json:"status"`
	By     string    `json:"by,omitempty"`
	Note   string    `json:"note,omitempty"`
}

// Ticket is a request as the corporate service sees it.
type Ticket struct {
	ID          string         `json:"id"`
	TypeKey     string         `json:"type_key"`
	RequesterID string         `json:"requester_id"`
	Fields      map[string]any `json:"fields"`
	Status      string         `json:"status"`
	// Approvers is the whole chain; Step points at the current one.
	Approvers []Approver `json:"approvers"`
	Step      int        `json:"step"`
	Reason    string     `json:"reason,omitempty"`
	History   []Change   `json:"history"`
	UpdatedAt time.Time  `json:"updated_at"`
}

// Approver returns who decides now, or who decided last for a finished request.
func (t Ticket) Approver() Approver {
	if len(t.Approvers) == 0 {
		return Approver{}
	}
	return t.Approvers[min(t.Step, len(t.Approvers)-1)]
}

type Submission struct {
	TypeKey     string
	RequesterID string
	Fields      map[string]any
	Approvers   []Approver
}

// Service is the company's request system (corp-requests). Buddy only submits and mirrors what it says.
type Service interface {
	Submit(ctx context.Context, s Submission) (Ticket, error)
	// Updates returns the requests that changed since the previous call.
	Updates(ctx context.Context) ([]Ticket, error)
}
