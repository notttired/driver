# driver — design

A macOS job board for `pi` coding-agent sessions. One long-lived Go daemon
(`driver serve`) owns a SQLite database and a tick loop; a React board served by
that daemon is shown in a frameless WKWebView shell (`Driver.app`) and in any
browser. Each job is a tmux session running `pi` in a directory the job names;
the board shows the real pane through an embedded terminal.

It is a deliberate fork-by-lift of [kraken](../../kraken): the board, its drag
rules, its look, the store and daemon patterns, and the WKWebView shell are
kraken's, carried over file by file and trimmed. Everything Claude/Codex/zeus,
multi-machine (tailnet, iOS, pairing), intake (Slack, Linear), and workflow
(crew, skills, threads, artifacts, doc questions, feedback, dream, mind, goals,
usage, accounts, self-update) is left out on purpose.

## Decisions (from the planning conversation, 2026-10-27)

- Native shell = WKWebView over the web board (kraken's approach), built with the
  Command Line Tools alone (no Xcode on this Mac): `swiftc`, `plutil`,
  `iconutil`, ad-hoc `codesign`.
- Engine = Go daemon derived from kraken; harness = `pi` 1.4.0, run as a TUI in a
  tmux session per job. The daemon never runs pi headless.
- No worktree/branch management: **the agent manages its own checkout.** A job
  names a working directory (`dir`), nothing more. Jobs may be multi-repo or
  repo-less.
- Card detail = the live tmux pane (xterm.js attached over a websocket) plus
  **notes** and **links**. No transcript panel, no composer, no question buttons:
  you read and type in the pane. Cards show status and age only (plus link chips).
- No spec step (same day, later): creating a job opens pi at once and the card
  opens on its pane; the first message is typed to pi itself. A job whose pi is
  up but has not been briefed stays in DRAFT (stored `ready`) and moves to
  WORKING by itself when pi starts on that message.
- Notes are private to the operator; they never enter pi's context.
- Links: URLs with a detected kind (GitHub PR, GitHub, Slack, Linear, other)
  and no labels — a chip reads as the PR number or the host. GitHub PR links
  show open/draft/merged/closed, polled with `gh`. Attached by hand (typed, or
  picked from the browser's tabs) or by the daemon: every URL in a message you
  send pi, and the PR the agent opens (`gh pr create` in a bash tool call). What
  you hand the agent is what the job is about, so it belongs on the card.
- macOS notifications when a job turns blocked (needs you) or its pi exits.
- ⌘-click on a card (or on the dir in the overlay head) opens the job's
  directory in VS Code: the daemon runs `code <dir>` (`DRIVER_EDITOR` swaps the
  command, `open -b com.microsoft.VSCode` covers a missing CLI). No editor
  links, no button: the gesture is the whole feature.
- Kept from kraken: four columns (draft · working · blocked · done), drag to
  reorder and to transition, archive drawer under DONE, 8-colour job stripe,
  rename, board search (⌘K), theme + accent, daemon health in the chrome.

## Layout

```
cmd/driver/           main.go — `driver serve [-p 7781]`, `driver version`
internal/store/       SQLite (modernc.org/sqlite, pure Go): schema, jobs, order, links, kv
internal/jobs/        Job model, submit/stop/done/kill/reactivate/archive, state
                      derivation, tmux + pi launch, tick loop, notifications, PR scan
internal/pi/          pi session JSONL reader (tail from offset), status.json reader,
                      `pi --list-models` parser, GitHub PR detection
internal/term/        pty ⇄ websocket bridge around `tmux attach-session`
internal/api/         HTTP handlers (see docs/API.md); the board payload
internal/util/        env file loader, small helpers
web/embed.go          //go:embed all:_ui/dist
web/serve.go          static files + /api mux on 127.0.0.1:<port>
web/_ui/              Vite + React + TypeScript SPA (kraken's board, trimmed) + xterm.js
ext/driver.ts         the pi extension (status.json writer); embedded in the binary and
                      written to ~/.driver/ext/driver.ts on every serve start
macos/                Driver.app: DriverApp.swift, ServerConfig.swift, project.yml,
                      Generated-Info.plist, build-without-xcode.sh, AppIcon-1024.png
Makefile              build · install · install-serve (launchd) · mac · install-mac · dev · test
docs/                 this file, API.md
```

## Names, paths, knobs

| thing | value |
|---|---|
| home | `~/.driver` (`DRIVER_HOME`) |
| database | `~/.driver/driver.db` (`DRIVER_DB_PATH`), WAL |
| env file | `~/.driver/env` — `KEY=VALUE` lines, `#` comments; loaded by `driver serve` and the Makefile |
| job dir | `~/.driver/jobs/<slug>-<id>/` with `session.jsonl`, `status.json` |
| log | `~/.driver/driver.log` (launchd stdout/stderr) |
| port | `7781` (`-p`, `DRIVER_PORT`); loopback only |
| tmux session | `drv-<slug>-<id>`, session option `@driver-job=<slug>` |
| launchd label | `dev.notttired.driver` (`DRIVER_LABEL`) |
| app bundle id | `dev.notttired.DriverMac`, product `Driver` |
| operator name | `DRIVER_OPERATOR` else `$USER` |
| default dir for new jobs | `DRIVER_DEFAULT_DIR` else `$HOME` |
| pi binary | `DRIVER_PI` else `pi` on PATH |
| extra pi flags | `DRIVER_PI_FLAGS` (default `-a`, trust project files; set empty to disable) |
| binary install dir | `DRIVER_BIN_DIR` else `~/.local/bin` |
| app install dir | `DRIVER_APP_DIR` else `/Applications` |
| notifications | `DRIVER_NOTIFY=off` disables |
| editor for ⌘-click | `DRIVER_EDITOR` (a command, dir appended) else `code`, else `open -b com.microsoft.VSCode` |
| PATH for launchd | `PATH=` line in the env file (pi, tmux, gh, node live outside launchd's PATH) |

Kraken (`~/.kraken`, port 7777, `cell-*` sessions, `@kraken-job`) coexists on
the same Mac; nothing here reads or writes its paths.

## Job model

Columns on `jobs`: `id, slug, dir, model, color, spec, notes, state, outcome,
pi_session, tmux, started_at, finished_at, created_at, updated_at, archived_at,
archived_name, rev`. `state` ∈ `'' | ready | working | blocked | done` (`''` = not
started, `ready` = pi open at its prompt with nothing sent yet; both display as
draft);
`archived_at` set = archived (slug freed: the unique index on slug is partial,
`WHERE archived_at IS NULL`; `archived_name` is `<slug>-<id>` for display). `rev`
guards concurrent updates. `color` ∈ `'' | red blue green yellow purple orange
pink cyan` (kraken's palette, same hexes). `spec` is a legacy column: nothing
writes or reads it since the spec step was dropped. `model` is a pi model id or
pattern (`--model`), empty = pi's default.

`PRAGMA user_version` tracks the schema. v2 rebuilt `jobs` for the `ready` state
(the `CHECK` on `state` cannot be altered in place); the migration runs with
foreign keys off, so dropping the old table does not cascade into `links`, and
restores `sqlite_sequence` afterwards, so a deleted job's id is never reused.
v3 dropped `links.label` and forgot the session scan offsets, so the scanner,
which from then on also attaches the URLs in your own messages, reads every
existing session once more (attaching is idempotent).

`board_order("column", position, job_id)` holds the manual order per column
exactly as kraken's does; the board emits each column's jobs in that order,
unlisted jobs first alphabetically.

`links(id, job_id, kind, url, status, status_at, created_at)` with
`UNIQUE(job_id, url)`; no label column — a link is its URL. `kind` ∈ `github_pr | github | slack | linear | url`,
derived from the URL on insert (`github.com/<o>/<r>/pull/<n>` → github_pr;
`github.com` → github; host ends `slack.com` → slack; `linear.app` → linear).
`status` for github_pr ∈ `'' | open | draft | merged | closed`.

`kv(key, value)` for daemon bookkeeping (session scan offsets, model cache).

## Lifecycle

**Start** (not started → ready; `POST /api/queue`): refuse (409 with reason) when
the job is already open (`ready`), is not a draft, `dir` is not an existing
directory, or `pi` / `tmux` cannot be resolved. Then:

1. make the job dir; remove any stale `status.json` (and a legacy `prompt.md`);
   an earlier run's `session.jsonl` is renamed `session-<unix>.jsonl`;
2. `tmux new-session -d -x 220 -y 50 -s drv-<slug>-<id> -c <dir> -n <slug> <cmd>`
   where `<cmd>` is a `sh` string:
   `env DRIVER_JOB_DIR='<jobdir>' DRIVER_JOB_SLUG='<slug>' DRIVER_SERVE='http://127.0.0.1:<port>' <pi> --session '<jobdir>/session.jsonl' --name '<slug>' [--model '<model>'] -e '<home>/ext/driver.ts' <DRIVER_PI_FLAGS>`
   — no positional prompt: pi opens at its editor, and whatever is typed in the
   pane first is its first prompt;
3. `tmux set-option -t <sess> @driver-job <slug>`;
   `tmux set-window-option -t <sess> remain-on-exit on` (a finished pi leaves a
   dead pane whose last screen the terminal can still show); `status off`;
4. store `tmux`, `pi_session`, `started_at`, `state=ready`. The tick moves the
   job to `working` when the extension reports pi running on that first message.

**Stop** (working → blocked drag; `POST /api/stop`): `tmux send-keys -t <sess> Escape`
(pi: interrupt the current task). Writes no state; the next tick reads pi idle.

**Done** (`POST /api/done`): `tmux kill-session -t <sess>` if it exists; `state=done`,
`outcome=accepted`, `finished_at`. Reachable by drag from blocked and by the
detail view's button from ready, working or blocked (for a job that was never
briefed it is the only way out: archive and delete need done).

**Kill / cancel** (`POST /api/kill`): kill the tmux session; `state=''` (back to
not started), clear `tmux`/`started_at`. Works from ready, working and blocked.
The session file stays on disk; a later start begins a fresh one (session.jsonl
renamed `session-<unix>.jsonl`).

**Reactivate** (done → working/blocked drags, or button; `POST /api/reactivate`):
the same launch as start on the existing `--session <file>`, so pi resumes the
conversation; `state=working` (the hold below absorbs the idle pi reports at
`session_start` before it picks the conversation back up).

**Archive / unarchive / delete**: kraken semantics. Archive only from done;
delete only when archived; both refuse a live tmux session.

## State derivation (tick every 2s)

```
stored ''      → draft (not started: no session, no pane)
stored done    → done
else (stored ready | working | blocked):
  live   := tmux session exists AND pane 0 not dead
  status := status.json (written by ext/driver.ts): {state, tool?, pid, session, ts}
  !live                              → blocked (reason "exited")
  status.state == ready              → ready (draft column; the card says "○ pi open")
  status.state == working            → working
  status.state ∈ idle|dialog|aborted → blocked (reason = state)
  no status.json yet                 → the stored ready/working (pi "starting") if started_at < 120s ago,
                                       else blocked ("silent")
```

A `blocked` derived for a working job must be observed on two consecutive ticks
before it is written (kraken's one-tick hold: it hides one-tick misreads and the
idle a resumed session reports at `session_start` before pi picks the conversation
back up). Ready → working is written at once: that is pi starting on the first
message. On a write to `blocked` the daemon posts a
notification `driver ⚑ <slug> — <reason>` via
`osascript -e 'display notification … with title …'`.

`ext/driver.ts` (loaded with `-e`; a no-op without `DRIVER_JOB_DIR`): writes
`status.json` atomically (tmp + rename) on `session_start` (`ready` when the
session branch holds no user message yet — a fresh job at its prompt — else idle),
`agent_start` (working), `tool_execution_start/end` (tool name), `ui_prompt_start`
(dialog) / `ui_prompt_end`, `agent_settled` (idle when `ctx.isIdle()`, aborted when
`e.aborted`), `session_shutdown` (exited). Type import:
`import type { ExtensionAPI } from "@earendil-works/pi-coding-agent";`.

## Link detection and PR status

Each tick, for every job with a `pi_session` file, the daemon reads the bytes
appended since the offset saved in `kv` (`scan:<job id>`) and parses complete
JSONL lines. Two kinds of `message` entry yield links, both attached idempotently
on `(job_id, url)` with the kind derived from the URL:

- role `user` — your own messages to pi: every `http(s)://` URL in their text
  blocks (string content too), trailing sentence punctuation and unbalanced
  closing brackets stripped, so `(https://x/a).` is `https://x/a` while a
  wiki-style `…/Foo_(bar)` keeps its parenthesis. Only your messages count: the
  agent's own text and tool output are not scanned (a `curl` of a page would
  otherwise litter the card).
- role `assistant` with a `toolCall` block `name == "bash"` whose
  `arguments.command` contains `gh pr create` is remembered; when the matching
  `toolResult` (`toolCallId`) arrives, every
  `https://github.com/<owner>/<repo>/pull/<n>` in its text is a `github_pr` link.

Links of kind github_pr whose
`status_at` is older than 60s are refreshed, at most 3 per tick, with
`gh pr view <url> --json state,isDraft` → `open|draft|merged|closed`; a failing
`gh` leaves the status untouched.

## Terminal

`GET /api/term?slug=<slug>` upgrades to a websocket. The server starts
`tmux attach-session -t <sess>` on a pty (`TERM=xterm-256color`,
`COLORTERM=truecolor`) sized from the first resize message, then pumps bytes both
ways. Wire: **binary frames** carry terminal bytes in both directions; **text
frames** are JSON control messages, today only `{"type":"resize","cols":N,"rows":N}`
from the client. Closing the socket kills the attach process (the tmux session
lives on). Because tmux's `window-size` is `latest` on this machine, the newest
client's size wins; the bridge does not fight other attached clients.

## Board rules (kraken's, trimmed)

Bucketing: archived → drawer; else `state` names the column (`''`/`ready`/unknown
→ draft). Same-column drop → `POST /api/reorder {column, slugs}` (full column,
optimistic). Cross-column moves:

| from → to | action |
|---|---|
| draft → working | not started: start (`/api/queue`); the card shows a spinner while the request runs and a 409 returns it with the reason in a toast. pi already open (`ready`): just open the card — brief it in the pane and pi moves it |
| working → blocked | `/api/stop` |
| blocked → done | `/api/done` |
| blocked → working | open the card (go talk to it in the pane); no write |
| done → working, done → blocked | `/api/reactivate` |
| done → archive drawer | `/api/archive` |
| anything else | no-op |

Card click → the job overlay: head (slug + rename, dir, colour swatch, actions:
start / stop / done / cancel / reactivate / archive by state), body = the pane
for every opened job (pi at its prompt, working, blocked, done) or the launch
form for a not-started one (dir with directory autocomplete, model datalist,
START), sidebar = notes (autosaved) and links (list, add, remove, PR status).
The NEW JOB dialog takes a name, dir and model and starts the job itself
(`/api/job/create`, then `/api/queue`), so the overlay it opens is already on
the pane. Escape closes; ⌘K focuses search. ⌘-click on a card skips the overlay
and posts `/api/job/open` (the directory in the editor); the overlay's dir label
answers the same gesture.
