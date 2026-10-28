package store

import (
	"database/sql"
	"errors"
	"fmt"
	"unicode/utf8"
)

// GetKV returns key's value, or ErrNotFound.
func (s *Store) GetKV(key string) (string, error) {
	var v string
	err := s.db.QueryRow(`SELECT value FROM kv WHERE key = ?`, key).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("get kv %s: %w", key, err)
	}
	return v, nil
}

// SetKV upserts key's value.
func (s *Store) SetKV(key, value string) error {
	if _, err := s.db.Exec(`INSERT INTO kv (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, value); err != nil {
		return fmt.Errorf("set kv %s: %w", key, err)
	}
	return nil
}

// DeleteKV removes key; deleting an unset key is not an error.
func (s *Store) DeleteKV(key string) error {
	if _, err := s.db.Exec(`DELETE FROM kv WHERE key = ?`, key); err != nil {
		return fmt.Errorf("delete kv %s: %w", key, err)
	}
	return nil
}

// DeleteKVPrefix removes every key that starts with prefix.
func (s *Store) DeleteKVPrefix(prefix string) error {
	if _, err := s.db.Exec(`DELETE FROM kv WHERE substr(key, 1, ?) = ?`, utf8.RuneCountInString(prefix), prefix); err != nil {
		return fmt.Errorf("delete kv %s*: %w", prefix, err)
	}
	return nil
}
