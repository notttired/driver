import type { WebJob } from "./types.ts";

export interface StatusToken {
  /** The token itself, glyph included. */
  text: string;
  /** A second, quieter token (the tool a working session is inside). */
  detail?: string;
  /** Class modifier: `status-<variant>`, beside the column class. */
  variant: "draft" | "ready" | "starting" | "working" | "needs-you" | "dialog" | "exited" | "silent" | "aborted" | "done";
}

/** Whether the job has a pi session to show: every column but DRAFT, and a
 *  draft whose pi is open (the daemon reports a `pi` state for it; a
 *  not-started draft has none). Decides pane vs launch form, and what a
 *  drag to WORKING means. */
export function isOpened(job: Pick<WebJob, "state" | "pi">): boolean {
  return job.state !== "draft" || job.pi !== undefined;
}

/**
 * The card's status token, from the job's board state plus what pi last said
 * (DESIGN.md "state derivation"). The COLUMN already says the state; the token
 * says what, inside that state, the job is doing or waiting for.
 */
export function statusToken(job: Pick<WebJob, "state" | "pi">): StatusToken {
  const pi = job.pi;
  switch (job.state) {
    case "draft":
      // an opened draft: pi at its prompt, waiting for its first message
      if (pi?.state === "ready") return { text: "○ pi open", variant: "ready" };
      if (pi?.state === "starting") return { text: "○ starting", variant: "starting" };
      return { text: "draft", variant: "draft" };
    case "working": {
      if (pi?.state === "starting") return { text: "○ starting", variant: "starting" };
      const out: StatusToken = { text: "● working", variant: "working" };
      if (pi?.tool) out.detail = `⚙ ${pi.tool}`;
      return out;
    }
    case "blocked":
      switch (pi?.state) {
        case "dialog":
          return { text: "? dialog", variant: "dialog" };
        case "exited":
          return { text: "✕ exited", variant: "exited" };
        case "silent":
          return { text: "… silent", variant: "silent" };
        case "aborted":
          return { text: "■ aborted", variant: "aborted" };
        default:
          // idle — pi is at its prompt waiting for a human (and the fallback
          // for a blocked job the daemon has no fresher word on)
          return { text: "⚑ needs you", variant: "needs-you" };
      }
    case "done":
      return { text: "✓ done", variant: "done" };
  }
}
