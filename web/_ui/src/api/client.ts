// The daemon's HTTP surface (docs/API.md), typed end to end. Every non-2xx
// answer carries `{"error": string}`; that sentence IS the user-facing message,
// so ApiError keeps it verbatim and the status beside it.

import type {
  AddLinkResult,
  CreateJobBody,
  DirsResponse,
  HealthResponse,
  JobForm,
  ModelsResponse,
  OkResponse,
  RenameResult,
  SaveFormResult,
  WebBoard, TabsResponse } from "../lib/types.ts";
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
export const archiveJob = slugAction("/api/archive");
export const unarchiveJob = slugAction("/api/unarchive");
export const deleteJob = slugAction("/api/delete");
/** ⌘-click on a card (or the overlay's dir): open the job's directory in the editor — VS Code unless DRIVER_EDITOR says otherwise. */
export const openJobDir = slugAction("/api/job/open");

export const reorderColumn = (column: Column, slugs: readonly string[]) =>
  postJSON<OkResponse>("/api/reorder", { column, slugs });

// ------------------------------------------------------------ job fields

export const setJobColor = (slug: string, color: string) =>
  postJSON<OkResponse>("/api/job/color", { slug, color });

export const renameJob = (slug: string, to: string) =>
  postJSON<RenameResult>("/api/job/rename", { slug, to });

export const saveJobNotes = (slug: string, text: string) =>
  postJSON<OkResponse>("/api/job/notes", { slug, text });

export const addJobLink = (slug: string, url: string) =>
  postJSON<AddLinkResult>("/api/job/links", { slug, op: "add", url });

export const removeJobLink = (slug: string, id: number) =>
  postJSON<OkResponse>("/api/job/links", { slug, op: "remove", id });

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
