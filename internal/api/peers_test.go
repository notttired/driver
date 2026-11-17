package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"driver/internal/jobs"
	"driver/internal/store"
)

func TestPeers(t *testing.T) {
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
		var r *http.Request
		if body != "" {
			r = httptest.NewRequest(method, path, strings.NewReader(body))
		} else {
			r = httptest.NewRequest(method, path, nil)
		}
		mux.ServeHTTP(rec, r)
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
	jobDir := svc.Cfg.FreshJobDir(j)
	os.MkdirAll(jobDir, 0o755)
	own := filepath.Join(jobDir, "session.jsonl")
	line := func(id, parent, role, text string) string {
		return `{"type":"message","id":"` + id + `","parentId":"` + parent + `","message":{"role":"` + role + `","content":[{"type":"text","text":"` + text + `"}],"stopReason":"stop"}}` + "\n"
	}
	header := `{"type":"session","version":3,"id":"h"}` + "\n"
	os.WriteFile(own, []byte(header+line("u1", "h", "user", "Go.")+line("a1", "u1", "assistant", "Done.")), 0o644)
	st.UpdateJobFields(j.ID, j.Rev, map[string]any{"state": "blocked", "pi_session": own})
	ci := &store.Thread{JobID: j.ID, Name: "CI"}
	st.CreateThread(ci)
	cidir := filepath.Join(jobDir, "t1")
	os.MkdirAll(cidir, 0o755)
	cis := filepath.Join(cidir, "session.jsonl")
	st.UpdateThreadFields(ci.ID, map[string]any{"pi_session": cis})
	os.WriteFile(cis, []byte(header+line("u1", "h", "user", "Go.")+line("a1", "u1", "assistant", "Done.")+line("u2", "a1", "user", "And CI?")+line("a2", "u2", "assistant", "Green.")), 0o644)

	q := func(dir string) string { return "job_dir=" + url.QueryEscape(dir) }

	var list struct {
		Self    int64             `json:"self"`
		Threads []jobs.ThreadInfo `json:"threads"`
	}
	if code := call("GET", "/api/job/threads?"+q(cidir), "", &list); code != http.StatusOK || list.Self != ci.ID || len(list.Threads) != 2 || list.Threads[0].Name != "Multi" || list.Threads[1].Name != "CI" {
		t.Fatalf("list from a thread: %d %+v", code, list)
	}
	if code := call("GET", "/api/job/threads?"+q(jobDir), "", &list); code != http.StatusOK || list.Self != 0 {
		t.Fatalf("list from the job's pi: %d %+v", code, list)
	}
	if code := call("GET", "/api/job/threads?"+q("/nowhere"), "", nil); code != http.StatusNotFound {
		t.Fatalf("unknown dir: %d", code)
	}

	var up struct {
		Text    string `json:"text"`
		Carried []struct {
			Thread int64  `json:"thread"`
			Entry  string `json:"entry"`
		} `json:"carried"`
		Peers int `json:"peers"`
	}
	if code := call("POST", "/api/job/threads/update", `{"job_dir":"`+jobDir+`","seen":["h","u1","a1"]}`, &up); code != http.StatusOK {
		t.Fatalf("update: %d %+v", code, up)
	}
	if up.Peers != 1 || !strings.Contains(up.Text, "user: And CI?") || !strings.Contains(up.Text, "CI: Green.") || len(up.Carried) != 2 || up.Carried[0].Thread != ci.ID || up.Carried[0].Entry != "u2" {
		t.Fatalf("update: %+v", up)
	}
	if code := call("POST", "/api/job/threads/update", `{"job_dir":"`+jobDir+`","seen":["h","u1","a1","u2","a2"]}`, &up); code != http.StatusOK || up.Text != "" || len(up.Carried) != 0 {
		t.Fatalf("nothing new: %d %+v", code, up)
	}
	if code := call("POST", "/api/job/threads/update", `{"job_dir":"/nowhere","seen":[]}`, nil); code != http.StatusNotFound {
		t.Fatalf("update from an unknown dir: %d", code)
	}

	var hist struct {
		Text string `json:"text"`
	}
	if code := call("GET", "/api/job/threads/history?"+q(jobDir)+"&thread="+jsonInt(ci.ID), "", &hist); code != http.StatusOK || !strings.Contains(hist.Text, "user: And CI?") || !strings.Contains(hist.Text, "assistant: Green.") {
		t.Fatalf("history: %d %+v", code, hist)
	}
	if code := call("GET", "/api/job/threads/history?"+q(cidir)+"&thread=0", "", &hist); code != http.StatusOK || strings.Contains(hist.Text, "And CI?") {
		t.Fatalf("the job's own history: %d %+v", code, hist)
	}
	if code := call("GET", "/api/job/threads/history?"+q(jobDir)+"&thread=99", "", nil); code != http.StatusNotFound {
		t.Fatalf("history of an unknown thread: %d", code)
	}
	if code := call("GET", "/api/job/threads/history?"+q(jobDir)+"&thread=x", "", nil); code != http.StatusBadRequest {
		t.Fatalf("history with a bad thread: %d", code)
	}
}
