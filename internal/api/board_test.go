package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
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

func TestBoardTabRings(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "d.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	home, work := t.TempDir(), t.TempDir()
	svc := &jobs.Service{Store: st, Cfg: jobs.Config{Home: home}, Tmux: jobs.NewTmux()}
	a := New(svc, jobs.NewDaemon(svc), "test")
	mux := http.NewServeMux()
	a.Register(mux)
	call := func(method, path, body string, into any) int {
		t.Helper()
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(method, path, strings.NewReader(body)))
		if into != nil {
			if err := json.Unmarshal(rec.Body.Bytes(), into); err != nil {
				t.Fatalf("%s: %s: %v", path, rec.Body.String(), err)
			}
		}
		return rec.Code
	}
	j, err := svc.Create("multi", "Multi", work, "")
	if err != nil {
		t.Fatal(err)
	}
	st.UpdateJobFields(j.ID, j.Rev, map[string]any{"state": "working", "started_at": int64(1)})
	ci := &store.Thread{JobID: j.ID, Name: "CI", StartedAt: 1}
	st.CreateThread(ci)
	docs := &store.Thread{JobID: j.ID, Name: "Docs", StartedAt: 1}
	st.CreateThread(docs)

	tabs := func() (card bool, rings map[int64]bool) {
		t.Helper()
		var b webBoard
		if code := call("GET", "/api/board", "", &b); code != http.StatusOK || len(b.Jobs) != 1 || len(b.Jobs[0].Threads) != 3 {
			t.Fatalf("board: %d %+v", code, b.Jobs)
		}
		rings = map[int64]bool{}
		for _, tab := range b.Jobs[0].Threads {
			rings[tab.ID] = tab.Attention
		}
		return b.Jobs[0].Attention, rings
	}
	if card, rings := tabs(); card || rings[0] || rings[ci.ID] || rings[docs.ID] {
		t.Fatalf("no rings yet: card %v tabs %v", card, rings)
	}

	st.UpdateThreadFields(ci.ID, map[string]any{"attention_at": int64(5)})
	st.UpdateThreadFields(docs.ID, map[string]any{"attention_at": int64(6)})
	if card, rings := tabs(); !card || rings[0] || !rings[ci.ID] || !rings[docs.ID] {
		t.Fatalf("a ringing tab rings the card, working or not: card %v tabs %v", card, rings)
	}
	if code := call("POST", "/api/job/seen", `{"slug":"multi","thread":`+jsonInt(ci.ID)+`}`, nil); code != http.StatusOK {
		t.Fatalf("seen on a tab: %d", code)
	}
	if card, rings := tabs(); !card || rings[ci.ID] || !rings[docs.ID] {
		t.Fatalf("seen takes only that tab's ring; the card rings while another does: card %v tabs %v", card, rings)
	}
	call("POST", "/api/job/seen", `{"slug":"multi","thread":`+jsonInt(docs.ID)+`}`, nil)
	if card, rings := tabs(); card || rings[docs.ID] {
		t.Fatalf("every tab seen: card %v tabs %v", card, rings)
	}

	j, _ = st.GetJobBySlug("multi")
	st.UpdateJobFields(j.ID, j.Rev, map[string]any{"own_attention_at": int64(7)})
	if card, rings := tabs(); !card || !rings[0] {
		t.Fatalf("the first tab rings: card %v tabs %v", card, rings)
	}
	if code := call("POST", "/api/job/seen", `{"slug":"multi"}`, nil); code != http.StatusOK {
		t.Fatalf("seen on the first tab: %d", code)
	}
	if card, rings := tabs(); card || rings[0] {
		t.Fatalf("seen with no thread is the first tab's: card %v tabs %v", card, rings)
	}

	j, _ = st.GetJobBySlug("multi")
	st.UpdateJobFields(j.ID, j.Rev, map[string]any{"attention_at": int64(8)})
	call("POST", "/api/job/seen", `{"slug":"multi","thread":`+jsonInt(docs.ID)+`}`, nil)
	if card, _ := tabs(); card {
		t.Fatal("seen on any tab takes the card's own ring")
	}
	if code := call("POST", "/api/job/seen", `{"slug":"multi","thread":99}`, nil); code != http.StatusNotFound {
		t.Fatalf("seen on an unknown thread: %d", code)
	}
	if code := call("POST", "/api/job/seen", `{"slug":"nope"}`, nil); code != http.StatusNotFound {
		t.Fatalf("seen on an unknown job: %d", code)
	}
}
