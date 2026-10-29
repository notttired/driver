# driver web API — wire contract

Every endpoint is served by `driver serve` on `http://127.0.0.1:7781` (loopback
only). JSON in, JSON out. Any non-2xx answer is `{"error": "<sentence>"}`; action
endpoints answer `{"ok": true}` on success. Validation refusals are **409**,
unknown slug **404**, bad input **400**, wrong method **405**. Times are **unix
seconds** unless a key says `_ms`. Keys marked `?` are omitted when empty/absent
(Go `omitempty`); everything else is always present.

The SPA (`web/_ui`) and the Go handlers (`internal/api`) must agree with this
file; change all three together.

## Health

`GET /api/health` → `{"ok": true, "pid": N, "version": "<string>", "contract": 1}`

## Board

`GET /api/board` → `webBoard`

```ts
type WebBoard = {
  daemon: { pid: number; started_at: number; last_tick: number; live: number; last_error?: string };
  daemon_ok: boolean;          // last_tick within 10s
  jobs: WebJob[];              // never null; archived jobs ride in the same array (archived: true)
  contract: number;            // 1
  operator: string;            // DRIVER_OPERATOR / $USER
  default_dir: string;         // DRIVER_DEFAULT_DIR / $HOME — seed for a new job's dir
};

type WebJob = {
  slug: string;                // active jobs: the slug. archived: the archive name "<slug>-<id>"
  display_slug?: string;       // archived only: the original slug
  name?: string;               // the name you typed; the UI shows it instead of the slug when set
  dir: string;                 // working directory ('' on a fresh draft)
  model?: string;
  color?: string;              // '' | red blue green yellow purple orange pink cyan
  state: 'draft' | 'working' | 'blocked' | 'done';   // the DISPLAY state (stored '' → 'draft')
  outcome?: string;            // done only: 'accepted'
  live: boolean;               // tmux session exists and its pane is not dead
  pi?: { state: string; tool?: string; reason?: string; ts: number };
                               // state: starting|ready|working|idle|dialog|aborted|exited|silent
                               // ready = pi at its prompt, nothing sent yet: a draft whose pane
                               // exists. A not-started draft has no pi at all.
                               // reason: why the board calls it blocked (same vocabulary)
  has_notes: boolean;
  links?: WebLink[];           // in created order
  started_at?: number;
  finished_at?: number;
  mtime: number;               // updated_at, for the age label
  archived?: boolean;
  reactivatable?: boolean;     // done and not archived
  attention?: boolean;         // turned blocked on its own and not opened since (the card's ring)
};

type WebLink = {
  id: number;
  kind: 'github_pr' | 'github' | 'slack' | 'linear' | 'url';
  url: string;                 // no label: the UI shows "#N" for a PR, else the host
  status?: 'open' | 'draft' | 'merged' | 'closed';   // github_pr only, '' until first poll
  status_at?: number;
};
```

Within one display state the array order IS the column order (see `/api/reorder`).
WORKING and BLOCKED read in arrival order: a job the daemon moves there (pi
starting, going quiet, picking the conversation back up, exiting), a cross-column
drag and a reactivation each land at the end; a same-column drag sets the order
explicitly and is kept. DRAFT and DONE list manually ordered jobs after the
unlisted ones (alphabetical).

`attention` comes on when the daemon writes `blocked` for a job that was not
blocked and the reason is not `aborted` (Escape in the pane, STOP, the working →
blocked drag — interrupts you sent yourself); it goes off when the job leaves
`blocked` or when you open it (`/api/job/seen`).

## Actions (request `{"slug": string}` unless noted)

