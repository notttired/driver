package jobs

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"driver/internal/pi"
	"driver/internal/store"
)

func TestObserveCombined(t *testing.T) {
	working := Observation{State: "working", Pi: &PiInfo{State: "working", Tool: "bash"}}
	idle := Observation{State: "blocked", Pi: &PiInfo{State: "idle", Reason: "idle"}}
	exited := Observation{State: "blocked", Pi: &PiInfo{State: "exited", Reason: "exited"}}
	ready := Observation{State: "ready", Pi: &PiInfo{State: "ready"}}
	cases := []struct {
		name string
		obs  []Observation
		want Observation
	}{
		{"one thread", []Observation{idle}, idle},
		{"a working thread keeps the job working", []Observation{idle, working}, working},
		{"the first working thread speaks for the job", []Observation{working, Observation{State: "working", Pi: &PiInfo{State: "working"}}}, working},
		{"all waiting: the first blocked one speaks", []Observation{exited, idle}, exited},
		{"ready only when nothing else", []Observation{ready, idle}, idle},
		{"ready", []Observation{ready, ready}, ready},
		{"draft and done pass through", []Observation{{State: "done"}}, Observation{State: "done"}},
	}
	for _, c := range cases {
		got := Combine(c.obs)
		if got.State != c.want.State || (got.Pi == nil) != (c.want.Pi == nil) || (got.Pi != nil && *got.Pi != *c.want.Pi) {
			t.Errorf("%s: %+v want %+v", c.name, got, c.want)
		}
	}
}

func TestObserveThread(t *testing.T) {
	now := int64(1000)
	if got := ObserveThread("blocked", now-10, true, nil, now); got.State != "blocked" || got.Pi.State != "starting" {
		t.Fatalf("a thread opened on a blocked job keeps the job's column while pi boots: %+v", got)
	}
	if got := ObserveThread("working", now-10, true, nil, now); got.State != "working" || got.Pi.State != "starting" {
		t.Fatalf("on a working job: %+v", got)
	}
	if got := ObserveThread("working", now-500, true, nil, now); got.State != "blocked" || got.Pi.Reason != "silent" {
		t.Fatalf("no word after the grace: %+v", got)
	}
	if got := ObserveThread("working", now-10, false, nil, now); got.State != "blocked" || got.Pi.Reason != "exited" {
		t.Fatalf("dead pane: %+v", got)
	}
	if got := ObserveThread("working", now-10, true, &pi.Status{State: "idle"}, now); got.State != "blocked" || got.Pi.Reason != "idle" {
		t.Fatalf("idle: %+v", got)
	}
}

