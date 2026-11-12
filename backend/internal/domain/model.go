// Package domain holds core types shared by store, agent, tools and API.
package domain

import (
	"encoding/json"
	"time"
)

// Principal is the authenticated caller; always taken from the session, never from the request.
type Principal struct {
	UserID string
	Role   Role
	TeamID string
}

type User struct {
	ID           string
	Login        string
	DisplayName  string
	Role         Role
	Direction    Direction
	TeamID       string
	ManagerID    string
	MentorID     string
	Repos        []string
	JiraProjects []string
	Location     string
	StartDate    time.Time
}

type Team struct {
	ID                string
	Name              string
	Unit              string
	Cluster           string
	MattermostChannel string
}

// Person mirrors the people directory mock.
type Person struct {
	ID               string
	UserID           string
	Name             string
	RoleTitle        string
	TeamID           string
	Responsibilities []string
	AskMeAbout       []string
	Contact          string
}

// ThreadPin is the context a thread is pinned to; at most one field is set.
type ThreadPin struct {
	PlanItemID string `json:"plan_item_id,omitempty"`
	RequestID  string `json:"request_id,omitempty"`
	Task       *Task  `json:"task,omitempty"`
}

type Task struct {
	Key   string `json:"key"`
	Title string `json:"title,omitempty"`
	Repo  string `json:"repo,omitempty"`
}

type Thread struct {
	ID        string
	UserID    string
	Type      ThreadType
	Title     string
	Pin       ThreadPin
	Summary   string
	CreatedAt time.Time
}

type Message struct {
	ID          string
	ThreadID    string
	Role        MessageRole
	Content     string
	Blocks      json.RawMessage
	Attachments json.RawMessage
	JobID       string
	CreatedAt   time.Time
}

// MapPosition places a plan item on the adaptation map.
type MapPosition struct {
	X     float64 `json:"x"`
	Y     float64 `json:"y"`
	Group string  `json:"group,omitempty"`
}

type PlanItem struct {
	ID        string
	Key       string
	Title     string
	Practice  string
	Period    PlanPeriod
	Direction Direction // zero means the item applies to every direction
	TeamID    string
	DependsOn []string
	Sources   []string
	Map       MapPosition
}

// PlanEntry is a plan item as one user sees it: with their status and note.
type PlanEntry struct {
	Item   PlanItem
	Status PlanItemStatus
	Note   string
}

type PlanStatus struct {
	UserID    string
	ItemID    string
	Status    PlanItemStatus
	Note      string
	UpdatedAt time.Time
}

// Document is general when TeamID is empty, team-scoped otherwise.
type Document struct {
	ID                string
	Source            SourceKind
	ExternalID        string
	Title             string
	TeamID            string
	Topic             string
	URL               string
	IsTestInstruction bool
	Version           int
	FetchedAt         time.Time
}

type Chunk struct {
	ID         string
	DocumentID string
	Ord        int
	Heading    string
	Text       string
	Embedding  []float32
}

// Request mirrors a request in the corp-requests mock; the mock is the source of truth for status.
type Request struct {
	ID         string
	UserID     string
	ThreadID   string
	TypeKey    string
	ExternalID string
	Status     string
	Fields     json.RawMessage
	Approver   json.RawMessage
	History    json.RawMessage
	UpdatedAt  time.Time
}

type Conflict struct {
	ID         string
	DocA       string
	DocB       string
	Topic      string
	Summary    string
	Status     ConflictStatus
	ReportedBy string
}

type Event struct {
	ID      int64
	ActorID string
	Type    EventType
	Payload json.RawMessage
	At      time.Time
}
