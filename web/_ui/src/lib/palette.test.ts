import { describe, expect, it } from "vitest";

import { searchPalette } from "./palette.ts";
import { makeJob } from "../test/fixtures.ts";
import type { WebJob, WebLane } from "./types.ts";

const NOW = Math.floor(Date.now() / 1000);
const lanes: WebLane[] = [{ id: 3, name: "Awaiting Review" }];
const jobs: WebJob[] = [
  makeJob({ slug: "stripe-webhooks", name: "Stripe webhooks retry", dir: "/Users/me/src/payments", state: "draft" }),
  makeJob({
    slug: "search-reindex",
    dir: "/Users/me/src/search",
    state: "working",
    live: true,
    model: "claude-opus-5-5",
    pi: { state: "working", tool: "bash", ts: NOW },
    links: [{ id: 1, kind: "github_pr", url: "https://github.com/acme/search/pull/482", status: "open", mentions: 1 }],
    threads: [
      { id: 0, name: "search-reindex", live: true, pi: { state: "working", tool: "bash", ts: NOW } },
      { id: 4, name: "CI flake", live: true, pi: { state: "idle", reason: "idle", ts: NOW } },
    ],
  }),
  makeJob({ slug: "onboarding-copy", dir: "/Users/me/src/web", state: "blocked", live: true, pi: { state: "idle", ts: NOW }, lane: 3 }),
  makeJob({ slug: "rate-limits", dir: "/Users/me/src/gateway", state: "done" }),
  makeJob({ slug: "spike-graphql-12", display_slug: "spike-graphql", dir: "/Users/me/src/api", state: "done", archived: true }),
];
const titles = (q: string) => searchPalette(jobs, lanes, q).map((i) => i.title);

describe("searchPalette", () => {
  it("lists every job and thread in board order when nothing is typed, archived last", () => {
    const items = searchPalette(jobs, lanes, "");
    expect(items.map((i) => i.key)).toEqual(["stripe-webhooks", "search-reindex", "search-reindex#t4", "onboarding-copy", "rate-limits", "archive:spike-graphql-12"]);
    expect(items[1]).toMatchObject({ slug: "search-reindex", thread: 0, title: "search-reindex", column: "WORKING" });
    expect(items[1]!.token.text).toBe("● working");
    expect(items[1]!.token.detail).toBe("⚙ bash");
    expect(items[2]).toMatchObject({ slug: "search-reindex", thread: 4, title: "search-reindex · CI flake", column: "WORKING" });
    expect(items[2]!.token.text).toBe("⚑ needs you");
    expect(items[3]!.column).toBe("Awaiting Review");
    expect(items[5]).toMatchObject({ title: "spike-graphql", column: "ARCHIVED" });
    expect(items[5]!.token.text).toBe("■ archived");
  });

  it("matches names and slugs first, by prefix before by piece", () => {
    expect(titles("stripe")).toEqual(["Stripe webhooks retry"]);
    expect(titles("reindex")[0]).toBe("search-reindex");
    expect(titles("sea")).toEqual(["search-reindex", "search-reindex · CI flake"]);
  });

  it("searches everything on the board: dirs, links, model, column, lane, state, tool, thread names", () => {
    expect(titles("payments")).toEqual(["Stripe webhooks retry"]);
    expect(titles("pull/482")).toEqual(["search-reindex", "search-reindex · CI flake"]);
    expect(titles("opus")).toEqual(["search-reindex", "search-reindex · CI flake"]);
    expect(titles("awaiting")).toEqual(["onboarding-copy"]);
    expect(titles("needs you")).toEqual(["search-reindex · CI flake", "onboarding-copy"]);
    expect(titles("blocked")).toEqual(["onboarding-copy"]);
    expect(titles("bash")).toEqual(["search-reindex"]);
    expect(titles("flake")).toEqual(["search-reindex · CI flake"]);
    expect(titles("archived")).toEqual(["spike-graphql"]);
    expect(titles("nothing-like-this")).toEqual([]);
  });

  it("says where a match outside the name came from", () => {
    const [hit] = searchPalette(jobs, lanes, "pull/482");
    expect(hit!.why).toBe("github.com/acme/search/pull/482");
    const [dir] = searchPalette(jobs, lanes, "gateway");
    expect(dir!.why).toBe("/Users/me/src/gateway");
    const [name] = searchPalette(jobs, lanes, "stripe");
    expect(name!.why).toBeUndefined();
  });

  it("needs every word to match somewhere, and still finds a name by its initials or a word's letters in order", () => {
    expect(titles("search done")).toEqual([]);
    expect(titles("rate done")).toEqual(["rate-limits"]);
    expect(titles("swr")).toEqual(["Stripe webhooks retry"]);
    expect(titles("lmts")).toEqual(["rate-limits"]);
    expect(titles("sg")).toEqual(["spike-graphql"]);
    expect(titles("sea")).toEqual(["search-reindex", "search-reindex · CI flake"]);
  });
});
