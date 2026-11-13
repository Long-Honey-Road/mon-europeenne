// Package storage keeps chats and messages in Postgres.
package storage

import "context"

type DB struct{}

func Open(configPath string) (*DB, error) { return &DB{}, nil }

func (db *DB) SaveMessage(ctx context.Context, chatID, text string) (string, error) { return "", nil }
