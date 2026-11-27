# Changelog

Everything that changed since driver was forked from
[kraken](https://github.com/car1zhang/kraken) on 2026-10-27, newest first.
Each entry names its commit; the schema version it moved `driver.db` to is
given where there was one. [docs/DESIGN.md](docs/DESIGN.md) has the reasoning
behind each, [docs/API.md](docs/API.md) the wire contract.

## 2026-11-27

- **👁 WATCH gets the work merged by itself** (`a19517c`). It used to
  wait and report. Now pi fixes failing checks, review comments and
  conflicts, merges once the work is approved and green, merges again
  when a merge fails, and does not stop before the merge. It asks you
  only about a critical design decision, or for something only you can
  clear. After the merge it follows the deploy and watches for 30 minutes
  as before.

## 2026-11-26

- **WATCH is a prompt, not a column** (`0fa25e0`, schema v13). 👁 WATCH
  now works as 🔍 REVIEW does: it sends the watch prompt to the pi of the
  tab you are on, on a working or blocked job, and nothing else. The
  WATCHING column, its prompt editor and the relaunch of a done job's pi
  are gone; the prompt is the built-in one.

## 2026-11-25

- **🔍 REVIEW asks for an adversarial review by a second agent** (`5ab58de`).
  Before WATCH on a working or blocked job. The pi of the tab you are on
  spawns the `ae-deep-reviewer` subagent on the pull request or the diff
  and reports its findings with its own view on each, changing no code
  until you say. No column: the card reads working meanwhile.

## 2026-11-23

- **Each tab rings when its pi stops and you haven't looked**
  (`f704b76`, schema v12). A thread that stopped while another worked left
  nothing on the board. Now its tab wears the card's ring, and so does the
  card, even in WORKING. Looking at a tab takes its ring off; the card's
  goes once you have seen every tab that rang.
- **The first tab names itself too** (`251d946`). Told what it is for, the
  job's own pi was refused a name. Now its tab and pi's footer take it,
  and the card keeps the job's name.
- **A thread names its own tab once you tell it what it is for**
  (`dedb56e`). Say "you're the thread for topic E" and its tab and pi's
  footer become a few words for that. It never names itself on a guess or
  from the task you give it, and the first tab keeps the job's name.
  "thread N" numbers come back as threads close, so a number alone did not
  say which thread held which work.

## 2026-11-19

- **⌘⇧V pastes your front Chrome tab into the pane; ⚙ settings** (`6e889b1`).
  URL, title and the selected text (else the page's text) land in the pi
  editor of the tab you are on, nothing sent. `POST /api/job/paste` is the
  door for anything else. The new ⚙ in the top bar asks macOS's questions
  up front: "ask now" reads the protected folders from the daemon so the
  prompts come there and then, buttons open Full Disk Access and Automation
  and show the binary, "test" tries the browser read and says what is
  missing.
- **The first tab closes too, and a job opens on the tab you left it on**
  (`b1985ff`, schema v11). Closing the first tab hands its place to the next
  thread — that pi becomes the job's own, under its own name, and the old
  one stays as a closed thread so what it said is still shared. The last
  tab always stays: ⌘W on it closes the overlay. Each job remembers which
  tab it was left on.
- **One Full Disk Access grant, kept across installs** (`32a22b1`). Every pi a
  job runs is driver's child to macOS, so a read of the Desktop, Documents
  or Downloads folder asked in driver's name, and Go's own signature made
  any grant die with the next build. `make install` now re-signs the binary
  under `dev.notttired.driver` with the identifier as its requirement, so
  granting it Full Disk Access once (System Settings › Privacy & Security)
  ends the prompts for good. The README says how.

## 2026-11-17

- **A thread opened mid-turn starts from the finished work** (`ce4abab`). The
  copy ends at the source's last final answer; the prompt under way and its
  tool calls stay behind, and the thread hears that turn's answer later as
  context, with its question.
- **⌘T opens a thread with the default name** (`916d936`): from the current
  tab, named "thread N", no box to fill; rename it later if you like.
- **Closing a thread asks nothing** (`cf96c3a`). ✕ and ⌘W close a thread at
  once; nothing is lost, since the thread's conversation stays with the job.
- **Driver.app shows the board's confirms and alerts** (`4ba86a3`). The web
  view had no handler for `window.confirm`, so in the app it answered false
  at once and showed nothing: closing a thread, CANCEL and DELETE FOREVER
  silently did nothing there. Now a sheet on the window asks.
- **⌘W closes what is on top, never Driver's window** (`9bb1bd6`). The
  palette, a dialog, the thread tab in view, else the job overlay. The app
  drops the File ▸ Close menu item so the key reaches the page; the traffic
  light still closes the window.
- **An answer brings its question along** (`e7ad4bb`). In the shared
  context, an answer whose question was heard earlier (in progress at the
  time, or already in a thread opened mid-turn) repeats the question, marked
  "asked earlier", so every pair reads whole.
- **A card shows five links and counts the rest** (`5b6427e`). The same first
  five as the overlay's links panel — pinned first, then newest — and a
  "+N" chip.
- **⌘⇧] and ⌘⇧[ step through a job's tabs** (`980d2ac`), round the end, from
  the pane or the sidebar.
- **⌘K is a palette over every pane, with its status** (`0969a25`). A popup,
  Raycast-fashion: every job and each of its threads, archived last, each
  with its column and status, narrowing as you type to what the words are
  found in — names and slugs first, then thread names, directories, columns
  and lanes, status words ("needs you", "working", "done", "archived"),
  models, links, the tool pi is in. Rows say where a match outside the title
  came from. ↑↓ move, ⏎ opens the pane (a thread's row opens the job on that
  tab), ⌘⏎ opens the directory in the editor. The board is no longer
  filtered; the bar keeps a button into the palette and the count.
- **The record has the sidebar, in cards that open on a click** (`5f75a1d`).
  CONVERSATION and NOTES are two tabs over one space (the conversation by
  default; both stay mounted, so unsaved typing survives the switch). Each
  row is a card six lines tall while it overflows that, with a fade and a
  "more" mark; a click opens it whole, scrolled to its top.
- **The threads of a job hear each other** (`51e1377`). Every message you
  send a thread arrives with what the other threads said since it last
  heard: your messages to them and their final answers (no narration, tool
  calls or thinking), the last one marked in progress while a peer is on it,
  as a `driver-peers` message that says Driver added it and that it is
  context, not instructions. Nothing wakes a thread: the extension asks the
  daemon when a message of yours starts a run and at a turn's end when one
  is queued. What a thread has heard is read from its live branch (every
  entry id on it and every id earlier updates carried), so a `/tree` jump
  hears again what the abandoned branch had. Closed threads speak too.
  Every pi on a job with threads carries the rules — a git worktree of its
  own before its first change, what it changed at the end of every answer,
  stop and tell you on a contradiction — and the `driver_threads` tool
  (`list`, `history`), which reads any thread's whole conversation, tool
  calls and all.
- **More than one pi on a job, each in a tab above the pane** (`61e640b`,
  schema v10). `+` opens a thread: another pi with the same tools in the
  same directory, resumed on a copy of the current tab's conversation, in a
  tmux session of its own (`drv-<slug>-<id>-t<n>`), under a name you type.
  Each tab has its own pane, record, STOP and RETRY; ✕ closes a thread's pi
  and keeps its conversation; double-click renames. The card is working
  while any thread works and needs you when every thread has stopped; a
  thread that stops while another works sends its own notification. DONE
  closes every thread, CANCEL drops them, a rename moves their sessions,
  and driver brings them back at start.

## 2026-11-15

- **Driver brings back the jobs a shutdown left exited** (`7effbbb`). At
  login the daemon retries, once each, every job whose pi was not live on
  its first tick: a turn cut off mid-way resumes with `continue`, a job that
  waited on you comes back at its prompt. A pi that exits while driver runs
  is left to you.

## 2026-11-13

- **pi reads, adds to and rewrites your notes when you ask** (`0a9cb68`).
  The extension gives pi a `driver_notes` tool (read, append, replace) whose
  guidelines tell it to touch the notes only when asked. The notes panel
  polls, so what pi writes shows within seconds; while you have unsaved
  typing, yours wins. Until then notes never entered pi's context.

## 2026-11-10

- **A card stays in WATCHING when pi stops** (`1a24488`). It had been marked
  done; now it reads blocked there, ringed and notified, until DONE or a
  drag.
- **Columns of your own, WATCHING, and columns that drag, fold and reorder**
  (`7af243a`, schema v8 lanes, v9 watching). `+` at the end of the board
  adds a lane; a card dragged into one keeps its state and stays put while
  pi works; dragged out it goes only to its state's column. WATCHING is a
  built-in lane for the last mile: 👁 WATCH pastes the watch prompt into pi
  (or relaunches pi with it) and parks the card there; ✎ edits the prompt.
  Every column drags by its head into any order (saved in the daemon), and
  `«` folds one to a strip.

## 2026-11-06

- **A link arrives with the message that first carries it** (`b583f68`,
  schema v7). "Newest first" had read the time the scanner saw a link, which
  a fork's reread reset; now it is the time it was first said.
- **One link per page** (`fbbe4ea`). No `#fragment`, no trailing slash, and
  every page of a GitHub or Delta pull request (files, changes, a commit)
  is the pull request; existing duplicates fold into one at start.
- **Pinned first, the last pinned on top; then the newest** (`d0e916d`). A
  link is pinned when you open it, press its 📌 or add it by hand; links you
  mention are no longer pinned by themselves.

## 2026-11-03

- **⌘K takes the search from anywhere; ⌘N opens NEW JOB** (`b5b9c64`), from
  the board or an open job (ctrl-K / ctrl-N stay pi's while a pane is
  open). The search covered the board, not the archive. Superseded by the
  palette on 2026-11-17.
- **Done jobs left alone for a day are archived** (`edea455`). DONE had grown
  to 34 cards; a live session is left alone.

## 2026-11-02

- **Five link rows at a time, a 📌 on every link** (`8d98c7c`). The panel
  scrolls for the rest; 📌 pins without opening.
- **⌘⌫ deletes the whole line in pi** (`5d21c7d`): End, then Ctrl+U.
- **Retry the jobs a network drop or a logout left blocked** (`91019ee`). A
  failed turn reads `✗ <error>`, an exited pi `✕ exited`; `↻ retry N` in the
  BLOCKED head and RETRY in the overlay type `continue` into a live pi or
  start it again on its conversation, continuing when it died mid-turn.
- **Links most-mentioned first; yours and the ones you open pinned on top**
  (`019922a`, schema v6). Mentions are counted per session entry, so rereads
  and pi's `/fork` count nothing twice. Reordered on 2026-11-06.

## 2026-10-29

- **The daemon follows the session file pi is writing** (`67b3cc7`). A
  `/fork` or `/new` inside pi starts a new file; the record, the link
  scanner and reactivate now follow it instead of freezing at the fork.

## 2026-10-28

- **⌘← / ⌘→ jump to the start / end of the line in pi** (`412159e`): the
  pane sends what Home / End send.
- **Arrival order in WORKING and BLOCKED; a ring on unopened blocked cards**
  (`4078095`, schema v5). A card that turned blocked on its own wears a ring
  until you open it; an interrupt you sent does not ring.
- **Fork a job** (`ca9ddbf`): a copy with its conversation, notes, links,
  directory and model under a new name, beside the original in its column;
  an open job's fork gets its own pi on the copied conversation.
- **pi's answers render as markdown** (`f9f1ab3`): tables, task lists,
  strikethrough, bare URLs; raw HTML shows as text.
- **A conversation record beside the notes** (`3f641f1`): what you typed and
  pi's final answer to each message, read from the session file, without
  the work in between.
- **README names the project's origin** (`81fa649`).
- Tried and reverted the same day: running pi in print mode so the pane
  showed only final answers (`2ca8fbf`, `4526cc3`).

## 2026-10-27

- **Call a job by the name you typed** (`aa8f09f`, schema v4). The card, the
  overlay head, pi's `--name` and the notifications go by it; the slug
  derived from it stays the identifier.
- **Scan pi's replies too; open pane and panel URLs without the -50 alert**
  (`d374011`). Every URL pi prints lands in the job's links; in Driver.app,
  pane links opened `about:blank` through LaunchServices — now the URL goes
  in the call.
- **Attach the URLs you send pi; drop link labels** (`a1821df`, schema v3). A
  chip reads as the PR number or the host.
- **driver: a macOS job board for pi coding-agent sessions** (`2652ba0`). The
  fork-by-lift of kraken: one Go daemon with a SQLite database and a tick
  loop, a React board in a WKWebView shell, every job a tmux session running
  pi in a directory the job names, the real pane shown through xterm.js. No
  worktree or branch management (the agent manages its own checkout), no
  spec step (NEW JOB opens pi at once; the first message is typed in the
  pane), state read from a pi extension that writes `status.json`. Kept
  from kraken: the four columns, drag rules and look, rename, colours,
  search, the archive drawer, notifications, the launchd install; the app
  builds with the Command Line Tools alone.
