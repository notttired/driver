package pi

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
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
		{Role: "user", Text: "What is in fruit.txt?", Time: ts},
		{Role: "assistant", Text: "It holds one word:\npineapple", Time: ts},
		{Role: "user", Text: "Rename it.", Time: ts},
		{Role: "assistant", Text: "", Time: ts, End: "aborted"},
		{Role: "user", Text: "Try again.", Time: ts},
		{Role: "assistant", Text: "", Time: ts, End: "error", Error: "Connection error."},
		{Role: "user", Text: "And now?", Time: ts},
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
