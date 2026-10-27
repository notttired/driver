package jobs

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"driver/internal/pi"
	"driver/internal/store"
)

func TestDisplayStateAndObserve(t *testing.T) {
	now := int64(1_000_000)
	cases := []struct {
		name            string
		j               store.Job
		live            bool
		st              *pi.Status
		wantState, want string // derived state, pi state
		reason          string
	}{
		{"draft", store.Job{State: ""}, false, nil, "draft", "", ""},
		{"done", store.Job{State: "done"}, true, &pi.Status{State: "working"}, "done", "", ""},
		{"exited", store.Job{State: "working", StartedAt: now - 10}, false, &pi.Status{State: "working"}, "blocked", "exited", "exited"},
		{"working", store.Job{State: "blocked"}, true, &pi.Status{State: "working", Tool: "bash"}, "working", "working", ""},
		{"idle", store.Job{State: "working"}, true, &pi.Status{State: "idle"}, "blocked", "idle", "idle"},
		{"dialog", store.Job{State: "working"}, true, &pi.Status{State: "dialog"}, "blocked", "dialog", "dialog"},
		{"aborted", store.Job{State: "working"}, true, &pi.Status{State: "aborted"}, "blocked", "aborted", "aborted"},
		{"starting", store.Job{State: "working", StartedAt: now - 30}, true, nil, "working", "starting", ""},
		{"silent", store.Job{State: "working", StartedAt: now - 200}, true, nil, "blocked", "silent", "silent"},
		// an opened draft: pi at its prompt, nothing sent — stays in DRAFT
		{"ready starting", store.Job{State: "ready", StartedAt: now - 3}, true, nil, "ready", "starting", ""},
		{"ready", store.Job{State: "ready", StartedAt: now - 300}, true, &pi.Status{State: "ready"}, "ready", "ready", ""},
		{"ready silent", store.Job{State: "ready", StartedAt: now - 300}, true, nil, "blocked", "silent", "silent"},
		{"ready exited", store.Job{State: "ready", StartedAt: now - 10}, false, nil, "blocked", "exited", "exited"},
		{"ready briefed", store.Job{State: "ready"}, true, &pi.Status{State: "working", Tool: "read"}, "working", "working", ""},
		{"ready dialog", store.Job{State: "ready"}, true, &pi.Status{State: "dialog"}, "blocked", "dialog", "dialog"},
		// pi reporting ready from a working job (a /new in the pane) is a draft again
		{"working to ready", store.Job{State: "working"}, true, &pi.Status{State: "ready"}, "ready", "ready", ""},
	}
	for _, c := range cases {
		o := Observe(&c.j, c.live, c.st, now)
		if o.State != c.wantState {
			t.Errorf("%s: state %q want %q", c.name, o.State, c.wantState)
		}
		if c.want == "" {
			if o.Pi != nil {
				t.Errorf("%s: pi must be nil", c.name)
			}
			continue
		}
		if o.Pi == nil || o.Pi.State != c.want || o.Pi.Reason != c.reason {
			t.Errorf("%s: pi = %+v", c.name, o.Pi)
		}
	}
	if DisplayState(&store.Job{State: ""}) != "draft" || DisplayState(&store.Job{State: "weird"}) != "draft" || DisplayState(&store.Job{State: "blocked"}) != "blocked" || DisplayState(&store.Job{State: "ready"}) != "draft" {
		t.Error("DisplayState")
	}
	if Opened(&store.Job{State: ""}) || !Opened(&store.Job{State: "ready"}) || !Opened(&store.Job{State: "done"}) {
		t.Error("Opened")
	}
}

func TestHoldBlocked(t *testing.T) {
	// first blocked reading of a working job is held
	state, first := holdBlocked("working", "blocked", 0, 5)
	if state != "working" || first != 5 {
		t.Fatalf("first reading: %s %d", state, first)
	}
	// adjacent second reading confirms
	state, first = holdBlocked("working", "blocked", 5, 6)
	if state != "blocked" || first != 5 {
		t.Fatalf("second reading: %s %d", state, first)
	}
	// a non-adjacent stale hold restarts
	state, first = holdBlocked("working", "blocked", 5, 9)
	if state != "working" || first != 9 {
		t.Fatalf("stale hold: %s %d", state, first)
	}
	// working readings clear the hold; blocked → working is immediate
	if s, f := holdBlocked("working", "working", 5, 6); s != "working" || f != 0 {
		t.Fatal("working clears")
	}
	if s, f := holdBlocked("blocked", "working", 0, 6); s != "working" || f != 0 {
		t.Fatal("unblock immediate")
	}
	if s, _ := holdBlocked("blocked", "blocked", 0, 6); s != "blocked" {
		t.Fatal("blocked stays")
	}
}

func TestLaunchCommand(t *testing.T) {
	cfg := Config{Home: "/h o/me", Port: 7781, PiFlags: []string{"-a", "--x"}}
	j := &store.Job{ID: 7, Slug: "my-job", Model: "it's"}
	cmd := cfg.LaunchCommand(j, "/opt/bin/pi", "/h o/me/jobs/my-job-7")
	want := `env DRIVER_JOB_DIR='/h o/me/jobs/my-job-7' DRIVER_JOB_SLUG='my-job' DRIVER_SERVE='http://127.0.0.1:7781' '/opt/bin/pi' --session '/h o/me/jobs/my-job-7/session.jsonl' --name 'my-job' --model 'it'\''s' -e '/h o/me/ext/driver.ts' '-a' '--x'`
	if cmd != want {
		t.Fatalf("\n got %s\nwant %s", cmd, want)
	}
	j.Model = ""
	cfg.PiFlags = nil
	cmd = cfg.LaunchCommand(j, "pi", "/d")
	if strings.Contains(cmd, "--model") || strings.Contains(cmd, " -- ") || !strings.HasSuffix(cmd, "-e '/h o/me/ext/driver.ts'") {
		t.Fatalf("bare cmd: %s", cmd)
	}
	if ShellQuote(`a'b`) != `'a'\''b'` {
		t.Fatal("ShellQuote")
	}
}

