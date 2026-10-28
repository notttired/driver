# driver

Built on car1zhang's kraken

A macOS job board for [`pi`](https://github.com/earendil-works/pi) coding-agent
sessions. One Go daemon (`driver serve`) owns a SQLite database and a two-second
tick loop; the React board it serves shows in a frameless WKWebView shell
(`Driver.app`) and in any browser. Every job is a tmux session running `pi` as a
TUI in a directory the job names, and the board shows that real pane through an
embedded terminal: you read and type in the pane, the board tells you which jobs
are waiting on you.

It is a deliberate fork-by-lift of kraken — the four columns (draft · working ·
blocked · done), the drag rules and the look, the store and daemon patterns, the
WKWebView shell — carried over and trimmed. No worktrees or branch management
(the agent manages its own checkout; a job is a directory and a pi session), no
transcript panel or composer, nothing multi-machine. The design, every name and
knob: [docs/DESIGN.md](docs/DESIGN.md). The wire contract: [docs/API.md](docs/API.md).

## Requirements

- macOS 15 or later. The app builds with the **Command Line Tools** alone
  (`xcode-select --install`); Xcode is optional.
- Go ≥ 1.24, Node ≥ 22 (with npm), tmux.
- `pi` ≥ 1.4 on your PATH.
- `gh` (authenticated) for pull-request status on link chips — optional.

## Install

```sh
make install       # SPA + binary → ~/.local/bin/driver, seeds ~/.driver/env,
                   # loads `driver serve` under launchd (dev.notttired.driver)
make install-mac   # builds Driver.app and swaps it into /Applications
```

Then `open -a Driver`, or visit <http://127.0.0.1:7781> in a browser. The
daemon starts at login and restarts if it dies; `make uninstall-serve` unloads it.

On a managed Mac whose endpoint security (Santa in lockdown mode) runs locally
built, unsigned code only from your source checkouts, both installs die on
launch from their default places — the daemon with `Killed: 9` in
`~/.driver/driver.log`, the app silently. Put them under the sanctioned tree on
the **first** install; the dirs are remembered in `~/.driver/env`, and
`~/.local/bin/driver` becomes a symlink to the real binary:

```sh
make install DRIVER_BIN_DIR=~/src/github.com/driver-bin DRIVER_APP_DIR=~/src/github.com/driver-bin
make install-mac                      # reads DRIVER_APP_DIR from ~/.driver/env
open ~/src/github.com/driver-bin/Driver.app   # once; Spotlight and the Dock know it from then on
```

`make install` writes `~/.driver/env` on a **first** install only (it never
rewrites an existing file). The file is `KEY=VALUE` lines with `#` comments,
read by `driver serve` at start and by `make`; after an edit, `make install-serve`
reloads the launchd job. Two lines matter most: `DRIVER_OPERATOR` (your name on
the board) and `PATH` — launchd jobs inherit none of your shell's PATH, so this
line is the PATH the daemon and every pi it starts run with; it is captured from
the shell that ran `make install`.

| knob | default | what |
|---|---|---|
| `DRIVER_HOME` | `~/.driver` | home: database (`driver.db`), `env`, `jobs/`, `driver.log`, `ext/` |
| `DRIVER_PORT` | `7781` | loopback port (`driver serve -p` too) |
| `DRIVER_LABEL` | `dev.notttired.driver` | launchd label |
| `DRIVER_OPERATOR` | `$USER` | operator name shown on the board |
| `DRIVER_DEFAULT_DIR` | `$HOME` | seed directory for a new job |
| `DRIVER_PI` | `pi` on PATH | the pi binary |
| `DRIVER_PI_FLAGS` | `-a` | extra pi flags (trust project files); set empty to pass none |
| `DRIVER_NOTIFY` | on | `off` disables macOS notifications |
| `DRIVER_BROWSER` | `Google Chrome` | the browser whose open tabs the links panel's `⊞ tabs` picker lists (any Chromium-family browser) |
| `DRIVER_EDITOR` | VS Code | the command ⌘-click runs with the job's directory appended (`cursor`, `code -r`, `zed`…); unset = `code <dir>`, or `open -b com.microsoft.VSCode <dir>` when the `code` CLI is not on PATH |
| `DRIVER_BIN_DIR` | `~/.local/bin` | where `make install` puts the binary |
| `DRIVER_APP_DIR` | `/Applications` | where `make install-mac` puts Driver.app |
| `DRIVER_MAC_BUILDER` | what `xcode-select -p` says | `clt` (Command Line Tools) or `xcode` (xcodegen + xcodebuild) |
| `PATH` | the installing shell's | PATH for the daemon and its pi sessions |

## Usage

**New job.** Give it a name and a working directory (autocompletes; the agent
manages its own checkout there — one repo, several, or none), pick a model or
leave pi's default, and press START: the daemon opens a tmux session
`drv-<slug>-<id>` in that directory running `pi`, and the card opens on its
pane. The card carries the name as you typed it; the slug derived from it
(`Fix the -50 bug` → `fix-the-50-bug`) names the tmux session and is what the
API and `tmux attach` go by. There is no spec to write first — whatever you
type into pi is its first prompt. Until pi starts on it the card sits in **draft** as `○ pi open`; it
moves to **working** by itself. (A job cancelled back to draft shows a small
form instead — directory, model, START — until it is started again.)

**Reading it.** Click a card for the job overlay: the live pane on the left;
on the right, your notes beside a record of the conversation — what you typed
and pi's final answer to each message, without the narration between tool
calls — and the links beneath. The pane is the real tmux session — type in it
exactly as you would in a terminal (`tmux attach -t drv-<slug>-<id>` works too),
and ⌘-click a URL in it to open it in your browser.
Cards show state and age; link chips show a PR's open/draft/merged/closed.
**⌘-click a card** (or the directory in the overlay's head) to open that
directory in VS Code — the window that already has it, else a new one.

**Columns.** *draft* — not started, or pi open and waiting for your first
message. *working* — pi is running. *blocked* — pi wants you: it went idle,
asked a question (dialog), was interrupted, or exited. Open the card and answer
in the pane; the board follows pi's state on its own. Drags: working → blocked
sends Escape (stop the current task); blocked → working just opens the card;
blocked → done ends the job (kills the tmux session); done → working or blocked
**reactivates** (pi resumes the same session file); done → the archive drawer
under DONE archives it. The overlay's buttons do the same: start · stop · done ·
cancel (close pi; the job goes back to not started) · reactivate · archive.
Archived jobs free their slug and can be unarchived or deleted.

**Notes, the record and links.** Notes are yours — autosaved, never shown to
pi. The conversation record follows pi's session file as it grows: each of
your messages, then the last thing pi said before it stopped (an interrupted
or failed turn is marked as such); URLs in it are clickable. Links are
any URL, shown as the PR number or the host (no labels); GitHub PRs, GitHub,
Slack and Linear are recognised by kind and PR status is polled with `gh`. Every
URL in your messages to pi and in its replies lands in the job's links by
itself (tool output is not scanned, apart from the PR a `gh pr create` prints);
add others by hand, or remove any with ✕. Click or ⌘-click a link to open it in
your browser.
Rename (the name on the card; the slug follows) and recolour cards from the
overlay; ⌘K searches names, slugs and directories. A
notification arrives when a job turns blocked or its pi exits. The `⊞ tabs`
button lists your browser's open tabs; click one to attach it. The first time,
macOS asks whether driver may control Google Chrome — allow it, or flip it later
under System Settings › Privacy & Security › Automation.

In Driver.app: ⌘= / ⌘- / ⌘0 zoom the board; links open in your default browser.

## Coexistence with kraken

Driver and kraken run side by side: separate homes (`~/.driver` vs `~/.kraken`),
ports (7781 vs 7777), tmux session prefixes (`drv-*` vs `cell-*`), launchd labels
and app bundles. Nothing here reads or writes kraken's paths.

## Troubleshooting

- **Driver.app says `driver serve is not answering at http://127.0.0.1:7781`.**
  `launchctl print gui/$(id -u)/dev.notttired.driver` shows whether the job is
  loaded and its last exit status; `tail -f ~/.driver/driver.log` has the
  daemon's output. `make install-serve` rewrites and reloads the job. If the
  port is taken by something else: `lsof -i :7781`.
- **Jobs go blocked `exited` within seconds, or start refuses with "pi or tmux
  not found".** Under launchd there is no login shell, so pi, tmux, gh and node
  resolve only through the `PATH=` line in `~/.driver/env`. Make sure it carries
  their directories. pi also needs its credentials under launchd: either put
  `SHOPIFY_PROXY_KEY=…` in `~/.driver/env` or make sure `devx` is on that PATH
  so the proxy wrapper can fetch one. Then `make install-serve`.
- **The pane shows pi's last screen but nothing moves.** pi exited; the window
  is kept (`remain-on-exit`). Reactivate from done, or cancel back to draft.
- **pi prints `Warning: tmux extended-keys is off` at the top of every pane.**
  That is pi asking for `set -g extended-keys on` in `~/.tmux.conf` so
  modified Enter keys (alt/shift+Enter) reach it; the warning is harmless
  otherwise. It is a tmux server option, so driver cannot set it per session;
  add the line and restart tmux (`tmux kill-server` when nothing else is
  running in it).
- **`⊞ tabs` says macOS has not let driver read Chrome's tabs.** Reading
  open tabs goes over Apple events, which need a one-time Automation grant for
  the process that asks: under launchd that is `driver` itself, in `make dev`
  it is your terminal. Allow it in System Settings › Privacy & Security ›
  Automation (or re-run after denying the prompt by mistake: toggle it there).
  Another Chromium-family browser works with `DRIVER_BROWSER=Brave Browser`
  and the like in `~/.driver/env`; Safari's scripting dictionary differs and
  is not supported.
- **Models in the picker look like `anthropic/claude-haiku-4-5`.** That is the
  `provider/model` form pi's `--model` takes; a bare model id that several
  providers offer makes pi refuse to start with "ambiguous across providers".
- **A different server URL for the app** (another port, a dev daemon):
  `defaults write dev.notttired.DriverMac serverURL http://127.0.0.1:7782`;
  `defaults delete dev.notttired.DriverMac serverURL` — or the failure notice's
  reset button — goes back to the default.
- **Driver.app dies on launch, or the daemon logs `Killed: 9`** — endpoint
  security refusing an unsigned local build outside your source tree. Set
  `DRIVER_APP_DIR` / `DRIVER_BIN_DIR` in `~/.driver/env` to a directory under
  that tree (see Install) and run `make install` and `make install-mac` again.

## Development

```sh
make dev         # daemon from source + Vite with hot reload, both under one make
make dev-serve   # …or the two in separate terminals: go run ./cmd/driver serve
make dev-web     #    and `npm run dev` in web/_ui
make test        # go test ./... and the SPA's tests
make mac         # compile-check Driver.app (built into a temp dir and removed)
make clean
```

The launchd daemon holds :7781, so either `make uninstall-serve` first or move the
dev daemon (`make dev-serve SERVE_ARGS="-p 7782"`) and point the dev board at it —
the Vite dev server's `/api` proxy target lives in `web/_ui/vite.config.ts`.
`web/_ui/dist` is embedded into the binary, so `go build` and `go test` need
`make web-assets` once on a fresh clone. The app icon is drawn by
`swift macos/make-icon.swift macos/AppIcon-1024.png`.
