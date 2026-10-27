// The drag table (DESIGN.md "board rules"): what dropping a card from one
// column onto another does. This module is the ONLY place that knowledge
// lives — the drop highlighting, the drop handler and the tests all read it.

import {
  archiveJob,
  doneJob,
  queueJob,
  reactivateJob,
  stopJob,
  SUBMIT_ENDPOINT,
} from "../api/client.ts";
import type { Column } from "./columns.ts";

export { SUBMIT_ENDPOINT };

export const ARCHIVED = "archived" as const;
export type DropColumn = Column | typeof ARCHIVED;

export interface PostMove {
  kind: "post";
  endpoint: string;
  run: (slug: string) => Promise<unknown>;
}

/** Open the job overlay instead of writing anything (blocked → working, and
 *  draft → working for a draft whose pi is already open). */
export interface OpenMove {
  kind: "open";
}

/** What the table cannot know from the columns alone. */
export interface MoveContext {
  /** the dragged job has a pi session (lib/status isOpened) */
  opened?: boolean;
}

export interface ReorderMove {
  kind: "reorder";
}

export type Move = PostMove | OpenMove | ReorderMove;

const post = (endpoint: string, run: (slug: string) => Promise<unknown>): PostMove => ({
  kind: "post",
  endpoint,
  run,
});

export const TRANSITIONS: Readonly<
  Record<Column, Readonly<Partial<Record<DropColumn, Move>>>>
> = {
  draft: {
    working: post(SUBMIT_ENDPOINT, (slug) => queueJob(slug)),
  },
  working: {
    blocked: post("/api/stop", (slug) => stopJob(slug)),
  },
  blocked: {
    done: post("/api/done", (slug) => doneJob(slug)),
    working: { kind: "open" },
  },
  done: {
    working: post("/api/reactivate", (slug) => reactivateJob(slug)),
    blocked: post("/api/reactivate", (slug) => reactivateJob(slug)),
    [ARCHIVED]: post("/api/archive", (slug) => archiveJob(slug)),
  },
};

/** Adjacency derived from the table: which targets light up for a source. */
export const MOVES: Readonly<Record<Column, readonly DropColumn[]>> = Object.freeze(
  Object.fromEntries(
    (Object.keys(TRANSITIONS) as Column[]).map((from) => [
      from,
      Object.freeze(Object.keys(TRANSITIONS[from]) as DropColumn[]),
    ]),
  ) as Record<Column, readonly DropColumn[]>,
);

export function resolveMove(from: Column, to: DropColumn, ctx: MoveContext = {}): Move | null {
  if (from === to) return { kind: "reorder" };
  // an opened draft has nothing to submit: to WORKING means go talk to it
  if (from === "draft" && to === "working" && ctx.opened) return { kind: "open" };
  const row = TRANSITIONS[from] as Record<string, Move> | undefined;
  if (row === undefined) return null;
  // own-property check: "constructor"/"toString" as a target must not resolve
  if (!Object.prototype.hasOwnProperty.call(row, to)) return null;
  return row[to] ?? null;
}

export function isLegalTarget(from: Column | null, to: DropColumn): boolean {
  if (from === null || from === to) return false;
  return resolveMove(from, to) !== null;
}
