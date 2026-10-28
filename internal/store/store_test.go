package store

import (
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
)

func open(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "driver.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestJobsLifecycle(t *testing.T) {
	s := open(t)
	j := &Job{Slug: "alpha", Dir: "/tmp", Spec: "do it"}
	id, err := s.CreateJob(j)
	if err != nil || id == 0 || j.Rev != 1 {
		t.Fatalf("create: %v id=%d rev=%d", err, id, j.Rev)
	}
	if _, err := s.CreateJob(&Job{Slug: "alpha"}); !errors.Is(err, ErrSlugTaken) {
		t.Fatalf("duplicate slug: %v", err)
	}
	got, err := s.GetJobBySlug("alpha")
	if err != nil || got.Spec != "do it" || got.CreatedAt == 0 {
		t.Fatalf("get: %v %+v", err, got)
	}
	rev, err := s.UpdateJobFields(id, got.Rev, map[string]any{"state": "working", "started_at": int64(5)})
	if err != nil || rev != 2 {
		t.Fatalf("update: %v rev=%d", err, rev)
	}
	if _, err := s.UpdateJobFields(id, 1, map[string]any{"state": "blocked"}); !errors.Is(err, ErrStaleRev) {
		t.Fatalf("stale rev: %v", err)
	}
	if _, err := s.UpdateJobFields(id, 2, map[string]any{"bogus": 1}); err == nil {
		t.Fatal("bogus field accepted")
	}
	if _, err := s.UpdateJobFields(999, 1, map[string]any{"state": "done"}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing: %v", err)
	}
}

func TestReopenIsIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "driver.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateJob(&Job{Slug: "keep"}); err != nil {
		t.Fatal(err)
	}
	s.Close()
	s, err = Open(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer s.Close()
	if _, err := s.GetJobBySlug("keep"); err != nil {
		t.Fatalf("row lost across reopen: %v", err)
	}
}

func TestArchiveUnarchiveDelete(t *testing.T) {
	s := open(t)
	j := &Job{Slug: "foo", State: "done"}
	id, _ := s.CreateJob(j)
	name, err := s.ArchiveJob(id)
	if err != nil || name != "foo-1" {
		t.Fatalf("archive: %v %q", err, name)
	}
	if _, err := s.GetJobBySlug("foo"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("archived job still active: %v", err)
	}
	a, err := s.GetArchivedJobByName("foo-1")
	if err != nil || !a.Archived() || a.Slug != "foo" {
		t.Fatalf("archived lookup: %v %+v", err, a)
	}
	if _, err := s.UpdateJobFields(id, a.Rev, map[string]any{"notes": "x"}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("archived must be immutable: %v", err)
	}
	// slug is free again
	id2, err := s.CreateJob(&Job{Slug: "foo"})
	if err != nil {
		t.Fatalf("reuse freed slug: %v", err)
	}
	if err := s.UnarchiveJob(id); !errors.Is(err, ErrSlugTaken) {
		t.Fatalf("unarchive onto taken slug: %v", err)
	}
	if err := s.DeleteJob(id2); err != nil {
		t.Fatal(err)
	}
	if err := s.UnarchiveJob(id); err != nil {
		t.Fatalf("unarchive: %v", err)
	}
	if got, err := s.GetJobBySlug("foo"); err != nil || got.ID != id || got.Archived() {
		t.Fatalf("restored: %v %+v", err, got)
	}
	if err := s.DeleteJob(42); !errors.Is(err, ErrNotFound) {
		t.Fatalf("delete missing: %v", err)
	}
	list, _ := s.ListArchivedJobs()
	if len(list) != 0 {
		t.Fatalf("archived list: %d", len(list))
	}
}

func TestBoardOrder(t *testing.T) {
	s := open(t)
	if err := s.SetBoardColumn("draft", []int64{3, 1, 3, 0, 2}); err != nil {
		t.Fatal(err)
	}
	if err := s.SetBoardColumn("working", []int64{2}); err != nil {
		t.Fatal(err)
	}
	o, err := s.GetBoardOrder()
	if err != nil {
		t.Fatal(err)
	}
	if got := o["draft"]; len(got) != 2 || got[0] != 3 || got[1] != 1 {
		t.Fatalf("draft order = %v (2 must have moved to working)", got)
	}
	if got := o["working"]; len(got) != 1 || got[0] != 2 {
		t.Fatalf("working order = %v", got)
	}
	// unlisted first (in given order), listed after in stored order, stale ignored
	got := ApplyColumnOrder([]int64{1, 5, 3, 9}, []int64{3, 7, 1})
	want := []int64{5, 9, 3, 1}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("ApplyColumnOrder = %v, want %v", got, want)
		}
	}
}

