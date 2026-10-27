import { COLUMNS, type Column } from "./columns.ts";
import type { WebJob } from "./types.ts";

/** The eight job colours, at the names the server's colour route accepts. */
export const JOB_COLORS = [
  "red",
  "blue",
  "green",
  "yellow",
  "purple",
  "orange",
  "pink",
  "cyan",
] as const;
export type JobColor = (typeof JOB_COLORS)[number];

export function isJobColor(c: string | undefined | null): c is JobColor {
  return !!c && (JOB_COLORS as readonly string[]).includes(c);
}

/** `var(--job-<color>)` for a coloured job, "" otherwise. Callers must OMIT
 *  the CSS property on "" so the stylesheet's fallback applies. */
export function jobColorVar(j: { color?: string } | null | undefined): string {
  const c = j?.color;
  return isJobColor(c) ? `var(--job-${c})` : "";
}

/** A stable tint for a name (the card's dir tag): djb2 over the string, into
 *  the job palette, so the same directory is the same colour on every card. */
export function nameColorVar(name: string | undefined | null): string {
  if (!name) return "";
  let h = 5381;
  for (let i = 0; i < name.length; i++) {
    h = (h * 33 + name.charCodeAt(i)) >>> 0;
  }
  return `var(--job-${JOB_COLORS[h % JOB_COLORS.length]})`;
}

/** Last path component of a directory, with a leading `~` left readable. */
export function dirBase(dir: string | undefined | null): string {
  if (!dir) return "";
  const trimmed = dir.replace(/\/+$/, "");
  const i = trimmed.lastIndexOf("/");
  return i < 0 ? trimmed : trimmed.slice(i + 1) || trimmed;
}

/** "Ns" / "Nm" / "Nh" / "Nd" since a unix-SECONDS stamp; "" for a missing one. */
export function age(unixSeconds: number | undefined | null): string {
  if (!unixSeconds) return "";
  const s = Math.max(0, Math.floor(Date.now() / 1000 - unixSeconds));
  if (s < 60) return s + "s";
  if (s < 3600) return Math.floor(s / 60) + "m";
  if (s < 86400) return Math.floor(s / 3600) + "h";
  return Math.floor(s / 86400) + "d";
}

export function columnFor(state: string): Column {
  return (COLUMNS as readonly string[]).includes(state) ? (state as Column) : "draft";
}

export interface BoardBuckets {
  byColumn: Record<Column, WebJob[]>;
  archived: WebJob[];
}

/** Archived jobs go to the drawer; everything else to the column its state
 *  names. Server order is preserved within each bucket. */
export function bucketJobs(jobs: readonly WebJob[]): BoardBuckets {
  const byColumn = Object.fromEntries(COLUMNS.map((c) => [c, [] as WebJob[]])) as Record<
    Column,
    WebJob[]
  >;
  const archived: WebJob[] = [];
  for (const j of jobs) {
    if (j.archived) {
      archived.push(j);
      continue;
    }
    byColumn[columnFor(j.state)].push(j);
  }
  return { byColumn, archived };
}
