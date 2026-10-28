package pane

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"driver/internal/pi"
)

// fakePi stands in for `pi -p`: the prompt on stdin, the answer on stdout,
// the conversation appended to the session file ($1). "hang" never answers,
// "fail" exits 3 with a complaint on stderr.
const fakePi = `#!/bin/sh
prompt=$(cat)
printf '{"type":"message","message":{"role":"user","content":[{"type":"text","text":"%s"}]}}\n' "$prompt" >> "$1"
case "$prompt" in
hang) exec sleep 30 ;;
fail) echo "boom" >&2; exit 3 ;;
esac
echo "ANSWER: $prompt"
`

type safeBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *safeBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *safeBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func TestLoop(t *testing.T) {
	dir := t.TempDir()
	session := filepath.Join(dir, "session.jsonl")
	script := filepath.Join(dir, "fakepi.sh")
	if err := os.WriteFile(script, []byte(fakePi), 0o755); err != nil {
		t.Fatal(err)
	}
	inR, inW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	outR, outW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	var out safeBuffer
	go io.Copy(&out, outR)
	state := func() string {
		st, _, _ := pi.ReadStatus(filepath.Join(dir, "status.json"))
		return st.State
	}
	waitState := func(want string) {
		t.Helper()
		waitFor(t, "status "+want, func() bool { return state() == want })
	}
	waitOut := func(want string) {
		t.Helper()
		waitFor(t, "output "+want, func() bool { return strings.Contains(out.String(), want) })
	}

	done := make(chan error, 1)
	go func() {
		done <- Run(context.Background(), Options{
			Title: "Fix the thing", Model: "anthropic/claude-haiku-4-5", Session: session, StatusDir: dir,
			Pi: []string{"/bin/sh", script, session}, In: inR, Out: outW,
		})
	}()

	// a fresh session: ready, with the header up
	waitState("ready")
	waitOut("Fix the thing")
	waitOut("anthropic/claude-haiku-4-5")

	// a prompt: working while pi runs, idle after; the pane shows pi's output
	fmt.Fprint(inW, "hello world\n")
	waitOut("ANSWER: hello world")
	waitState("idle")
	if st, _, _ := pi.ReadStatus(filepath.Join(dir, "status.json")); st.Session != session || st.PID != os.Getpid() {
		t.Fatalf("status = %+v", st)
	}

	// blank lines are not prompts; Esc while pi runs stops it → aborted
	fmt.Fprint(inW, "\n   \n")
	fmt.Fprint(inW, "hang\n")
	waitState("working")
	waitOut("pi is working")
	fmt.Fprint(inW, "\x1b")
	waitState("aborted")
	waitOut("stopped")

	// pi failing: its stderr shows, the exit status is named, the pane goes idle
	fmt.Fprint(inW, "fail\n")
	waitOut("pi exited with status 3")
	waitOut("boom")
	waitState("idle")

	// ^C mid-run stops it too
	fmt.Fprint(inW, "hang\n")
	waitState("working")
	fmt.Fprint(inW, "\x03")
	waitState("aborted")

	// the input closing closes the pane: exited
	inW.Close()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Run did not return after EOF")
	}
	outW.Close()
	if state() != "exited" {
		t.Fatalf("final state %q", state())
	}
	waitOut("closed")
	data, _ := os.ReadFile(session)
	if n := strings.Count(string(data), `"role":"user"`); n != 4 {
		t.Fatalf("session has %d user messages, want 4:\n%s", n, data)
	}
	if o := out.String(); strings.Count(o, "ANSWER:") != 1 {
		t.Fatalf("output:\n%s", o)
	}
}

func TestBriefed(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "s.jsonl")
	if briefed(f) {
		t.Fatal("no file should not be briefed")
	}
	os.WriteFile(f, []byte(`{"type":"session","id":"x"}`+"\n"+`{"type":"message","message":{"role":"assistant","content":[{"type":"text","text":"hi"}]}}`+"\n"), 0o644)
	if briefed(f) {
		t.Fatal("no user message should not be briefed")
	}
	os.WriteFile(f, []byte(`{"type":"message","message":{"role":"user","content":[{"type":"text","text":"do it"}]}}`+"\n"), 0o644)
	if !briefed(f) {
		t.Fatal("a user message should be briefed")
	}
}

func TestInput(t *testing.T) {
	for _, c := range []struct {
		b    string
		stop bool
	}{
		{"\x1b", true}, {"\x1b\x1b", true}, {"\x03", true}, {"abc\x03", true},
		{"\x1b[A", false}, {"x", false}, {"", false},
	} {
		if got := isStop([]byte(c.b)); got != c.stop {
			t.Errorf("isStop(%q) = %v", c.b, got)
		}
	}
	// ^C read through the editor's reader clears the line instead of ending input
	in := newInput(context.Background(), strings.NewReader("ab\x03cd"))
	got, _ := io.ReadAll(in)
	if string(got) != "ab\x15\x0bcd" {
		t.Fatalf("Read = %q", got)
	}
	// a short Read keeps the rest pending
	in = newInput(context.Background(), strings.NewReader("hello"))
	buf := make([]byte, 2)
	n, _ := in.Read(buf)
	rest, _ := io.ReadAll(in)
	if n != 2 || string(buf) != "he" || string(rest) != "llo" {
		t.Fatalf("short read: %q then %q", buf[:n], rest)
	}
	// a cancelled context ends a blocked read
	ctx, cancel := context.WithCancel(context.Background())
	r, _ := io.Pipe()
	in = newInput(ctx, r)
	cancel()
	if _, err := in.Read(buf); err == nil {
		t.Fatal("expected an error after cancel")
	}
}