func threadsFixture(t *testing.T) (*Service, *store.Store, string) {
	t.Helper()
	if !NewTmux().Available() {
		t.Skip("tmux not on PATH")
	}
	sock, err := os.MkdirTemp("", "drvtmux")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(sock) })
	t.Setenv("TMUX_TMPDIR", sock)
	t.Setenv("TMUX", "")
	t.Cleanup(func() { NewTmux().run("kill-server") })
	st, err := store.Open(filepath.Join(t.TempDir(), "d.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	home, work := t.TempDir(), t.TempDir()
	fakePi := filepath.Join(t.TempDir(), "pi")
	os.WriteFile(fakePi, []byte("#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$DRIVER_JOB_DIR/argv\"\ncat > \"$DRIVER_JOB_DIR/typed\"\n"), 0o755)
	return &Service{Store: st, Cfg: Config{Home: home, Port: 7781, PiBin: fakePi}, Tmux: NewTmux()}, st, work
}

func waitFor(t *testing.T, what string, ok func() bool) {
	t.Helper()
	for deadline := time.Now().Add(5 * time.Second); !ok(); time.Sleep(20 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
	}
}

func readFile(path string) string {
	b, _ := os.ReadFile(path)
	return string(b)
}

const (
	threadHeader = `{"type":"session","version":3,"id":"h"}` + "\n"
	threadUser   = `{"type":"message","id":"u1","parentId":"h","message":{"role":"user","content":"Go."}}` + "\n"
	threadAnswer = `{"type":"message","id":"a1","parentId":"u1","message":{"role":"assistant","content":[{"type":"text","text":"Done."}],"stopReason":"stop"}}` + "\n"
)

func TestThreads(t *testing.T) {
	svc, st, work := threadsFixture(t)
	d := NewDaemon(svc)
	tick := func() {
		t.Helper()
		svc.Tmux.Invalidate()
		if _, err := d.tick(); err != nil {
			t.Fatal(err)
		}
	}

	draft, err := svc.Create("solo", "", work, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.OpenThread(draft, 0, "early"); !IsRefusal(err) {
		t.Fatalf("a not-started job has nothing to thread: %v", err)
	}

	j, err := svc.Create("multi", "Multi", work, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Submit(j); err != nil {
		t.Fatal(err)
	}
	j, _ = st.GetJobBySlug("multi")
	jobDir := filepath.Dir(j.PiSession)
	waitFor(t, "pi to start", func() bool { return readFile(filepath.Join(jobDir, "argv")) != "" })
	os.WriteFile(j.PiSession, []byte(threadHeader+threadUser+threadAnswer), 0o644)
	os.WriteFile(filepath.Join(jobDir, "status.json"), []byte(`{"state":"idle"}`), 0o644)
	if _, err := st.UpdateJobFields(j.ID, j.Rev, map[string]any{"state": "blocked"}); err != nil {
		t.Fatal(err)
	}
	j, _ = st.GetJobBySlug("multi")

	if _, err := svc.OpenThread(j, 0, "  "); !IsRefusal(err) {
		t.Fatalf("a thread needs a name: %v", err)
	}
	if _, err := svc.OpenThread(j, 77, "CI"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("an unknown source thread: %v", err)
	}
	th, err := svc.OpenThread(j, 0, " CI  fixes ")
	if err != nil {
		t.Fatal(err)
	}
	if th.Name != "CI fixes" || th.JobID != j.ID || th.Tmux != ThreadSessionName(j, th) || th.PiSession != filepath.Join(jobDir, "t1", "session.jsonl") {
		t.Fatalf("thread: %+v", th)
	}
	tdir := filepath.Dir(th.PiSession)
	waitFor(t, "the thread's pi to start", func() bool { return readFile(filepath.Join(tdir, "argv")) != "" })
	if args := readFile(filepath.Join(tdir, "argv")); !strings.Contains(args, "--session\n"+th.PiSession+"\n") || !strings.Contains(args, "--name\nMulti · CI fixes\n") {
		t.Fatalf("the thread's pi resumes the copied conversation under the job's and its own name:\n%s", args)
	}
	copied := readFile(th.PiSession)
	if !strings.Contains(copied, `"parentSession":"`+j.PiSession+`"`) || !strings.Contains(copied, threadUser) || !strings.Contains(copied, threadAnswer) {
		t.Fatalf("the thread starts from a copy of the conversation:\n%s", copied)
	}
	if got, _ := svc.OpenThreads(j); len(got) != 1 || got[0].ID != th.ID {
		t.Fatalf("open threads: %+v", got)
	}

	// the job stays blocked while the thread's pi boots, then follows it
	tick()
	if got, _ := st.GetJobBySlug("multi"); got.State != "blocked" {
		t.Fatalf("a thread opened on a blocked job keeps it in BLOCKED while pi boots: %+v", got)
	}
	os.WriteFile(filepath.Join(tdir, "status.json"), []byte(`{"state":"working"}`), 0o644)
	tick()
	if got, _ := st.GetJobBySlug("multi"); got.State != "working" {
		t.Fatalf("a working thread makes the job working: %+v", got)
	}
	os.WriteFile(filepath.Join(tdir, "status.json"), []byte(`{"state":"idle"}`), 0o644)
	tick()
	tick()
	got, _ := st.GetJobBySlug("multi")
	if got.State != "blocked" || got.AttentionAt == 0 {
		t.Fatalf("every thread waiting: blocked, with the ring: %+v", got)
	}

	// a second thread, forked from the first one's conversation — which is
	// mid-turn: the copy stops at the finished work, the prompt under way
	// and the tool call stay behind
	os.WriteFile(th.PiSession, []byte(threadHeader+threadUser+threadAnswer+
		`{"type":"message","id":"u2","parentId":"a1","message":{"role":"user","content":"More."}}`+"\n"+
		`{"type":"message","id":"a2","parentId":"u2","message":{"role":"assistant","content":[{"type":"toolCall","id":"c1","name":"bash","arguments":{"command":"make"}}],"stopReason":"toolUse"}}`+"\n"), 0o644)
	th2, err := svc.OpenThread(got, th.ID, "Docs")
	if err != nil {
		t.Fatal(err)
	}
	if copied := readFile(th2.PiSession); strings.Contains(copied, `"id":"u2"`) || strings.Contains(copied, `"id":"a2"`) || !strings.Contains(copied, `"id":"a1"`) || !strings.Contains(copied, `"parentSession":"`+th.PiSession+`"`) {
		t.Fatalf("the second thread copies the thread it was opened from, up to its finished work:\n%s", copied)
	}
	t2dir := filepath.Dir(th2.PiSession)
	waitFor(t, "the second thread's pi to start", func() bool { return readFile(filepath.Join(t2dir, "argv")) != "" })

	// stop: Escape into that thread's pane
	if err := svc.StopThread(got, th2); err != nil {
		t.Fatal(err)
	}

	// rename
	if err := svc.RenameThread(got, th2, "Docs and tests"); err != nil {
		t.Fatal(err)
	}
	if one, _ := st.GetThread(th2.ID); one.Name != "Docs and tests" {
		t.Fatalf("renamed: %+v", one)
	}

	// retry: the thread's pi exited after answering → back at its prompt
	// (two C-d: the first only flushes the Escape left on the tty's line)
	svc.Tmux.run("send-keys", "-t", "="+th2.Tmux+":", "C-d", "C-d")
	waitFor(t, "thread 2's pi to exit", func() bool { return svc.Tmux.PaneDead(th2.Tmux) })
	os.WriteFile(th2.PiSession, []byte(threadHeader+threadUser+threadAnswer), 0o644)
	os.Remove(filepath.Join(t2dir, "argv"))
	if err := svc.RetryThread(got, th2); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "thread 2's pi to come back", func() bool { return readFile(filepath.Join(t2dir, "argv")) != "" })
	if args := readFile(filepath.Join(t2dir, "argv")); strings.Contains(args, "continue") || svc.Tmux.PaneDead(th2.Tmux) {
		t.Fatalf("after retry:\n%s", args)
	}
	if err := svc.RetryThread(got, th2); !IsRefusal(err) {
		t.Fatalf("nothing to retry on a live thread at its prompt: %v", err)
	}
	svc.Tmux.run("send-keys", "-t", "="+th2.Tmux+":", "C-d")
	waitFor(t, "thread 2's pi to exit again", func() bool { return svc.Tmux.PaneDead(th2.Tmux) })
	if err := svc.StopThread(got, th2); !IsRefusal(err) {
		t.Fatalf("nothing to stop on a dead pane: %v", err)
	}

	// close: the pi goes, the row and its file stay
	if err := svc.CloseThread(got, th2); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "thread 2's session to go", func() bool { svc.Tmux.Invalidate(); return !svc.Tmux.HasSession(th2.Tmux) })
	if one, _ := st.GetThread(th2.ID); !one.Closed() {
		t.Fatalf("closed: %+v", one)
	}
	if _, err := os.Stat(th2.PiSession); err != nil {
		t.Fatalf("the closed thread's conversation stays: %v", err)
	}
	if err := svc.CloseThread(got, th2); !IsRefusal(err) {
		t.Fatalf("closing twice: %v", err)
	}
	if got, _ := svc.OpenThreads(got); len(got) != 1 || got[0].ID != th.ID {
		t.Fatalf("open threads after close: %+v", got)
	}

	// the thread dir resolves to the job (notes) and the thread
	if lj, lt, err := svc.LookupThreadDir(tdir); err != nil || lj.ID != j.ID || lt == nil || lt.ID != th.ID {
		t.Fatalf("lookup thread dir: %v %+v %+v", err, lj, lt)
	}
	if lj, lt, err := svc.LookupThreadDir(jobDir); err != nil || lj.ID != j.ID || lt != nil {
		t.Fatalf("lookup job dir: %v %+v %+v", err, lj, lt)
	}
	if nj, err := svc.LookupJobDir(tdir); err != nil || nj.ID != j.ID {
		t.Fatalf("notes from a thread reach the job: %v", err)
	}

	// rename the job: the thread's session follows
	got, _ = st.GetJobBySlug("multi")
	if _, err := svc.Rename(got, "many", ""); err != nil {
		t.Fatal(err)
	}
	renamed, _ := st.GetJobBySlug("many")
	one, _ := st.GetThread(th.ID)
	if one.Tmux != ThreadSessionName(renamed, one) || !svc.Tmux.HasSession(one.Tmux) {
		t.Fatalf("thread session after rename: %+v", one)
	}

	// done kills every thread and closes them
	if err := svc.Done(renamed); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "thread 1's session to go", func() bool { svc.Tmux.Invalidate(); return !svc.Tmux.HasSession(one.Tmux) })
	if one, _ := st.GetThread(th.ID); !one.Closed() {
		t.Fatalf("done closes the threads: %+v", one)
	}
	all, _ := st.ListThreads(j.ID)
	if len(all) != 2 {
		t.Fatalf("done keeps the rows: %+v", all)
	}
}

