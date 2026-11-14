package domain

import (
	"errors"
	"testing"
)

func TestEnumsRoundTripThroughNames(t *testing.T) {
	for table, names := range Lookups() {
		t.Run(table, func(t *testing.T) {
			if len(names) == 0 {
				t.Fatal("lookup is empty")
			}
			seen := map[string]bool{}
			for id, name := range names {
				if id <= 0 {
					t.Errorf("id %d for %q: ids start at 1, 0 means unset", id, name)
				}
				if name == "" || seen[name] {
					t.Errorf("name %q for id %d is empty or duplicated", name, id)
				}
				seen[name] = true
			}
		})
	}
}

func TestParseRole(t *testing.T) {
	got, err := ParseRole("hr_admin")
	if err != nil || got != RoleHRAdmin {
		t.Fatalf("ParseRole(hr_admin) = %v, %v; want RoleHRAdmin", got, err)
	}
	if RoleHRAdmin.String() != "hr_admin" {
		t.Fatalf("RoleHRAdmin.String() = %q", RoleHRAdmin.String())
	}
}

func TestParseRejectsUnknownName(t *testing.T) {
	if _, err := ParseThreadType("chat"); !errors.Is(err, ErrUnknownValue) {
		t.Fatalf("ParseThreadType(chat) error = %v, want ErrUnknownValue", err)
	}
}

func TestStringOfUnsetValue(t *testing.T) {
	var s PlanItemStatus
	if s.String() != "unknown(0)" {
		t.Fatalf("zero PlanItemStatus.String() = %q, want unknown(0)", s.String())
	}
}

func TestParseCoversEveryLookup(t *testing.T) {
	parsers := map[string]func(string) (int16, error){
		"roles":              wrap(ParseRole),
		"directions":         wrap(ParseDirection),
		"thread_types":       wrap(ParseThreadType),
		"message_roles":      wrap(ParseMessageRole),
		"plan_periods":       wrap(ParsePlanPeriod),
		"plan_item_statuses": wrap(ParsePlanItemStatus),
		"source_kinds":       wrap(ParseSourceKind),
		"conflict_statuses":  wrap(ParseConflictStatus),
		"event_types":        wrap(ParseEventType),
		"focus_kinds":        wrap(ParseFocusKind),
	}
	lookups := Lookups()
	if len(parsers) != len(lookups) {
		t.Fatalf("%d parsers for %d lookups", len(parsers), len(lookups))
	}
	for table, names := range lookups {
		parse, ok := parsers[table]
		if !ok {
			t.Fatalf("no parser for %s", table)
		}
		for id, name := range names {
			got, err := parse(name)
			if err != nil || got != id {
				t.Errorf("%s: parse(%q) = %d, %v; want %d", table, name, got, err, id)
			}
		}
	}
}

func TestStringMatchesLookupName(t *testing.T) {
	stringers := map[string]func(int16) string{
		"roles":              func(v int16) string { return Role(v).String() },
		"directions":         func(v int16) string { return Direction(v).String() },
		"thread_types":       func(v int16) string { return ThreadType(v).String() },
		"message_roles":      func(v int16) string { return MessageRole(v).String() },
		"plan_periods":       func(v int16) string { return PlanPeriod(v).String() },
		"plan_item_statuses": func(v int16) string { return PlanItemStatus(v).String() },
		"source_kinds":       func(v int16) string { return SourceKind(v).String() },
		"conflict_statuses":  func(v int16) string { return ConflictStatus(v).String() },
		"event_types":        func(v int16) string { return EventType(v).String() },
		"focus_kinds":        func(v int16) string { return FocusKind(v).String() },
	}
	for table, names := range Lookups() {
		for id, name := range names {
			if got := stringers[table](id); got != name {
				t.Errorf("%s: String(%d) = %q, want %q", table, id, got, name)
			}
		}
	}
}

func wrap[T ~int16](parse func(string) (T, error)) func(string) (int16, error) {
	return func(s string) (int16, error) {
		v, err := parse(s)
		return int16(v), err
	}
}
