package jobs

import (
	"errors"
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
		// a blocked job's fork: its own pi is booting; the card stays in BLOCKED
		{"blocked starting", store.Job{State: "blocked", StartedAt: now - 3}, true, nil, "blocked", "starting", ""},
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

func TestClassify(t *testing.T) {
	cases := []struct {
		prev, next, reason string
		enters             string
		attention          int
	}{
		{"working", "blocked", "idle", "blocked", 1},
		{"working", "blocked", "dialog", "blocked", 1},
		{"working", "blocked", "exited", "blocked", 1},
		{"working", "blocked", "silent", "blocked", 1},
		{"ready", "blocked", "exited", "blocked", 1},
		// an interrupt the operator sent: no ring
		{"working", "blocked", "aborted", "blocked", 0},
		{"blocked", "working", "", "working", -1},
		{"blocked", "ready", "", "draft", -1},
		{"ready", "working", "", "working", 0},
		{"working", "ready", "", "draft", 0},
		// a draft whose pi opened: still DRAFT, nothing to do
		{"", "ready", "", "", 0},
	}
	for _, c := range cases {
		got := classify(c.prev, c.next, c.reason)
		if got.enters != c.enters || got.attention != c.attention {
			t.Errorf("classify(%q, %q, %q) = %+v, want enters %q attention %d", c.prev, c.next, c.reason, got, c.enters, c.attention)
		}
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
	j := &store.Job{ID: 7, Slug: "my-job", Name: "My Job", Model: "it's"}
	cmd := cfg.LaunchCommand(j, "/opt/bin/pi", "/h o/me/jobs/my-job-7", "/h o/me/jobs/my-job-7/session.jsonl")
	want := `env DRIVER_JOB_DIR='/h o/me/jobs/my-job-7' DRIVER_JOB_SLUG='my-job' DRIVER_SERVE='http://127.0.0.1:7781' '/opt/bin/pi' --session '/h o/me/jobs/my-job-7/session.jsonl' --name 'My Job' --model 'it'\''s' -e '/h o/me/ext/driver.ts' '-a' '--x'`
	if cmd != want {
		t.Fatalf("\n got %s\nwant %s", cmd, want)
	}
	j.Model = ""
	j.Name = "" // no name recorded: pi's session is called after the slug
	cfg.PiFlags = nil
	cmd = cfg.LaunchCommand(j, "pi", "/d", "/d/session.jsonl")
	if strings.Contains(cmd, "--model") || !strings.Contains(cmd, "--name 'my-job'") || strings.Contains(cmd, " -- ") || !strings.HasSuffix(cmd, "-e '/h o/me/ext/driver.ts'") {
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
	if n, err := CleanName("  Fix  the\n-50  bug "); err != nil || n != "Fix the -50 bug" {
		t.Errorf("CleanName = %q, %v", n, err)
	}
	if n, err := CleanName(strings.Repeat("é", MaxNameLen)); err != nil || n == "" {
		t.Errorf("CleanName at the limit: %q, %v", n, err)
	}
	if _, err := CleanName(strings.Repeat("a", MaxNameLen+1)); !errors.Is(err, ErrBadInput) {
		t.Errorf("CleanName over the limit: %v", err)
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
	sc := newSessionScanner(st)
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
	sc = newSessionScanner(st)
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

// TestConversationLinkScanner: every URL in your messages and in pi's
// replies becomes a link of its detected kind; tool output is not scanned.
func TestConversationLinkScanner(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "d.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	j := &store.Job{Slug: "u", State: "working"}
	st.CreateJob(j)
	sess := filepath.Join(t.TempDir(), "session.jsonl")
	j.PiSession = sess
	lines := []string{
		`{"type":"session","version":3}`,
		`{"type":"message","id":"u1","message":{"role":"user","content":[{"type":"text","text":"ticket https://linear.app/t/issue/X-1 and the PR https://github.com/o/r/pull/5. notes: (https://example.com/doc)"},{"type":"image","data":"x"}],"timestamp":1}}`,
		`{"type":"message","id":"a1","message":{"role":"assistant","content":[{"type":"thinking","thinking":"https://example.com/thinking-is-private"},{"type":"text","text":"Reading https://example.com/pi-said-this first."},{"type":"toolCall","id":"c9","name":"bash","arguments":{"command":"curl -s https://example.com/in-the-command"}}]}}`,
		`{"type":"message","id":"r1","message":{"role":"toolResult","toolCallId":"c9","toolName":"bash","content":[{"type":"text","text":"<a href=https://example.com/tool-output-is-not-scanned>"}]}}`,
		`{"type":"message","id":"u2","message":{"role":"user","content":"https://linear.app/t/issue/X-1 again, plus http://host.local:8080/x","timestamp":2}}`,
	}
	os.WriteFile(sess, []byte(strings.Join(lines, "\n")+"\n"), 0o644)
	sc := newSessionScanner(st)
	if err := sc.scan(j); err != nil {
		t.Fatal(err)
	}
	ls, _ := st.ListLinks(j.ID)
	var got []string
	for _, l := range ls {
		got = append(got, l.Kind+" "+l.URL)
	}
	want := []string{
		"linear https://linear.app/t/issue/X-1",
		"github_pr https://github.com/o/r/pull/5",
		"url https://example.com/doc",
		"url https://example.com/pi-said-this",
		"url http://host.local:8080/x",
	}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("\n got %q\nwant %q", got, want)
	}
	// rescanning attaches nothing twice
	if err := sc.scan(j); err != nil {
		t.Fatal(err)
	}
	if ls, _ := st.ListLinks(j.ID); len(ls) != 5 {
		t.Fatalf("rescan duplicated: %d", len(ls))
	}
}

// TestScanGeneration: offsets saved by an older scanner generation are
// forgotten once, so a widened scanner rereads what is already on disk; the
// same generation continues where it left off.
func TestScanGeneration(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "d.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	j := &store.Job{Slug: "g", State: "working"}
	st.CreateJob(j)
	j.PiSession = filepath.Join(t.TempDir(), "session.jsonl")
	os.WriteFile(j.PiSession, []byte(`{"type":"message","message":{"role":"user","content":"see https://example.com/once"}}`+"\n"), 0o644)
	if err := newSessionScanner(st).scan(j); err != nil {
		t.Fatal(err)
	}
	if v, _ := st.GetKV(scanGenerationKey); v != scanGeneration {
		t.Fatalf("generation %q not recorded", v)
	}
	unlink := func() {
		ls, _ := st.ListLinks(j.ID)
		for _, l := range ls {
			st.RemoveLink(j.ID, l.ID)
		}
	}
	// same generation: a fresh scanner starts at the saved offset and sees nothing new
	unlink()
	if err := newSessionScanner(st).scan(j); err != nil {
		t.Fatal(err)
	}
	if ls, _ := st.ListLinks(j.ID); len(ls) != 0 {
		t.Fatalf("same generation reread the file: %+v", ls)
	}
	// an older generation on disk: the offset is forgotten and the link comes back
	st.SetKV(scanGenerationKey, "1")
	if err := newSessionScanner(st).scan(j); err != nil {
		t.Fatal(err)
	}
	if ls, _ := st.ListLinks(j.ID); len(ls) != 1 || ls[0].URL != "https://example.com/once" {
		t.Fatalf("older generation did not reread: %+v", ls)
	}
	if v, _ := st.GetKV(scanGenerationKey); v != scanGeneration {
		t.Fatalf("generation %q after reread", v)
	}
}

func TestCreateRename(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "d.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	svc := &Service{Store: st, Cfg: Config{DefaultDir: t.TempDir()}, Tmux: NewTmux()}

	j, err := svc.Create("my-job", "  My   Job ", "", "")
	if err != nil || j.Slug != "my-job" || j.Name != "My Job" || j.DisplayName() != "My Job" {
		t.Fatalf("create: %v %+v", err, j)
	}
	if _, err := svc.Create("other", strings.Repeat("x", MaxNameLen+1), "", ""); !errors.Is(err, ErrBadInput) {
		t.Fatalf("long name: %v", err)
	}

	// the name alone: same slug, no session business
	if to, err := svc.Rename(j, "my-job", "Renamed"); err != nil || to != "my-job" {
		t.Fatalf("rename name only: %v %q", err, to)
	}
	j, _ = st.GetJobBySlug("my-job")
	if j.Name != "Renamed" {
		t.Fatalf("name after rename: %+v", j)
	}
	// nothing changes: nothing is written
	rev := j.Rev
	if to, err := svc.Rename(j, "my-job", "Renamed"); err != nil || to != "my-job" {
		t.Fatalf("no-op rename: %v %q", err, to)
	}
	if j, _ = st.GetJobBySlug("my-job"); j.Rev != rev {
		t.Fatalf("no-op rename bumped rev %d → %d", rev, j.Rev)
	}
	// slug and name together
	if to, err := svc.Rename(j, "new-job", "New Job"); err != nil || to != "new-job" {
		t.Fatalf("rename both: %v %q", err, to)
	}
	if _, err := st.GetJobBySlug("my-job"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("old slug still there: %v", err)
	}
	j, _ = st.GetJobBySlug("new-job")
	if j.Name != "New Job" || j.DisplayName() != "New Job" {
		t.Fatalf("after rename both: %+v", j)
	}
	// clearing the name: the card goes by the slug again
	if _, err := svc.Rename(j, "new-job", ""); err != nil {
		t.Fatal(err)
	}
	j, _ = st.GetJobBySlug("new-job")
	if j.Name != "" || j.DisplayName() != "new-job" {
		t.Fatalf("cleared name: %+v", j)
	}
	if _, err := svc.Rename(j, "Bad Slug", "x"); err == nil {
		t.Fatal("invalid slug accepted")
	}
}

func TestFork(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "d.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	home, work := t.TempDir(), t.TempDir()
	svc := &Service{Store: st, Cfg: Config{Home: home, DefaultDir: work}, Tmux: NewTmux()}

	// a not-started draft carrying everything a draft can
	src := &store.Job{Slug: "orig", Name: "Original", Dir: work, Model: "sonnet", Color: "red", Spec: "the brief", Notes: "remember this"}
	if _, err := st.CreateJob(src); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AddLink(src, "https://github.com/o/r/pull/9"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AddLink(src, "https://example.com/doc"); err != nil {
		t.Fatal(err)
	}
	aaa, _ := svc.Create("aaa", "", work, "")
	zzz, _ := svc.Create("zzz", "", work, "")
	if err := st.SetBoardColumn("draft", []int64{zzz.ID, src.ID, aaa.ID}); err != nil {
		t.Fatal(err)
	}

	fork, err := svc.Fork(src, "orig-fork", "Original fork")
	if err != nil {
		t.Fatal(err)
	}
	if fork.Slug != "orig-fork" || fork.Name != "Original fork" || fork.Dir != work || fork.Model != "sonnet" ||
		fork.Color != "red" || fork.Spec != "the brief" || fork.Notes != "remember this" ||
		fork.State != "" || fork.PiSession != "" || fork.Tmux != "" || fork.ID == src.ID {
		t.Fatalf("draft fork: %+v", fork)
	}
	ls, _ := st.ListLinks(fork.ID)
	if len(ls) != 2 || ls[0].URL != "https://github.com/o/r/pull/9" || ls[0].Kind != "github_pr" || ls[1].URL != "https://example.com/doc" {
		t.Fatalf("fork links: %+v", ls)
	}
	order, _ := st.GetBoardOrder()
	if want := []int64{zzz.ID, src.ID, fork.ID, aaa.ID}; !equalIDs(order["draft"], want) {
		t.Fatalf("draft order %v want %v", order["draft"], want)
	}
	if _, err := os.Stat(svc.Cfg.FreshJobDir(fork)); !os.IsNotExist(err) {
		t.Errorf("a draft without a transcript needs no job dir: %v", err)
	}

	// refusals leave nothing behind
	for _, c := range []struct{ to, name string }{{"orig", "x"}, {"orig-fork", "x"}, {"Bad Slug", "x"}} {
		if _, err := svc.Fork(src, c.to, c.name); !IsRefusal(err) {
			t.Errorf("fork to %q: %v", c.to, err)
		}
	}
	if _, err := svc.Fork(src, "long", strings.Repeat("x", MaxNameLen+1)); !errors.Is(err, ErrBadInput) {
		t.Errorf("long name: %v", err)
	}
	if all, _ := st.ListJobs(); len(all) != 4 {
		t.Fatalf("jobs after refusals: %d", len(all))
	}

	// a done job with a transcript: the fork is done too, on a copy of it
	done, _ := svc.Create("done-job", "Done Job", work, "")
	doneDir := svc.Cfg.FreshJobDir(done)
	if err := os.MkdirAll(doneDir, 0o755); err != nil {
		t.Fatal(err)
	}
	transcript := `{"type":"session","version":3,"id":"0199f2a0-0000-7000-8000-000000000001","timestamp":"2026-10-01T10:00:00.000Z","cwd":"` + work + `"}` + "\n" +
		`{"type":"message","id":"a","parentId":null,"timestamp":"2026-10-01T10:00:02.000Z","message":{"role":"user","content":[{"type":"text","text":"hi https://example.com/from-chat"}]}}` + "\n" +
		`{"type":"message","id":"b","parentId":"a","timestamp":"2026-10-01T10:00:03.000Z","message":{"role":"assistant","content":[{"type":"text","text":"hello"}],"stopReason":"stop"}}` + "\n"
	sessionFile := filepath.Join(doneDir, "session.jsonl")
	if err := os.WriteFile(sessionFile, []byte(transcript), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(doneDir, "status.json"), []byte(`{"state":"idle"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := st.UpdateJobFields(done.ID, done.Rev, map[string]any{
		"state": "done", "outcome": "accepted", "pi_session": sessionFile, "started_at": int64(100), "finished_at": int64(200),
	}); err != nil {
		t.Fatal(err)
	}
	done, _ = st.GetJobBySlug("done-job")
	f2, err := svc.Fork(done, "done-fork", "")
	if err != nil {
		t.Fatal(err)
	}
	f2Dir := svc.Cfg.FreshJobDir(f2)
	if f2.State != "done" || f2.Outcome != "accepted" || f2.StartedAt != 100 || f2.FinishedAt != 200 || f2.Tmux != "" ||
		f2.Name != "" || f2.DisplayName() != "done-fork" || f2.PiSession != filepath.Join(f2Dir, "session.jsonl") {
		t.Fatalf("done fork: %+v", f2)
	}
	copied, err := os.ReadFile(f2.PiSession)
	if err != nil {
		t.Fatal(err)
	}
	gotLines, wantLines := strings.Split(strings.TrimSuffix(string(copied), "\n"), "\n"), strings.Split(strings.TrimSuffix(transcript, "\n"), "\n")
	if len(gotLines) != 3 || gotLines[0] == wantLines[0] || !strings.Contains(gotLines[0], `"parentSession":"`+sessionFile+`"`) ||
		gotLines[1] != wantLines[1] || gotLines[2] != wantLines[2] {
		t.Fatalf("copied transcript:\n%s", copied)
	}
	if _, err := os.Stat(filepath.Join(f2Dir, "status.json")); !os.IsNotExist(err) {
		t.Error("status.json must not be copied")
	}
	if back, _ := os.ReadFile(sessionFile); string(back) != transcript {
		t.Error("the original transcript changed")
	}
	if off, err := st.GetKV(scanOffsetKey(f2.ID)); err != nil || off != strconv.Itoa(len(copied)) {
		t.Errorf("fork scan offset %q %v (file is %d bytes)", off, err, len(copied))
	}
	order, _ = st.GetBoardOrder()
	if want := []int64{done.ID, f2.ID}; !equalIDs(order["done"], want) {
		t.Fatalf("done order %v want %v", order["done"], want)
	}

	// an archived original is refused
	if err := svc.Archive(done); err != nil {
		t.Fatal(err)
	}
	arch, err := st.GetJobByID(done.ID)
	if err != nil || !arch.Archived() {
		t.Fatalf("archived original: %+v %v", arch, err)
	}
	if _, err := svc.Fork(arch, "arch-fork", ""); !IsRefusal(err) {
		t.Errorf("archived: %v", err)
	}

	// an open job whose tools cannot be resolved, or whose dir is gone, is
	// refused before anything is made
	open, _ := svc.Create("open-job", "", work, "")
	if _, err := st.UpdateJobFields(open.ID, open.Rev, map[string]any{"state": "working", "started_at": int64(1000), "tmux": "drv-open-job-0"}); err != nil {
		t.Fatal(err)
	}
	open, _ = st.GetJobBySlug("open-job")
	noPi := &Service{Store: st, Cfg: Config{Home: home, DefaultDir: work, PiBin: filepath.Join(home, "no-such-pi")}, Tmux: NewTmux()}
	if _, err := noPi.Fork(open, "open-fork", ""); !IsRefusal(err) {
		t.Errorf("no pi: %v", err)
	}
	gone := &store.Job{}
	*gone = *open
	gone.Dir = filepath.Join(work, "gone")
	if _, err := svc.Fork(gone, "open-fork", ""); !IsRefusal(err) {
		t.Errorf("gone dir: %v", err)
	}
	if _, err := st.GetJobBySlug("open-fork"); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("a refused fork must not exist: %v", err)
	}
}

func equalIDs(a, b []int64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestFollowSession(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "d.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	home, work := t.TempDir(), t.TempDir()
	svc := &Service{Store: st, Cfg: Config{Home: home, DefaultDir: work}, Tmux: NewTmux()}
	d := NewDaemon(svc)
	j, err := svc.Create("mover", "Mover", work, "")
	if err != nil {
		t.Fatal(err)
	}
	jobDir := svc.Cfg.FreshJobDir(j)
	if err := os.MkdirAll(jobDir, 0o755); err != nil {
		t.Fatal(err)
	}
	line := func(id, parent, text string) string {
		return `{"type":"message","id":"` + id + `","parentId":` + parent + `,"timestamp":"2026-10-29T13:19:00.000Z","message":{"role":"user","content":[{"type":"text","text":"` + text + `"}]}}` + "\n"
	}
	header := `{"type":"session","version":3,"id":"0199f2a0-0000-7000-8000-000000000002","timestamp":"2026-10-29T13:19:38.000Z","cwd":"` + work + `"}` + "\n"
	launched := filepath.Join(jobDir, "session.jsonl")
	if err := os.WriteFile(launched, []byte(header+line("a", "null", "see https://example.com/first")), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := st.UpdateJobFields(j.ID, j.Rev, map[string]any{"state": "working", "pi_session": launched, "started_at": int64(100)}); err != nil {
		t.Fatal(err)
	}
	j, _ = st.GetJobBySlug("mover")
	if err := d.scanner.scan(j); err != nil { // the launch file: a link and a saved offset
		t.Fatal(err)
	}
	if off, err := st.GetKV(scanOffsetKey(j.ID)); err != nil || off == "0" {
		t.Fatalf("offset after the first scan: %q %v", off, err)
	}
	rev := j.Rev

	// nothing to follow: no path, the launch file (also under another
	// spelling), a file pi has not written yet, a file outside the job dir
	outside := filepath.Join(t.TempDir(), "elsewhere.jsonl")
	if err := os.WriteFile(outside, []byte(header), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"", launched, filepath.Join(jobDir, ".", "session.jsonl"), filepath.Join(jobDir, "ghost.jsonl"), outside} {
		if err := d.followSession(j, p); err != nil {
			t.Fatalf("follow %q: %v", p, err)
		}
		if j.PiSession != launched || j.Rev != rev {
			t.Fatalf("follow %q moved the job: %+v", p, j)
		}
	}
	if d.elsewhere[j.ID] != outside {
		t.Errorf("the outside path is remembered so it is logged once: %q", d.elsewhere[j.ID])
	}
	if got, _ := st.GetJobBySlug("mover"); got.PiSession != launched || got.Rev != rev {
		t.Fatalf("stored job moved: %+v", got)
	}

	// pi's /fork: a new file beside the launch file holding the branch and
	// then more, named by status.json → the job follows it, and the scanner
	// starts over on the new file
	moved := filepath.Join(jobDir, "2026-10-29T13-19-45-964Z_0199f2a0-0000-7000-8000-000000000003.jsonl")
	body := strings.Replace(header, "000000000002", "000000000003", 1) + line("a", "null", "see https://example.com/first") + line("b", `"a"`, "and https://example.com/moved")
	if err := os.WriteFile(moved, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := d.followSession(j, moved); err != nil {
		t.Fatal(err)
	}
	if j.PiSession != moved || j.Rev == rev {
		t.Fatalf("job after the move: %+v", j)
	}
	if got, _ := st.GetJobBySlug("mover"); got.PiSession != moved || got.Rev != j.Rev {
		t.Fatalf("stored job after the move: %+v (in memory rev %d)", got, j.Rev)
	}
	if v, err := st.GetKV(scanOffsetKey(j.ID)); err == nil {
		t.Fatalf("the scan offset must be forgotten, still %q", v)
	}
	if _, ok := d.elsewhere[j.ID]; ok {
		t.Error("the outside note goes once the job moved")
	}
	if err := d.scanner.scan(j); err != nil {
		t.Fatal(err)
	}
	if off, _ := st.GetKV(scanOffsetKey(j.ID)); off != strconv.Itoa(len(body)) {
		t.Fatalf("offset after rescanning the new file: %q (file is %d bytes)", off, len(body))
	}
	for _, u := range []string{"https://example.com/first", "https://example.com/moved"} {
		_, created, err := st.AddLink(j.ID, "link", u)
		if err != nil && !errors.Is(err, store.ErrDuplicate) {
			t.Fatal(err)
		}
		if err == nil && created {
			t.Errorf("%s should have been attached by the rescan", u)
		}
	}
	// the same path again changes nothing
	rev = j.Rev
	if err := d.followSession(j, moved); err != nil || j.Rev != rev {
		t.Fatalf("second follow: %v, rev %d → %d", err, rev, j.Rev)
	}
}
