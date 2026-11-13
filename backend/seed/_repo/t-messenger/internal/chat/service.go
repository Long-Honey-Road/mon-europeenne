// Package chat holds the logic of chats and messages.
package chat

import (
	"context"
	"errors"
	"fmt"
)

var ErrEmptyMessage = errors.New("empty message")

type Store interface {
	SaveMessage(ctx context.Context, chatID, text string) (string, error)
}

type Service struct {
	store Store
}

func NewService(store Store) *Service {
	return &Service{store: store}
}

// SendMessage saves a message and returns its id.
func (s *Service) SendMessage(ctx context.Context, chatID, text string) (string, error) {
	if text == "" {
		return "", ErrEmptyMessage
	}
	id, err := s.store.SaveMessage(ctx, chatID, text)
	if err != nil {
		return "", fmt.Errorf("send message: %w", err)
	}
	return id, nil
}
