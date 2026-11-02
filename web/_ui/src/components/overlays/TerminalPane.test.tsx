import { afterEach, describe, expect, it, vi } from "vitest";

import { activatePaneLink } from "./TerminalPane.tsx";

describe("activatePaneLink", () => {
  afterEach(() => vi.restoreAllMocks());

  it("opens a pane URL on ⌘-click and says which one, and leaves a plain click alone", () => {
    const open = vi.spyOn(window, "open").mockImplementation(() => null);
    const opened = vi.fn();
    activatePaneLink({ metaKey: false, ctrlKey: false }, "https://x.io/a", opened);
    expect(open).not.toHaveBeenCalled();
    expect(opened).not.toHaveBeenCalled();

    activatePaneLink({ metaKey: true, ctrlKey: false }, "https://x.io/a", opened);
    expect(open).toHaveBeenCalledWith("https://x.io/a", "_blank", "noopener,noreferrer");
    expect(opened).toHaveBeenCalledWith("https://x.io/a");
  });
});
