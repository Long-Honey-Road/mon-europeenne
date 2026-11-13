package domain

import (
	"errors"
	"fmt"
	"maps"
)

var ErrUnknownValue = errors.New("unknown enum value")

// ErrNotFound is shared by storage and tools so callers can tell "nothing there" from failures.
var ErrNotFound = errors.New("not found")

// Enum ids equal the smallint keys of the lookup tables; store tests keep them in sync.
type (
	Role           int16
	Direction      int16
	ThreadType     int16
	MessageRole    int16
	PlanPeriod     int16
	PlanItemStatus int16
	SourceKind     int16
	ConflictStatus int16
	EventType      int16
)

const (
	RoleIntern Role = iota + 1
	RoleMentor
	RoleManager
	RoleHRBP
	RoleHRAdmin
)

const (
	DirectionBackend Direction = iota + 1
	DirectionFrontend
	DirectionQA
)

const (
	ThreadGeneral ThreadType = iota + 1
	ThreadPlan
	ThreadRequest
	ThreadTask
)

const (
	MessageUser MessageRole = iota + 1
	MessageAssistant
	MessageTool
)

const (
	PeriodWeek1 PlanPeriod = iota + 1
	PeriodWeek2
	PeriodMonth1
	PeriodMonth2
	PeriodEquator
	PeriodFinal
)

const (
	ItemTodo PlanItemStatus = iota + 1
	ItemDoing
	ItemBlocked
	ItemDone
)

const (
	SourceDoc SourceKind = iota + 1
	SourceRepo
	SourcePlan
	SourcePlaybook
)

const (
	ConflictOpen ConflictStatus = iota + 1
	ConflictReported
	ConflictResolved
)

const (
	EventMessageSent EventType = iota + 1
	EventToolCalled
	EventAnswerWithSources
	EventNotFound
	EventEscalated
	EventRequestSubmitted
	EventRequestStatusChanged
	EventPlanItemDone
	EventConflictDetected
	EventConflictReported
	EventFeedback
	EventInvariantViolation
	EventGeneralKnowledge
)

var (
	roleNames = map[Role]string{
		RoleIntern: "intern", RoleMentor: "mentor", RoleManager: "manager",
		RoleHRBP: "hr_bp", RoleHRAdmin: "hr_admin",
	}
	directionNames = map[Direction]string{
		DirectionBackend: "backend", DirectionFrontend: "frontend", DirectionQA: "qa",
	}
	threadTypeNames = map[ThreadType]string{
		ThreadGeneral: "general", ThreadPlan: "plan", ThreadRequest: "request", ThreadTask: "task",
	}
	messageRoleNames = map[MessageRole]string{
		MessageUser: "user", MessageAssistant: "assistant", MessageTool: "tool",
	}
	planPeriodNames = map[PlanPeriod]string{
		PeriodWeek1: "w1", PeriodWeek2: "w2", PeriodMonth1: "m1", PeriodMonth2: "m2",
		PeriodEquator: "equator", PeriodFinal: "final",
	}
	planItemStatusNames = map[PlanItemStatus]string{
		ItemTodo: "todo", ItemDoing: "doing", ItemBlocked: "blocked", ItemDone: "done",
	}
	sourceKindNames = map[SourceKind]string{
		SourceDoc: "doc", SourceRepo: "repo", SourcePlan: "plan", SourcePlaybook: "playbook",
	}
	conflictStatusNames = map[ConflictStatus]string{
		ConflictOpen: "open", ConflictReported: "reported", ConflictResolved: "resolved",
	}
	eventTypeNames = map[EventType]string{
		EventMessageSent:          "message_sent",
		EventToolCalled:           "tool_called",
		EventAnswerWithSources:    "answer_with_sources",
		EventNotFound:             "not_found",
		EventEscalated:            "escalated",
		EventRequestSubmitted:     "request_submitted",
		EventRequestStatusChanged: "request_status_changed",
		EventPlanItemDone:         "plan_item_done",
		EventConflictDetected:     "conflict_detected",
		EventConflictReported:     "conflict_reported",
		EventFeedback:             "feedback",
		EventInvariantViolation:   "invariant_violation",
		EventGeneralKnowledge:     "general_knowledge",
	}
)

// Lookups returns every enum keyed by its lookup table name.
func Lookups() map[string]map[int16]string {
	return map[string]map[int16]string{
		"roles":              ids(roleNames),
		"directions":         ids(directionNames),
		"thread_types":       ids(threadTypeNames),
		"message_roles":      ids(messageRoleNames),
		"plan_periods":       ids(planPeriodNames),
		"plan_item_statuses": ids(planItemStatusNames),
		"source_kinds":       ids(sourceKindNames),
		"conflict_statuses":  ids(conflictStatusNames),
		"event_types":        ids(eventTypeNames),
	}
}

func (v Role) String() string           { return nameOf(roleNames, v) }
func (v Direction) String() string      { return nameOf(directionNames, v) }
func (v ThreadType) String() string     { return nameOf(threadTypeNames, v) }
func (v MessageRole) String() string    { return nameOf(messageRoleNames, v) }
func (v PlanPeriod) String() string     { return nameOf(planPeriodNames, v) }
func (v PlanItemStatus) String() string { return nameOf(planItemStatusNames, v) }
func (v SourceKind) String() string     { return nameOf(sourceKindNames, v) }
func (v ConflictStatus) String() string { return nameOf(conflictStatusNames, v) }
func (v EventType) String() string      { return nameOf(eventTypeNames, v) }

func ParseRole(s string) (Role, error)                     { return parse(roleNames, s) }
func ParseDirection(s string) (Direction, error)           { return parse(directionNames, s) }
func ParseThreadType(s string) (ThreadType, error)         { return parse(threadTypeNames, s) }
func ParseMessageRole(s string) (MessageRole, error)       { return parse(messageRoleNames, s) }
func ParsePlanPeriod(s string) (PlanPeriod, error)         { return parse(planPeriodNames, s) }
func ParsePlanItemStatus(s string) (PlanItemStatus, error) { return parse(planItemStatusNames, s) }
func ParseSourceKind(s string) (SourceKind, error)         { return parse(sourceKindNames, s) }
func ParseConflictStatus(s string) (ConflictStatus, error) { return parse(conflictStatusNames, s) }
func ParseEventType(s string) (EventType, error)           { return parse(eventTypeNames, s) }

func nameOf[T ~int16](names map[T]string, v T) string {
	if name, ok := names[v]; ok {
		return name
	}
	return fmt.Sprintf("unknown(%d)", int16(v))
}

func parse[T ~int16](names map[T]string, s string) (T, error) {
	for v, name := range names {
		if name == s {
			return v, nil
		}
	}
	return 0, fmt.Errorf("%q: %w", s, ErrUnknownValue)
}

func ids[T ~int16](names map[T]string) map[int16]string {
	out := make(map[int16]string, len(names))
	for v, name := range maps.All(names) {
		out[int16(v)] = name
	}
	return out
}
