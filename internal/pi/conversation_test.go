package pi

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

// entry builds one session line. role "" makes a non-message entry of kind.
func entry(id, parent, kind, role string, content any, extra map[string]any) string {
	o := map[string]any{"type": kind, "id": id, "parentId": parent, "timestamp": "2026-10-28T10:00:00.000Z"}
	if parent == "" {
		delete(o, "parentId")
	}
	if role != "" {
		m := map[string]any{"role": role, "content": content}
		for k, v := range extra {
			m[k] = v
		}
		o["message"] = m
	} else {
		for k, v := range extra {
			o[k] = v
		}
	}
	b, _ := json.Marshal(o)
	return string(b)
}

func text(s string) []map[string]any { return []map[string]any{{"type": "text", "text": s}} }

func TestBuildConversation(t *testing.T) {
	lines := []string{
		entry("h", "", "session", "", nil, map[string]any{"version": 3}),
		entry("sys", "h", "message", "system", "be brief", nil),
		entry("u1", "sys", "message", "user", text("What is in fruit.txt?"), nil),
		entry("a1", "u1", "message", "assistant", []map[string]any{
			{"type": "thinking", "thinking": "hmm"},
			{"type": "text", "text": "Let me check the file."},
			{"type": "toolCall", "id": "c1", "name": "bash", "arguments": map[string]any{"command": "cat fruit.txt"}},
		}, map[string]any{"stopReason": "toolUse"}),
		entry("r1", "a1", "message", "toolResult", text("pineapple\n"), map[string]any{"toolCallId": "c1", "toolName": "bash"}),
		entry("a2", "r1", "message", "assistant", []map[string]any{
			{"type": "text", "text": "It holds one word:"},
			{"type": "text", "text": "pineapple\n"},
		}, map[string]any{"stopReason": "stop"}),
		// an extension's message and a usage entry ride along and are skipped
		entry("x1", "a2", "custom_message", "", nil, map[string]any{"customType": "interview_result", "content": "done", "display": true}),
		entry("us", "x1", "usage", "", nil, nil),
		entry("u2", "us", "message", "user", text("Rename it."), nil),
		entry("a3", "u2", "message", "assistant", []map[string]any{}, map[string]any{"stopReason": "aborted", "errorMessage": "Operation aborted"}),
		entry("u3", "a3", "message", "user", text("Try again."), nil),
		entry("a4", "u3", "message", "assistant", text(""), map[string]any{"stopReason": "error", "errorMessage": "Connection error."}),
		entry("u4", "a4", "message", "user", text("And now?"), nil),
		entry("a5", "u4", "message", "assistant", []map[string]any{
			{"type": "toolCall", "id": "c2", "name": "bash", "arguments": map[string]any{"command": "mv fruit.txt f.txt"}},
		}, map[string]any{"stopReason": "toolUse"}),
	}
	var entries []Entry
	for _, l := range lines {
		e, ok := ParseEntry([]byte(l))
		if !ok {
			t.Fatalf("parse %s", l)
		}
		entries = append(entries, e)
	}
	got := BuildConversation(entries)
	ts := int64(1793181600) // 2026-10-28T10:00:00Z
	want := []Message{
		{ID: "u1", Role: "user", Text: "What is in fruit.txt?", Time: ts},
		{ID: "a2", Role: "assistant", Text: "It holds one word:\npineapple", Time: ts},
		{ID: "u2", Role: "user", Text: "Rename it.", Time: ts},
		{ID: "a3", Role: "assistant", Text: "", Time: ts, End: "aborted"},
		{ID: "u3", Role: "user", Text: "Try again.", Time: ts},
		{ID: "a4", Role: "assistant", Text: "", Time: ts, End: "error", Error: "Connection error."},
		{ID: "u4", Role: "user", Text: "And now?", Time: ts},
	}
	if !reflect.DeepEqual(got.Messages, want) {
		t.Fatalf("messages:\n got %+v\nwant %+v", got.Messages, want)
	}
	if !got.Pending {
		t.Fatalf("last turn asked for a tool: pending expected")
	}

	// the turn answered: not pending; a length stop is an answer with End set
	entries = append(entries,
		mustParse(t, entry("r2", "a5", "message", "toolResult", text(""), map[string]any{"toolCallId": "c2", "toolName": "bash"})),
		mustParse(t, entry("a6", "r2", "message", "assistant", text("Renamed to f.txt."), map[string]any{"stopReason": "length"})),
	)
	got = BuildConversation(entries)
	if got.Pending {
		t.Fatalf("answered turn reported pending")
	}
	last := got.Messages[len(got.Messages)-1]
	if last.Role != "assistant" || last.Text != "Renamed to f.txt." || last.End != "length" {
		t.Fatalf("last = %+v", last)
	}
}

