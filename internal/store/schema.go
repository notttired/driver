package store

import (
	"database/sql"
	"errors"
	"fmt"
)

// migrations is the append-only list of schema steps; migrations[i] brings a
// database at user_version i to i+1. Never edit a shipped step — append.
var migrations = []string{schemaV1, schemaV2, schemaV3}

// schemaV1 creates the four tables the design calls for. Every column a Go
// scan reads into a plain string/int64 is NOT NULL with a same-typed default;
// only archived_at/archived_name are nullable, since NULL there is the
// meaningful "not archived". Timestamps are unix seconds.
const schemaV1 = `
CREATE TABLE jobs (
	id            INTEGER PRIMARY KEY AUTOINCREMENT,
	slug          TEXT NOT NULL,
	dir           TEXT NOT NULL DEFAULT '',
	model         TEXT NOT NULL DEFAULT '',
	color         TEXT NOT NULL DEFAULT '',
	spec          TEXT NOT NULL DEFAULT '',
	notes         TEXT NOT NULL DEFAULT '',
	state         TEXT NOT NULL DEFAULT '' CHECK(state IN ('','working','blocked','done')),
	outcome       TEXT NOT NULL DEFAULT '',
	pi_session    TEXT NOT NULL DEFAULT '',
	tmux          TEXT NOT NULL DEFAULT '',
	started_at    INTEGER NOT NULL DEFAULT 0,
	finished_at   INTEGER NOT NULL DEFAULT 0,
	created_at    INTEGER NOT NULL DEFAULT 0,
	updated_at    INTEGER NOT NULL DEFAULT 0,
	archived_at   INTEGER NULL,
	archived_name TEXT NULL,
	rev           INTEGER NOT NULL DEFAULT 1
);
-- slug uniqueness only among active rows: archiving frees the slug
CREATE UNIQUE INDEX jobs_active_slug ON jobs(slug) WHERE archived_at IS NULL;
CREATE UNIQUE INDEX jobs_archived_name ON jobs(archived_name) WHERE archived_at IS NOT NULL;

-- the board's manual order per column, keyed on the immutable job id; no FK
-- on purpose: a stale row is simply ignored by the render and pruned on the
-- next save of that column
CREATE TABLE board_order (
	"column"  TEXT NOT NULL,
	position  INTEGER NOT NULL,
	job_id    INTEGER NOT NULL,
	PRIMARY KEY ("column", position)
);

CREATE TABLE links (
	id         INTEGER PRIMARY KEY AUTOINCREMENT,
	job_id     INTEGER NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,
	kind       TEXT NOT NULL DEFAULT 'url',
	url        TEXT NOT NULL,
	label      TEXT NOT NULL DEFAULT '',
	status     TEXT NOT NULL DEFAULT '',
	status_at  INTEGER NOT NULL DEFAULT 0,
	created_at INTEGER NOT NULL DEFAULT 0,
	UNIQUE(job_id, url)
);

CREATE TABLE kv (
	key   TEXT PRIMARY KEY NOT NULL,
	value TEXT NOT NULL DEFAULT ''
);
`

// schemaV2 admits the stored state 'ready' (pi open at its prompt, nothing
// sent yet — docs/DESIGN.md "Lifecycle"). SQLite cannot alter a CHECK, so the
// jobs table is rebuilt: a copy with the new constraint, the rows moved, the
// indexes recreated. The links table keeps referencing jobs by id; see
// migrate for why foreign keys are off while this runs.
const schemaV2 = `
CREATE TABLE jobs_v2 (
	id            INTEGER PRIMARY KEY AUTOINCREMENT,
	slug          TEXT NOT NULL,
	dir           TEXT NOT NULL DEFAULT '',
	model         TEXT NOT NULL DEFAULT '',
	color         TEXT NOT NULL DEFAULT '',
	spec          TEXT NOT NULL DEFAULT '',
	notes         TEXT NOT NULL DEFAULT '',
	state         TEXT NOT NULL DEFAULT '' CHECK(state IN ('','ready','working','blocked','done')),
	outcome       TEXT NOT NULL DEFAULT '',
	pi_session    TEXT NOT NULL DEFAULT '',
	tmux          TEXT NOT NULL DEFAULT '',
	started_at    INTEGER NOT NULL DEFAULT 0,
	finished_at   INTEGER NOT NULL DEFAULT 0,
	created_at    INTEGER NOT NULL DEFAULT 0,
	updated_at    INTEGER NOT NULL DEFAULT 0,
	archived_at   INTEGER NULL,
	archived_name TEXT NULL,
	rev           INTEGER NOT NULL DEFAULT 1
);
INSERT INTO jobs_v2 SELECT id, slug, dir, model, color, spec, notes, state, outcome, pi_session, tmux,
	started_at, finished_at, created_at, updated_at, archived_at, archived_name, rev FROM jobs;
DROP TABLE jobs;
ALTER TABLE jobs_v2 RENAME TO jobs;
CREATE UNIQUE INDEX jobs_active_slug ON jobs(slug) WHERE archived_at IS NULL;
CREATE UNIQUE INDEX jobs_archived_name ON jobs(archived_name) WHERE archived_at IS NOT NULL;
`

