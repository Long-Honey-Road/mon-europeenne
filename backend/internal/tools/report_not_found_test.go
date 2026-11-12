package tools

import (
	"context"
	"encoding/json"
	"slices"
	"testing"

	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/domain"
)

func TestReportNotFoundReturnsTopicAndEscalation(t *testing.T) {
	tool, err := ReportNotFound(testDirectory())
	if err != nil {
		t.Fatalf("ReportNotFound: %v", err)
	}
	r := newRegistry(t, tool)

	raw, err := r.Call(context.Background(), intern, domain.ThreadGeneral, "report_not_found", json.RawMessage(`{"topic":"ДМС"}`))
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	var out NotFoundReport
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	want := []EscalationStep{
		{Level: 1, Role: "mentor", PersonID: "p-anna", Name: "Аня"},
		{Level: 2, Role: "manager", PersonID: "p-dima", Name: "Дима"},
		{Level: 3, Role: "hr_bp", PersonID: "p-eva", Name: "Ева"},
		{Level: 4, Role: "open_sources"},
	}
	if out.Topic != "ДМС" || !slices.Equal(out.Escalation, want) {
		t.Fatalf("report = %+v", out)
	}
}

func TestReportNotFoundIsOpenEverywhere(t *testing.T) {
	tool, err := ReportNotFound(testDirectory())
	if err != nil {
		t.Fatalf("ReportNotFound: %v", err)
	}
	if len(tool.Roles) != len(allRoles) || len(tool.Threads) != len(allThreads) {
		t.Fatalf("roles %v, threads %v", tool.Roles, tool.Threads)
	}
}

func TestReportNotFoundReportsDirectoryFailure(t *testing.T) {
	tool, _ := ReportNotFound(failingDirectory{PeopleDirectory: testDirectory(), broken: "relations"})
	r := newRegistry(t, tool)
	if _, err := r.Call(context.Background(), intern, domain.ThreadGeneral, "report_not_found", json.RawMessage(`{"topic":"x"}`)); err == nil {
		t.Fatal("directory failure swallowed")
	}
}
