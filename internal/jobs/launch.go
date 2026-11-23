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

// LaunchCommand assembles the sh command a job's tmux session runs:
//
//	env DRIVER_JOB_DIR='<jobdir>' DRIVER_JOB_SLUG='<slug>' DRIVER_SERVE='http://127.0.0.1:<port>' \
//	  <pi> --session '<sessionFile>' --name '<name>' [--model '<model>'] \
//	  -e '<home>/ext/driver.ts' <DRIVER_PI_FLAGS>
//
// No prompt unless one is given (a retry resumes a turn pi was cut off in
// with `-- 'continue'`): pi opens at its editor and the job is briefed in
// its pane. The same command serves a submit (sessionFile is a fresh
// `<jobdir>/session.jsonl`), a reactivate and a retry (the job's current
// session file, which pi resumes; pi may have moved off session.jsonl with a
// /fork or /new of its own — the tick follows it, see daemon.followSession).
// pi's session name is the job's display name (the name the operator typed,
// else the slug), so pi's footer reads as the board does.
func (c Config) LaunchCommand(j *store.Job, piBin, jobDir, sessionFile string, prompt ...string) string {
	name := j.DisplayName()
	if j.ThreadName != "" {
		name += " · " + j.ThreadName
	}
	return c.launchCommand(j, name, piBin, jobDir, sessionFile, prompt...)
}

// ThreadLaunchCommand is LaunchCommand for one of the job's threads: the
// same job, slug, model and flags; its own job dir (the thread's, where its
// status.json goes) and session file; named "<job> · <thread>".
func (c Config) ThreadLaunchCommand(j *store.Job, t *store.Thread, piBin, sessionFile string, prompt ...string) string {
	return c.launchCommand(j, ThreadPiName(j, t), piBin, filepath.Dir(sessionFile), sessionFile, prompt...)
}

func ThreadPiName(j *store.Job, t *store.Thread) string {
	return j.DisplayName() + " · " + t.Name
}

func (c Config) launchCommand(j *store.Job, name, piBin, jobDir, sessionFile string, prompt ...string) string {
	words := []string{
		"env",
		"DRIVER_JOB_DIR=" + ShellQuote(jobDir),
		"DRIVER_JOB_SLUG=" + ShellQuote(j.Slug),
		"DRIVER_SERVE=" + ShellQuote(c.ServeURL()),
		ShellQuote(piBin),
		"--session", ShellQuote(sessionFile),
		"--name", ShellQuote(name),
	}
	if j.Model != "" {
		words = append(words, "--model", ShellQuote(j.Model))
	}
	words = append(words, "-e", ShellQuote(c.ExtPath()))
	for _, f := range c.PiFlags {
		words = append(words, ShellQuote(f))
	}
	if len(prompt) > 0 {
		words = append(words, "--")
		for _, p := range prompt {
			words = append(words, ShellQuote(p))
		}
	}
	return strings.Join(words, " ")
}
