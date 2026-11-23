// The daemon's HTTP surface (docs/API.md), typed end to end. Every non-2xx
// answer carries `{"error": string}`; that sentence IS the user-facing message,
// so ApiError keeps it verbatim and the status beside it.

import type {
  AddLaneResult,
  AddLinkResult,
  BrowserContext,
  BrowserProbeResult,
  CreateJobBody,
  FoldersProbeResult,
  DirsResponse,
  ForkJobBody,
  HealthResponse,
  JobForm,
  ModelsResponse,
  OkResponse,
  OpenThreadResult,
  PinLinkResult,
  RenameResult,
  SaveFormResult,
  SettingsInfo,
  WebBoard,
  WebConversation,
  AddPlanResult,
  PlanOp,
  PlanOpsResult,
  WebPlan,
  WebPlanSummary,
  TabsResponse,
} from "../lib/types.ts";
import type { Column } from "../lib/columns.ts";

const BASE = "";

export class ApiError extends Error {
  readonly status: number;
  constructor(status: number, message: string) {
    super(message);
    this.name = "ApiError";
    this.status = status;
  }
}

async function parseError(res: Response): Promise<ApiError> {
  let msg = `${res.status} ${res.statusText}`.trim();
  try {
    const body = (await res.json()) as { error?: unknown };
    if (body && typeof body.error === "string" && body.error) msg = body.error;
  } catch {
    // not JSON — keep the status line
  }
  return new ApiError(res.status, msg);
}

export async function getJSON<T>(path: string, signal?: AbortSignal): Promise<T> {
  const res = await fetch(BASE + path, { signal, headers: { Accept: "application/json" } });
  if (!res.ok) throw await parseError(res);
  return (await res.json()) as T;
}

export interface PostOptions {
  signal?: AbortSignal;
  /** ms before the client gives up waiting. Default 15s. */
  timeoutMs?: number;
}

export class ApiTimeout extends Error {
  constructor(path: string) {
    super(`${path} is taking longer than expected`);
    this.name = "ApiTimeout";
  }
}

export async function postJSON<T = OkResponse>(
  path: string,
  body: unknown,
  opts: PostOptions = {},
): Promise<T> {
  const { signal, timeoutMs = 15_000 } = opts;
  const ac = new AbortController();
  const onOuter = () => ac.abort();
  signal?.addEventListener("abort", onOuter);
  let timedOut = false;
  const timer = window.setTimeout(() => {
    timedOut = true;
    ac.abort();
  }, timeoutMs);
  try {
    const res = await fetch(BASE + path, {
      method: "POST",
      headers: { "Content-Type": "application/json", Accept: "application/json" },
      body: JSON.stringify(body),
      signal: ac.signal,
    });
    if (!res.ok) throw await parseError(res);
    const text = await res.text();
    return (text ? JSON.parse(text) : { ok: true }) as T;
  } catch (e) {
    if (timedOut) throw new ApiTimeout(path);
    throw e;
  } finally {
    window.clearTimeout(timer);
    signal?.removeEventListener("abort", onOuter);
  }
}

// ------------------------------------------------------------------ reads

export const fetchHealth = (signal?: AbortSignal) =>
  getJSON<HealthResponse>("/api/health", signal);

export const fetchBoard = (signal?: AbortSignal) => getJSON<WebBoard>("/api/board", signal);

export const fetchJobForm = (slug: string, signal?: AbortSignal) =>
  getJSON<JobForm>(`/api/job/form?slug=${encodeURIComponent(slug)}`, signal);

export const fetchDirs = (q: string, signal?: AbortSignal) =>
  getJSON<DirsResponse>(`/api/dirs?q=${encodeURIComponent(q)}`, signal);

export const fetchModels = (signal?: AbortSignal) =>
  getJSON<ModelsResponse>("/api/models", signal);

export const fetchTabs = (signal?: AbortSignal) => getJSON<TabsResponse>("/api/tabs", signal);

/** The record of a job's conversation: your messages and pi's final answers.
 *  thread 0 (or absent) is the job's own pi; else that thread's. */
export const fetchConversation = (slug: string, thread = 0, signal?: AbortSignal) =>
  getJSON<WebConversation>(`/api/job/conversation?slug=${encodeURIComponent(slug)}${thread ? `&thread=${thread}` : ""}`, signal);

// ------------------------------------------------- job state transitions