func TestBuildConversationFollowsTheCurrentBranch(t *testing.T) {
	lines := []string{
		entry("h", "", "session", "", nil, nil),
		entry("u1", "h", "message", "user", text("Q1"), nil),
		entry("a1", "u1", "message", "assistant", text("A1"), map[string]any{"stopReason": "stop"}),
		// a branch off u1 that was later left: its answer is not on the path
		entry("a1b", "u1", "message", "assistant", text("A1 — other branch"), map[string]any{"stopReason": "stop"}),
		entry("u2", "a1", "message", "user", text("Q2"), nil),
		entry("a2", "u2", "message", "assistant", text("A2"), map[string]any{"stopReason": "stop"}),
	}
	var entries []Entry
	for _, l := range lines {
		entries = append(entries, mustParse(t, l))
	}
	got := BuildConversation(entries)
	var texts []string
	for _, m := range got.Messages {
		texts = append(texts, m.Text)
	}
	if want := []string{"Q1", "A1", "Q2", "A2"}; !reflect.DeepEqual(texts, want) {
		t.Fatalf("texts = %v, want %v", texts, want)
	}
	if got.Pending {
		t.Fatalf("pending on an answered record")
	}

	// a steering message mid-turn: the first turn ends on a toolUse (no row
	// for it), the answer belongs to the second
	steer := []string{
		entry("h", "", "session", "", nil, nil),
		entry("u1", "h", "message", "user", text("Do X"), nil),
		entry("a1", "u1", "message", "assistant", text("On it."), map[string]any{"stopReason": "toolUse"}),
		entry("u2", "a1", "message", "user", text("Also Y"), nil),
		entry("a2", "u2", "message", "assistant", text("Did X and Y."), map[string]any{"stopReason": "stop"}),
	}
	entries = entries[:0]
	for _, l := range steer {
		entries = append(entries, mustParse(t, l))
	}
	got = BuildConversation(entries)
	texts = texts[:0]
	for _, m := range got.Messages {
		texts = append(texts, m.Role+":"+m.Text)
	}
	if want := []string{"user:Do X", "user:Also Y", "assistant:Did X and Y."}; !reflect.DeepEqual(texts, want) {
		t.Fatalf("steer texts = %v, want %v", texts, want)
	}
}

func TestReadConversation(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "session.jsonl")
	conv, err := ReadConversation(p)
	if err != nil || len(conv.Messages) != 0 || conv.Messages == nil || conv.Pending {
		t.Fatalf("missing file: %+v %v", conv, err)
	}
	body := strings.Join([]string{
		entry("h", "", "session", "", nil, nil),
		entry("u1", "h", "message", "user", text("Hello"), nil),
		entry("a1", "u1", "message", "assistant", text("Hi."), map[string]any{"stopReason": "stop"}),
	}, "\n") + "\n" + `{"type":"message","id":"u2","parentId":"a1","message":{"role":"user","content":[{"type":"te`
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	conv, err = ReadConversation(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(conv.Messages) != 2 || conv.Messages[1].Text != "Hi." || conv.Pending {
		t.Fatalf("partial line not skipped: %+v", conv)
	}
	// entries without ids: file order
	old := `{"type":"message","message":{"role":"user","content":"Old format?"}}` + "\n" +
		`{"type":"message","message":{"role":"assistant","content":[{"type":"text","text":"Yes."}]}}` + "\n"
	if err := os.WriteFile(p, []byte(old), 0o644); err != nil {
		t.Fatal(err)
	}
	conv, err = ReadConversation(p)
	if err != nil || len(conv.Messages) != 2 || conv.Messages[0].Text != "Old format?" || conv.Messages[1].Text != "Yes." {
		t.Fatalf("old format: %+v %v", conv, err)
	}
}

func TestConversationFailed(t *testing.T) {
	turn := func(last ...string) Conversation {
		lines := []string{entry("h", "", "session", "", nil, nil), entry("u1", "h", "message", "user", text("Go."), nil)}
		return BuildConversation(parse(append(lines, last...)))
	}
	cases := []struct {
		name   string
		conv   Conversation
		msg    string
		failed bool
	}{
		{"gave up on the network", turn(
			entry("a1", "u1", "message", "assistant", []map[string]any{}, map[string]any{"stopReason": "error", "errorMessage": "Connection error."}),
			entry("ce", "a1", "context_edit", "", nil, map[string]any{"targetId": "a1", "replacement": nil}),
			entry("a2", "ce", "message", "assistant", []map[string]any{}, map[string]any{"stopReason": "error", "errorMessage": "Request timed out."}),
		), "Request timed out.", true},
		{"failed after a tool call", turn(
			entry("a1", "u1", "message", "assistant", []map[string]any{{"type": "toolCall", "id": "c1", "name": "bash", "arguments": map[string]any{"command": "ls"}}}, map[string]any{"stopReason": "toolUse"}),
			entry("r1", "a1", "message", "toolResult", text("x"), map[string]any{"toolCallId": "c1"}),
			entry("a2", "r1", "message", "assistant", text("Half an"), map[string]any{"stopReason": "error", "errorMessage": "terminated"}),
		), "terminated", true},
		{"answered", turn(entry("a1", "u1", "message", "assistant", text("Done."), map[string]any{"stopReason": "stop"})), "", false},
		{"interrupted", turn(entry("a1", "u1", "message", "assistant", text(""), map[string]any{"stopReason": "aborted", "errorMessage": "Operation aborted"})), "", false},
		{"your message is the last", turn(), "", false},
		{"nothing yet", Conversation{}, "", false},
	}
	for _, c := range cases {
		if msg, failed := c.conv.Failed(); msg != c.msg || failed != c.failed {
			t.Errorf("%s: Failed() = %q, %v", c.name, msg, failed)
		}
	}
}

