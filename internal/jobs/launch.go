package jobs

import (
	"path/filepath"
	"strings"

	"driver/internal/store"
)

// ShellQuote wraps s in single quotes for sh, escaping embedded quotes.
func ShellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// LaunchCommand assembles the sh command a job's tmux session runs. In print
// mode (DRIVER_PI_MODE=print, the default) it is driver's own pane program
// around pi:
//
//	env DRIVER_JOB_DIR='<jobdir>' DRIVER_JOB_SLUG='<slug>' DRIVER_SERVE='http://127.0.0.1:<port>' \
//	  '<driver>' pane --title '<name>' --session '<jobdir>/session.jsonl' [--model '<model>'] -- \
//	  <pi> -p --session '<jobdir>/session.jsonl' --name '<name>' [--model '<model>'] \
//	  -e '<home>/ext/driver.ts' <DRIVER_PI_FLAGS>
//
// The pane (internal/pane) reads a prompt, runs that pi command once with
// the prompt on stdin — print mode prints only the text of pi's last
// message, the final answer, none of the "let me check…" between tool
// calls — and asks for the next; pi resumes the session file every run, so
// the conversation carries over. In tui mode the command is pi itself with
// the same flags minus -p: its interactive UI, opened with no prompt and
// briefed in its pane. Either way the same command serves a submit (a fresh
// session file) and a reactivate (the existing one), and pi's session name
// is the job's display name (the name the operator typed, else the slug).
func (c Config) LaunchCommand(j *store.Job, piBin, jobDir string) string {
	session := filepath.Join(jobDir, "session.jsonl")
	name := j.DisplayName()
	words := []string{
		"env",
		"DRIVER_JOB_DIR=" + ShellQuote(jobDir),
		"DRIVER_JOB_SLUG=" + ShellQuote(j.Slug),
		"DRIVER_SERVE=" + ShellQuote(c.ServeURL()),
	}
	if c.PiMode == "tui" {
		words = append(words, ShellQuote(piBin))
	} else {
		words = append(words, ShellQuote(c.SelfExe), "pane", "--title", ShellQuote(name), "--session", ShellQuote(session))
		if j.Model != "" {
			words = append(words, "--model", ShellQuote(j.Model))
		}
		words = append(words, "--", ShellQuote(piBin), "-p")
	}
	words = append(words, "--session", ShellQuote(session), "--name", ShellQuote(name))
	if j.Model != "" {
		words = append(words, "--model", ShellQuote(j.Model))
	}
	words = append(words, "-e", ShellQuote(c.ExtPath()))
	for _, f := range c.PiFlags {
		words = append(words, ShellQuote(f))
	}
	return strings.Join(words, " ")
}
