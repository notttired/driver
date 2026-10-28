package pi

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadNewLinesPartial(t *testing.T) {
	p := filepath.Join(t.TempDir(), "s.jsonl")
	if err := os.WriteFile(p, []byte("{\"a\":1}\n{\"b\":2}\n{\"c\":"), 0o644); err != nil {
		t.Fatal(err)
	}
	lines, off, err := ReadNewLines(p, 0)
	if err != nil || len(lines) != 2 || off != 16 {
		t.Fatalf("lines=%d off=%d err=%v", len(lines), off, err)
	}
	if string(lines[1]) != `{"b":2}` {
		t.Fatalf("line = %q", lines[1])
	}
	// nothing new, partial line still pending
	lines, off2, _ := ReadNewLines(p, off)
	if len(lines) != 0 || off2 != off {
		t.Fatalf("partial consumed: %d %d", len(lines), off2)
	}
	// complete the line
	f, _ := os.OpenFile(p, os.O_APPEND|os.O_WRONLY, 0)
	f.WriteString("3}\n\n")
	f.Close()
	lines, off3, _ := ReadNewLines(p, off)
	if len(lines) != 1 || string(lines[0]) != `{"c":3}` {
		t.Fatalf("completed line: %q", lines)
	}
	st, _ := os.Stat(p)
	if off3 != st.Size() {
		t.Fatalf("off3 = %d, size %d", off3, st.Size())
	}
	// truncation restarts at 0
	os.WriteFile(p, []byte("{\"z\":0}\n"), 0o644)
	lines, off4, _ := ReadNewLines(p, off3)
	if len(lines) != 1 || off4 != 8 {
		t.Fatalf("after truncate: %d %d", len(lines), off4)
	}
	// missing file
	if l, o, err := ReadNewLines(filepath.Join(t.TempDir(), "nope"), 7); l != nil || o != 7 || err != nil {
		t.Fatal("missing file must be a no-op")
	}
}

const callLine = `{"type":"message","id":"a1","parentId":"p","timestamp":"t","message":{"role":"assistant","content":[{"type":"text","text":"opening"},{"type":"thinking","thinking":"hm"},{"type":"toolCall","id":"call_1","name":"bash","arguments":{"command":"git push && gh pr create --fill"}}],"stopReason":"toolUse"}}`
const resultLine = `{"type":"message","id":"a2","parentId":"a1","timestamp":"t","message":{"role":"toolResult","toolCallId":"call_1","toolName":"bash","content":[{"type":"text","text":"Creating pull request\nhttps://github.com/o/r/pull/12\n"}],"isError":false}}`

func TestParseEntry(t *testing.T) {
	e, ok := ParseEntry([]byte(`{"type":"session","version":3,"id":"x","cwd":"/"}`))
	if !ok || e.Kind != "session" || e.Role != "" {
		t.Fatalf("session: %+v %v", e, ok)
	}
	if _, ok := ParseEntry([]byte(`not json`)); ok {
		t.Fatal("garbage parsed")
	}
	e, ok = ParseEntry([]byte(callLine))
	if !ok || e.Role != "assistant" || len(e.ToolCalls) != 1 {
		t.Fatalf("call: %+v", e)
	}
	tc := e.ToolCalls[0]
	if tc.ID != "call_1" || tc.Name != "bash" || !IsPRCreateCommand(tc.Command) {
		t.Fatalf("toolcall = %+v", tc)
	}
	e, ok = ParseEntry([]byte(resultLine))
	if !ok || e.Role != "toolResult" || e.Result == nil || e.Result.ToolCallID != "call_1" {
		t.Fatalf("result: %+v", e)
	}
	urls := DetectPRURLs(e.Result.Text)
	if len(urls) != 1 || urls[0] != "https://github.com/o/r/pull/12" {
		t.Fatalf("urls = %v", urls)
	}
}

const userLine = `{"type":"message","id":"u1","parentId":null,"timestamp":"t","message":{"role":"user","content":[{"type":"text","text":"see https://linear.app/t/issue/X-1"},{"type":"image","data":"…","mimeType":"image/png"},{"type":"text","text":"and (https://example.com/doc)."}],"timestamp":1}}`