// schemaV3 drops links.label: a link reads as its URL (the board shows a PR
// as #N, anything else as its host) and nothing writes a label any more. It
// also forgets the session scan offsets: from this version the scanner
// attaches the URLs in the operator's own messages too, so every existing
// session is read once more from its start (attaching is idempotent on
// (job_id, url); the pending PR calls are rebuilt by the same pass).
const schemaV3 = `
ALTER TABLE links DROP COLUMN label;
DELETE FROM kv WHERE key LIKE 'scan:%' OR key LIKE 'scan-pending:%';
`

// migrate applies every migration past the database's PRAGMA user_version,
// each step and its version bump in one transaction. Two things a table
// rebuild (v2) needs are handled around every step so no step has to know:
// foreign keys are off for the run (the pragma is a no-op inside a
// transaction, so it is set outside; with them on, DROP TABLE jobs would run
// an implicit DELETE first and cascade every link away), and the jobs
// AUTOINCREMENT counter is carried across (SQLite rebuilds it from the copied
// rows, which would hand a deleted job's id to the next job — board_order and
// the kv scan offsets are keyed on ids and must never see one reused).
func (s *Store) migrate() error {
	var version int
	if err := s.db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		return fmt.Errorf("read user_version: %w", err)
	}
	if version > len(migrations) {
		return fmt.Errorf("database is schema v%d but this binary only knows v%d — upgrade driver", version, len(migrations))
	}
	if version == len(migrations) {
		return nil
	}
	if _, err := s.db.Exec(`PRAGMA foreign_keys = OFF`); err != nil {
		return fmt.Errorf("foreign_keys off: %w", err)
	}
	defer s.db.Exec(`PRAGMA foreign_keys = ON`)
	for v := version; v < len(migrations); v++ {
		tx, err := s.db.Begin()
		if err != nil {
			return fmt.Errorf("begin migration %d: %w", v+1, err)
		}
		seq, err := jobSeq(tx)
		if err != nil {
			tx.Rollback()
			return fmt.Errorf("migration %d: read job id counter: %w", v+1, err)
		}
		if _, err := tx.Exec(migrations[v]); err != nil {
			tx.Rollback()
			return fmt.Errorf("apply migration %d: %w", v+1, err)
		}
		if err := restoreJobSeq(tx, seq); err != nil {
			tx.Rollback()
			return fmt.Errorf("migration %d: restore job id counter: %w", v+1, err)
		}
		if _, err := tx.Exec(fmt.Sprintf(`PRAGMA user_version = %d`, v+1)); err != nil {
			tx.Rollback()
			return fmt.Errorf("set user_version %d: %w", v+1, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit migration %d: %w", v+1, err)
		}
	}
	return nil
}

// jobSeq reads the jobs table's AUTOINCREMENT counter: the highest id ever
// issued, 0 when no job was ever created (or before the table exists).
func jobSeq(tx *sql.Tx) (int64, error) {
	var n int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE name = 'sqlite_sequence'`).Scan(&n); err != nil || n == 0 {
		return 0, err
	}
	var seq int64
	err := tx.QueryRow(`SELECT seq FROM sqlite_sequence WHERE name = 'jobs'`).Scan(&seq)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	return seq, err
}

// restoreJobSeq raises the jobs counter back to seq when a step lowered it.
func restoreJobSeq(tx *sql.Tx, seq int64) error {
	if seq == 0 {
		return nil
	}
	if _, err := tx.Exec(`DELETE FROM sqlite_sequence WHERE name = 'jobs' AND seq < ?`, seq); err != nil {
		return err
	}
	_, err := tx.Exec(`INSERT INTO sqlite_sequence(name, seq) SELECT 'jobs', ? WHERE NOT EXISTS (SELECT 1 FROM sqlite_sequence WHERE name = 'jobs')`, seq)
	return err
}
