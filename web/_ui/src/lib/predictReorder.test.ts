import { describe, expect, it } from "vitest";

import { columnSlugs, predictReorderedJobs } from "./predictReorder.ts";
import { makeJob } from "../test/fixtures.ts";

const jobs = [
  makeJob({ slug: "a", state: "draft" }),
  makeJob({ slug: "b", state: "working" }),
  makeJob({ slug: "c", state: "draft" }),
  makeJob({ slug: "arch-1", display_slug: "arch", state: "draft", archived: true }),
  makeJob({ slug: "d", state: "draft" }),
];

describe("columnSlugs", () => {
  it("lists a column's slugs in payload order, skipping archived jobs", () => {
    expect(columnSlugs(jobs, "draft")).toEqual(["a", "c", "d"]);
    expect(columnSlugs(jobs, "working")).toEqual(["b"]);
    expect(columnSlugs(jobs, "done")).toEqual([]);
  });
});

describe("predictReorderedJobs", () => {
  it("rewrites the column's slots in the new order and leaves other jobs in place", () => {
    const out = predictReorderedJobs(jobs, "draft", ["d", "a", "c"]);
    expect(out?.map((j) => j.slug)).toEqual(["d", "b", "a", "arch-1", "c"]);
    // same objects, not copies
    expect(out?.[0]).toBe(jobs[4]);
  });

  it("is a no-op for the current order", () => {
    const out = predictReorderedJobs(jobs, "draft", ["a", "c", "d"]);
    expect(out?.map((j) => j.slug)).toEqual(jobs.map((j) => j.slug));
  });

  it("refuses a slug set that does not match the column", () => {
    expect(predictReorderedJobs(jobs, "draft", ["a", "c"])).toBeNull();
    expect(predictReorderedJobs(jobs, "draft", ["a", "c", "d", "b"])).toBeNull();
    expect(predictReorderedJobs(jobs, "draft", ["a", "c", "nope"])).toBeNull();
    expect(predictReorderedJobs(jobs, "working", ["a"])).toBeNull();
  });

  it("handles an empty column", () => {
    expect(predictReorderedJobs(jobs, "done", [])).toEqual(jobs);
  });
});
