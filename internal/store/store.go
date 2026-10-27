// Package store owns driver.db: the single SQLite database (modernc.org/sqlite,
// pure Go) that holds jobs, the board's manual column order, a job's links and
// the daemon's kv bookkeeping. It is pure storage — no HTTP, no tmux, no file
// walking — and sits at the bottom of the dependency chain:
// util ← store ← pi ← jobs ← api ← web.
//
// The connection pool is capped at one connection: this process is the sole
// writer, and a single connection means every statement shares one
// busy_timeout / journal_mode setting and transactions never contend with
// each other inside the process.
package store

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	sqlite "modernc.org/sqlite"
)

// Sentinel errors callers compare with errors.Is; the API layer maps them to
// status codes (ErrNotFound → 404, the rest → 409).
var (
	// ErrNotFound is returned by single-row lookups and by updates whose row
	// is gone (or archived, which this package treats as immutable).
	ErrNotFound = errors.New("not found")
	// ErrStaleRev is returned by a rev-checked update when the row exists but
	// its rev moved on — someone else wrote first.
	ErrStaleRev = errors.New("stale rev")
	// ErrSlugTaken is returned when a slug is already held by an active job.
	ErrSlugTaken = errors.New("slug taken")
	// ErrDuplicate is returned by AddLink when (job_id, url) already exists.
	ErrDuplicate = errors.New("duplicate")
)

// Store wraps the single *sql.DB handle for driver.db.
type Store struct {
	db *sql.DB
}

// Open opens (creating if absent) the database at path, applies the PRAGMAs
// (WAL, busy_timeout, foreign_keys, immediate transactions) and runs the
// migrations. The pragmas ride the DSN so they apply to the connection
// however database/sql opens it.
func Open(path string) (*Store, error) {
	if dir := filepath.Dir(path); dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("mkdir %s: %w", dir, err)
		}
	}
	dsn := "file:" + path + "?_txlock=immediate" +
		"&_pragma=busy_timeout(5000)" +
		"&_pragma=journal_mode(WAL)" +
		"&_pragma=foreign_keys(1)" +
		"&_pragma=synchronous(NORMAL)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	db.SetMaxOpenConns(1)
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping %s: %w", path, err)
	}
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate %s: %w", path, err)
	}
	return s, nil
}

// OpenMemory opens a private in-memory database — for tests.
func OpenMemory() (*Store, error) {
	db, err := sql.Open("sqlite", "file::memory:?_pragma=foreign_keys(1)")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

// Close closes the underlying database handle.
func (s *Store) Close() error { return s.db.Close() }

// now is the store's clock: unix seconds, the unit every timestamp column
// and the wire contract share.
func now() int64 { return time.Now().Unix() }

// isUniqueErr reports whether err is a UNIQUE / PRIMARY KEY constraint
// violation, by SQLite extended code first and by message as a backstop.
func isUniqueErr(err error) bool {
	if err == nil {
		return false
	}
	var se *sqlite.Error
	if errors.As(err, &se) {
		code := se.Code()
		return code == 2067 || code == 1555 // SQLITE_CONSTRAINT_UNIQUE, _PRIMARYKEY
	}
	return strings.Contains(err.Error(), "UNIQUE constraint failed")
}
