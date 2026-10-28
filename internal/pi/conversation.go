package pi

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

// Message is one row of a job's conversation record: what the operator
// typed (role user) or pi's final answer to it (role assistant). A turn's
// final answer is the LAST assistant message of the turn — the one pi
// stopped on — so the narration between tool calls ("let me check X…"),
// the tool calls and their results, thinking, system messages and messages
// injected by extensions are all left out.
type Message struct {
	Role string // user | assistant
	Text string // the text blocks, newline-joined, trailing newlines trimmed
	Time int64  // unix seconds, 0 when the entry carried no timestamp
	// End says how an assistant turn ended when it did not end in an answer
	// of pi's own choosing: "" (it stopped), length (cut off by the output
	// limit), aborted (interrupted), error (Error says what failed).
	End   string
	Error string
}

// Conversation is a session's record plus whether its last turn is still
// waiting for pi's final answer: pi's latest message asked for tools, or
// nothing has followed the operator's message yet.
type Conversation struct {
	Messages []Message
	Pending  bool
}

// ReadConversation derives the record from the session JSONL at path. The
// branch read is the one the file's last entry sits on (every entry names
// its parent; a `/tree` jump away from an older branch leaves that branch's
// entries in the file, off the path). A partial trailing line (pi
// mid-write) is skipped; a missing file is an empty record.
func ReadConversation(path string) (Conversation, error) {
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Conversation{Messages: []Message{}}, nil
		}
		return Conversation{}, fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()
	var entries []Entry
	r := bufio.NewReaderSize(f, 1<<20)
	for {
		line, err := r.ReadBytes('\n')
		if len(line) > 0 && (err == nil || errors.Is(err, io.EOF)) {
			// a line without its newline is the partial one — unless the
			// file simply ends without a trailing newline; ParseEntry's
			// JSON check tells the two apart
			if e, ok := ParseEntry(line); ok {
				entries = append(entries, e)
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return Conversation{}, fmt.Errorf("read %s: %w", path, err)
		}
	}
	return BuildConversation(entries), nil
}

// BuildConversation is ReadConversation on already-parsed entries, in file
// order.
func BuildConversation(entries []Entry) Conversation {
	conv := Conversation{Messages: []Message{}}
	path := currentBranch(entries)
	var last *Entry // the current turn's latest assistant message
	open := false   // a turn has started (an operator message was seen)
	flush := func() {
		if m, ok := answerOf(last); ok {
			conv.Messages = append(conv.Messages, m)
		}
	}
	for i := range path {
		e := &path[i]
		if e.Kind != "message" {
			continue
		}
		switch e.Role {
		case "user":
			flush()
			conv.Messages = append(conv.Messages, Message{Role: "user", Text: trimText(e.Text), Time: e.Time})
			open, last = true, nil
		case "assistant":
			if open {
				last = e
			}
		}
	}
	flush()
	conv.Pending = open && (last == nil || midTurn(last.StopReason))
	return conv
}

// midTurn reports a stop reason that means pi has not finished its turn:
// it asked for tools (the next assistant message continues the work) or the
// message is still being produced.
func midTurn(stop string) bool {
	switch stop {
	case "toolUse", "pending", "deferred":
		return true
	}
	return false
}

// answerOf turns a turn's last assistant message into its record row. No
// row when there was no assistant message, when pi was still mid-turn (the
// operator steered it with another message, or it was cut off before
// answering), or when a plain stop carried no text.
func answerOf(e *Entry) (Message, bool) {
	if e == nil || midTurn(e.StopReason) {
		return Message{}, false
	}
	m := Message{Role: "assistant", Text: trimText(e.Text), Time: e.Time}
	switch e.StopReason {
	case "", "stop":
		if m.Text == "" {
			return Message{}, false
		}
	case "length", "aborted":
		m.End = e.StopReason
	case "error":
		m.End = "error"
		m.Error = strings.TrimSpace(e.ErrorMessage)
	default:
		// a stop reason this version does not know: show what pi said and
		// name the reason, rather than hide the turn
		m.End = e.StopReason
	}
	return m, true
}

// currentBranch returns the entries on the path from the root to the file's
// last entry, in order. Entries without ids (an older format) fall back to
// file order.
func currentBranch(entries []Entry) []Entry {
	if len(entries) == 0 {
		return nil
	}
	leaf := len(entries) - 1
	if entries[leaf].ID == "" {
		return entries
	}
	byID := make(map[string]int, len(entries))
	for i, e := range entries {
		if e.ID != "" {
			if _, dup := byID[e.ID]; !dup {
				byID[e.ID] = i
			}
		}
	}
	idx := make([]int, 0, len(entries))
	seen := make(map[int]bool, len(entries))
	for cur := leaf; ; {
		if seen[cur] {
			break // a cycle: a corrupt file; stop rather than loop
		}
		seen[cur] = true
		idx = append(idx, cur)
		parent, ok := byID[entries[cur].ParentID]
		if entries[cur].ParentID == "" || !ok {
			break
		}
		cur = parent
	}
	path := make([]Entry, 0, len(idx))
	for i := len(idx) - 1; i >= 0; i-- {
		path = append(path, entries[idx[i]])
	}
	return path
}

func trimText(s string) string { return strings.TrimRight(s, "\n") }
