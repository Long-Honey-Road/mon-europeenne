package chat

import (
	"context"
	"errors"
	"testing"
)

type memStore struct{}

func (memStore) SaveMessage(context.Context, string, string) (string, error) { return "m1", nil }

func TestSendMessageRejectsEmptyText(t *testing.T) {
	_, err := NewService(memStore{}).SendMessage(context.Background(), "c1", "")
	if !errors.Is(err, ErrEmptyMessage) {
		t.Fatalf("err = %v", err)
	}
}