func parse(lines []string) []Entry {
	var out []Entry
	for _, l := range lines {
		if e, ok := ParseEntry([]byte(l)); ok {
			out = append(out, e)
		}
	}
	return out
}

func TestParseEntryTree(t *testing.T) {
	e := mustParse(t, entry("a1", "u1", "message", "assistant", text("x"), map[string]any{"stopReason": "error", "errorMessage": "terminated"}))
	if e.ID != "a1" || e.ParentID != "u1" || e.StopReason != "error" || e.ErrorMessage != "terminated" || e.Time != 1793181600 {
		t.Fatalf("entry = %+v", e)
	}
	e = mustParse(t, `{"type":"message","id":"u","timestamp":1793181600123,"message":{"role":"user","content":"hi"}}`)
	if e.Time != 1793181600 {
		t.Fatalf("ms timestamp: %d", e.Time)
	}
	e = mustParse(t, `{"type":"message","id":"u","timestamp":"soon","message":{"role":"user","content":"hi"}}`)
	if e.Time != 0 {
		t.Fatalf("bad timestamp: %d", e.Time)
	}
}

func mustParse(t *testing.T, line string) Entry {
	t.Helper()
	e, ok := ParseEntry([]byte(line))
	if !ok {
		t.Fatalf("parse %s", line)
	}
	return e
}

func TestParseCustomMessage(t *testing.T) {
	line := `{"type":"custom_message","id":"x1","parentId":"a2","timestamp":"2026-10-28T10:00:00.000Z","customType":"driver-peers","content":"Context from…","display":true,"details":{"carried":[{"thread":0,"entry":"a9"},{"thread":3,"entry":"u7"}]}}`
	e, ok := ParseEntry([]byte(line))
	if !ok || e.Kind != "custom_message" || e.CustomType != "driver-peers" || e.Text != "Context from…\n" {
		t.Fatalf("custom message: %+v", e)
	}
	if len(e.Carried) != 2 || e.Carried[0] != (Carried{Thread: 0, Entry: "a9"}) || e.Carried[1] != (Carried{Thread: 3, Entry: "u7"}) {
		t.Fatalf("carried: %+v", e.Carried)
	}
	// another extension's custom message carries nothing
	e, _ = ParseEntry([]byte(`{"type":"custom_message","id":"x2","parentId":"x1","customType":"other","content":[{"type":"text","text":"hi"}],"display":false}`))
	if e.CustomType != "other" || len(e.Carried) != 0 || e.Text != "hi\n" {
		t.Fatalf("other custom message: %+v", e)
	}
}

func TestTranscript(t *testing.T) {
	lines := []string{
		entry("h", "", "session", "", nil, map[string]any{"version": 3}),
		entry("u1", "h", "message", "user", text("What is in fruit.txt?"), nil),
		entry("a1", "u1", "message", "assistant", []map[string]any{
			{"type": "thinking", "thinking": "hmm"},
			{"type": "text", "text": "Let me check the file."},
			{"type": "toolCall", "id": "c1", "name": "bash", "arguments": map[string]any{"command": "cat fruit.txt"}},
		}, map[string]any{"stopReason": "toolUse"}),
		entry("r1", "a1", "message", "toolResult", text("pineapple\n"), map[string]any{"toolCallId": "c1", "toolName": "bash"}),
		entry("a2", "r1", "message", "assistant", text("It holds one word: pineapple"), map[string]any{"stopReason": "stop"}),
		entry("x1", "a2", "custom_message", "", nil, map[string]any{"customType": "driver-peers", "content": "Context from the other threads…", "display": true}),
		entry("u2", "x1", "message", "user", text("Rename it."), nil),
		entry("a3", "u2", "message", "assistant", []map[string]any{}, map[string]any{"stopReason": "error", "errorMessage": "Connection error."}),
		entry("u3", "a3", "message", "user", text("Again."), nil),
	}
	var entries []Entry
	for _, l := range lines {
		entries = append(entries, mustParse(t, l))
	}
	hh := time.Unix(1793181600, 0).Format("[15:04] ") // the entries' stamp, in local time
	got := Transcript(entries, 12)
	want := strings.Join([]string{
		hh + "user: What is in fruit.txt?",
		hh + "assistant: Let me check the file.",
		hh + "assistant → bash: cat fruit.txt",
		hh + "bash result: pineapple",
		hh + "assistant: It holds one word: pineapple",
		hh + "context from the other threads (added by Driver): Context from the other threads…",
		hh + "user: Rename it.",
		hh + "assistant: (turn failed: Connection error.)",
		hh + "user: Again.",
		"(no answer yet)",
	}, "\n")
	if got != want {
		t.Fatalf("transcript:\n%s\nwant:\n%s", got, want)
	}
}