func TestEditorArgv(t *testing.T) {
	cases := []struct{ editor, code, dir, want string }{
		{"", "/usr/local/bin/code", "/w d", "/usr/local/bin/code|/w d"},
		{"", "", "/w", "open|-b|com.microsoft.VSCode|/w"},
		{"cursor -r", "/usr/local/bin/code", "/w", "cursor|-r|/w"},
		{"  zed  ", "", "/w", "zed|/w"},
	}
	for _, c := range cases {
		if got := strings.Join(EditorArgv(c.editor, c.code, c.dir), "|"); got != c.want {
			t.Errorf("EditorArgv(%q, %q, %q) = %q, want %q", c.editor, c.code, c.dir, got, c.want)
		}
	}
}

func TestSlugColorKind(t *testing.T) {
	for _, ok := range []string{"a", "a-b", "0", strings.Repeat("a", 64)} {
		if !ValidSlug(ok) {
			t.Errorf("%q should be valid", ok)
		}
	}
	for _, bad := range []string{"", "-a", "A", "a_b", "a b", strings.Repeat("a", 65), "é"} {
		if ValidSlug(bad) {
			t.Errorf("%q should be invalid", bad)
		}
	}
	if !ValidColor("cyan") || ValidColor("") || ValidColor("black") {
		t.Error("ValidColor")
	}
	kinds := map[string]string{
		"https://github.com/o/r/pull/12":   "github_pr",
		"https://github.com/o/r/pull/12/":  "github_pr",
		"https://github.com/o/r/issues/12": "github",
		"https://gist.github.com/x":        "github",
		"https://foo.slack.com/archives/C": "slack",
		"https://linear.app/t/issue/X-1":   "linear",
		"https://example.com/":             "url",
	}
	for u, k := range kinds {
		if got := LinkKind(u); got != k {
			t.Errorf("%s: %s want %s", u, got, k)
		}
	}
	if ValidLinkURL("ftp://x") || ValidLinkURL("github.com/x") || !ValidLinkURL("http://x/y") {
		t.Error("ValidLinkURL")
	}
}

func TestPRScanner(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "d.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	j := &store.Job{Slug: "s", State: "working"}
	st.CreateJob(j)
	sess := filepath.Join(t.TempDir(), "session.jsonl")
	j.PiSession = sess
	call := `{"type":"message","id":"a1","message":{"role":"assistant","content":[{"type":"toolCall","id":"c1","name":"bash","arguments":{"command":"gh pr create --fill"}},{"type":"toolCall","id":"c2","name":"bash","arguments":{"command":"ls"}}]}}` + "\n"
	os.WriteFile(sess, []byte(`{"type":"session","version":3}`+"\n"+call+`{"type":"message","id":"a2","message":{"role":"toolResult","toolCallId":"c2","content":[{"type":"text","text":"https://github.com/o/r/pull/99"}]}}`+"\n"), 0o644)
	sc := newPRScanner(st)
	if err := sc.scan(j); err != nil {
		t.Fatal(err)
	}
	if ls, _ := st.ListLinks(j.ID); len(ls) != 0 {
		t.Fatalf("a non-create result must not attach: %+v", ls)
	}
	if v, _ := st.GetKV(scanPendingKey(j.ID)); v != `["c1"]` {
		t.Fatalf("pending = %q", v)
	}
	// restart: a fresh scanner reloads pending from kv, then the result lands
	sc = newPRScanner(st)
	f, _ := os.OpenFile(sess, os.O_APPEND|os.O_WRONLY, 0)
	f.WriteString(`{"type":"message","id":"a3","message":{"role":"toolResult","toolCallId":"c1","content":[{"type":"text","text":"https://github.com/o/r/pull/12\nhttps://github.com/o/r/pull/12\n"}]}}` + "\n" + `{"type":"usage"}` + "\n" + `{"partial":`)
	f.Close()
	if err := sc.scan(j); err != nil {
		t.Fatal(err)
	}
	ls, _ := st.ListLinks(j.ID)
	if len(ls) != 1 || ls[0].URL != "https://github.com/o/r/pull/12" || ls[0].Kind != "github_pr" {
		t.Fatalf("links = %+v", ls)
	}
	if _, err := st.GetKV(scanPendingKey(j.ID)); err == nil {
		t.Fatal("pending must be cleared")
	}
	off, _ := st.GetKV(scanOffsetKey(j.ID))
	info, _ := os.Stat(sess)
	if off == "" || off == "0" || off == strconv.FormatInt(info.Size(), 10) {
		t.Fatalf("offset %q must stop before the partial line (size %d)", off, info.Size())
	}
	// rescanning is idempotent
	if err := sc.scan(j); err != nil {
		t.Fatal(err)
	}
	if ls, _ := st.ListLinks(j.ID); len(ls) != 1 {
		t.Fatal("rescan duplicated")
	}
	if mapPRState("OPEN", true) != "draft" || mapPRState("OPEN", false) != "open" || mapPRState("MERGED", false) != "merged" || mapPRState("CLOSED", false) != "closed" || mapPRState("?", false) != "" {
		t.Error("mapPRState")
	}
}
