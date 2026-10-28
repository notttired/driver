// Wire types — a one-to-one mirror of docs/API.md. Nothing here is derived or
// decorated; the UI's own shapes live beside the code that needs them.

export type JobState = "draft" | "working" | "blocked" | "done";

/** The pi-derived session state (DESIGN.md "state derivation"). Open on the
 *  wire — the daemon may grow a value before this list does, so readers treat
 *  anything unlisted as "working"/"needs you" by column rather than failing.
 *  "ready" is pi open at its prompt with nothing sent yet: a draft that has
 *  its pane. A not-started draft carries no `pi` at all. */
export type PiState =
  | "starting"
  | "ready"
  | "working"
  | "idle"
  | "dialog"
  | "aborted"
  | "exited"
  | "silent";

export type LinkKind = "github_pr" | "github" | "slack" | "linear" | "url";
export type PRStatus = "open" | "draft" | "merged" | "closed";

export interface WebLink {
  id: number;
  kind: LinkKind;
  url: string;
  status?: PRStatus;
  /** unix seconds */
  status_at?: number;
}

export interface WebPi {
  state: PiState | string;
  tool?: string;
  reason?: string;
  /** unix seconds */
  ts: number;
}

export interface WebJob {
  slug: string;
  display_slug?: string;
  /** the name you typed; cards and the overlay show it instead of the slug when set */
  name?: string;
  dir: string;
  model?: string;
  color?: string;
  state: JobState;
  outcome?: string;
  live: boolean;
  pi?: WebPi;
  has_notes: boolean;
  links?: WebLink[];
  /** unix seconds */
  started_at?: number;
  /** unix seconds */
  finished_at?: number;
  /** unix seconds */
  mtime: number;
  archived?: boolean;
  reactivatable?: boolean;
}

export interface WebDaemon {
  pid: number;
  /** unix seconds */
  started_at: number;
  /** unix seconds */
  last_tick: number;
  live: number;
  last_error?: string;
}

export interface WebBoard {
  daemon: WebDaemon;
  daemon_ok: boolean;
  jobs: WebJob[];
  contract: number;
  operator: string;
  default_dir: string;
}

/** GET /api/job/conversation: the overlay's record of a session — what you
 *  typed and pi's final answer to each message. */
export type MessageRole = "user" | "assistant";

export interface WebMessage {
  role: MessageRole;
  /** text blocks newline-joined; "" for an interrupted turn that said nothing */
  text: string;
  /** unix seconds; absent when the entry carried no time */
  ts?: number;
  /** assistant only: how the turn ended when not with an answer of pi's own
   *  choosing. Open on the wire — an unlisted value is shown as it reads. */
  end?: "length" | "aborted" | "error" | string;
  /** end === "error": what failed */
  error?: string;
}

export interface WebConversation {
  /** in conversation order, never null */
  messages: WebMessage[];
  /** the last turn still awaits pi's final answer */
  pending: boolean;
  /** changes when the record does; equal revs are equal records */
  rev: string;
}

export interface JobForm {
  slug: string;
  name: string;
  dir: string;
  model: string;
  color: string;
  notes: string;
  state: JobState;
  /** dir/model editable: a not-started draft only (and not archived) */
  editable: boolean;
  archived: boolean;
  /** unix seconds; echoed back on save for the optimistic lock */
  mtime: number;
  links: WebLink[];
}

/** POST /api/job/create: dir "" means the daemon's default dir, model "" pi's default */
export interface CreateJobBody {
  slug: string;
  /** what the board calls the job; "" shows the slug */
  name?: string;
  dir?: string;
  model?: string;
}

export interface HealthResponse {
  ok: boolean;
  pid: number;
  version: string;
  contract: number;
}

export interface OkResponse {
  ok: true;
}

export interface RenameResult extends OkResponse {
  slug: string;
}

export interface SaveFormResult extends OkResponse {
  mtime: number;
}

export interface AddLinkResult extends OkResponse {
  link: WebLink;
}

export interface DirsResponse {
  dirs: string[];
}

export interface ModelEntry {
  provider: string;
  id: string;
}

export interface ModelsResponse {
  models: ModelEntry[];
  default: string;
}

/** one open browser tab, from GET /api/tabs */
export interface BrowserTab {
  url: string;
  title: string;
  /** 1 = the front window */
  window: number;
  /** 1-based position within its window */
  index: number;
  /** the window's selected tab */
  active: boolean;
}

export interface TabsResponse {
  browser: string;
  running: boolean;
  tabs: BrowserTab[];
}