func TestThreadKillAndRetryAtStart(t *testing.T) {
	svc, st, work := threadsFixture(t)
	j, err := svc.Create("k", "", work, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Submit(j); err != nil {
		t.Fatal(err)
	}
	j, _ = st.GetJobBySlug("k")
	jobDir := filepath.Dir(j.PiSession)
	waitFor(t, "pi to start", func() bool { return readFile(filepath.Join(jobDir, "argv")) != "" })
	os.WriteFile(j.PiSession, []byte(threadHeader+threadUser+threadAnswer), 0o644)
	th, err := svc.OpenThread(j, 0, "side")
	if err != nil {
		t.Fatal(err)
	}
	tdir := filepath.Dir(th.PiSession)
	waitFor(t, "the thread's pi to start", func() bool { return readFile(filepath.Join(tdir, "argv")) != "" })

	// the machine went down: every session gone; driver starts and brings both back
	svc.Tmux.run("kill-server")
	os.Remove(filepath.Join(jobDir, "argv"))
	os.Remove(filepath.Join(tdir, "argv"))
	os.WriteFile(th.PiSession, []byte(threadHeader+threadUser+threadAnswer+`{"type":"message","id":"u2","parentId":"a1","message":{"role":"user","content":"More."}}`+"\n"), 0o644)
	d := NewDaemon(svc)
	for i := 0; i < 3; i++ {
		svc.Tmux.Invalidate()
		if _, err := d.tick(); err != nil {
			t.Fatal(err)
		}
	}
	waitFor(t, "the job's pi to come back", func() bool { return readFile(filepath.Join(jobDir, "argv")) != "" })
	waitFor(t, "the thread's pi to come back", func() bool { return readFile(filepath.Join(tdir, "argv")) != "" })
	if args := readFile(filepath.Join(tdir, "argv")); !strings.HasSuffix(args, "--\ncontinue\n") {
		t.Fatalf("a thread cut off mid-turn resumes with continue:\n%s", args)
	}
	one, _ := st.GetThread(th.ID)
	if svc.Tmux.PaneDead(one.Tmux) {
		t.Fatalf("thread after start: %+v", one)
	}

	// cancel: the threads go for good
	j, _ = st.GetJobBySlug("k")
	if err := svc.Kill(j); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the thread's session to go", func() bool { svc.Tmux.Invalidate(); return !svc.Tmux.HasSession(one.Tmux) })
	if all, _ := st.ListThreads(j.ID); len(all) != 0 {
		t.Fatalf("cancel drops the threads: %+v", all)
	}
}

func TestCloseOwnThread(t *testing.T) {
	svc, st, work := threadsFixture(t)
	j, err := svc.Create("multi", "Multi", work, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Submit(j); err != nil {
		t.Fatal(err)
	}
	j, _ = st.GetJobBySlug("multi")
	root := filepath.Dir(j.PiSession)
	waitFor(t, "pi to start", func() bool { return readFile(filepath.Join(root, "argv")) != "" })
	os.WriteFile(j.PiSession, []byte(threadHeader+threadUser+threadAnswer), 0o644)
	os.WriteFile(filepath.Join(root, "status.json"), []byte(`{"state":"idle"}`), 0o644)
	st.UpdateJobFields(j.ID, j.Rev, map[string]any{"state": "blocked"})
	j, _ = st.GetJobBySlug("multi")

	// the only tab stays
	if err := svc.CloseOwnThread(j); !IsRefusal(err) {
		t.Fatalf("the last tab cannot be closed: %v", err)
	}

	ci, err := svc.OpenThread(j, 0, "CI")
	if err != nil {
		t.Fatal(err)
	}
	docs, err := svc.OpenThread(j, 0, "Docs")
	if err != nil {
		t.Fatal(err)
	}
	waitFor(t, "CI's pi to start", func() bool { return readFile(filepath.Join(filepath.Dir(ci.PiSession), "argv")) != "" })
	os.WriteFile(filepath.Join(filepath.Dir(ci.PiSession), "status.json"), []byte(`{"state":"working"}`), 0o644)
	ownSess := SessionName(j)
	st.SetKV(scanOffsetKey(jobKey(j)), "11")
	st.SetKV(scanOffsetKey(threadKey(j, ci)), "22")
	if list, _ := svc.ThreadList(j); list[0].Name != "Multi" || list[1].Name != "CI" {
		t.Fatalf("tabs before: %+v", list)
	}

	// closing the first tab: its pi goes and it stays as a closed thread;
	// the next tab becomes the job's own pi, keeping its name, its session
	// and its files where they are
	if err := svc.CloseOwnThread(j); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the old own session to go", func() bool { svc.Tmux.Invalidate(); return !svc.Tmux.HasSession(ownSess) })
	j, _ = st.GetJobBySlug("multi")
	if j.PiSession != ci.PiSession || j.Tmux != ci.Tmux || j.StartedAt != ci.StartedAt || j.ThreadName != "CI" || OwnName(j) != "CI" {
		t.Fatalf("the job's own pi after the close: %+v", j)
	}
	if !svc.Tmux.HasSession(j.Tmux) || svc.Tmux.PaneDead(j.Tmux) {
		t.Fatalf("the promoted pi keeps running")
	}
	if svc.Cfg.OwnDir(j) != filepath.Dir(ci.PiSession) || svc.Cfg.JobDir(j) != root {
		t.Fatalf("dirs: own %s, job %s", svc.Cfg.OwnDir(j), svc.Cfg.JobDir(j))
	}
	if st := svc.Cfg.ReadStatus(j); st == nil || st.State != "working" {
		t.Fatalf("the promoted pi's status is the job's: %+v", st)
	}
	all, _ := st.ListThreads(j.ID)
	if len(all) != 2 {
		t.Fatalf("threads after: %+v", all)
	}
	old := all[1]
	if old.ID <= docs.ID || !old.Closed() || old.Name != "Multi" || old.PiSession != filepath.Join(root, "session.jsonl") {
		t.Fatalf("the old own pi as a closed thread: %+v", old)
	}
	if _, err := st.GetThread(ci.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("the promoted thread's row is gone: %v", err)
	}
	if v, _ := st.GetKV(scanOffsetKey(jobKey(j))); v != "22" {
		t.Fatalf("the scanner follows the promoted session: %q", v)
	}
	if v, _ := st.GetKV(scanOffsetKey(threadKey(j, old))); v != "11" {
		t.Fatalf("the scanner keeps the old session's place under its thread: %q", v)
	}
	list, _ := svc.ThreadList(j)
	if len(list) != 3 || list[0].ID != 0 || list[0].Name != "CI" || list[1].Name != "Docs" || list[2].Name != "Multi" || list[2].State != "closed" {
		t.Fatalf("tabs after: %+v", list)
	}
	open, _ := svc.OpenThreads(j)
	if len(open) != 1 || open[0].ID != docs.ID {
		t.Fatalf("open threads after: %+v", open)
	}
	// every dir still resolves: the promoted pi's to the job, the old one's
	// to its thread, Docs' to its thread
	if lj, lt, err := svc.LookupThreadDir(filepath.Dir(ci.PiSession)); err != nil || lj.ID != j.ID || lt != nil {
		t.Fatalf("the promoted dir: %v %+v", err, lt)
	}
	if _, lt, err := svc.LookupThreadDir(root); err != nil || lt == nil || lt.ID != old.ID {
		t.Fatalf("the old own dir: %v %+v", err, lt)
	}
	if _, lt, err := svc.LookupThreadDir(filepath.Dir(docs.PiSession)); err != nil || lt == nil || lt.ID != docs.ID {
		t.Fatalf("Docs' dir: %v %+v", err, lt)
	}
	// a new thread lands beside the others, under the job's root
	more, err := svc.OpenThread(j, 0, "More")
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(filepath.Dir(more.PiSession)) != root {
		t.Fatalf("a new thread's dir: %s", more.PiSession)
	}
	// the old own pi's words stay shared
	up, err := svc.PeerUpdate(j, docs.ID, nil)
	if err != nil || !strings.Contains(up.Text, `## Thread "Multi" (closed)`) || !strings.Contains(up.Text, "Multi: Done.") {
		t.Fatalf("the closed first pi still speaks: %v\n%s", err, up.Text)
	}
	// and once more: closing the (promoted) first tab hands over to Docs
	if err := svc.CloseOwnThread(j); err != nil {
		t.Fatal(err)
	}
	j, _ = st.GetJobBySlug("multi")
	if j.PiSession != docs.PiSession || OwnName(j) != "Docs" {
		t.Fatalf("second hand-over: %+v", j)
	}
	// a fresh start forgets the inherited name
	if err := svc.Kill(j); err != nil {
		t.Fatal(err)
	}
	j, _ = st.GetJobBySlug("multi")
	if err := svc.Submit(j); err != nil {
		t.Fatal(err)
	}
	if j, _ = st.GetJobBySlug("multi"); j.ThreadName != "" || OwnName(j) != "Multi" {
		t.Fatalf("after a restart: %+v", j)
	}
}
