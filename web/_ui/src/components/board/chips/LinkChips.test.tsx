import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";

import type { WebLink } from "../../../lib/types.ts";
import { LinkChips, MAX_CHIPS } from "./LinkChips.tsx";

const link = (id: number): WebLink => ({ id, kind: "url", url: `https://example.com/${id}`, mentions: 1 });

describe("LinkChips", () => {
  afterEach(cleanup);

  it("shows at most five links, the first in display order, and counts the rest", () => {
    const links = Array.from({ length: 8 }, (_, i) => link(i + 1));
    render(<LinkChips links={links} />);
    expect(MAX_CHIPS).toBe(5);
    const chips = screen.getAllByRole("link");
    expect(chips).toHaveLength(5);
    expect(chips.map((c) => c.getAttribute("href"))).toEqual(links.slice(0, 5).map((l) => l.url));
    expect(screen.getByText("+3")).toHaveAttribute("title", "3 more links — open the job to see them all");
  });

  it("shows every link when there are five or fewer, with no count", () => {
    render(<LinkChips links={Array.from({ length: 5 }, (_, i) => link(i + 1))} />);
    expect(screen.getAllByRole("link")).toHaveLength(5);
    expect(screen.queryByText(/^\+\d+$/)).toBeNull();
  });
});
