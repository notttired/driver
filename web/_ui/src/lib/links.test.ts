import { afterEach, describe, expect, it, vi } from "vitest";

import { isOpenClick, linkLabel, openExternal, prNumber } from "./links.ts";

describe("link labels", () => {
  it("reads a PR as #N and anything else as its host", () => {
    expect(prNumber("https://github.com/o/r/pull/482")).toBe("#482");
    expect(prNumber("https://github.com/o/r/issues/4")).toBeNull();
    expect(linkLabel({ kind: "github_pr", url: "https://github.com/o/r/pull/7" })).toBe("#7");
    expect(linkLabel({ kind: "url", url: "https://www.example.com/a/b?c" })).toBe("example.com");
    expect(linkLabel({ kind: "linear", url: "https://linear.app/t/issue/X-1" })).toBe("linear.app");
  });
});

describe("openExternal", () => {
  afterEach(() => vi.restoreAllMocks());

  it("opens an http(s) URL in a new window with the URL in the call", () => {
    const open = vi.spyOn(window, "open").mockImplementation(() => null);
    expect(openExternal("https://example.com/x")).toBe(true);
    expect(open).toHaveBeenCalledWith("https://example.com/x", "_blank", "noopener,noreferrer");
  });

  it("refuses anything that is not http(s)", () => {
    const open = vi.spyOn(window, "open").mockImplementation(() => null);
    expect(openExternal("file:///etc/hosts")).toBe(false);
    expect(openExternal("javascript:alert(1)")).toBe(false);
    expect(open).not.toHaveBeenCalled();
  });

  it("pane links want ⌘ (or ctrl)", () => {
    expect(isOpenClick({ metaKey: true, ctrlKey: false })).toBe(true);
    expect(isOpenClick({ metaKey: false, ctrlKey: true })).toBe(true);
    expect(isOpenClick({ metaKey: false, ctrlKey: false })).toBe(false);
  });
});