func TestLinks(t *testing.T) {
	s := open(t)
	id, _ := s.CreateJob(&Job{Slug: "l"})
	l1, created, err := s.AddLink(id, "github_pr", "https://github.com/o/r/pull/1")
	if err != nil || !created || l1.ID == 0 || l1.Kind != "github_pr" {
		t.Fatalf("add: %v %v %+v", err, created, l1)
	}
	l1b, created, err := s.AddLink(id, "github_pr", "https://github.com/o/r/pull/1")
	if err != nil || created || l1b.ID != l1.ID {
		t.Fatalf("idempotent add: %v %v %+v", err, created, l1b)
	}
	l2, _, _ := s.AddLink(id, "url", "https://example.com")
	if err := s.SetLinkStatus(l1.ID, "open", 100); err != nil {
		t.Fatal(err)
	}
	ls, _ := s.ListLinks(id)
	if len(ls) != 2 || ls[0].Status != "open" || ls[0].StatusAt != 100 || ls[1].Kind != "url" || ls[1].URL != "https://example.com" {
		t.Fatalf("list: %+v %+v", ls[0], ls[1])
	}
	stale, _ := s.StalePRLinks(50, 3)
	if len(stale) != 0 {
		t.Fatalf("stale: %d", len(stale))
	}
	stale, _ = s.StalePRLinks(200, 3)
	if len(stale) != 1 {
		t.Fatalf("stale: %d", len(stale))
	}
	if err := s.RemoveLink(id, l2.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.RemoveLink(id, l2.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("remove twice: %v", err)
	}
	if err := s.RemoveLink(id+1, l1.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("remove under another job: %v", err)
	}
	all, _ := s.ListAllLinks()
	if len(all[id]) != 1 {
		t.Fatalf("all: %d", len(all[id]))
	}
	// cascade on delete
	if err := s.DeleteJob(id); err != nil {
		t.Fatal(err)
	}
	all, _ = s.ListAllLinks()
	if len(all) != 0 {
		t.Fatalf("links must cascade: %v", all)
	}
}

// TestMigrateV3 upgrades a v2 database by hand: the label column goes, the
// links stay, the scan offsets are forgotten and other kv rows are kept.
func TestMigrateV3(t *testing.T) {
	path := filepath.Join(t.TempDir(), "v2.db")
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	for _, stmt := range []string{schemaV1, schemaV2, `PRAGMA user_version = 2`,
		`INSERT INTO jobs (id, slug) VALUES (4, 'a')`,
		`INSERT INTO links (job_id, kind, url, label) VALUES (4, 'url', 'https://e.com/x', 'old label')`,
		`INSERT INTO kv VALUES ('scan:4', '120'), ('scan-pending:4', '["c1"]'), ('models', 'keep')`} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}
	db.Close()
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	var version int
	s.db.QueryRow(`PRAGMA user_version`).Scan(&version)
	if version != 3 {
		t.Fatalf("user_version %d", version)
	}
	var hasLabel int
	s.db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('links') WHERE name = 'label'`).Scan(&hasLabel)
	if hasLabel != 0 {
		t.Fatal("label column survived")
	}
	ls, _ := s.ListLinks(4)
	if len(ls) != 1 || ls[0].URL != "https://e.com/x" || ls[0].Kind != "url" {
		t.Fatalf("links after migration: %+v", ls)
	}
	for _, k := range []string{"scan:4", "scan-pending:4"} {
		if _, err := s.GetKV(k); !errors.Is(err, ErrNotFound) {
			t.Fatalf("%s must be forgotten: %v", k, err)
		}
	}
	if v, _ := s.GetKV("models"); v != "keep" {
		t.Fatalf("models kv = %q", v)
	}
	// the next job id is still past the one that exists
	id, _ := s.CreateJob(&Job{Slug: "b"})
	if id != 5 {
		t.Fatalf("next id %d", id)
	}
}

func TestKV(t *testing.T) {
	s := open(t)
	if _, err := s.GetKV("x"); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
	_ = s.SetKV("x", "1")
	_ = s.SetKV("x", "2")
	if v, _ := s.GetKV("x"); v != "2" {
		t.Fatalf("v = %q", v)
	}
	_ = s.DeleteKV("x")
	if _, err := s.GetKV("x"); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
}

func TestDeleteKVPrefix(t *testing.T) {
	s := open(t)
	for k, v := range map[string]string{"scan:1": "a", "scan:2": "b", "scan-pending:1": "x", "scanner-generation": "2"} {
		s.SetKV(k, v)
	}
	if err := s.DeleteKVPrefix("scan:"); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"scan:1", "scan:2"} {
		if _, err := s.GetKV(k); !errors.Is(err, ErrNotFound) {
			t.Fatalf("%s survived: %v", k, err)
		}
	}
	if v, _ := s.GetKV("scan-pending:1"); v != "x" {
		t.Fatalf("scan-pending:1 = %q", v)
	}
	if v, _ := s.GetKV("scanner-generation"); v != "2" {
		t.Fatalf("scanner-generation = %q (the prefix must not reach it)", v)
	}
}
