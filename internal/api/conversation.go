package api

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"driver/internal/pi"
	"driver/internal/store"
)

// webConversation is GET /api/job/conversation (docs/API.md "Conversation"):
// the overlay's record of a session — the operator's messages and pi's
// final answer to each.
type webConversation struct {
	Messages []webMessage `json:"messages"`
	Pending  bool         `json:"pending"`
	Rev      string       `json:"rev"`
}

type webMessage struct {
	Role  string `json:"role"`
	Text  string `json:"text"`
	TS    int64  `json:"ts,omitempty"`
	End   string `json:"end,omitempty"`
	Error string `json:"error,omitempty"`
}

func webConversationFrom(c pi.Conversation, rev string) *webConversation {
	out := &webConversation{Messages: make([]webMessage, 0, len(c.Messages)), Pending: c.Pending, Rev: rev}
	for _, m := range c.Messages {
		out.Messages = append(out.Messages, webMessage{Role: m.Role, Text: m.Text, TS: m.Time, End: m.End, Error: m.Error})
	}
	return out
}

// conversationCache keeps the record last derived for each job together
// with the size and mtime of the session file it came from, so the
// overlay's poll re-reads a multi-megabyte session only when pi appended
// to it. Rev is that size and mtime, which lets the SPA tell an unchanged
// record apart without comparing it.
type conversationCache struct {
	mu    sync.Mutex
	byJob map[int64]cachedConversation
}

type cachedConversation struct {
	size int64
	mod  time.Time
	conv *webConversation
}

func newConversationCache() *conversationCache {
	return &conversationCache{byJob: map[int64]cachedConversation{}}
}

// get returns j's record. A job without a session file — not started, or
// its file gone — has an empty one.
func (c *conversationCache) get(j *store.Job) (*webConversation, error) {
	if j.PiSession == "" {
		return webConversationFrom(pi.Conversation{}, ""), nil
	}
	st, err := os.Stat(j.PiSession)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return webConversationFrom(pi.Conversation{}, ""), nil
		}
		return nil, fmt.Errorf("stat %s: %w", j.PiSession, err)
	}
	c.mu.Lock()
	hit, ok := c.byJob[j.ID]
	c.mu.Unlock()
	if ok && hit.size == st.Size() && hit.mod.Equal(st.ModTime()) {
		return hit.conv, nil
	}
	// pi may append between the stat and the read; the record is then ahead
	// of the stamp it is filed under, and the next request re-reads — fine
	conv, err := pi.ReadConversation(j.PiSession)
	if err != nil {
		return nil, err
	}
	out := webConversationFrom(conv, fmt.Sprintf("%d-%d", st.Size(), st.ModTime().UnixNano()))
	c.mu.Lock()
	c.byJob[j.ID] = cachedConversation{size: st.Size(), mod: st.ModTime(), conv: out}
	c.mu.Unlock()
	return out, nil
}

func (a *API) jobConversation(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "GET only")
		return
	}
	j, err := a.Svc.Lookup(strings.TrimSpace(r.URL.Query().Get("slug")))
	if err != nil {
		writeFailure(w, "conversation", err)
		return
	}
	conv, err := a.conversations.get(j)
	if err != nil {
		writeFailure(w, "conversation", err)
		return
	}
	writeJSON(w, http.StatusOK, conv)
}
