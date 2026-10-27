import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { age, bucketJobs, columnFor, dirBase, isJobColor, jobColorVar, JOB_COLORS, nameColorVar } from "./format.ts";
import { makeJob } from "../test/fixtures.ts";

describe("bucketJobs", () => {
  it("routes archived jobs to the drawer whatever their state, others by state", () => {
    const jobs = [
      makeJob({ slug: "a", state: "draft" }),
      makeJob({ slug: "b", state: "working" }),
      makeJob({ slug: "c", state: "blocked" }),
      makeJob({ slug: "d", state: "done" }),
      makeJob({ slug: "e-12", display_slug: "e", state: "done", archived: true }),
      makeJob({ slug: "f-3", display_slug: "f", state: "working", archived: true }),
    ];
    const b = bucketJobs(jobs);
    expect(b.byColumn.draft.map((j) => j.slug)).toEqual(["a"]);
    expect(b.byColumn.working.map((j) => j.slug)).toEqual(["b"]);
    expect(b.byColumn.blocked.map((j) => j.slug)).toEqual(["c"]);
    expect(b.byColumn.done.map((j) => j.slug)).toEqual(["d"]);
    expect(b.archived.map((j) => j.slug)).toEqual(["e-12", "f-3"]);
  });

  it("treats an empty or unknown state as draft and keeps payload order", () => {
    const jobs = [
      makeJob({ slug: "z", state: "" as "draft" }),
      makeJob({ slug: "y", state: "weird" as "draft" }),
      makeJob({ slug: "x", state: "draft" }),
    ];
    expect(bucketJobs(jobs).byColumn.draft.map((j) => j.slug)).toEqual(["z", "y", "x"]);
    expect(columnFor("")).toBe("draft");
    expect(columnFor("nope")).toBe("draft");
    expect(columnFor("blocked")).toBe("blocked");
  });

  it("returns empty buckets for an empty board", () => {
    const b = bucketJobs([]);
    expect(b.byColumn.draft).toEqual([]);
    expect(b.byColumn.done).toEqual([]);
    expect(b.archived).toEqual([]);
  });
});

describe("age", () => {
  beforeEach(() => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date("2026-10-27T12:00:00Z"));
  });
  afterEach(() => vi.useRealTimers());

  it("formats unix seconds as s/m/h/d and blanks a missing timestamp", () => {
    const now = Math.floor(Date.now() / 1000);
    expect(age(now - 30)).toBe("30s");
    expect(age(now - 90)).toBe("1m");
    expect(age(now - 7200)).toBe("2h");
    expect(age(now - 3 * 86400)).toBe("3d");
    expect(age(now + 500)).toBe("0s");
    expect(age(0)).toBe("");
    expect(age(undefined)).toBe("");
  });
});

describe("dirBase / colours", () => {
  it("takes the last path component", () => {
    expect(dirBase("/Users/me/src/demo")).toBe("demo");
    expect(dirBase("/Users/me/src/demo/")).toBe("demo");
    expect(dirBase("")).toBe("");
    expect(dirBase(undefined)).toBe("");
  });

  it("maps a set job colour to its CSS variable and blanks everything else", () => {
    expect(jobColorVar({ color: "blue" })).toBe("var(--job-blue)");
    expect(jobColorVar({ color: "" })).toBe("");
    expect(jobColorVar({ color: "magenta" })).toBe("");
    expect(jobColorVar(undefined)).toBe("");
    expect(isJobColor("cyan")).toBe(true);
    expect(isJobColor("")).toBe(false);
  });

  it("hashes a name to a stable palette colour", () => {
    const v = nameColorVar("payments");
    expect(v).toBe(nameColorVar("payments"));
    expect(JOB_COLORS.map((c) => `var(--job-${c})`)).toContain(v);
    expect(nameColorVar("")).toBe("");
  });
});
