package api

import (
	"os"
	"path/filepath"
	"testing"

	"driver/internal/store"
)

func TestConversationCache(t *testing.T) {
	c := newConversationCache()

	// no session yet: an empty record, messages present as []
	conv, err := c.get(&store.Job{ID: 1})
	if err != nil || conv.Messages == nil || len(conv.Messages) != 0 || conv.Pending || conv.Rev != "" {
		t.Fatalf("no session: %+v %v", conv, err)
	}
	conv, err = c.get(&store.Job{ID: 1, PiSession: filepath.Join(t.TempDir(), "gone.jsonl")})
	if err != nil || len(conv.Messages) != 0 {
		t.Fatalf("missing file: %+v %v", conv, err)
	}

	p := filepath.Join(t.TempDir(), "session.jsonl")
	body := `{"type":"session","id":"h"}` + "\n" +
		`{"type":"message","id":"u1","parentId":"h","timestamp":"2026-10-28T10:00:00Z","message":{"role":"user","content":[{"type":"text","text":"Hello"}]}}` + "\n" +
		`{"type":"message","id":"a1","parentId":"u1","timestamp":"2026-10-28T10:00:05Z","message":{"role":"assistant","stopReason":"stop","content":[{"type":"text","text":"Hi there."}]}}` + "\n"
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	j := &store.Job{ID: 2, PiSession: p}
	first, err := c.get(j)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Messages) != 2 || first.Messages[1].Text != "Hi there." || first.Messages[1].TS != 1793181605 || first.Pending || first.Rev == "" {
		t.Fatalf("first = %+v", first)
	}
	again, err := c.get(j)
	if err != nil || again != first {
		t.Fatalf("unchanged file should hit the cache: %p vs %p (%v)", again, first, err)
	}

	// pi appends a new turn: the size changes, so the record is re-read
	f, _ := os.OpenFile(p, os.O_APPEND|os.O_WRONLY, 0)
	f.WriteString(`{"type":"message","id":"u2","parentId":"a1","message":{"role":"user","content":[{"type":"text","text":"More?"}]}}` + "\n")
	f.Close()
	third, err := c.get(j)
	if err != nil {
		t.Fatal(err)
	}
	if third == first || len(third.Messages) != 3 || !third.Pending || third.Rev == first.Rev {
		t.Fatalf("third = %+v (same object: %v)", third, third == first)
	}
}
