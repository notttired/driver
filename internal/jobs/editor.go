package jobs

import (
	"bytes"
	"context"
	"os/exec"
	"strings"
	"time"

	"driver/internal/store"
	"driver/internal/util"
)

// VSCodeBundleID is what ⌘-click falls back to when the `code` CLI is not on
// the daemon's PATH: `open -b` needs only the app to be installed.
const VSCodeBundleID = "com.microsoft.VSCode"

// editorTimeout bounds one launch. The code CLI returns as soon as it has
// handed the folder to a running VS Code (or spawned one), `open` likewise;
// anything slower is a hang, not a slow editor.
const editorTimeout = 15 * time.Second

// EditorArgv is the command that opens dir in the operator's editor:
// DRIVER_EDITOR split on spaces with dir appended when it is set (so
// `cursor`, `code -r`, `zed` all work); otherwise `code <dir>` when the VS
// Code CLI resolved on PATH (codePath), else `open -b com.microsoft.VSCode
// <dir>`. VS Code focuses the window that already has the folder open and
// opens a new one otherwise.
func EditorArgv(editor, codePath, dir string) []string {
	if f := strings.Fields(editor); len(f) > 0 {
		return append(f, dir)
	}
	if codePath != "" {
		return []string{codePath, dir}
	}
	return []string{"open", "-b", VSCodeBundleID, dir}
}

// OpenEditor opens the job's directory in the editor — what a ⌘-click on a
// card does. Allowed in every state; a job's dir is its own and stays
// meaningful after pi is gone. Failures come back as refusals so the board
// can show the editor's own words in a toast.
func (s *Service) OpenEditor(j *store.Job) error {
	dir := strings.TrimSpace(j.Dir)
	if dir == "" || !util.IsDir(dir) {
		return Refuse("dir %q is not an existing directory", j.Dir)
	}
	codePath := ""
	if s.Cfg.Editor == "" {
		codePath, _ = exec.LookPath("code")
	}
	argv := EditorArgv(s.Cfg.Editor, codePath, dir)
	ctx, cancel := context.WithTimeout(context.Background(), editorTimeout)
	defer cancel()
	var out bytes.Buffer
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Stdout, cmd.Stderr = &out, &out
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(out.String())
		if i := strings.IndexByte(msg, '\n'); i >= 0 {
			msg = msg[:i]
		}
		switch {
		case ctx.Err() != nil:
			return Refuse("%s did not return within %s", argv[0], editorTimeout)
		case msg != "":
			return Refuse("%s: %s", argv[0], msg)
		default:
			return Refuse("%s: %v", argv[0], err)
		}
	}
	return nil
}
