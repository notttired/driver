package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"driver/internal/jobs"
	"driver/internal/store"
)

func TestJobNotes(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "d.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	svc := &jobs.Service{Store: st, Cfg: jobs.Config{Home: t.TempDir()}, Tmux: jobs.NewTmux()}
	a := New(svc, jobs.NewDaemon(svc), "test")
	j, err := svc.Create("notes", "", t.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	jobDir := svc.Cfg.FreshJobDir(j)
	if _, err := st.UpdateJobFields(j.ID, j.Rev, map[string]any{"pi_session": filepath.Join(jobDir, "session.jsonl")}); err != nil {
		t.Fatal(err)
	}
	post := func(body string) (int, map[string]any) {
		t.Helper()
		rec := httptest.NewRecorder()
		a.jobNotes(rec, httptest.NewRequest(http.MethodPost, "/api/job/notes", strings.NewReader(body)))
		var out map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatalf("%s: %v", rec.Body.String(), err)
		}
		return rec.Code, out
	}
	get := func(query string) (int, map[string]any) {
		t.Helper()
		rec := httptest.NewRecorder()
		a.jobNotes(rec, httptest.NewRequest(http.MethodGet, "/api/job/notes?"+query, nil))
		var out map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatalf("%s: %v", rec.Body.String(), err)
		}
		return rec.Code, out
	}
	byDir := "job_dir=" + url.QueryEscape(jobDir)
	dirJSON, _ := json.Marshal(jobDir)

	if code, out := post(`{"slug":"notes","text":"mine\n"}`); code != http.StatusOK || out["ok"] != true {
		t.Fatalf("replace by slug: %d %v", code, out)
	}
	if code, out := get("slug=notes"); code != http.StatusOK || out["text"] != "mine\n" {
		t.Fatalf("read by slug: %d %v", code, out)
	}
	if code, out := get(byDir); code != http.StatusOK || out["text"] != "mine\n" {
		t.Fatalf("pi reads them by its job dir: %d %v", code, out)
	}

	if code, out := post(`{"job_dir":` + string(dirJSON) + `,"op":"append","text":"from pi"}`); code != http.StatusOK || out["ok"] != true {
		t.Fatalf("append: %d %v", code, out)
	}
	if _, out := get("slug=notes"); out["text"] != "mine\n\nfrom pi" {
		t.Fatalf("an append goes at the end after a blank line: %q", out["text"])
	}

	if _, err := svc.Rename(mustLookup(t, svc, "notes"), "renamed", ""); err != nil {
		t.Fatal(err)
	}
	if code, out := post(`{"job_dir":` + string(dirJSON) + `,"op":"replace","text":"rewritten"}`); code != http.StatusOK || out["ok"] != true {
		t.Fatalf("replace by job dir after a rename: %d %v", code, out)
	}
	if _, out := get("slug=renamed"); out["text"] != "rewritten" {
		t.Fatalf("after replace: %q", out["text"])
	}

	if _, err := svc.Create("empty", "", t.TempDir(), ""); err != nil {
		t.Fatal(err)
	}
	if code, out := post(`{"slug":"empty","op":"append","text":"first"}`); code != http.StatusOK || out["ok"] != true {
		t.Fatalf("append by slug: %d %v", code, out)
	}
	if _, out := get("slug=empty"); out["text"] != "first" {
		t.Fatalf("an append to empty notes is the text alone: %q", out["text"])
	}

	if code, out := post(`{"slug":"renamed","op":"append","text":"  \n"}`); code != http.StatusBadRequest || out["error"] != "nothing to add" {
		t.Fatalf("empty append: %d %v", code, out)
	}
	if code, out := post(`{"slug":"renamed","op":"sort","text":"x"}`); code != http.StatusBadRequest || out["error"] != "op must be replace or append" {
		t.Fatalf("unknown op: %d %v", code, out)
	}
	if code, out := post(`{"slug":"renamed","op":"append","text":"` + strings.Repeat("x", jobs.MaxNotesRunes) + `"}`); code != http.StatusConflict ||
		out["error"] != "notes exceed 64000 characters" {
		t.Fatalf("an append past the cap: %d %v", code, out)
	}
	if code, out := get("job_dir=" + url.QueryEscape(filepath.Join(svc.Cfg.JobsDir(), "gone-99"))); code != http.StatusNotFound || out["error"] != "no such job" {
		t.Fatalf("unknown job dir: %d %v", code, out)
	}
}

func mustLookup(t *testing.T, svc *jobs.Service, slug string) *store.Job {
	t.Helper()
	j, err := svc.Lookup(slug)
	if err != nil {
		t.Fatal(err)
	}
	return j
}
