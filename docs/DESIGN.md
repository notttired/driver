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
- Notes are the operator's; pi reads or changes them only when asked
  (2026-11-13). The extension gives pi a `driver_notes` tool (read, append,
  replace) whose guidelines tell it to use the tool only when the operator
  asks — a prompt rule, not a lock. The tool finds its job by the job dir it
  was started in (`DRIVER_JOB_DIR`), which a rename leaves alone, never by
  `DRIVER_JOB_SLUG`, which a rename makes stale. The notes panel polls them
  every 3s and takes what is on the daemon whenever it has nothing unsaved;
  while the operator has unsaved typing, theirs wins and the autosave
  overwrites whatever pi wrote in those few hundred milliseconds. Until
  2026-11-13 notes never entered pi's context.
- Links: URLs with a detected kind (GitHub PR, GitHub, Slack, Linear, other)
  and no labels — a chip reads as the PR number or the host. GitHub PR links
  show open/draft/merged/closed, polled with `gh`. Attached by hand (typed, or
  picked from the browser's tabs) or by the daemon: every URL in the
  conversation — your messages and pi's replies — and the PR the agent opens
  (`gh pr create` in a bash tool call). What you hand the agent and what it
  hands back is what the job is about, so it belongs on the card. Chips, the
  URLs in the links panel and the URLs in the pane open in the browser (click
  or ⌘-click; the pane wants ⌘, as any terminal does).
- Link order (2026-11-06): the pinned links first, the last pinned on top,
  then the rest, newest first (by when the link first arrived). A link is
  pinned when you open it (anywhere — chip, links panel, record, pane), press
  its 📌 in the links panel or add it by hand, and stays pinned until you
  press 📌 again; opening a pinned link puts it back on top. Until 2026-11-06
  the links you mentioned in a message came first, then the pinned ones, each
  group most mentioned first: a first message carrying nine links held all
  five visible rows, and a fresh 📌 landed out of sight. Mentions are still
  counted, shown as ×N: a mention is a message that carries the URL (yours or
  pi's, or the result of the `gh pr create` that printed it); a link added by
  hand counts one. The panel's list shows five rows and scrolls for the rest;
  the card shows the same first five as chips and "+N" for the rest.
- One link per page (2026-11-06): a link is stored under its page's URL — no
  `#…` fragment (a `#/…` or `#!…` route stays: single-page apps route on
  it), no trailing `/`, and every page of a GitHub or Delta pull request
  (`/files`, `/changes`, `/commits/<sha>`, …) as the pull request itself. The
  same page arriving under another URL is a mention of the link already
  there, adding it by hand is refused as a duplicate, and opening or pinning
  it under any form pins that link. Until then `…/pull/7454` and two
  `…/pull/7454#discussion_r…` were three links; `driver serve` folds such
  links into one when it starts (every mention, the earliest arrival and the
  latest pin kept). The cost: a link to one commit or line opens the pull
  request or the file; the exact URL stays clickable in the record.
- macOS notifications when a job turns blocked (needs you) or its pi exits.
- Retry (2026-11-02): network drops leave pi up but stuck on a failed turn
  ("Connection error.", "Request timed out.", "terminated" — after its own
  three retries), and a console logout kills every pi at once (dead panes,
  `exited`). Both are told apart from a job that finished and waits on you:
  such cards read `✗ <error>` or `✕ exited`, the BLOCKED head carries
  `↻ retry N` for all of them, and the overlay a RETRY for one.
- Retry at start (2026-11-15): a shutdown or a logout kills every pi, and
  `driver serve` comes back at login to a board of `✕ exited` cards that sat
  there until each was retried by hand. Now the daemon retries them itself.
- ⌘-click on a card (or on the dir in the overlay head) opens the job's
  directory in VS Code: the daemon runs `code <dir>` (`DRIVER_EDITOR` swaps the
  command, `open -b com.microsoft.VSCode` covers a missing CLI). No editor
  links, no button: the gesture is the whole feature.
- Kept from kraken: four columns (draft · working · blocked · done), drag to
  reorder and to transition, archive drawer under DONE, 8-colour job stripe,
  rename, theme + accent, daemon health in the chrome. kraken's board filter
  (⌘K narrowing the columns) became the palette (2026-11-17): a popup over
  everything, Raycast-fashion, listing every pane — each job and each of its
  threads, archived last — with its column and status, narrowing as you type
  to what the words are found in. A word counts where it is found (lib/
  palette.ts): the title or slug most (a prefix more, the whole more still),
  then thread names, the directory, the column or lane name, the status
  words, the model, a link, the tool pi is in; a title the word only
  abbreviates (its initials, or letters in order within one word) counts
  least, and letters scattered across words do not count. Every word must
  land somewhere; rows carry the reason when it was not the title (the
  link, the dir, the model). ⏎ opens the pane — a thread's row opens the job
  on that tab — ⌘⏎ opens the directory in the editor. The columns are no
  longer filtered: the board is always the whole board.
- Lanes: columns of your own (`+` at the end of the board; rename by
  clicking the name, ✕ to remove). A lane is a parking place, not a state: a
  card dragged into one keeps its state and stays put while the state changes;
  dragged back out it can only land in the column its state names, so a drag
  out of a lane is never a way to start, stop or finish anything. DONE (the
  overlay button) finishes the job and takes it out of the lane. Removing a
  lane returns its cards to their state columns too. The day-old-done sweep
  leaves parked cards alone.
- Threads (2026-11-17): a job can carry more than one pi. The tabs above
  the pane are its pis — the job's own first, then every thread opened with
  `+`: a copy of the conversation in the current tab up to its finished work
  (`pi.ForkSessionFinished`: when the branch's last turn has no final answer
  yet, the copy ends at the last answer, so a thread opened mid-turn starts
  from settled ground rather than an unanswered prompt and tool calls in
  flight — it hears that turn's answer later, with its question, as peer
  context; the job fork still copies whole) with a pi of its own resumed on
  it, in a tmux session of
  its own (`drv-<slug>-<id>-t<n>`: windows in one session would share the
  session's current window across every attached client, so the board's pane
  and a `tmux attach` would fight over the tab shown), in the job's dir,
  under a name typed in the tab row (⌘T skips the row: the default "thread
  N", renamed later if wanted). Every thread is a full agent with the
  same tools; they share one checkout, so the operator's rule is a git
  worktree per thread that changes code. The card folds its threads
  together: working while any works, else blocked when any waits (the first
  such pi speaks for the card), with the one-tick hold and the ring as ever;
  a thread that stops while the card stays working sends its own
  notification (`<job> · <thread> — <reason>`). Each tab has its own pane
  (`/api/term?thread=`), its own record (`/api/job/conversation?thread=`),
  STOP and RETRY; the tab a job is left on is remembered per job (this
  browser's, in localStorage, like the folded columns); ⌘⇧] / ⌘⇧[ step
  through the tabs, round the end (a
  document listener keyed on the bracket key codes, so the pane — which
  leaves ⌘-combinations to the page — and the sidebar both hear it). ⌘W
  closes what is on top: the palette, a dialog, the thread tab in view
  (at once, as ✕ does — nothing is lost: the row and its conversation stay),
  else the job overlay; never the board. The first tab closes like the
  rest while another is open: every job-level action (fork, retry,
  reactivate, the record) works on the job's own pi, so closing it means
  the next open thread takes that place — `CloseOwnThread` kills the own
  session, keeps the old pi as a closed thread row (its session file where
  it is, so it still speaks in the peer context), and moves the thread's
  `pi_session`, `tmux` and `started_at` into the job row with its name as
  `jobs.thread_name` (v11; `OwnName`: the tab's and pi's name, "" = the
  job's; a fresh start clears it), the scanner's place in each session
  following. The promoted pi keeps its `t<id>` dir: `OwnDir` is where the
  own pi lives (its session file's dir), `JobDir` the job's root — the own
  dir, or its parent when that is a thread's dir — which the threads' dirs
  sit under. The last tab cannot be closed (⌘W on it closes the overlay). Driver.app
  drops the File ▸ Close menu item (`CommandGroup(replacing: .saveItem)`)
  so the key reaches the page instead of closing the window — which the
  traffic light still does. ✕ closes a thread's pi; the row and its conversation stay
  (what it said is still part of the job: the tab goes, the record remains
  readable). DONE closes every thread, CANCEL drops them; the job's own pi
  is the one fork, reactivate and rename work on, and a job fork
  copies the job's own pi only. The daemon follows a thread's session file,
  scans it for links (scan key `<job id>/t<thread id>`) and brings it back
  at start as it does the job's. v10 adds `threads`.
- Threads: shared context (2026-11-17). The threads hear each other, at the
  operator's pace. Every message the operator sends a thread arrives with
  what the other threads said since that thread last heard from them — the
  operator's messages to them and their final answers (what the record
  shows: no narration, tool calls, results or thinking), the last message
  marked "in progress, no answer yet" while a peer is on it, failed and
  interrupted turns marked as such, and every answer with its question: an
  answer whose question was heard before it was answered (in progress then,
  or the hearer opened mid-turn) brings the question along again, marked
  "asked earlier", so a pair always reads whole — grouped by thread with its state
  (working, waiting, exited, closed), under a preamble that says Driver
  added it, that it is context and not instructions, and that a
  contradiction with the thread's own work is reason to stop and ask. An
  update never wakes a thread: the extension asks the daemon at
  `before_agent_start` (a message of the operator's starting a run) and at
  `turn_end` when a message of the operator's is queued for delivery (it
  lands right before it). The update is a `custom_message` of type
  `driver-peers` (`pi.PeersType`) — a user-role message in the model's
  context — whose `details.carried` names every peer message it brought
  (thread id, entry id). What a thread has heard is read from its live
  branch, never a stored marker: the extension sends the daemon every entry
  id on the branch (a thread opened from another shares its entries up to
  the fork, so a peer's older messages are not news) and every id its
  `driver-peers` messages carried; a `/tree` jump to an older point
  therefore hears again what the abandoned branch had heard. Closed threads
  speak too. Final answers only is a deliberate trade: a peer's reasoning is
  noise to another agent, and the evidence (errors, file contents, output)
  is one `driver_threads` call away — `history` renders a thread's whole
  branch (`pi.Transcript`: every message, tool call and result, results cut
  to 2000 characters). While a job has other threads, open or closed, each
  pi's system prompt carries a `driver_threads` section with the rules: a
  git worktree of its own before the first change to files or git (threads
  share one checkout; by instruction, since the agent manages its own
  checkout and a worktree in a monorepo is expensive to set up — the
  operator cleans worktrees up), what it changed at the end of every answer
  (that is all the others see), stop and tell the operator on a
  contradiction, and the tool. Driver never removes worktrees.
- Threads name themselves (2026-11-23). "thread N" is reused as threads
  close, so a number does not say which thread holds which work, and a
  thread asked an ambiguous question answers for its own work. The rules
  therefore tell a thread to name its tab once the operator says what the
  thread is for, in a few words, and only then: never on its own guess or
  from the task it was given. `driver_threads` `rename` posts
  `/api/job/threads/rename`, which renames the asking pi's thread as a
  double-click does and answers the name pi's session takes
  (`ThreadPiName`, "<job> · <thread>", as at launch), which the extension
  sets with `pi.setSessionName` so the footer follows the tab. The job's
  own pi names the first tab: `jobs.thread_name`, the name a thread brings
  when it takes the first tab's place (`OwnName`, `OwnPiName`), so the
  card keeps the job's name and a fresh start clears the tab's.
- Tab rings (2026-11-23). The card's ring comes on only when the whole
  card turns blocked, so a thread that stopped while another worked left
  nothing on the board. Each tab now has a ring of its own
  (`threads.attention_at`, `jobs.own_attention_at` for the first tab, v12),
  set and cleared by the tick the way the card's is: each pi's state goes
  through `holdBlocked` and `classify` on its own (`Daemon.ring`, in
  memory per scan key; nothing rings on the first reading after a start),
  on when it stops on its own, off when it works again. The card shows its
  ring while it has its own or any tab does. The overlay acknowledges the
  tab in view (`/api/job/seen` with `thread`), which takes the card's own
  ring too; a tab that is not in view keeps its ring, so the card keeps
  ringing until every tab that rang has been looked at. A thread taking
  the first tab's place brings its ring.
- Review (2026-11-25). REVIEW, before WATCH in the overlay of a working or
  blocked job, asks the pi of the tab in view for an adversarial review of
  its own work by a second agent: `/api/review` pastes `ReviewPrompt` into
  that pane (one bracketed paste, then Enter, so a multi-line prompt
  arrives as one message), and that is all. No column and no stored
  state: the tick moves the card to working while pi is on it and back
  when it reports. The prompt names the `ae-deep-reviewer` subagent
  (the operator's own, in `~/.pi/agent/agents/`, run through pi-subagents)
  and is the operator's go-ahead to delegate; it asks for the findings
  with pi's view on each and no code changes, and for pi to stop rather
  than review in the agent's place when the agent cannot be spawned. The
  agent is not shipped with driver: every pi a job runs loads the
  operator's pi setup. The prompt is fixed. The button is disabled on a
  tab whose pi is gone.
- Watch is a prompt, not a column (2026-11-26). WATCH used to park the job
  in a built-in WATCHING column, always spoke to the job's own pi,
  relaunched a pi that was gone, and sent a prompt edited from the
  column's head. It is REVIEW with another text now: `/api/watch` pastes
  `WatchPrompt` (wait for the merge, follow the deploy, 30 minutes of
  health) into the pane of the tab in view through the same
  `Service.ask` and `tabAction`, on a working or blocked job whose pi is
  up. Gone with the column: `jobs.watching_at` (v13 drops it, and the
  column's saved order), `/api/watch-prompt`, `watch_prompt` and
  `watching` on the board, the prompt editor, the drag rules for the
  column. A `watching` key in a saved column order is dropped as any
  unknown key is.
- Paste context and the ⚙ settings (2026-11-19). ⌘⇧V in a job puts the
  browser's front tab into the pane in view — URL, title and the selected
  text, else the page's text — as one bracketed paste with nothing sent
  (`Tmux.PasteOnly`; REVIEW's paste with the Enter left off), headed
  "Context from my browser — <title>" so pi knows where it came from, cut
  at 20,000 characters. The daemon reads the tab over Apple events
  (`FrontTabContext`, beside `ListTabs`); the text step runs JavaScript in
  the tab, which the browser allows only with its "Allow JavaScript from
  Apple Events" switch on — off, the paste is URL and title. Our own key,
  not a Raycast script: the board must not depend on another tool being
  installed; `/api/job/paste` is public all the same, so one can. Slack has
  no Apple events for what is on screen; its route is a copied message
  link, which pi's Slack tool reads. The ⚙ in the chrome is where macOS's
  questions are asked for rather than met mid-task: "ask now" has the daemon
  read the three protected folders (`ProbeFolders`), so the prompts come up
  in driver's name there and then; buttons open the Full Disk Access and
  Automation panes and show the binary in the Finder (`OpenSystemPane`);
  "test" reads the front tab and says which of the two switches is missing.
- Every column, state columns and lanes alike, is dragged by its head into
  any order; the order is saved in the daemon (kv `board-columns`). `«` in a
  head folds the column to a strip (name, count; still a drop target) and the
  strip expands it; which columns are folded is this browser's to remember
  (localStorage), like the archive drawer.

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
ext/driver.ts         the pi extension (status.json writer, the driver_notes tool); embedded
                      in the binary and written to ~/.driver/ext/driver.ts on every serve start;
                      ext/driver.test.ts runs under `node --test`
macos/                Driver.app: DriverApp.swift, ServerConfig.swift, project.yml,
                      Generated-Info.plist, build-without-xcode.sh, AppIcon-1024.png
Makefile              build · install · install-serve (launchd) · mac · install-mac · dev · test
docs/                 this file, API.md
CHANGELOG.md          every change since the fork from kraken, newest first
```

## Names, paths, knobs

| thing | value |
|---|---|
| home | `~/.driver` (`DRIVER_HOME`) |
| database | `~/.driver/driver.db` (`DRIVER_DB_PATH`), WAL |
| env file | `~/.driver/env` — `KEY=VALUE` lines, `#` comments; loaded by `driver serve` and the Makefile |
| job dir | `~/.driver/jobs/<slug>-<id>/` with `session.jsonl` (and any session file pi moves to — "State derivation"), `status.json`; a thread's files in `t<thread id>/` beneath |
| log | `~/.driver/driver.log` (launchd stdout/stderr) |
| port | `7781` (`-p`, `DRIVER_PORT`); loopback only |
| tmux session | `drv-<slug>-<id>`, session option `@driver-job=<slug>`; a thread's `drv-<slug>-<id>-t<thread id>` |
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
| code-signing identifier | `dev.notttired.driver` (`DRIVER_CODE_ID`): `make install` ad-hoc re-signs the binary under it with a designated requirement of the identifier alone, so a Full Disk Access grant (which Privacy & Security keys on that requirement — Go's own signature, `a.out`, makes it the build's hash) holds across installs. Every pi a job runs is driver's child to TCC (the launchd job is the responsible process), so without the grant a read of the Desktop, Documents or Downloads folder prompts in driver's name |

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
and it has not been opened since (0 otherwise). v8 added `lanes(id, name,
position, created_at)` and `jobs.lane_id` (0 = none). v9 added
`jobs.watching_at` (when WATCH was pressed; 0 = not watching; v13 dropped
it with the WATCHING column). v10 added
`threads(id, job_id, name, pi_session, tmux, started_at, created_at,
closed_at)`, cascading with its job: the job's further pis ("Threads"
above; the job row carries the first). `closed_at` set = its pi was closed;
the row and its session file stay. v12 added `jobs.own_attention_at` and
`threads.attention_at`, the tab rings ("Tab rings" above). A thread's files live in
`<job dir>/t<id>/` (`session.jsonl`, `status.json`), which is the
`DRIVER_JOB_DIR` its pi runs with; `LookupThreadDir` resolves either kind of
dir to its job (and thread).

`board_order("column", position, job_id)` holds the manual order per column
exactly as kraken's does; the board emits each column's jobs in that order,
unlisted jobs first alphabetically. WORKING and BLOCKED read in arrival order:
whenever the tick's state write moves a job into one of them (and when
reactivate does), the daemon appends it to that column's order
(`AppendToColumn`: a no-op when a drag already listed it there, otherwise out of
every other column and onto the end). Cross-column drags do not reorder the
destination, so they count as arrivals too. A lane's order is keyed
`lane:<id>` and reads in arrival order;
`AppendToColumn` leaves a parked job alone unless the column is the one it is
parked in (`ParkedColumn`), so the tick's state writes never pull it out.

`links(id, job_id, kind, url, status, status_at, created_at, pinned_at)` with
`UNIQUE(job_id, url)`; no label column — a link is its URL, in its page's
form (`LinkURL`; "One link per page" above). `kind` ∈ `github_pr | github | slack | linear | url`,
derived from the URL on insert (`github.com/<o>/<r>/pull/<n>` → github_pr;
`github.com` → github; host ends `slack.com` → slack; `linear.app` → linear).
`status` for github_pr ∈ `'' | open | draft | merged | closed`. `pinned_at`:
when you last opened or pinned it, or added it by hand; 0 when not pinned.
`created_at`: when the link arrived — the time of the first message that
carries it ("Link detection"), else when it was added.

`link_mentions(link_id, entry_id, role)`, keyed on `(link_id, entry_id)` and
cascading with its link: one row per session entry that mentions the link,
with that entry's role (`user | assistant | toolResult`); a link added by hand
carries one `user` row with an empty entry id. Keying on the entry id makes
counting idempotent: pi's `/fork` copies entries under their ids, and a
reread counts nothing twice. v6 added both and a `scan-mentions:<job id>`
backfill mark for every job already scanned ("Link detection").

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
the same launch as start with `--session` naming the job's current `pi_session`
(the launch's `session.jsonl`, or the file pi moved to — "State derivation"),
so pi resumes the conversation; `state=working` (the hold below absorbs the
idle pi reports at `session_start` before it picks the conversation back up).

**Retry** (`POST /api/retry`, blocked jobs only): the plan comes from tmux
liveness, status.json and the conversation record (`PlanRetry`, shared with
the board's `retryable`). pi up, at its prompt (`idle`) and its last turn
ended in an error → `tmux send-keys -l continue` + Enter, what the operator
typed by hand after most failures; no state write, the tick moves the card
when pi starts on it. pi exited — a dead pane, or no session at all — → the
dead session is killed and the reactivate launch runs on the job's current
`pi_session` in a fresh session (`new-session` takes the daemon's
environment, so a re-login's `SSH_AUTH_SOCK` is current, which
`respawn-pane` would not give). When the turn pi died in was unfinished (it
failed, or the record is pending: cut off before pi answered), `-- 'continue'`
is its first prompt and the job is written `working` (ring off, last in
WORKING); otherwise pi comes back at its prompt and the job stays blocked.
Anything else — pi answered and waits, a dialog, an interrupt you sent — is
refused: nothing to retry.

**Retry at start**: the jobs whose pi is not live on the daemon's first tick
(ready, working or blocked) are retried by the daemon once each, as soon as
the tick has written them `blocked` (at once for a blocked or ready job, after
the one-tick hold for a working one); the log reads `<slug>: brought back —
its pi was gone when driver started`, or `not brought back: <refusal>`. A job
whose pi comes back first (a RETRY by hand) or that leaves the open states is
dropped from the set. A pi that exits while the daemon runs is left to you.

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
delete only when archived; both refuse a live tmux session. Each tick also
archives every done job untouched for 24 hours (`updated_at` — the done
write, a fork, an unarchive or an edit restarts the day; a live session is
left alone) and logs `<slug>: archived — done for a day`.

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

A job with threads derives each pi this way (`ObserveThread`, the stored
state being the job's) and folds them (`Combine`): working if any, else
blocked if any, else ready; the first pi in the winning state is the card's.

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
`e.aborted`), `session_shutdown` (exited). With `DRIVER_SERVE` set it also
registers `driver_notes` (`action` read | append | replace, `text`), which
calls `/api/job/notes` with `job_dir` (docs/API.md), and the threads' shared
context ("Threads: shared context" above): `before_agent_start` and
`turn_end` post `/api/job/threads/update` with `job_dir` and the ids the
branch holds, inject the answer as a `driver-peers` custom message and, when
the job has other threads, set the `driver_threads` system prompt section;
`driver_threads` (`action` list | history | rename, `thread`, `name`) reads
`/api/job/threads` and `/api/job/threads/history` and posts
`/api/job/threads/rename`. A daemon that does not
answer within 5s costs nothing but the update. Its parameters are plain
JSON Schema and it imports nothing but Node built-ins, so `node --test` loads
the file outside pi. Type import:
`import type { ExtensionAPI } from "@earendil-works/pi-coding-agent";`.

**Following pi's session file.** pi can leave the file it was launched on
without leaving the job: its `/fork` (a new session holding the branch up to
the chosen message, which the operator then re-sends) and `/new` both create
`<timestamp>_<id>.jsonl` beside it — `--session` pins only the first file — and
`/resume` switches to any file. `status.json`'s `session` names the file pi is
writing. Each tick, when that is another regular file in the job dir, the job
follows: `pi_session` moves (the record, the link scanner, reactivate and fork
all read it), the scanner forgets the job's offset and pending PR calls and
reads the new file from its start (attaching is idempotent, so the branch the
new file copied costs nothing), and the daemon logs `<slug>: pi moved to
<file>`. A file outside the job dir is not followed — the job dir is
`pi_session`'s directory — and is logged once; a file pi has not written yet (a
`/new` before its first message) is picked up on a later tick. Until 2026-10-29
a `/fork` inside pi froze the record, the scanner and reactivate on the
abandoned `session.jsonl`.

## Link detection and PR status

Each tick, for every job with a `pi_session` file, the daemon reads the bytes
appended since the offset saved in `kv` (`scan:<job id>`) — from the start again
once pi moved to another file ("State derivation") — and parses complete JSONL
lines. Two kinds of `message` entry yield links, both attached idempotently
on `(job_id, url)`, the URL in its page's form and the kind derived from it:

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

Every link the scanner sees is also a mention of it (`link_mentions`, the
entry's id and role), the first sighting included. A job that was scanned
before v6 carries a `scan-mentions:<job id>` mark: its session is read once
more from that mark up to the scan offset, counting mentions of the links it
still has and attaching nothing — the links you removed stay removed — then
the mark goes.

A link arrives with the message that first carries it (the entry's
`timestamp`), however late the scanner reads that message: a session read
again from its start — after pi moved to another file, as a fork's pi does
when it starts, or after a generation bump — leaves each link at the time it
was said. Until 2026-11-06 a link arrived when the scanner first read it,
so a fork's links that had been removed from the original came back as the
newest; v7 sets the `scan-mentions` mark on every scanned job once more, and
that reading moves the links already stored back to the time they were said.

`kv.scanner-generation` names what the scanner looks for (`2`); a daemon that
finds an older value, or none, forgets every `scan:*` offset once and reads each
session again from the start, so widening the scanner never misses what is
already on disk (v3's migration did the same by hand, before the key existed).

Links of kind github_pr whose
`status_at` is older than 60s are refreshed, at most 3 per tick, with
`gh pr view <url> --json state,isDraft` → `open|draft|merged|closed`; a failing
`gh` leaves the status untouched.

## Conversation record

`GET /api/job/conversation` derives the overlay's record from the job's
current session file (`pi_session`, which the tick keeps on the file pi is
writing — "State derivation") on request, cached on the file's path, size and
mtime, so the overlay's 3-second poll re-reads a session only when pi appended
to it or moved (`rev` is the size-and-mtime stamp; the SPA keeps its rendered
list while it does not change). The entries read are those
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
panel. Each row is a card clamped to six lines (`--convo-clamp`) while its
body overflows that — measured after layout and on resize, so a short card is
just a card — with a fade and a "more" mark; a click anywhere on it but a link
or a selection opens it whole (scrolled to its top) and closes it again. The
record has the sidebar's top to itself: CONVERSATION and NOTES are two tabs
in its head over one space (the conversation by default), both panels staying
mounted — the notes keep unsaved typing and the record its scroll — with the
one not shown hidden; the links keep their place beneath.

## Terminal

`GET /api/term?slug=<slug>` upgrades to a websocket. The server starts
`tmux attach-session -t <sess>` on a pty (`TERM=xterm-256color`,
`COLORTERM=truecolor`) sized from the first resize message, then pumps bytes both
ways. Wire: **binary frames** carry terminal bytes in both directions; **text
frames** are JSON control messages, today only `{"type":"resize","cols":N,"rows":N}`
from the client. Closing the socket kills the attach process (the tmux session
lives on). Because tmux's `window-size` is `latest` on this machine, the newest
client's size wins; the bridge does not fight other attached clients.

Keys go through xterm untouched except ⌘← / ⌘→ and ⌘⌫: xterm has no sequence for a
⌘-arrow and drops it (WebKit then moves the caret of xterm's hidden textarea),
so the pane sends what the Home / End keys send instead (`ESC [ H` / `ESC [ F`,
or `ESC O H` / `ESC O F` in application cursor keys mode — `lib/paneKeys.ts`).
tmux knows both forms and pi binds `home` / `end` to its editor's line start /
line end, so the macOS line jumps work in pi. ⌘⌫, which xterm sends as a plain
backspace, deletes the whole line: End, then Ctrl+U (pi's delete to line
start) — on an empty line Ctrl+U joins it to the line above, so the line goes
either way. Other modifiers (⌥-arrows for words, ⇧) are left to xterm.

URLs in the pane — plain text found by xterm's web-links addon, and OSC 8
hyperlinks — open in the browser on ⌘-click through `window.open(url, "_blank")`.
Both addons' default activation opens an *empty* window first and then sets its
location; in the shell that arrives as `about:blank`, which LaunchServices
answers with "The application can't be opened. -50" while the link never opens.
The shell also ignores anything but an http(s) URL in `createWebViewWith`.

## Board rules (kraken's, trimmed)

Bucketing: archived → drawer; else `lane` names the column when set, else
`state` does (`''`/`ready`/unknown → draft). Same-column drop → `POST
/api/reorder {column, slugs}` (full column, optimistic). WORKING and BLOCKED are
in arrival order (above); a card that turned blocked on its own wears a ring
until it is opened, and so does a card while one of its tabs rings. Cross-column moves:

| from → to | action |
|---|---|
| draft → working | not started: start (`/api/queue`); the card shows a spinner while the request runs and a 409 returns it with the reason in a toast. pi already open (`ready`): just open the card — brief it in the pane and pi moves it |
| working → blocked | `/api/stop` |
| blocked → done | `/api/done` |
| blocked → working | open the card (go talk to it in the pane); no write |
| done → working, done → blocked | `/api/reactivate` |
| done → archive drawer | `/api/archive` |
| anything → a lane | `/api/job/lane {lane: id}`; no state change |
| a lane → the column the card's state names | `/api/job/lane {lane: 0}`; only that column lights up |
| anything else | no-op |

Card click → the job overlay: head (name + rename, with `as <slug>` beside a name that differs from the slug, dir, colour swatch, actions:
retry / start / stop / done / cancel / reactivate / archive by state), body = the pane
for every opened job (pi at its prompt, working, blocked, done) or the launch
form for a not-started one (dir with directory autocomplete, model datalist,
START), sidebar = the conversation record with the notes (autosaved) a tab
away, links (list, add, remove, PR status) beneath.
The NEW JOB dialog takes a name, dir and model and starts the job itself
(`/api/job/create`, then `/api/queue`), so the overlay it opens is already on
the pane. Escape closes; ⌘K opens the palette from anywhere, over whatever
is open (ctrl-K too while nothing is, since in the pane it is pi's); Escape
closes the palette first (its escape layer sits above the job's), a pick
closes it and lands on the pane. ⌘N opens NEW JOB the same way. ⌘-click on a card skips the overlay
and posts `/api/job/open` (the directory in the editor); the overlay's dir label
answers the same gesture.
