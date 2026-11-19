package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"driver/internal/jobs"
	"driver/internal/store"
)

func TestThreads(t *testing.T) {
	tmux := jobs.NewTmux()
	if !tmux.Available() {
		t.Skip("tmux not on PATH")
	}
	sock, err := os.MkdirTemp("", "drvtmux")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(sock) })
	t.Setenv("TMUX_TMPDIR", sock)
	t.Setenv("TMUX", "")
	t.Cleanup(func() {
		for _, s := range []string{"drv-multi-1", "drv-multi-1-t1", "drv-multi-1-t2", "drv-multi-1-t3"} {
			tmux.KillSession(s)
		}
	})

	st, err := store.Open(filepath.Join(t.TempDir(), "d.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	home, work := t.TempDir(), t.TempDir()
	fakePi := filepath.Join(t.TempDir(), "pi")
	os.WriteFile(fakePi, []byte("#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$DRIVER_JOB_DIR/argv\"\ncat > \"$DRIVER_JOB_DIR/typed\"\n"), 0o755)
	svc := &jobs.Service{Store: st, Cfg: jobs.Config{Home: home, Port: 7781, PiBin: fakePi}, Tmux: tmux}
	a := New(svc, jobs.NewDaemon(svc), "test")
	mux := http.NewServeMux()
	a.Register(mux)

	post := func(path, body string) (int, map[string]any) {
		t.Helper()
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, path, strings.NewReader(body)))
		var out map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatalf("%s: %s: %v", path, rec.Body.String(), err)
		}
		return rec.Code, out
	}
	get := func(path string, into any) int {
		t.Helper()
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if into != nil {
			if err := json.Unmarshal(rec.Body.Bytes(), into); err != nil {
				t.Fatalf("%s: %s: %v", path, rec.Body.String(), err)
			}
		}
		return rec.Code
	}
	waitFor := func(what string, ok func() bool) {
		t.Helper()
		for deadline := time.Now().Add(5 * time.Second); !ok(); time.Sleep(20 * time.Millisecond) {
			if time.Now().After(deadline) {
				t.Fatalf("timed out waiting for %s", what)
			}
		}
	}

	j, err := svc.Create("multi", "Multi", work, "")
	if err != nil {
		t.Fatal(err)
	}
	if code, out := post("/api/job/thread", `{"slug":"multi","op":"open","name":"CI"}`); code != http.StatusConflict {
		t.Fatalf("a thread on a not-started job: %d %v", code, out)
	}
	if err := svc.Submit(j); err != nil {
		t.Fatal(err)
	}
	j, _ = st.GetJobBySlug("multi")
	jobDir := filepath.Dir(j.PiSession)
	os.WriteFile(j.PiSession, []byte(`{"type":"session","version":3,"id":"h"}`+"\n"+
		`{"type":"message","id":"u1","parentId":"h","message":{"role":"user","content":"Go."}}`+"\n"+
		`{"type":"message","id":"a1","parentId":"u1","message":{"role":"assistant","content":[{"type":"text","text":"Done."}],"stopReason":"stop"}}`+"\n"), 0o644)
	os.WriteFile(filepath.Join(jobDir, "status.json"), []byte(`{"state":"idle"}`), 0o644)
	st.UpdateJobFields(j.ID, j.Rev, map[string]any{"state": "blocked"})

	var b webBoard
	get("/api/board", &b)
	if len(b.Jobs) != 1 || len(b.Jobs[0].Threads) != 1 || b.Jobs[0].Threads[0].ID != 0 || b.Jobs[0].Threads[0].Name != "Multi" || !b.Jobs[0].Threads[0].Live {
		t.Fatalf("a job's own pi is its first thread: %+v", b.Jobs[0].Threads)
	}

	code, out := post("/api/job/thread", `{"slug":"multi","op":"open","name":"  CI "}`)
	if code != http.StatusOK {
		t.Fatalf("open: %d %v", code, out)
	}
	th := out["thread"].(map[string]any)
	id := int64(th["id"].(float64))
	if id == 0 || th["name"] != "CI" || th["live"] != true {
		t.Fatalf("open: %v", out)
	}
	if code, out := post("/api/job/thread", `{"slug":"multi","op":"open","name":"   "}`); code != http.StatusConflict {
		t.Fatalf("a nameless thread: %d %v", code, out)
	}
	if code, out := post("/api/job/thread", `{"slug":"multi","op":"open","name":"`+strings.Repeat("x", 61)+`"}`); code != http.StatusBadRequest {
		t.Fatalf("a long name: %d %v", code, out)
	}
	if code, out := post("/api/job/thread", `{"slug":"multi","op":"open","from":99,"name":"x"}`); code != http.StatusNotFound {
		t.Fatalf("from an unknown thread: %d %v", code, out)
	}
	if code, out := post("/api/job/thread", `{"slug":"nope","op":"open","name":"x"}`); code != http.StatusNotFound {
		t.Fatalf("unknown job: %d %v", code, out)
	}
	if code, out := post("/api/job/thread", `{"slug":"multi","op":"dance","id":`+jsonInt(id)+`}`); code != http.StatusBadRequest {
		t.Fatalf("unknown op: %d %v", code, out)
	}

	get("/api/board", &b)
	threads := b.Jobs[0].Threads
	if len(threads) != 2 || threads[1].ID != id || threads[1].Name != "CI" || !threads[1].Live || threads[1].Pi == nil || threads[1].Pi.State != "starting" {
		t.Fatalf("threads on the board: %+v", threads)
	}

	tdir := filepath.Join(jobDir, "t"+jsonInt(id))
	waitFor("the thread's pi to start", func() bool { _, err := os.Stat(filepath.Join(tdir, "argv")); return err == nil })

	// the thread's record and pane
	var conv webConversation
	if code := get("/api/job/conversation?slug=multi&thread="+jsonInt(id), &conv); code != http.StatusOK || len(conv.Messages) != 2 || conv.Messages[1].Text != "Done." {
		t.Fatalf("the thread's record starts as a copy: %d %+v", code, conv)
	}
	os.WriteFile(filepath.Join(tdir, "session.jsonl"), []byte(`{"type":"session","version":3,"id":"h2"}`+"\n"+
		`{"type":"message","id":"u1","parentId":"h2","message":{"role":"user","content":"Go."}}`+"\n"), 0o644)
	if get("/api/job/conversation?slug=multi&thread="+jsonInt(id), &conv); len(conv.Messages) != 1 || !conv.Pending {
		t.Fatalf("the thread's own record: %+v", conv)
	}
	if get("/api/job/conversation?slug=multi", &conv); len(conv.Messages) != 2 {
		t.Fatalf("the job's record is the first thread's: %+v", conv)
	}
	if code := get("/api/job/conversation?slug=multi&thread=99", nil); code != http.StatusNotFound {
		t.Fatalf("the record of an unknown thread: %d", code)
	}
	if code := get("/api/term?slug=multi&thread=99", nil); code != http.StatusNotFound {
		t.Fatalf("the pane of an unknown thread: %d", code)
	}

	if code, out := post("/api/job/thread", `{"slug":"multi","op":"rename","id":`+jsonInt(id)+`,"name":"CI and more"}`); code != http.StatusOK {
		t.Fatalf("rename: %d %v", code, out)
	}
	if code, out := post("/api/job/thread", `{"slug":"multi","op":"stop","id":`+jsonInt(id)+`}`); code != http.StatusOK {
		t.Fatalf("stop: %d %v", code, out)
	}
	if code, out := post("/api/job/thread", `{"slug":"multi","op":"retry","id":`+jsonInt(id)+`}`); code != http.StatusConflict {
		t.Fatalf("retry a thread with nothing to retry: %d %v", code, out)
	}
	if code, out := post("/api/job/thread", `{"slug":"multi","op":"close","id":`+jsonInt(id)+`}`); code != http.StatusOK {
		t.Fatalf("close: %d %v", code, out)
	}
	if code, out := post("/api/job/thread", `{"slug":"multi","op":"close","id":`+jsonInt(id)+`}`); code != http.StatusConflict {
		t.Fatalf("close twice: %d %v", code, out)
	}
	get("/api/board", &b)
	if len(b.Jobs[0].Threads) != 1 {
		t.Fatalf("a closed thread leaves the tabs: %+v", b.Jobs[0].Threads)
	}

	// the first tab closes too, when another is open: the next one takes
	// its place and its name comes with it
	if code, out := post("/api/job/thread", `{"slug":"multi","op":"close","id":0}`); code != http.StatusConflict {
		t.Fatalf("the last tab stays: %d %v", code, out)
	}
	code, out = post("/api/job/thread", `{"slug":"multi","op":"open","name":"Second"}`)
	if code != http.StatusOK {
		t.Fatalf("open: %d %v", code, out)
	}
	second := int64(out["thread"].(map[string]any)["id"].(float64))
	if code, out := post("/api/job/thread", `{"slug":"multi","op":"close","id":0}`); code != http.StatusOK {
		t.Fatalf("close the first tab: %d %v", code, out)
	}
	get("/api/board", &b)
	if len(b.Jobs[0].Threads) != 1 || b.Jobs[0].Threads[0].ID != 0 || b.Jobs[0].Threads[0].Name != "Second" {
		t.Fatalf("the next tab is the job's own now, under its name: %+v", b.Jobs[0].Threads)
	}
	if code := get("/api/job/conversation?slug=multi&thread="+jsonInt(second), nil); code != http.StatusNotFound {
		t.Fatalf("the promoted thread's old id is gone: %d", code)
	}
	if code := get("/api/job/conversation?slug=multi&thread="+jsonInt(id), &conv); code != http.StatusOK || len(conv.Messages) != 1 {
		t.Fatalf("a closed thread's record is still readable: %d %+v", code, conv)
	}
}
