import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { WebLink } from "../../lib/types.ts";

const { pinJobLink, unpinJobLink } = vi.hoisted(() => ({
  pinJobLink: vi.fn(async () => ({ ok: true as const, pinned: true })),
  unpinJobLink: vi.fn(async () => ({ ok: true as const })),
}));
vi.mock("../../api/client.ts", () => ({
  addJobLink: vi.fn(),
  removeJobLink: vi.fn(),
  fetchTabs: vi.fn(),
  pinJobLink,
  unpinJobLink,
}));

import { LinksPanel } from "./LinksPanel.tsx";

const LINKS: WebLink[] = [
  { id: 1, kind: "url", url: "https://you.io/brief", mentions: 2, yours: true },
  { id: 2, kind: "github_pr", url: "https://github.com/o/r/pull/7", status: "open", mentions: 1, pinned_at: 100 },
  { id: 3, kind: "url", url: "https://pi.io/often", mentions: 4 },
  { id: 4, kind: "url", url: "https://pi.io/once", mentions: 1 },
];

describe("LinksPanel", () => {
  afterEach(() => {
    cleanup();
    pinJobLink.mockClear();
    unpinJobLink.mockClear();
  });

  it("lists the links in the server's order, marks the pinned ones and counts repeat mentions", () => {
    const { container } = render(<LinksPanel slug="demo" links={LINKS} onChanged={() => {}} />);
    const rows = [...container.querySelectorAll<HTMLElement>(".link-row")];
    expect(rows.map((r) => r.querySelector(".link-text")?.textContent)).toEqual([
      "you.io/brief",
      "github.com/o/r/pull/7",
      "pi.io/often",
      "pi.io/once",
    ]);
    expect(within(rows[0]!).getByTitle("pinned: you mentioned it")).toBeInTheDocument();
    expect(within(rows[0]!).queryByRole("button", { name: /unpin/ })).toBeNull();
    expect(within(rows[1]!).getByRole("button", { name: "unpin #7" })).toBeInTheDocument();
    expect(rows[2]!.querySelector(".link-pin")).toBeNull();
    expect(within(rows[2]!).getByTitle("4 mentions")).toHaveTextContent("×4");
    expect(within(rows[0]!).getByTitle("2 mentions")).toHaveTextContent("×2");
    expect(rows[3]!.querySelector(".link-count")).toBeNull();
  });

  it("pins a link you open and unpins one on its pin", async () => {
    const onChanged = vi.fn();
    render(<LinksPanel slug="demo" links={LINKS} onChanged={onChanged} />);
    fireEvent.click(screen.getByRole("link", { name: "pi.io/often" }));
    await waitFor(() => expect(pinJobLink).toHaveBeenCalledWith("demo", "https://pi.io/often"));
    await waitFor(() => expect(onChanged).toHaveBeenCalledTimes(1));

    fireEvent.click(screen.getByText("#7").closest("a")!);
    await waitFor(() => expect(pinJobLink).toHaveBeenCalledWith("demo", "https://github.com/o/r/pull/7"));

    fireEvent.click(screen.getByRole("button", { name: "unpin #7" }));
    await waitFor(() => expect(unpinJobLink).toHaveBeenCalledWith("demo", 2));
    await waitFor(() => expect(onChanged).toHaveBeenCalledTimes(3));
  });
});