| route | effect | refusals (409) |
|---|---|---|
| `POST /api/queue` | **start**: open the tmux session + pi at its prompt (stored `ready`, still `draft` on the board until pi runs on the first message); answers after the session is up | already open · not a draft · dir missing/not a directory · pi or tmux not found · tmux failure (text) |
| `POST /api/stop` | send Escape to the pane; no state write | not working/blocked · no live session |
| `POST /api/done` | kill session, state done (allowed for an open, unbriefed draft) | not started · done |
| `POST /api/kill` | kill session, back to not started ("cancel") | not started |
| `POST /api/reactivate` | relaunch pi on the same session file, state working, last in WORKING | not done · archived · dir gone |
| `POST /api/archive` | archive (slug freed) | not done · live session |
| `POST /api/unarchive` | restore; refuses if an active job now holds the slug | — |
| `POST /api/delete` | delete forever; removes the job dir | not archived |
| `POST /api/reorder` | body `{"column": "draft\|working\|blocked\|done", "slugs": string[]}` — the column's full intended order; slugs whose display state no longer matches are dropped silently | unknown column |
| `POST /api/job/color` | `{"slug", "color"}`; `""` clears; allowed in every state | unknown colour (400) |
| `POST /api/job/rename` | `{"slug", "to", "name"?}` → `{"ok": true, "slug": "<to>"}`: `to` is the new slug (rules below), `name` the new display name (whitespace-collapsed, ≤ 200 characters; `""` or absent = the job goes by its slug). Either may stay as it is. Allowed in every state; when the slug changes on a live job its tmux session is renamed too | taken · invalid · name too long (400) |
| `POST /api/job/fork` | `{"slug", "to", "name"?}` → the fork's `JobForm`: a copy of the job — dir, model, colour, notes, links, and its conversation (the session file copied under a fresh session id, `parentSession` naming the original's) — as slug `to`, named `name` (same rules as rename), placed in the original's column right after it. A not-started draft's fork is a draft; a done job's fork is done (reactivate it to carry on from where the original stood); an open job's fork (ready, working, blocked) gets its own tmux session and pi resumed on the copied conversation, keeps the original's column until its own pi reports, and answers after that session is up (the start timeout applies). Archived jobs cannot be forked — unarchive first | archived · slug taken · invalid slug · the original's own slug · name too long (400) · for an open job: dir gone · pi or tmux not found · tmux failure (text) |
| `POST /api/job/notes` | `{"slug", "text"}`; full replace, allowed in every state | > 64000 runes |
| `POST /api/job/links` | `{"slug", "op": "add", "url"}` → `{"ok": true, "link": WebLink}`; `{"slug", "op": "remove", "id"}` → `{"ok": true}`. The daemon adds links on its own too: every URL in your messages and in pi's replies, and the PR a `gh pr create` prints (docs/DESIGN.md "Link detection") | bad URL (must be http/https) · duplicate url |
| `POST /api/job/seen` | you opened the job: clears `attention` (the overlay posts it on open; no rev bump, nothing else changes) | — |
| `POST /api/job/open` | open the job's directory in the editor: `DRIVER_EDITOR` with the dir appended, else `code <dir>`, else `open -b com.microsoft.VSCode <dir>` (⌘-click on a card); allowed in every state, no state write | dir missing/not a directory · the editor command failed (its first output line) · it did not return within 15s |
| `POST /api/pr-attach` | `{"slug", "url"}` — idempotent add of a `github_pr` link (used by the daemon's own scanner; kept public for scripts) → `{"ok": true, "attached": boolean}` | not a GitHub PR URL |

Slug rules: `^[a-z0-9][a-z0-9-]{0,63}$`, unique among non-archived jobs.

`POST /api/queue` (and `/api/job/fork` of an open job) can take seconds (tmux +
pi startup). The client uses a longer timeout for them; a client-side timeout
is not a failed start.

## Job form

`GET /api/job/form?slug=<slug>` → `JobForm`

```ts
type JobForm = {
  slug: string; name: string; dir: string; model: string; color: string; notes: string;
  state: 'draft' | 'working' | 'blocked' | 'done';
  editable: boolean;           // dir/model editable: a not-started draft only (and not archived)
  archived: boolean;
  mtime: number;               // seconds; echo back on save for the optimistic lock
  links: WebLink[];
};
```

`POST /api/job/saveform` `{"slug", "dir", "model", "mtime"}` → `{"ok": true, "mtime": number}`.
Full replace of the two fields. `mtime` 0 skips the lock; otherwise a mismatch is
409 `"edited elsewhere — reload"`. Refuses (409) when not editable. Colour and notes
have their own routes and are not part of the form save.

`POST /api/job/create` `{"slug", "name"?, "dir"?, "model"?}` → `JobForm` (a
not-started draft; `name` is what the board calls the job — whitespace-collapsed,
≤ 200 characters, `""` = the slug; empty `dir` → `default_dir`). Slug taken, or
`dir` not an existing directory → 409; a name too long → 400. The board's NEW JOB dialog follows it with `/api/queue` at once,
so a new job opens on its pane; there is no spec anywhere in the API.

`GET /api/dirs?q=<path prefix>` → `{"dirs": string[]}` — directory autocomplete:
expands a leading `~`, lists subdirectories of the typed path's parent whose
names start with the last path component, absolute, sorted, max 50, hidden dirs
only when the typed component starts with `.`. Empty `q` lists `default_dir`'s
subdirectories.

`GET /api/models` → `{"models": [{"provider": string, "id": string}], "default": string}`
— parsed from `pi --list-models` (cached 10 minutes; `?refresh=1` re-runs it).
`default` is the `defaultModel` in `~/.pi/agent/settings.json` when readable.

## Conversation

`GET /api/job/conversation?slug=<slug>` → `WebConversation` — the overlay's
record of the session: your messages and pi's final answer to each
(docs/DESIGN.md "Conversation record"). Derived on each request from the job's
current session file — the one pi is writing; the daemon follows pi to the new
file a `/fork` or `/new` inside it starts (docs/DESIGN.md "State derivation") —
cached on the file's path, size and mtime; a job with no session yet answers an
empty record. 404 for an unknown slug.

```ts
type WebConversation = {
  messages: WebMessage[];      // never null, in conversation order
  pending: boolean;            // the last turn still awaits pi's final answer
  rev: string;                 // changes when the record does; equal revs are equal records
};

type WebMessage = {
  role: 'user' | 'assistant';
  text: string;                // text blocks newline-joined; '' for an interrupted turn that said nothing
  ts?: number;                 // the entry's time
  end?: 'length' | 'aborted' | 'error' | string;   // assistant only; absent when pi stopped by itself
  error?: string;              // end === 'error': pi's errorMessage
};
```

## Browser tabs

`GET /api/tabs` → `{"browser": "Google Chrome", "running": true, "tabs": [{"url", "title", "window", "index", "active"}]}`

The open http(s) tabs of the operator's browser (`DRIVER_BROWSER`, default `Google Chrome`; any Chromium-family browser with Chrome's AppleScript dictionary works), windows front to back, `window`/`index` 1-based, `active` marking each window's selected tab. A browser with no process answers `running: false` and an empty list rather than being launched. 503 with a sentence when macOS has not granted driver Automation access to the browser (System Settings › Privacy & Security › Automation) or the browser does not answer within 20s. The links panel's `⊞ tabs` picker is the only consumer; attaching is the ordinary `/api/job/links` add of the tab's URL.

## Terminal

`GET /api/term?slug=<slug>` — websocket upgrade. 404 `{"error":"no such job"}` /
409 `{"error":"no live session"}` as plain HTTP before upgrading when there is
nothing to attach to. After upgrade:

- **client → server**: binary frames = raw input bytes for the pty; text frames =
  JSON `{"type":"resize","cols":N,"rows":N}` (send one immediately after open).
- **server → client**: binary frames = pty output bytes; text frames = JSON
  `{"type":"exit","code":N}` right before the server closes (the attach process
  ended: session killed or detached).

The server runs `tmux attach-session -t <session>` on a pty per socket;
closing the socket ends the attach. Many sockets per job are fine.

## Static

`/` and `/assets/*` serve the embedded SPA (`web/_ui/dist`); `/assets/*` is
immutable-cached, everything else `no-cache`. Hash routing (`#/`) means no SPA
fallback is needed.
