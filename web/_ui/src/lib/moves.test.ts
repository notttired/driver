import { describe, expect, it, vi } from "vitest";

// moves.ts binds each row to a client function; stub the client so the table
// can be exercised without a network and the slug plumbing checked.
const calls = vi.hoisted(() => ({ list: [] as Array<[string, unknown[]]> }));
vi.mock("../api/client.ts", () => {
  const mk = (ep: string) =>
    vi.fn(async (...args: unknown[]) => {
      calls.list.push([ep, args]);
      return { ok: true };
    });
  return {
    SUBMIT_ENDPOINT: "/api/queue",
    queueJob: mk("/api/queue"),
    stopJob: mk("/api/stop"),
    doneJob: mk("/api/done"),
    reactivateJob: mk("/api/reactivate"),
    archiveJob: mk("/api/archive"),
  };
});

import { COLUMNS, type Column } from "./columns.ts";
import { ARCHIVED, isLegalTarget, MOVES, resolveMove, SUBMIT_ENDPOINT, type DropColumn } from "./moves.ts";

const TARGETS: readonly DropColumn[] = [...COLUMNS, ARCHIVED];

const LISTED: ReadonlyArray<[Column, DropColumn, string]> = [
  ["draft", "working", "/api/queue"],
  ["working", "blocked", "/api/stop"],
  ["blocked", "done", "/api/done"],
  ["blocked", "working", "open"],
  ["done", "working", "/api/reactivate"],
  ["done", "blocked", "/api/reactivate"],
  ["done", ARCHIVED, "/api/archive"],
];

describe("resolveMove — the drag table", () => {
  it.each(LISTED)("%s → %s is %s", (from, to, expected) => {
    const move = resolveMove(from, to);
    expect(move).not.toBeNull();
    if (expected === "open") {
      expect(move?.kind).toBe("open");
    } else {
      expect(move?.kind).toBe("post");
      if (move?.kind === "post") expect(move.endpoint).toBe(expected);
    }
  });

  it("the submit row carries the client's submit endpoint", () => {
    const move = resolveMove("draft", "working");
    expect(move?.kind === "post" && move.endpoint).toBe(SUBMIT_ENDPOINT);
    expect(SUBMIT_ENDPOINT).toBe("/api/queue");
  });

  it.each(COLUMNS)("same column (%s) is a reorder", (col) => {
    expect(resolveMove(col, col)).toEqual({ kind: "reorder" });
  });

  it("every unlisted cross-column pair is a no-op (null)", () => {
    const listed = new Set(LISTED.map(([f, t]) => `${f}>${t}`));
    const unlisted: string[] = [];
    for (const from of COLUMNS) {
      for (const to of TARGETS) {
        if (from === to || listed.has(`${from}>${to}`)) continue;
        unlisted.push(`${from}>${to}`);
        expect(resolveMove(from, to), `${from} → ${to}`).toBeNull();
      }
    }
    // 4 columns × 5 targets − 4 reorders − 7 listed
    expect(unlisted).toHaveLength(9);
  });

  it("ignores prototype names as targets", () => {
    expect(resolveMove("draft", "constructor" as DropColumn)).toBeNull();
    expect(resolveMove("done", "toString" as DropColumn)).toBeNull();
    expect(resolveMove("hasOwnProperty" as Column, "draft")).toBeNull();
  });

  it("MOVES lists exactly the legal targets of each column", () => {
    for (const from of COLUMNS) {
      const legal = TARGETS.filter((to) => to !== from && resolveMove(from, to) !== null);
      expect([...MOVES[from]].sort()).toEqual([...legal].sort());
    }
    expect(isLegalTarget("draft", "working")).toBe(true);
    expect(isLegalTarget("draft", "done")).toBe(false);
    expect(isLegalTarget(null, "done")).toBe(false);
  });

  it("post moves call the client with the slug only", async () => {
    calls.list.length = 0;
    for (const [from, to, endpoint] of LISTED) {
      const move = resolveMove(from, to);
      if (move?.kind !== "post") continue;
      await move.run("job-x");
      const last = calls.list.at(-1);
      expect(last?.[0]).toBe(endpoint);
      expect(last?.[1]).toEqual(["job-x"]);
    }
    expect(calls.list).toHaveLength(6);
  });
});

describe("resolveMove — an opened draft", () => {
  it("draft → working opens the pane instead of submitting", () => {
    expect(resolveMove("draft", "working", { opened: true })).toEqual({ kind: "open" });
    expect(resolveMove("draft", "working", { opened: false })).toMatchObject({ kind: "post", endpoint: SUBMIT_ENDPOINT });
    expect(resolveMove("draft", "working")).toMatchObject({ kind: "post" });
    expect(resolveMove("draft", "done", { opened: true })).toBeNull();
    expect(isLegalTarget("draft", "working")).toBe(true);
  });
});
