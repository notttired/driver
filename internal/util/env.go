package util

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// EnvFilePath is ~/.driver/env (below Home()): KEY=VALUE lines read by
// `driver serve` at startup. It exists because a launchd job's environment is
// whatever its plist declares, so knobs set in a login shell never reach the
// daemon; one file both the daemon and the Makefile read does.
func EnvFilePath() string {
	return filepath.Join(Home(), "env")
}

// ApplyEnvFile reads path as KEY=VALUE lines and sets each key the process
// environment does not already carry, returning the keys it set in file
// order. Blank lines and lines whose first non-space byte is '#' are skipped,
// as is any line without '='. There is no `export` keyword, no quoting and
// no $VAR expansion: the value is everything after the first '=', trimmed,
// with one substitution — a leading "~" or "~/" becomes the operator's home
// (per element for PATH).
//
// PATH is the one exception to "already set wins": a PATH line is PREPENDED
// to the inherited PATH (unless it already starts with it), because the
// process always inherits some PATH and the whole point of the line is to
// reach pi/tmux/gh/node from under launchd's narrow default.
//
// A missing or unreadable file sets nothing and returns nil.
func ApplyEnvFile(path string) (applied []string) {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	seen := map[string]bool{}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 4096), 1<<20)
	first := true
	for sc.Scan() {
		line := sc.Text()
		if first {
			line = strings.TrimPrefix(line, "\uFEFF")
			first = false
		}
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		key, value, ok := strings.Cut(trimmed, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		if key == "" || seen[key] || strings.ContainsAny(key, " \t") {
			continue
		}
		seen[key] = true
		value = strings.TrimSpace(value)
		if key == "PATH" {
			value = joinPath(expandPathElems(value))
			if value == "" {
				continue
			}
			if inherited := os.Getenv("PATH"); inherited != "" {
				if inherited == value || strings.HasPrefix(inherited, value+":") {
					continue
				}
				value = value + ":" + inherited
			}
		} else {
			if _, present := os.LookupEnv(key); present {
				continue
			}
			value = ExpandTilde(value)
		}
		if err := os.Setenv(key, value); err != nil {
			continue
		}
		applied = append(applied, key)
	}
	return applied
}

func expandPathElems(v string) []string {
	parts := strings.Split(v, ":")
	out := parts[:0]
	for _, p := range parts {
		if p == "" {
			continue // an empty element would put the cwd on PATH
		}
		out = append(out, ExpandTilde(p))
	}
	return out
}

func joinPath(parts []string) string { return strings.Join(parts, ":") }
