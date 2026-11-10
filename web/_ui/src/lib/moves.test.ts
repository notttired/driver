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
    LANE_ENDPOINT: "/api/job/lane",
    queueJob: mk("/api/queue"),
    stopJob: mk("/api/stop"),
    doneJob: mk("/api/done"),
    reactivateJob: mk("/api/reactivate"),
    archiveJob: mk("/api/archive"),
    setJobLane: mk("/api/job/lane"),
  };
});

import { COLUMNS, WATCHING, type Column } from "./columns.ts";
import { ARCHIVED, isLegalTarget, LANE_ENDPOINT, MOVES, resolveMove, SUBMIT_ENDPOINT, type DropColumn } from "./moves.ts";

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

describe("resolveMove — lanes", () => {
  it("any card can be dropped into a lane", async () => {
    calls.list.length = 0;
    for (const from of COLUMNS) {
      const move = resolveMove(from, "lane:3");
      expect(move?.kind, from).toBe("post");
      if (move?.kind !== "post") continue;
      expect(move.endpoint).toBe(LANE_ENDPOINT);
      await move.run("job-x");
      expect(calls.list.at(-1)).toEqual(["/api/job/lane", ["job-x", 3]]);
    }
    expect(resolveMove("lane:3", "lane:4")).toMatchObject({ kind: "post", endpoint: LANE_ENDPOINT });
  });

  it("a parked card goes back to its own state column and nowhere else", async () => {
    calls.list.length = 0;
    const home = resolveMove("lane:3", "blocked", { state: "blocked" });
    expect(home).toMatchObject({ kind: "post", endpoint: LANE_ENDPOINT });
    if (home?.kind === "post") await home.run("job-x");
    expect(calls.list.at(-1)).toEqual(["/api/job/lane", ["job-x", 0]]);
    expect(resolveMove("lane:3", "done", { state: "blocked" })).toBeNull();
    expect(resolveMove("lane:3", "working", { state: "blocked", opened: true })).toBeNull();
    expect(resolveMove("lane:3", ARCHIVED, { state: "done" })).toBeNull();
    expect(resolveMove("lane:3", "lane:3")).toEqual({ kind: "reorder" });
    expect(isLegalTarget("lane:3", "draft")).toBe(true);
    expect(isLegalTarget("lane:3", ARCHIVED)).toBe(false);
  });
});

describe("resolveMove — WATCHING", () => {
  it("is not a drop target; a watched card goes back to its state column or into a lane", () => {
    for (const from of [...COLUMNS, "lane:3" as const]) {
      expect(resolveMove(from, WATCHING, { opened: true, state: "blocked" }), from).toBeNull();
    }
    expect(resolveMove(WATCHING, WATCHING)).toEqual({ kind: "reorder" });
    expect(resolveMove(WATCHING, "working", { state: "working" })).toMatchObject({ kind: "post", endpoint: LANE_ENDPOINT });
    expect(resolveMove(WATCHING, "done", { state: "working" })).toBeNull();
    expect(resolveMove(WATCHING, ARCHIVED, { state: "done" })).toBeNull();
    expect(resolveMove(WATCHING, "lane:3")).toMatchObject({ kind: "post", endpoint: LANE_ENDPOINT });
    expect(isLegalTarget("draft", WATCHING)).toBe(false);
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
