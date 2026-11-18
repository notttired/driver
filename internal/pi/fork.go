package pi

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

// ForkSession copies the pi session file at src to dst as a new session: the
// header line (`{"type":"session",…}`, always the first) gets a fresh id and
// timestamp and names src as its parentSession; every other line is copied
// byte for byte, so the entry tree (ids, parentIds) is intact and pi, opened
// on dst with --session, resumes the conversation exactly where src stood.
// dst is written whole or not at all (a temp file beside it, then renamed).
func ForkSession(src, dst string, now time.Time) error {
	return forkSession(src, dst, now, true, "")
}

// ForkSessionFinished is ForkSession up to the finished work: when the
// branch's last turn is still under way (the operator's message has no
// final answer yet — pi is on it, or was cut off), the copy ends at the
// last final answer, leaving that message, whatever came with it and the
// tool calls in flight behind; with nothing under way it is ForkSession.
// A session whose first turn is still under way copies bare.
func ForkSessionFinished(src, dst string, now time.Time) error {
	entries, err := ReadEntries(src)
	if err != nil {
		return fmt.Errorf("fork session: %w", err)
	}
	conv := BuildConversation(entries)
	if !conv.Pending {
		return forkSession(src, dst, now, true, "")
	}
	upTo := "" // nothing finished: the header alone
	for i := len(conv.Messages) - 1; i >= 0; i-- {
		if conv.Messages[i].Role == "assistant" {
			upTo = conv.Messages[i].ID
			break
		}
	}
	return forkSession(src, dst, now, false, upTo)
}

// forkSession copies src to dst as ForkSession does: the whole file, or —
// whole false — the lines up to and including the entry with id upTo, so
// that entry is the copy's last and pi resumes there ("" copies the header
// alone).
func forkSession(src, dst string, now time.Time, whole bool, upTo string) error {
	if src == dst {
		return errors.New("fork session: source and destination are the same file")
	}
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("fork session: %w", err)
	}
	defer in.Close()
	r := bufio.NewReaderSize(in, 64*1024)
	first, err := r.ReadBytes('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return fmt.Errorf("fork session: %w", err)
	}
	header, err := forkHeader(first, src, now)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(dst), ".session-*.jsonl")
	if err != nil {
		return fmt.Errorf("fork session: %w", err)
	}
	tmpName := tmp.Name()
	fail := func(err error) error {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return fmt.Errorf("fork session: %w", err)
	}
	if err := tmp.Chmod(0o644); err != nil {
		return fail(err)
	}
	if _, err := tmp.Write(append(header, '\n')); err != nil {
		return fail(err)
	}
	if err := copyUpTo(tmp, r, whole, upTo); err != nil {
		return fail(err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("fork session: %w", err)
	}
	if err := os.Rename(tmpName, dst); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("fork session: %w", err)
	}
	return nil
}

// forkHeader is src's header line rewritten for a fork made at now: a new
// id and timestamp, parentSession = src, everything else (version, cwd) as
// it was.
func forkHeader(line []byte, src string, now time.Time) ([]byte, error) {
	var h map[string]any
	dec := json.NewDecoder(bytes.NewReader(line))
	dec.UseNumber()
	if err := dec.Decode(&h); err != nil || h["type"] != "session" {
		return nil, fmt.Errorf("fork session: %s does not start with a session header", src)
	}
	h["id"] = NewSessionID(now)
	h["timestamp"] = now.UTC().Format("2006-01-02T15:04:05.000Z07:00")
	h["parentSession"] = src
	out, err := json.Marshal(h)
	if err != nil {
		return nil, fmt.Errorf("fork session: %w", err)
	}
	return out, nil
}

// NewSessionID is a fresh UUID v7 — time-ordered on now, like pi's own
// session ids — in the canonical 8-4-4-4-12 form.
func NewSessionID(now time.Time) string {
	var b [16]byte
	ms := uint64(now.UnixMilli())
	b[0], b[1], b[2] = byte(ms>>40), byte(ms>>32), byte(ms>>24)
	b[3], b[4], b[5] = byte(ms>>16), byte(ms>>8), byte(ms)
	_, _ = rand.Read(b[6:])
	b[6] = (b[6] & 0x0f) | 0x70 // version 7
	b[8] = (b[8] & 0x3f) | 0x80 // RFC 4122 variant
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// copyUpTo copies r to w whole, or line by line through the line whose
// entry id is upTo (nothing for "").
func copyUpTo(w io.Writer, r *bufio.Reader, whole bool, upTo string) error {
	if whole {
		_, err := io.Copy(w, r)
		return err
	}
	if upTo == "" {
		return nil
	}
	for {
		line, err := r.ReadBytes('\n')
		if len(line) > 0 {
			if _, werr := w.Write(line); werr != nil {
				return werr
			}
			if e, ok := ParseEntry(line); ok && e.ID == upTo {
				return nil
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
	}
}
