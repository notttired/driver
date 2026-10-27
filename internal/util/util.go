// Package util holds the small helpers every other driver package leans on:
// the home-directory resolution, the ~/.driver/env loader, atomic file writes
// and a couple of string helpers. It imports nothing from the rest of driver.
package util

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"
)

// UserHome is the operator's home directory ($HOME), the fallback for every
// path knob that is unset.
func UserHome() string {
	if h, err := os.UserHomeDir(); err == nil && h != "" {
		return h
	}
	if h := os.Getenv("HOME"); h != "" {
		return h
	}
	return "/"
}

// Home is driver's own home: DRIVER_HOME when set, else ~/.driver. Everything
// the daemon writes (db, jobs, ext, daemon.json, log) lives below it.
func Home() string {
	if h := os.Getenv("DRIVER_HOME"); h != "" {
		return ExpandTilde(h)
	}
	return filepath.Join(UserHome(), ".driver")
}

// ExpandTilde turns a leading "~" or "~/" into the operator's home directory;
// every other value is returned unchanged.
func ExpandTilde(p string) string {
	if p == "~" {
		return UserHome()
	}
	if strings.HasPrefix(p, "~/") {
		return filepath.Join(UserHome(), p[2:])
	}
	return p
}

// AtomicWrite writes data to path through a temp file in the same directory
// plus a rename, so a concurrent reader never sees a half-written file.
func AtomicWrite(path string, data []byte, perm os.FileMode) error {
	tmp := fmt.Sprintf("%s.%d.tmp", path, os.Getpid())
	if err := os.WriteFile(tmp, data, perm); err != nil {
		return fmt.Errorf("write %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("rename %s: %w", path, err)
	}
	return nil
}

// OneLine collapses s to a single line — control characters and newlines
// become spaces, runs of whitespace collapse — and truncates it to max runes
// (with an ellipsis) when max > 0. Used wherever free text ends up in a log
// line, a notification or an error message.
func OneLine(s string, max int) string {
	var b strings.Builder
	lastSpace := true
	for _, r := range s {
		if unicode.IsSpace(r) || unicode.IsControl(r) {
			if !lastSpace {
				b.WriteByte(' ')
				lastSpace = true
			}
			continue
		}
		b.WriteRune(r)
		lastSpace = false
	}
	out := strings.TrimSpace(b.String())
	if max > 0 {
		rs := []rune(out)
		if len(rs) > max {
			out = string(rs[:max-1]) + "…"
		}
	}
	return out
}

// IsDir reports whether path exists and is a directory.
func IsDir(path string) bool {
	st, err := os.Stat(path)
	return err == nil && st.IsDir()
}