func TestParseUserEntry(t *testing.T) {
	e, ok := ParseEntry([]byte(userLine))
	if !ok || e.Role != "user" || e.ToolCalls != nil || e.Result != nil {
		t.Fatalf("user: %+v %v", e, ok)
	}
	if e.Text != "see https://linear.app/t/issue/X-1\nand (https://example.com/doc).\n" {
		t.Fatalf("text = %q", e.Text)
	}
	if got := DetectURLs(e.Text); len(got) != 2 || got[0] != "https://linear.app/t/issue/X-1" || got[1] != "https://example.com/doc" {
		t.Fatalf("urls = %v", got)
	}
	// a bare-string content, as older pi versions wrote it
	e, _ = ParseEntry([]byte(`{"type":"message","message":{"role":"user","content":"plain https://a.b/c"}}`))
	if e.Text != "plain https://a.b/c\n" {
		t.Fatalf("string content: %q", e.Text)
	}
}

func TestDetectURLs(t *testing.T) {
	text := "look at https://linear.app/t/issue/X-1, then [doc](https://example.com/a?b=1&c=2) and <https://en.wikipedia.org/wiki/Foo_(bar)>;" +
		" again https://linear.app/t/issue/X-1. HTTP://Example.org/x! `https://code.example/q` not ftp://x nor https:// alone, https://e.com/a).  http://localhost:8080/p"
	want := []string{
		"https://linear.app/t/issue/X-1",
		"https://example.com/a?b=1&c=2",
		"https://en.wikipedia.org/wiki/Foo_(bar)",
		"HTTP://Example.org/x",
		"https://code.example/q",
		"https://e.com/a",
		"http://localhost:8080/p",
	}
	got := DetectURLs(text)
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Fatalf("\n got %q\nwant %q", got, want)
	}
	if got := DetectURLs("nothing here, https://. neither"); len(got) != 0 {
		t.Fatalf("hostless: %v", got)
	}
}

func TestDetectPRURLs(t *testing.T) {
	got := DetectPRURLs("see https://github.com/a/b/pull/1. and (https://github.com/a/b/pull/1) then https://github.com/a/b-c.d/pull/22/files https://github.com/a/b/issues/3")
	if len(got) != 2 || got[0] != "https://github.com/a/b/pull/1" || got[1] != "https://github.com/a/b-c.d/pull/22" {
		t.Fatalf("got %v", got)
	}
	if !IsPRURL("https://github.com/a/b/pull/7") || IsPRURL("https://github.com/a/b/pull/7/files") || IsPRURL("https://github.com/a/b") {
		t.Fatal("IsPRURL")
	}
	if !IsPRCreateCommand("cd x &&  gh  pr   create -t 'x'") || IsPRCreateCommand("gh pr view 3") {
		t.Fatal("IsPRCreateCommand")
	}
}

func TestParseModelList(t *testing.T) {
	out := "provider                              model                 context  max-out  thinking  images\nanthropic                             claude-fable-5-1      1M       128K     yes       yes   \n\nopenai   gpt-5   400K  128K  yes  yes\n"
	ms := ParseModelList(out)
	if len(ms) != 2 || ms[0].Provider != "anthropic" || ms[0].ID != "claude-fable-5-1" || ms[1].ID != "gpt-5" {
		t.Fatalf("models = %+v", ms)
	}
}

func TestReadStatus(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "status.json")
	if _, found, err := ReadStatus(p); found || err != nil {
		t.Fatal("missing must be not-found")
	}
	os.WriteFile(p, []byte(`{"state":"working","tool":"bash","pid":12,"session":"s","ts":1761580000123}`), 0o644)
	st, found, err := ReadStatus(p)
	if err != nil || !found || st.State != "working" || st.Tool != "bash" || st.TS != 1761580000 {
		t.Fatalf("status = %+v %v %v", st, found, err)
	}
	os.WriteFile(p, []byte(`{"state":"idle","ts":1761580000}`), 0o644)
	st, _, _ = ReadStatus(p)
	if st.TS != 1761580000 || st.State != "idle" {
		t.Fatalf("seconds ts: %+v", st)
	}
	os.WriteFile(p, []byte(`{"state":`), 0o644)
	if _, _, err := ReadStatus(p); err == nil {
		t.Fatal("malformed must error")
	}
	home := t.TempDir()
	os.MkdirAll(filepath.Join(home, ".pi", "agent"), 0o755)
	os.WriteFile(filepath.Join(home, ".pi", "agent", "settings.json"), []byte(`{"defaultModel":"m1"}`), 0o644)
	if DefaultModel(home) != "m1" || DefaultModel(t.TempDir()) != "" {
		t.Fatal("DefaultModel")
	}
}
