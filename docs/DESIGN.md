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
  **notes**, a **conversation record** and **links**. The record (2026-10-28)
  is read-only: your messages and pi's final answer to each, read from the
  session file — the narration between tool calls, the tool calls and their
  output stay in the pane. No composer, no question buttons: you type in the
  pane. Cards show status and age only (plus link chips).
- No spec step (same day, later): creating a job opens pi at once and the card
  opens on its pane; the first message is typed to pi itself. A job whose pi is
  up but has not been briefed stays in DRAFT (stored `ready`) and moves to
  WORKING by itself when pi starts on that message.
- Notes are private to the operator; they never enter pi's context.
- Links: URLs with a detected kind (GitHub PR, GitHub, Slack, Linear, other)
  and no labels — a chip reads as the PR number or the host. GitHub PR links
  show open/draft/merged/closed, polled with `gh`. Attached by hand (typed, or
  picked from the browser's tabs) or by the daemon: every URL in the
  conversation — your messages and pi's replies — and the PR the agent opens
  (`gh pr create` in a bash tool call). What you hand the agent and what it
  hands back is what the job is about, so it belongs on the card. Chips, the
  URLs in the links panel and the URLs in the pane open in the browser (click
  or ⌘-click; the pane wants ⌘, as any terminal does).
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
internal/pi/          pi session JSONL reader (tail from offset; the conversation record),
                      status.json reader, `pi --list-models` parser, GitHub PR detection
internal/term/        pty ⇄ websocket bridge around `tmux attach-session`
internal/api/         HTTP handlers (see docs/API.md); the board payload
internal/util/        env file loader, small helpers
web/embed.go          //go:embed all:_ui/dist
web/serve.go          static files + /api mux on 127.0.0.1:<port>
web/_ui/              Vite + React + TypeScript SPA (kraken's board, trimmed) + xterm.js + react-markdown
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

Columns on `jobs`: `id, slug, name, dir, model, color, spec, notes, state, outcome,
pi_session, tmux, started_at, finished_at, created_at, updated_at, archived_at,
archived_name, rev`. `state` ∈ `'' | ready | working | blocked | done` (`''` = not
started, `ready` = pi open at its prompt with nothing sent yet; both display as
draft);
`archived_at` set = archived (slug freed: the unique index on slug is partial,
`WHERE archived_at IS NULL`; `archived_name` is `<slug>-<id>` for display). `rev`
guards concurrent updates. `color` ∈ `'' | red blue green yellow purple orange
pink cyan` (kraken's palette, same hexes). `spec` is a legacy column: nothing
writes or reads it since the spec step was dropped. `model` is a pi model id or
pattern (`--model`), empty = pi's default. `name` is what the operator typed in
NEW JOB (whitespace-collapsed, ≤ 200 runes): the card, the overlay head, pi's
`--name` and the notifications go by it, while the slug derived from it stays
the identifier — the API, `drv-<slug>-<id>`, the job dir. Empty (every job from
before v4) = the slug.

`PRAGMA user_version` tracks the schema. v2 rebuilt `jobs` for the `ready` state
(the `CHECK` on `state` cannot be altered in place); the migration runs with
foreign keys off, so dropping the old table does not cascade into `links`, and
restores `sqlite_sequence` afterwards, so a deleted job's id is never reused.
v3 dropped `links.label` and forgot the session scan offsets, so the scanner,
which from then on also attaches the URLs in your own messages, reads every
existing session once more (attaching is idempotent). v4 added `jobs.name`. v5
added `jobs.attention_at`: when the daemon moved the job into blocked on its own
and it has not been opened since (0 otherwise).

`board_order("column", position, job_id)` holds the manual order per column
exactly as kraken's does; the board emits each column's jobs in that order,
unlisted jobs first alphabetically. WORKING and BLOCKED read in arrival order:
whenever the tick's state write moves a job into one of them (and when
reactivate does), the daemon appends it to that column's order
(`AppendToColumn`: a no-op when a drag already listed it there, otherwise out of
every other column and onto the end). Cross-column drags do not reorder the
destination, so they count as arrivals too.

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
   `env DRIVER_JOB_DIR='<jobdir>' DRIVER_JOB_SLUG='<slug>' DRIVER_SERVE='http://127.0.0.1:<port>' <pi> --session '<jobdir>/session.jsonl' --name '<name>' [--model '<model>'] -e '<home>/ext/driver.ts' <DRIVER_PI_FLAGS>`
   — no positional prompt: pi opens at its editor, and whatever is typed in the
   pane first is its first prompt; `<name>` is the job's name, else its slug,
   so pi's footer reads as the board does;
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

**Fork** (`POST /api/job/fork {slug, to, name}`): a copy of the job under a new
slug and name — dir, model, colour, spec, notes and links (`CopyLinks`, status
included) — placed in the original's column right after it (the column's
shown order is written back with the fork slotted in). When the original has
a session file it is copied into the fork's job dir by `pi.ForkSession`: the
header line gets a fresh UUID v7 id and timestamp and `parentSession` = the
original's path; every other line is byte-identical, so the entry tree holds
and pi resumes the copy where the original stood (pi's own `--fork` writes the
same shape; its code is not readable from the Nix install, so driver writes
the header itself). `status.json` is never copied. The fork's link-scanner
offset starts at the end of the copied file: its links came along from the
original, removals included. Then by the original's stored state: `''` → a
draft (nothing launched); `done` → `done` with the outcome and times copied
(reactivate to carry on); `ready`/`working`/`blocked` → the start launch on
the copied session (dir must exist, pi and tmux resolvable, else 409 before
anything is made) with the ORIGINAL's stored state, so the card lands in the
same column while pi boots — hence the `blocked`-stays-`blocked` branch of
the starting grace below. A fork that fails halfway is removed again (tmux
session, job dir, row). Archived originals are refused; the UI hides the
button for them. The overlay re-points itself at the fork when it lands.

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
  no status.json yet                 → the stored ready/blocked, else working (pi "starting") if started_at < 120s ago,
                                       else blocked ("silent")
```

A `blocked` derived for a working job must be observed on two consecutive ticks
before it is written (kraken's one-tick hold: it hides one-tick misreads and the
idle a resumed session reports at `session_start` before pi picks the conversation
back up). Ready → working is written at once: that is pi starting on the first
message. On a write to `blocked` the daemon posts a
notification `driver ⚑ <name> — <reason>` (the slug when the job has no name) via
`osascript -e 'display notification … with title …'`, and — unless the reason
is `aborted`, an interrupt the operator sent (Escape, STOP, the working → blocked
drag) — sets `attention_at`, which the board shows as a ring around the card
until the job leaves blocked or the operator opens it (`POST /api/job/seen`,
written without a rev bump so it neither ages the card nor trips the tick).

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

- role `user` and role `assistant` — the conversation: every `http(s)://` URL
  in their `text` blocks (string content too; `thinking` and tool-call
  arguments are left out), trailing sentence punctuation and unbalanced
  closing brackets stripped, so `(https://x/a).` is `https://x/a` while a
  wiki-style `…/Foo_(bar)` keeps its parenthesis. Tool output is not scanned
  (a `curl` of a page would otherwise litter the card) — except:
- role `assistant` with a `toolCall` block `name == "bash"` whose
  `arguments.command` contains `gh pr create` is remembered; when the matching
  `toolResult` (`toolCallId`) arrives, every
  `https://github.com/<owner>/<repo>/pull/<n>` in its text is a `github_pr` link.

`kv.scanner-generation` names what the scanner looks for (`2`); a daemon that
finds an older value, or none, forgets every `scan:*` offset once and reads each
session again from the start, so widening the scanner never misses what is
already on disk (v3's migration did the same by hand, before the key existed).

Links of kind github_pr whose
`status_at` is older than 60s are refreshed, at most 3 per tick, with
`gh pr view <url> --json state,isDraft` → `open|draft|merged|closed`; a failing
`gh` leaves the status untouched.

## Conversation record

`GET /api/job/conversation` derives the overlay's record from the session file
on request, cached on the file's size and mtime, so the overlay's 3-second poll
re-reads a session only when pi appended to it (`rev` is that stamp; the SPA
keeps its rendered list while it does not change). The entries read are those
on the branch of the file's last entry: every entry names its parent, and a
`/tree` jump leaves the abandoned branch in the file, off the path. Walking that
path, every `message` of role `user` is a row of yours (its text blocks,
newline-joined), and each turn — a user message and everything up to the next
one — adds at most one row of pi's: its last `assistant` message, provided that
message's `stopReason` is not a mid-turn one (`toolUse`, `pending`, `deferred`).
`stop` and `length` are answers (`length` marked as cut off); `aborted` and
`error` are rows carrying whatever partial text there was, marked with the
reason (and pi's `errorMessage`). A turn whose last assistant message asked for
tools gets no row — pi was steered with a further message mid-turn, or was cut
off before answering; when that is the LAST turn the record is `pending`, and
the panel's closing row says "working…", "asking you something in the pane" or
"stopped before answering" from the job's pi state. `system` messages,
`custom_message` entries (extensions'), tool results, thinking blocks and every
non-message entry are skipped. pi's rows render as markdown (react-markdown
with GitHub flavour: tables, task lists, strikethrough, bare URLs linked; a
single newline is a line break, as in the pane; raw HTML shows as text, never
as markup; only http(s) targets become links, opening in the browser like every
link on the board). Your rows stay as typed, their URLs anchors as in the links
panel.

## Terminal

`GET /api/term?slug=<slug>` upgrades to a websocket. The server starts
`tmux attach-session -t <sess>` on a pty (`TERM=xterm-256color`,
`COLORTERM=truecolor`) sized from the first resize message, then pumps bytes both
ways. Wire: **binary frames** carry terminal bytes in both directions; **text
frames** are JSON control messages, today only `{"type":"resize","cols":N,"rows":N}`
from the client. Closing the socket kills the attach process (the tmux session
lives on). Because tmux's `window-size` is `latest` on this machine, the newest
client's size wins; the bridge does not fight other attached clients.

Keys go through xterm untouched except ⌘← / ⌘→: xterm has no sequence for a
⌘-arrow and drops it (WebKit then moves the caret of xterm's hidden textarea),
so the pane sends what the Home / End keys send instead (`ESC [ H` / `ESC [ F`,
or `ESC O H` / `ESC O F` in application cursor keys mode — `lib/paneKeys.ts`).
tmux knows both forms and pi binds `home` / `end` to its editor's line start /
line end, so the macOS line jumps work in pi. Other modifiers (⌥-arrows for
words, ⇧) are left to xterm.

URLs in the pane — plain text found by xterm's web-links addon, and OSC 8
hyperlinks — open in the browser on ⌘-click through `window.open(url, "_blank")`.
Both addons' default activation opens an *empty* window first and then sets its
location; in the shell that arrives as `about:blank`, which LaunchServices
answers with "The application can't be opened. -50" while the link never opens.
The shell also ignores anything but an http(s) URL in `createWebViewWith`.

## Board rules (kraken's, trimmed)

Bucketing: archived → drawer; else `state` names the column (`''`/`ready`/unknown
→ draft). Same-column drop → `POST /api/reorder {column, slugs}` (full column,
optimistic). WORKING and BLOCKED are in arrival order (above); a card that turned
blocked on its own wears a ring until it is opened. Cross-column moves:

| from → to | action |
|---|---|
| draft → working | not started: start (`/api/queue`); the card shows a spinner while the request runs and a 409 returns it with the reason in a toast. pi already open (`ready`): just open the card — brief it in the pane and pi moves it |
| working → blocked | `/api/stop` |
| blocked → done | `/api/done` |
| blocked → working | open the card (go talk to it in the pane); no write |
| done → working, done → blocked | `/api/reactivate` |
| done → archive drawer | `/api/archive` |
| anything else | no-op |

Card click → the job overlay: head (name + rename, with `as <slug>` beside a name that differs from the slug, dir, colour swatch, actions:
start / stop / done / cancel / reactivate / archive by state), body = the pane
for every opened job (pi at its prompt, working, blocked, done) or the launch
form for a not-started one (dir with directory autocomplete, model datalist,
START), sidebar = notes (autosaved) and the conversation record side by side,
links (list, add, remove, PR status) beneath.
The NEW JOB dialog takes a name, dir and model and starts the job itself
(`/api/job/create`, then `/api/queue`), so the overlay it opens is already on
the pane. Escape closes; ⌘K focuses search. ⌘-click on a card skips the overlay
and posts `/api/job/open` (the directory in the editor); the overlay's dir label
answers the same gesture.
