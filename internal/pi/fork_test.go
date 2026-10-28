package pi

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

var uuidV7 = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

func TestForkSession(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "session.jsonl")
	big := strings.Repeat("x", 300_000) // a tool result far past bufio's default line limit
	lines := []string{
		`{"type":"session","version":3,"id":"0199f2a0-0000-7000-8000-000000000001","timestamp":"2026-10-01T10:00:00.000Z","cwd":"/work/repo"}`,
		`{"type":"session_info","name":"demo","timestamp":"2026-10-01T10:00:01.000Z"}`,
		`{"type":"message","id":"a","parentId":null,"timestamp":"2026-10-01T10:00:02.000Z","message":{"role":"user","content":[{"type":"text","text":"` + big + `"}]}}`,
		`{"type":"message","id":"b","parentId":"a","timestamp":"2026-10-01T10:00:03.000Z","message":{"role":"assistant","content":[{"type":"text","text":"ok — ünïcödé"}],"stopReason":"stop"}}`,
	}
	original := strings.Join(lines, "\n") + "\n"
	if err := os.WriteFile(src, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(dir, "fork", "session.jsonl")
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 28, 12, 0, 0, 500_000_000, time.UTC)
	if err := ForkSession(src, dst, now); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
	if len(got) != len(lines) {
		t.Fatalf("fork has %d lines, want %d", len(got), len(lines))
	}
	var h map[string]any
	if err := json.Unmarshal([]byte(got[0]), &h); err != nil {
		t.Fatalf("header: %v", err)
	}
	id, _ := h["id"].(string)
	if id == "0199f2a0-0000-7000-8000-000000000001" || !uuidV7.MatchString(id) {
		t.Errorf("header id %q: want a fresh UUID v7", id)
	}
	if want := fmt.Sprintf("%012x", now.UnixMilli()); id[:8]+id[9:13] != want {
		t.Errorf("header id %q is not ordered on now (%s)", id, want)
	}
	if h["timestamp"] != "2026-10-28T12:00:00.500Z" {
		t.Errorf("header timestamp %v", h["timestamp"])
	}
	if h["type"] != "session" || h["version"] != float64(3) || h["cwd"] != "/work/repo" || h["parentSession"] != src {
		t.Errorf("header lost fields: %v", h)
	}
	for i := 1; i < len(lines); i++ {
		if got[i] != lines[i] {
			t.Errorf("line %d changed in the copy", i)
		}
	}
	if back, _ := os.ReadFile(src); string(back) != original {
		t.Error("the original changed")
	}
	if left, _ := filepath.Glob(filepath.Join(dir, "fork", ".session-*")); len(left) != 0 {
		t.Errorf("temp files left behind: %v", left)
	}
	if fi, err := os.Stat(dst); err != nil || fi.Mode().Perm() != 0o644 {
		t.Errorf("fork mode: %v %v", fi, err)
	}

	// a header-only session without a trailing newline forks to a header line
	solo := filepath.Join(dir, "solo.jsonl")
	if err := os.WriteFile(solo, []byte(lines[0]), 0o644); err != nil {
		t.Fatal(err)
	}
	soloFork := filepath.Join(dir, "solo-fork.jsonl")
	if err := ForkSession(solo, soloFork, now); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(soloFork); strings.Count(string(data), "\n") != 1 || !strings.Contains(string(data), `"parentSession"`) {
		t.Errorf("header-only fork: %q", data)
	}

	// refusals: not a session, missing, same file
	notes := filepath.Join(dir, "notes.jsonl")
	if err := os.WriteFile(notes, []byte(lines[1]+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ForkSession(notes, filepath.Join(dir, "x.jsonl"), now); err == nil || !strings.Contains(err.Error(), "session header") {
		t.Errorf("non-session file: %v", err)
	}
	if err := ForkSession(filepath.Join(dir, "missing.jsonl"), filepath.Join(dir, "y.jsonl"), now); err == nil {
		t.Error("missing source must fail")
	}
	if err := ForkSession(src, src, now); err == nil {
		t.Error("same file must fail")
	}
	if _, err := os.Stat(filepath.Join(dir, "x.jsonl")); err == nil {
		t.Error("a refused fork must not leave a destination")
	}
}

func TestNewSessionID(t *testing.T) {
	now := time.Now()
	a, b := NewSessionID(now), NewSessionID(now)
	if a == b {
		t.Error("ids must differ")
	}
	for _, id := range []string{a, b} {
		if !uuidV7.MatchString(id) {
			t.Errorf("%q is not a UUID v7", id)
		}
	}
}
