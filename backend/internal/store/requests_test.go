package store

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/domain"
)

func request(ext, user, status string) domain.Request {
	return domain.Request{
		UserID: user, TypeKey: "access.jira_project", ExternalID: ext, Status: status,
		Fields:   json.RawMessage(`{"project_key":"MSG"}`),
		Approver: json.RawMessage(`{"person_id":"p-dmitry","name":"Дмитрий"}`),
		History:  json.RawMessage(`[{"status":"submitted"}]`),
	}
}

func TestSaveRequestUpsertsByExternalID(t *testing.T) {
	s := seeded(t)
	ctx := context.Background()

	if err := s.SaveRequest(ctx, request("REQ-1", "u-liza", "submitted")); err != nil {
		t.Fatalf("SaveRequest: %v", err)
	}
	if err := s.SaveRequest(ctx, request("REQ-1", "u-liza", "approved")); err != nil {
		t.Fatalf("SaveRequest again: %v", err)
	}

	got, team, err := s.Request(ctx, "REQ-1")
	if err != nil {
		t.Fatalf("Request: %v", err)
	}
	if got.Status != "approved" || got.UserID != "u-liza" || team != "t-messenger" || got.TypeKey != "access.jira_project" {
		t.Fatalf("request = %+v, team %s", got, team)
	}
	var approver map[string]string
	if err := json.Unmarshal(got.Approver, &approver); err != nil || approver["person_id"] != "p-dmitry" {
		t.Errorf("approver = %s", got.Approver)
	}
}

func TestRequestUnknown(t *testing.T) {
	s := seeded(t)
	if _, _, err := s.Request(context.Background(), "REQ-0"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("error = %v, want ErrNotFound", err)
	}
}

func TestRequestsOfUserNewestFirst(t *testing.T) {
	s := seeded(t)
	ctx := context.Background()
	for _, r := range []domain.Request{request("REQ-1", "u-liza", "approved"), request("REQ-2", "u-dmitry", "submitted"), request("REQ-3", "u-liza", "submitted")} {
		if err := s.SaveRequest(ctx, r); err != nil {
			t.Fatalf("SaveRequest: %v", err)
		}
	}

	list, err := s.RequestsOf(ctx, "u-liza")
	if err != nil {
		t.Fatalf("RequestsOf: %v", err)
	}
	if len(list) != 2 || list[0].ExternalID != "REQ-3" || list[1].ExternalID != "REQ-1" {
		t.Fatalf("requests = %+v", list)
	}
}

func TestSaveRequestOfUnknownUser(t *testing.T) {
	s := seeded(t)
	if err := s.SaveRequest(context.Background(), request("REQ-9", "u-ghost", "submitted")); err == nil {
		t.Fatal("request of an unknown user saved")
	}
}
