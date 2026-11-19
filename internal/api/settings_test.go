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

func TestPasteAndSettings(t *testing.T) {
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
	t.Cleanup(func() { tmux.KillSession("drv-p-1") })

	st, err := store.Open(filepath.Join(t.TempDir(), "d.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	home, work, userHome := t.TempDir(), t.TempDir(), t.TempDir()
	os.MkdirAll(filepath.Join(userHome, "Desktop"), 0o755)
	fakePi := filepath.Join(t.TempDir(), "pi")
	os.WriteFile(fakePi, []byte("#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$DRIVER_JOB_DIR/argv\"\ncat > \"$DRIVER_JOB_DIR/typed\"\n"), 0o755)
	svc := &jobs.Service{Store: st, Cfg: jobs.Config{Home: home, Port: 7781, PiBin: fakePi, UserHome: userHome, Browser: "No Such Browser"}, Tmux: tmux}
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

	j, err := svc.Create("p", "", work, "")
	if err != nil {
		t.Fatal(err)
	}
	if code := call("POST", "/api/job/paste", `{"slug":"p","text":"hi"}`, nil); code != http.StatusConflict {
		t.Fatalf("paste into a job with no pane: %d", code)
	}
	if err := svc.Submit(j); err != nil {
		t.Fatal(err)
	}
	j, _ = st.GetJobBySlug("p")
	jobDir := filepath.Dir(j.PiSession)
	for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(20 * time.Millisecond) {
		if b, _ := os.ReadFile(filepath.Join(jobDir, "argv")); len(b) > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("pi never started")
		}
	}
	if code := call("POST", "/api/job/paste", `{"slug":"p","text":"from the board\n"}`, nil); code != http.StatusOK {
		t.Fatalf("paste: %d", code)
	}
	for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(20 * time.Millisecond) {
		if b, _ := os.ReadFile(filepath.Join(jobDir, "typed")); strings.Contains(string(b), "from the board") {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the paste never landed")
		}
	}
	if code := call("POST", "/api/job/paste", `{"slug":"p","text":"  "}`, nil); code != http.StatusConflict {
		t.Fatalf("an empty paste: %d", code)
	}
	if code := call("POST", "/api/job/paste", `{"slug":"p","text":"`+strings.Repeat("x", 64001)+`"}`, nil); code != http.StatusBadRequest {
		t.Fatalf("a long paste: %d", code)
	}
	if code := call("POST", "/api/job/paste", `{"slug":"p","thread":99,"text":"x"}`, nil); code != http.StatusNotFound {
		t.Fatalf("an unknown thread: %d", code)
	}
	if code := call("POST", "/api/job/paste", `{"slug":"nope","text":"x"}`, nil); code != http.StatusNotFound {
		t.Fatalf("an unknown job: %d", code)
	}

	// the front tab of a browser that is not running
	if code := call("GET", "/api/context/browser", "", nil); code != http.StatusConflict {
		t.Fatalf("browser context without a browser: %d", code)
	}

	var settings struct {
		Version string `json:"version"`
		Binary  string `json:"binary"`
		Home    string `json:"home"`
		Browser string `json:"browser"`
	}
	if code := call("GET", "/api/settings", "", &settings); code != http.StatusOK || settings.Version != "test" || settings.Binary == "" || settings.Home != home || settings.Browser != "No Such Browser" {
		t.Fatalf("settings: %d %+v", code, settings)
	}
	var probe struct {
		Folders []jobs.FolderProbe `json:"folders"`
	}
	if code := call("POST", "/api/settings/probe", `{"what":"folders"}`, &probe); code != http.StatusOK || len(probe.Folders) != 3 || probe.Folders[0].State != "ok" || probe.Folders[1].State != "missing" {
		t.Fatalf("folders probe: %d %+v", code, probe)
	}
	var bp struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}
	if code := call("POST", "/api/settings/probe", `{"what":"browser"}`, &bp); code != http.StatusOK || bp.OK || !strings.Contains(bp.Error, "not running") {
		t.Fatalf("browser probe: %d %+v", code, bp)
	}
	if code := call("POST", "/api/settings/probe", `{"what":"weather"}`, nil); code != http.StatusBadRequest {
		t.Fatalf("unknown probe: %d", code)
	}
	if code := call("POST", "/api/settings/open", `{"pane":"nowhere"}`, nil); code != http.StatusBadRequest {
		t.Fatalf("unknown pane: %d", code)
	}
}