const slugAction =
  (path: string, opts?: PostOptions) =>
  (slug: string): Promise<OkResponse> =>
    postJSON<OkResponse>(path, { slug }, opts);

/** The submit URL. The drag table and the overlay's start button both post
 *  it, and the drag hook tells the submit apart from every other post by this
 *  string — so it has one spelling. */
export const SUBMIT_ENDPOINT = "/api/queue";

/** How long a start may hold before the client stops waiting (tmux + pi
 *  startup). A timeout is NOT a failed start — the caller refetches instead. */
export const SUBMIT_TIMEOUT_MS = 60_000;

/** START a draft: the server spawns tmux + pi before answering (seconds),
 *  and refuses with a 409 whose body carries the reason. */
export const queueJob = slugAction(SUBMIT_ENDPOINT, { timeoutMs: SUBMIT_TIMEOUT_MS });
export const stopJob = slugAction("/api/stop");
export const doneJob = slugAction("/api/done");
/** "cancel": kill the session, back to draft. */
export const killJob = slugAction("/api/kill");
export const reactivateJob = slugAction("/api/reactivate", { timeoutMs: SUBMIT_TIMEOUT_MS });
export const retryJob = slugAction("/api/retry", { timeoutMs: SUBMIT_TIMEOUT_MS });
export const archiveJob = slugAction("/api/archive");
/** WATCH: pi is told to watch over the shipped work (the watch prompt) and the job parks in WATCHING; relaunches pi when it is gone */
export const watchJob = slugAction("/api/watch", { timeoutMs: SUBMIT_TIMEOUT_MS });
export const unarchiveJob = slugAction("/api/unarchive");
export const deleteJob = slugAction("/api/delete");
/** ⌘-click on a card (or the overlay's dir): open the job's directory in the editor — VS Code unless DRIVER_EDITOR says otherwise. */
export const openJobDir = slugAction("/api/job/open");
/** the operator is looking at a tab of the job (thread 0: its own pi): takes that tab's ring, and the card's own, off */
export const seenJob = (slug: string, thread: number) => postJSON<OkResponse>("/api/job/seen", { slug, thread });

export const reorderColumn = (column: Column, slugs: readonly string[]) =>
  postJSON<OkResponse>("/api/reorder", { column, slugs });

/** The board's full column order, state columns and lanes alike. */
export const reorderColumns = (columns: readonly Column[]) =>
  postJSON<OkResponse>("/api/columns", { columns });

/** What WATCH says to pi; "" goes back to the default. */
export const setWatchPrompt = (text: string) => postJSON<OkResponse>("/api/watch-prompt", { text });

// ---------------------------------------------------------------- threads

const THREAD_ENDPOINT = "/api/job/thread";

/** Open a thread on the job: a copy of thread `from`'s conversation (0 = the
 *  job's own) with a pi of its own, named `name`. */
export const openThread = (slug: string, from: number, name: string) =>
  postJSON<OpenThreadResult>(THREAD_ENDPOINT, { slug, op: "open", from, name }, { timeoutMs: SUBMIT_TIMEOUT_MS });
export const closeThread = (slug: string, id: number) => postJSON<OkResponse>(THREAD_ENDPOINT, { slug, op: "close", id });
export const stopThread = (slug: string, id: number) => postJSON<OkResponse>(THREAD_ENDPOINT, { slug, op: "stop", id });
export const retryThread = (slug: string, id: number) =>
  postJSON<OkResponse>(THREAD_ENDPOINT, { slug, op: "retry", id }, { timeoutMs: SUBMIT_TIMEOUT_MS });
export const renameThread = (slug: string, id: number, name: string) =>
  postJSON<OkResponse>(THREAD_ENDPOINT, { slug, op: "rename", id, name });

/** Put text into a pane's pi editor, nothing sent (thread 0: the job's own pi). */
export const pasteIntoJob = (slug: string, thread: number, text: string) =>
  postJSON<OkResponse>("/api/job/paste", { slug, thread, text });

/** The browser's front tab as a paste for pi. */
export const fetchBrowserContext = (signal?: AbortSignal) => getJSON<BrowserContext>("/api/context/browser", signal);

// --------------------------------------------------------------- settings

