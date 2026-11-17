import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { makeJob } from "../../test/fixtures.ts";
import type { WebJob, WebLane } from "../../lib/types.ts";
import { CommandPalette } from "./CommandPalette.tsx";

const NOW = Math.floor(Date.now() / 1000);
const lanes: WebLane[] = [{ id: 3, name: "Awaiting Review" }];
const jobs: WebJob[] = [
  makeJob({ slug: "stripe-webhooks", name: "Stripe webhooks retry", dir: "/Users/me/src/payments", state: "draft" }),
  makeJob({
    slug: "search-reindex",
    dir: "/Users/me/src/search",
    state: "working",
    live: true,
    pi: { state: "working", tool: "bash", ts: NOW },
    threads: [
      { id: 0, name: "search-reindex", live: true, pi: { state: "working", tool: "bash", ts: NOW } },
      { id: 4, name: "CI flake", live: true, pi: { state: "idle", reason: "idle", ts: NOW } },
    ],
  }),
  makeJob({ slug: "onboarding-copy", dir: "/Users/me/src/web", state: "blocked", live: true, pi: { state: "idle", ts: NOW }, lane: 3 }),
  makeJob({ slug: "spike-graphql-12", display_slug: "spike-graphql", dir: "/Users/me/src/api", state: "done", archived: true }),
];

const setup = (over: Partial<Parameters<typeof CommandPalette>[0]> = {}) => {
  const onPick = vi.fn();
  const onOpenDir = vi.fn();
  const onClose = vi.fn();
  render(<CommandPalette open jobs={jobs} lanes={lanes} onPick={onPick} onOpenDir={onOpenDir} onClose={onClose} {...over} />);
  return { onPick, onOpenDir, onClose, input: screen.getByRole("combobox") };
};
const rows = () => screen.getAllByRole("option");
const selected = () => screen.getAllByRole("option").find((o) => o.getAttribute("aria-selected") === "true");

describe("CommandPalette", () => {
  afterEach(cleanup);

  it("lists every pane with its status, focused on the input, the first row selected", () => {
    const { input } = setup();
    expect(document.activeElement).toBe(input);
    const all = rows();
    expect(all.map((r) => r.querySelector(".pal-title")?.textContent)).toEqual([
      "Stripe webhooks retry",
      "search-reindex",
      "search-reindex · CI flake",
      "onboarding-copy",
      "spike-graphql",
    ]);
    expect(all[1]!.querySelector(".status")).toHaveTextContent("● working");
    expect(all[1]!.querySelector(".status")).toHaveTextContent("⚙ bash");
    expect(all[1]!.querySelector(".pal-col")).toHaveTextContent("WORKING");
    expect(all[2]!.querySelector(".status")).toHaveTextContent("⚑ needs you");
    expect(all[3]!.querySelector(".pal-col")).toHaveTextContent("Awaiting Review");
    expect(all[4]!.querySelector(".status")).toHaveTextContent("■ archived");
    expect(selected()).toBe(all[0]);
    expect(screen.getByText("5 panes")).toBeInTheDocument();
  });

  it("narrows as you type and says where a match came from", () => {
    const { input } = setup();
    fireEvent.change(input, { target: { value: "payments" } });
    expect(rows()).toHaveLength(1);
    expect(rows()[0]!.querySelector(".pal-why")).toHaveTextContent("/Users/me/src/payments");
    expect(screen.getByText("1 of 5 panes")).toBeInTheDocument();
    fireEvent.change(input, { target: { value: "zzz" } });
    expect(screen.queryAllByRole("option")).toHaveLength(0);
    expect(screen.getByText(/nothing matches/)).toBeInTheDocument();
  });

  it("moves with the arrows, wraps, and opens the selected pane on Enter", () => {
    const { input, onPick, onClose } = setup();
    fireEvent.keyDown(input, { key: "ArrowDown" });
    fireEvent.keyDown(input, { key: "ArrowDown" });
    expect(selected()).toBe(rows()[2]);
    fireEvent.keyDown(input, { key: "ArrowUp" });
    expect(selected()).toBe(rows()[1]);
    fireEvent.keyDown(input, { key: "ArrowUp" });
    fireEvent.keyDown(input, { key: "ArrowUp" });
    expect(selected()).toBe(rows()[4]);
    fireEvent.keyDown(input, { key: "Enter" });
    expect(onPick).toHaveBeenCalledWith({ key: "archive:spike-graphql-12", thread: 0 });
    expect(onClose).toHaveBeenCalled();
  });

  it("opens a thread on its own tab, by click too, and ⌘-Enter opens the directory instead", () => {
    const { input, onPick, onOpenDir } = setup();
    fireEvent.click(rows()[2]!);
    expect(onPick).toHaveBeenCalledWith({ key: "search-reindex", thread: 4 });
    fireEvent.change(input, { target: { value: "stripe" } });
    fireEvent.keyDown(input, { key: "Enter", metaKey: true });
    expect(onOpenDir).toHaveBeenCalledWith("stripe-webhooks");
  });

  it("goes back to the first row when the query changes", () => {
    const { input } = setup();
    fireEvent.keyDown(input, { key: "ArrowDown" });
    fireEvent.change(input, { target: { value: "s" } });
    expect(selected()).toBe(rows()[0]);
  });

  it("closes on Escape and on the scrim", () => {
    const { onClose } = setup();
    fireEvent.keyDown(document, { key: "Escape" });
    expect(onClose).toHaveBeenCalledTimes(1);
    fireEvent.click(document.getElementById("palette")!);
    expect(onClose).toHaveBeenCalledTimes(2);
  });

  it("renders nothing while closed", () => {
    render(<CommandPalette open={false} jobs={jobs} lanes={lanes} onPick={() => {}} onOpenDir={() => {}} onClose={() => {}} />);
    expect(screen.queryByRole("combobox")).toBeNull();
  });
});
