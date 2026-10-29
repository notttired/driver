package jobs

import (
	"strings"

	"driver/internal/store"
)

// ShellQuote wraps s in single quotes for sh, escaping embedded quotes.
func ShellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// LaunchCommand assembles the sh command a job's tmux session runs:
//
//	env DRIVER_JOB_DIR='<jobdir>' DRIVER_JOB_SLUG='<slug>' DRIVER_SERVE='http://127.0.0.1:<port>' \
//	  <pi> --session '<sessionFile>' --name '<name>' [--model '<model>'] \
//	  -e '<home>/ext/driver.ts' <DRIVER_PI_FLAGS>
//
// No prompt: pi opens at its editor and the job is briefed in its pane. The
// same command serves a submit (sessionFile is a fresh `<jobdir>/session.jsonl`)
// and a reactivate (the job's current session file, which pi resumes; pi may
// have moved off session.jsonl with a /fork or /new of its own — the tick
// follows it, see daemon.followSession). pi's session name is the job's
// display name (the name the operator typed, else the slug), so pi's footer
// reads as the board does.
func (c Config) LaunchCommand(j *store.Job, piBin, jobDir, sessionFile string) string {
	words := []string{
		"env",
		"DRIVER_JOB_DIR=" + ShellQuote(jobDir),
		"DRIVER_JOB_SLUG=" + ShellQuote(j.Slug),
		"DRIVER_SERVE=" + ShellQuote(c.ServeURL()),
		ShellQuote(piBin),
		"--session", ShellQuote(sessionFile),
		"--name", ShellQuote(j.DisplayName()),
	}
	if j.Model != "" {
		words = append(words, "--model", ShellQuote(j.Model))
	}
	words = append(words, "-e", ShellQuote(c.ExtPath()))
	for _, f := range c.PiFlags {
		words = append(words, ShellQuote(f))
	}
	return strings.Join(words, " ")
}
