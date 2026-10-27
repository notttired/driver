import { act, cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { App } from "../../App.tsx";
import type { WebBoard } from "../../lib/types.ts";
import { makeBoard, makeJob } from "../../test/fixtures.ts";

const NOW = Math.floor(Date.now() / 1000);

function fixtureBoard(): WebBoard {
  return makeBoard({
    daemon: { pid: 4242, started_at: NOW - 3600, last_tick: NOW - 1, live: 2 },
    jobs: [
      makeJob({ slug: "stripe-webhooks", dir: "/Users/me/src/payments", color: "blue", state: "draft", has_notes: true }),
      makeJob({ slug: "flaky-ci", dir: "/Users/me/src/platform", state: "draft" }),
      makeJob({
        slug: "search-reindex",
        dir: "/Users/me/src/search",
        state: "working",
        live: true,
        pi: { state: "working", tool: "bash", ts: NOW - 5 },
        links: [{ id: 1, kind: "github_pr", url: "https://github.com/acme/search/pull/482", status: "open" }],
      }),
      makeJob({ slug: "onboarding-copy", dir: "/Users/me/src/web", state: "blocked", live: true, pi: { state: "idle", ts: NOW - 100 } }),
      makeJob({ slug: "rate-limits", dir: "/Users/me/src/gateway", state: "done", finished_at: NOW - 86400 }),
      makeJob({ slug: "legacy-export", dir: "/Users/me/src/exports", state: "done", finished_at: NOW - 172800 }),
      makeJob({ slug: "spike-graphql-12", display_slug: "spike-graphql", dir: "/Users/me/src/api", state: "done", archived: true }),
    ],
  });
}

const json = (body: unknown, status = 200) =>
  new Response(JSON.stringify(body), { status, headers: { "content-type": "application/json" } });

const requests: string[] = [];

function installFetch(board: WebBoard) {
  const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = typeof input === "string" ? input : input instanceof URL ? input.href : input.url;
    const u = new URL(url, "http://localhost");
    requests.push(`${init?.method ?? "GET"} ${u.pathname}${u.search}`);
    switch (u.pathname) {
      case "/api/board":
        return json(board);
      case "/api/job/form": {
        const slug = u.searchParams.get("slug");
        const job = board.jobs.find((j) => j.slug === slug);
        if (!job) return json({ error: "no such job" }, 404);
        return json({
          slug: job.slug,
          dir: job.dir,
          model: "",
          color: job.color ?? "",
          notes: job.has_notes ? "check staging" : "",
          state: job.state,
          editable: job.state === "draft" && !job.archived,
          archived: !!job.archived,
          mtime: job.mtime,
          links: job.links ?? [],
        });
      }
      case "/api/models":
        return json({ models: [{ provider: "anthropic", id: "claude-sonnet-4-5" }], default: "claude-sonnet-4-5" });
      case "/api/dirs":
        return json({ dirs: ["/Users/me/src/payments", "/Users/me/src/platform"] });
      case "/api/job/saveform": {
        const body = JSON.parse(String(init?.body ?? "{}")) as { mtime?: number; slug?: string };
        expect(body.slug).toBe("flaky-ci");
        expect(body.mtime).toBe(board.jobs.find((j) => j.slug === "flaky-ci")?.mtime);
        return json({ ok: true, mtime: (body.mtime ?? 0) + 1 });
      }
      default:
        return json({ ok: true });
    }
  });
  vi.stubGlobal("fetch", fetchMock);
  return fetchMock;
}