export const fetchSettings = (signal?: AbortSignal) => getJSON<SettingsInfo>("/api/settings", signal);
/** Try reading the protected folders from the daemon: any prompt macOS puts up is driver's, now. */
export const probeFolders = () => postJSON<FoldersProbeResult>("/api/settings/probe", { what: "folders" }, { timeoutMs: 120_000 });
/** Try reading the browser's front tab from the daemon. */
export const probeBrowser = () => postJSON<BrowserProbeResult>("/api/settings/probe", { what: "browser" }, { timeoutMs: 120_000 });
/** Open System Settings on a Privacy & Security pane, or show the binary in the Finder. */
export const openSystemPane = (pane: "full-disk-access" | "files" | "automation" | "binary") =>
  postJSON<OkResponse>("/api/settings/open", { pane });

// ------------------------------------------------------------------ lanes

export const addLane = (name: string) => postJSON<AddLaneResult>("/api/lanes", { op: "add", name });

export const renameLane = (id: number, name: string) =>
  postJSON<OkResponse>("/api/lanes", { op: "rename", id, name });

export const removeLane = (id: number) => postJSON<OkResponse>("/api/lanes", { op: "remove", id });

/** The lane endpoint; the drag table posts it for every drop into or out of a lane. */
export const LANE_ENDPOINT = "/api/job/lane";

/** Park a job in a lane; 0 returns it to the column its state names. */
export const setJobLane = (slug: string, lane: number) => postJSON<OkResponse>(LANE_ENDPOINT, { slug, lane });

// ------------------------------------------------------------------ plans

export const fetchPlans = (signal?: AbortSignal) => getJSON<{ plans: WebPlanSummary[] }>("/api/plans", signal);

/** One plan with every step; `plan` is its id or its name. */
export const fetchPlan = (plan: number | string, signal?: AbortSignal) =>
  getJSON<WebPlan>(`/api/plan?plan=${encodeURIComponent(String(plan))}`, signal);

export const addPlan = (name: string) => postJSON<AddPlanResult>("/api/plans", { op: "add", name });
export const renamePlan = (plan: number, name: string) => postJSON<OkResponse>("/api/plans", { op: "rename", plan, name });
/** The plan goes, with its steps. */
export const removePlan = (plan: number) => postJSON<OkResponse>("/api/plans", { op: "remove", plan });

/** Change a plan's steps: every op or none. */
export const applyPlanOps = (plan: number, ops: PlanOp[]) => postJSON<PlanOpsResult>("/api/plan/ops", { plan, ops });

// ------------------------------------------------------------ job fields

export const setJobColor = (slug: string, color: string) =>
  postJSON<OkResponse>("/api/job/color", { slug, color });

export const renameJob = (slug: string, to: string, name: string) =>
  postJSON<RenameResult>("/api/job/rename", { slug, to, name });

export const fetchJobNotes = (slug: string, signal?: AbortSignal) =>
  getJSON<{ text: string }>(`/api/job/notes?slug=${encodeURIComponent(slug)}`, signal);

export const saveJobNotes = (slug: string, text: string) =>
  postJSON<OkResponse>("/api/job/notes", { slug, text });

export const addJobLink = (slug: string, url: string) =>
  postJSON<AddLinkResult>("/api/job/links", { slug, op: "add", url });

export const removeJobLink = (slug: string, id: number) =>
  postJSON<OkResponse>("/api/job/links", { slug, op: "remove", id });

export const pinJobLink = (slug: string, url: string) =>
  postJSON<PinLinkResult>("/api/job/links", { slug, op: "pin", url });

export const unpinJobLink = (slug: string, id: number) =>
  postJSON<OkResponse>("/api/job/links", { slug, op: "unpin", id });

// ------------------------------------------------------------- the editor

export interface SaveFormBody {
  slug: string;
  dir: string;
  model: string;
  /** echo the loaded mtime for the optimistic lock; 0 skips it */
  mtime: number;
}

export const saveJobForm = (body: SaveFormBody) =>
  postJSON<SaveFormResult>("/api/job/saveform", body);

/** Make a not-started draft; queueJob then opens pi in it. */
export const createJob = (body: CreateJobBody) => postJSON<JobForm>("/api/job/create", body);

/** Copy a job — its conversation, notes, links, directory and model — as a new
 *  job in the same column. An open job's copy gets its own pi, so this can
 *  take as long as a start. */
export const forkJob = (body: ForkJobBody) =>
  postJSON<JobForm>("/api/job/fork", body, { timeoutMs: SUBMIT_TIMEOUT_MS });
