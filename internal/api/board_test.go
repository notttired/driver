package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"driver/internal/jobs"
	"driver/internal/store"
)

func TestBoardRetryable(t *testing.T) {
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

	st, err := store.Open(filepath.Join(t.TempDir(), "d.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	home, work := t.TempDir(), t.TempDir()
	svc := &jobs.Service{Store: st, Cfg: jobs.Config{Home: home}, Tmux: tmux}
	a := New(svc, jobs.NewDaemon(svc), "test")
	t.Cleanup(func() {
		for _, s := range []string{"drv-failed-1", "drv-waiting-2"} {
			tmux.KillSession(s)
		}
	})

	failed := `{"type":"message","id":"a1","parentId":"u1","message":{"role":"assistant","content":[],"stopReason":"error","errorMessage":"Connection error."}}`
	answered := `{"type":"message","id":"a1","parentId":"u1","message":{"role":"assistant","content":[{"type":"text","text":"Done."}],"stopReason":"stop"}}`
	put := func(slug, state, last string, live bool) {
		t.Helper()
		j, err := svc.Create(slug, "", work, "")
		if err != nil {
			t.Fatal(err)
		}
		dir := svc.Cfg.FreshJobDir(j)
		os.MkdirAll(dir, 0o755)
		session := filepath.Join(dir, "session.jsonl")
		os.WriteFile(session, []byte(`{"type":"message","id":"u1","message":{"role":"user","content":"Go."}}`+"\n"+last+"\n"), 0o644)
		os.WriteFile(filepath.Join(dir, "status.json"), []byte(`{"state":"idle"}`), 0o644)
		sess := jobs.FreshSessionName(j)
		if live {
			if err := tmux.NewSession(sess, work, slug, slug, "sleep 60"); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := st.UpdateJobFields(j.ID, j.Rev, map[string]any{"state": state, "pi_session": session, "tmux": sess, "started_at": int64(1)}); err != nil {
			t.Fatal(err)
		}
	}
	put("failed", "blocked", failed, true)
	put("waiting", "blocked", answered, true)
	put("exited", "blocked", answered, false)
	put("exited-failed", "blocked", failed, false)
	put("busy", "working", failed, false)

	rec := httptest.NewRecorder()
	a.board(rec, httptest.NewRequest(http.MethodGet, "/api/board", nil))
	var b webBoard
	if err := json.Unmarshal(rec.Body.Bytes(), &b); err != nil {
		t.Fatalf("%s: %v", rec.Body.String(), err)
	}
	got := map[string]webJob{}
	for _, j := range b.Jobs {
		got[j.Slug] = j
	}
	for slug, want := range map[string]struct {
		retryable     bool
		state, errMsg string
	}{
		"failed":        {true, "idle", "Connection error."},
		"waiting":       {false, "idle", ""},
		"exited":        {true, "exited", ""},
		"exited-failed": {true, "exited", "Connection error."},
		"busy":          {false, "", ""},
	} {
		j := got[slug]
		state, errMsg := "", ""
		if j.Pi != nil && j.State == "blocked" {
			state, errMsg = j.Pi.State, j.Pi.Error
		}
		if j.Retryable != want.retryable || state != want.state || errMsg != want.errMsg {
			t.Errorf("%s: retryable=%v pi=%+v", slug, j.Retryable, j.Pi)
		}
	}
}