describe("BoardView (DOM smoke test)", () => {
  beforeEach(() => {
    requests.length = 0;
    installFetch(fixtureBoard());
  });
  afterEach(() => {
    cleanup();
    vi.unstubAllGlobals();
  });

  it("renders the four columns, their counts, every card, status tokens and the archive drawer", async () => {
    render(<App />);

    const draft = await screen.findByText("stripe-webhooks");
    expect(draft).toBeInTheDocument();

    const col = (name: string) => document.querySelector<HTMLElement>(`.col[data-col="${name}"]`)!;
    for (const name of ["draft", "working", "blocked", "done"]) {
      expect(col(name), name).not.toBeNull();
      expect(within(col(name)).getByText(name.toUpperCase())).toBeInTheDocument();
    }
    expect(within(col("draft")).getByText("2")).toBeInTheDocument();
    expect(within(col("working")).getByText("1")).toBeInTheDocument();
    expect(within(col("blocked")).getByText("1")).toBeInTheDocument();
    expect(within(col("done")).getByText("2")).toBeInTheDocument();

    for (const slug of ["flaky-ci", "search-reindex", "onboarding-copy", "rate-limits", "legacy-export"]) {
      expect(screen.getByText(slug)).toBeInTheDocument();
    }

    // status tokens
    expect(within(col("working")).getByText(/● working/)).toBeInTheDocument();
    expect(within(col("working")).getByText("⚙ bash")).toBeInTheDocument();
    expect(within(col("blocked")).getByText(/⚑ needs you/)).toBeInTheDocument();
    expect(within(col("done")).getAllByText(/✓ done/)).toHaveLength(2);
    expect(within(col("draft")).getAllByText("draft")).toHaveLength(2);

    // the notes mark and the PR chip (an anchor that opens in a new tab)
    expect(within(col("draft")).getByTitle("has notes")).toBeInTheDocument();
    const chip = within(col("working")).getByText("#482").closest("a");
    expect(chip).not.toBeNull();
    expect(chip).toHaveAttribute("target", "_blank");
    expect(chip).toHaveClass("pr-open");

    // the archived job is in the drawer under DONE, not in a column
    const drawer = document.querySelector<HTMLElement>(".archive-drawer")!;
    expect(drawer).not.toBeNull();
    const head = within(drawer).getByRole("button", { name: /ARCHIVED/ });
    expect(head).toBeInTheDocument();
    expect(within(head).getByText("1")).toBeInTheDocument();
    expect(within(drawer).getByText("spike-graphql")).toBeInTheDocument();
    expect(within(col("done")).getAllByText(/✓ done/)).toHaveLength(2);

    // chrome: the daemon line and the Mac-shell hook element
    expect(document.querySelector("header.chrome")).not.toBeNull();
    expect(screen.getByText(/daemon pid 4242/)).toBeInTheDocument();
    expect(screen.getByText("7 jobs")).toBeInTheDocument();
  });

  it("filters cards by search and shows the matched read-out", async () => {
    render(<App />);
    await screen.findByText("stripe-webhooks");
    const input = screen.getByLabelText("search jobs");
    fireEvent.change(input, { target: { value: "reindex" } });
    expect(screen.getByText("search-reindex")).toBeInTheDocument();
    expect(screen.queryByText("flaky-ci")).not.toBeInTheDocument();
    expect(screen.getByText("of 7 jobs", { exact: false })).toBeInTheDocument();
    fireEvent.keyDown(input, { key: "Escape" });
    expect(await screen.findByText("flaky-ci")).toBeInTheDocument();
  });

  it("opens the job overlay on a card click with the launch form, and Escape closes it", async () => {
    render(<App />);
    const card = (await screen.findByText("stripe-webhooks")).closest(".card")!;
    fireEvent.click(card);

    const overlay = document.getElementById("job-overlay")!;
    await waitFor(() => expect(overlay).not.toHaveAttribute("hidden"));
    expect(within(overlay).getByTitle("stripe-webhooks")).toBeInTheDocument();
    expect(await within(overlay).findByLabelText(/DIRECTORY/)).toHaveValue("/Users/me/src/payments");
    expect(within(overlay).getByRole("button", { name: /START/ })).toBeInTheDocument();
    // no spec step: a job is briefed in its pane once pi is open
    expect(within(overlay).queryByLabelText(/SPEC/)).toBeNull();
    expect((within(overlay).getByPlaceholderText(/notes to self/) as HTMLTextAreaElement).value).toBe("check staging");
    expect(requests.some((r) => r.startsWith("GET /api/job/form?slug=stripe-webhooks"))).toBe(true);

    await act(async () => {
      fireEvent.keyDown(document, { key: "Escape" });
    });
    expect(overlay).toHaveAttribute("hidden");
  });

  it("⌘-click on a card opens its directory in the editor instead of the overlay", async () => {
    render(<App />);
    const card = (await screen.findByText("flaky-ci")).closest(".card")!;
    fireEvent.click(card, { metaKey: true });
    await waitFor(() => expect(requests).toContain("POST /api/job/open"));
    expect(document.getElementById("job-overlay")!).toHaveAttribute("hidden");
    expect(requests.some((r) => r.startsWith("GET /api/job/form"))).toBe(false);
  });

  it("autosaves the launch form once, 600ms after typing, under the optimistic lock", async () => {
    render(<App />);
    const card = (await screen.findByText("flaky-ci")).closest(".card")!;
    fireEvent.click(card);
    const overlay = document.getElementById("job-overlay")!;
    const model = (await within(overlay).findByLabelText(/MODEL/)) as HTMLInputElement;
    const before = requests.filter((r) => r.startsWith("POST /api/job/saveform")).length;
    expect(before).toBe(0);

    fireEvent.change(model, { target: { value: "anthropic/claude-" } });
    fireEvent.change(model, { target: { value: "anthropic/claude-sonnet-4-5" } });
    expect(within(overlay).getByText("…")).toBeInTheDocument();

    await waitFor(
      () => expect(requests.filter((r) => r.startsWith("POST /api/job/saveform"))).toHaveLength(1),
      { timeout: 2000 },
    );
    expect(await within(overlay).findByText("saved")).toBeInTheDocument();
    // re-renders after the save must not schedule another save
    await new Promise((r) => setTimeout(r, 900));
    expect(requests.filter((r) => r.startsWith("POST /api/job/saveform"))).toHaveLength(1);
    const dirLookups = requests.filter((r) => r.startsWith("GET /api/dirs")).length;
    expect(dirLookups).toBeGreaterThan(0);
    expect(dirLookups).toBeLessThanOrEqual(2);
  });

  it("shows the session-ended banner for a finished job's terminal", async () => {
    render(<App />);
    const card = (await screen.findByText("rate-limits")).closest(".card")!;
    fireEvent.click(card);
    const overlay = document.getElementById("job-overlay")!;
    expect(await within(overlay).findByText(/session ended — reactivate to continue/)).toBeInTheDocument();
    expect(within(overlay).getByRole("button", { name: /REACTIVATE/ })).toBeInTheDocument();
    expect(within(overlay).getByRole("button", { name: /ARCHIVE/ })).toBeInTheDocument();
  });
});
