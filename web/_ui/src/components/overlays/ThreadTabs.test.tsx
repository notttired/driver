import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

const { openThread, closeThread, renameThread, restoreThread, orderThreads } = vi.hoisted(() => ({
  openThread: vi.fn<(slug: string, from: number, name: string) => Promise<{ ok: true; thread: { id: number; name: string; live: boolean } }>>(),
  closeThread: vi.fn<(slug: string, id: number) => Promise<{ ok: true }>>(),
  renameThread: vi.fn<(slug: string, id: number, name: string) => Promise<{ ok: true }>>(),
  restoreThread: vi.fn<(slug: string) => Promise<{ ok: true; thread: { id: number; name: string; live: boolean } }>>(),
  orderThreads: vi.fn<(slug: string, order: number[]) => Promise<{ ok: true }>>(),
}));
vi.mock("../../api/client.ts", async (original) => ({
  ...(await original<typeof import("../../api/client.ts")>()),
  openThread,
  closeThread,
  renameThread,
  restoreThread,
  orderThreads,
}));

import { ThreadTabs, suggestThreadName } from "./ThreadTabs.tsx";
import type { WebThread } from "../../lib/types.ts";

const threads: WebThread[] = [
  { id: 0, name: "Multi", live: true, pi: { state: "working", tool: "bash", ts: 1 } },
  { id: 4, name: "CI", live: true, pi: { state: "idle", reason: "idle", ts: 1 } },
  { id: 7, name: "Docs", live: false, pi: { state: "exited", reason: "exited", ts: 1 }, retryable: true },
];

describe("suggestThreadName", () => {
  it("counts from the tabs there are, skipping names in use", () => {
    expect(suggestThreadName([{ name: "Multi" }])).toBe("thread 2");
    expect(suggestThreadName(threads)).toBe("thread 4");
    expect(suggestThreadName([{ name: "a" }, { name: "thread 2" }])).toBe("thread 3");
  });
});

describe("ThreadTabs", () => {
  afterEach(() => {
    cleanup();
    openThread.mockReset();
    closeThread.mockReset();
    renameThread.mockReset();
  });

  it("shows one tab per thread with its state, the current one marked", () => {
    render(<ThreadTabs slug="multi" threads={threads} current={4} onSelect={() => {}} onToast={() => {}} />);
    const tabs = screen.getAllByRole("tab");
    expect(tabs.map((t) => t.querySelector(".thread-name")?.textContent)).toEqual(["Multi", "CI", "Docs"]);
    expect(tabs[1]).toHaveAttribute("aria-selected", "true");
    expect(tabs[0]).toHaveAttribute("aria-selected", "false");
    expect(tabs[0]!.querySelector(".thread-dot")).toHaveClass("status-working");
    expect(tabs[1]!.querySelector(".thread-dot")).toHaveClass("status-needs-you");
    expect(tabs[2]!.querySelector(".thread-dot")).toHaveClass("status-exited");
  });

  it("rings a tab that stopped since you last looked at it", () => {
    const ringed = threads.map((t) => (t.id === 4 ? { ...t, attention: true } : t));
    render(<ThreadTabs slug="multi" threads={ringed} current={0} onSelect={() => {}} onToast={() => {}} />);
    const tabs = screen.getAllByRole("tab");
    expect(tabs[1]).toHaveClass("attention");
    expect(tabs[1]!.getAttribute("title")).toMatch(/stopped since you last looked at it/);
    expect(tabs[0]).not.toHaveClass("attention");
    expect(tabs[2]).not.toHaveClass("attention");
  });

  it("selects a tab on click", () => {
    const onSelect = vi.fn();
    render(<ThreadTabs slug="multi" threads={threads} current={0} onSelect={onSelect} onToast={() => {}} />);
    fireEvent.click(screen.getByRole("tab", { name: /Docs/ }));
    expect(onSelect).toHaveBeenCalledWith(7);
  });

  it("opens a new thread from the current tab under the typed name and selects it", async () => {
    openThread.mockResolvedValue({ ok: true, thread: { id: 9, name: "Tests", live: true } });
    const onSelect = vi.fn();
    const onChanged = vi.fn();
    render(<ThreadTabs slug="multi" threads={threads} current={4} onSelect={onSelect} onChanged={onChanged} onToast={() => {}} />);
    fireEvent.click(screen.getByTitle(/new thread/i));
    const input = screen.getByPlaceholderText(/name of the thread/i);
    expect(input).toHaveValue("thread 4");
    fireEvent.change(input, { target: { value: "Tests" } });
    fireEvent.keyDown(input, { key: "Enter" });
    await waitFor(() => expect(openThread).toHaveBeenCalledWith("multi", 4, "Tests"));
    await waitFor(() => expect(onSelect).toHaveBeenCalledWith(9));
    expect(onChanged).toHaveBeenCalled();
    expect(screen.queryByPlaceholderText(/name of the thread/i)).toBeNull();
  });

  it("shows the daemon's refusal in the row and keeps it open", async () => {
    openThread.mockRejectedValue(new Error("dir is gone"));
    render(<ThreadTabs slug="multi" threads={threads} current={0} onSelect={() => {}} onToast={() => {}} />);
    fireEvent.click(screen.getByTitle(/new thread/i));
    fireEvent.keyDown(screen.getByPlaceholderText(/name of the thread/i), { key: "Enter" });
    await screen.findByText(/dir is gone/);
    expect(screen.getByPlaceholderText(/name of the thread/i)).toBeInTheDocument();
  });

  it("closes a thread at once, no question asked — the first tab too, while another is open", async () => {
    closeThread.mockResolvedValue({ ok: true });
    const onSelect = vi.fn();
    const onChanged = vi.fn();
    const confirm = vi.spyOn(window, "confirm");
    render(<ThreadTabs slug="multi" threads={threads} current={7} onSelect={onSelect} onChanged={onChanged} onToast={() => {}} />);
    fireEvent.click(screen.getByRole("tab", { name: /Docs/ }).querySelector(".thread-close")!);
    await waitFor(() => expect(closeThread).toHaveBeenCalledWith("multi", 7));
    expect(confirm).not.toHaveBeenCalled();
    expect(onSelect).toHaveBeenCalledWith(0);
    expect(onChanged).toHaveBeenCalled();
    fireEvent.click(screen.getByRole("tab", { name: /Multi/ }).querySelector(".thread-close")!);
    await waitFor(() => expect(closeThread).toHaveBeenCalledWith("multi", 0));
  });

  it("the last tab has no ✕", () => {
    render(<ThreadTabs slug="multi" threads={threads.slice(0, 1)} current={0} onSelect={() => {}} onToast={() => {}} />);
    expect(screen.getByRole("tab", { name: /Multi/ }).querySelector(".thread-close")).toBeNull();
  });

  it("renames a thread on double-click", async () => {
    renameThread.mockResolvedValue({ ok: true });
    const onChanged = vi.fn();
    render(<ThreadTabs slug="multi" threads={threads} current={4} onSelect={() => {}} onChanged={onChanged} onToast={() => {}} />);
    fireEvent.doubleClick(screen.getByRole("tab", { name: /CI/ }));
    const input = screen.getByDisplayValue("CI");
    fireEvent.change(input, { target: { value: "CI fixes" } });
    fireEvent.keyDown(input, { key: "Enter" });
    await waitFor(() => expect(renameThread).toHaveBeenCalledWith("multi", 4, "CI fixes"));
    expect(onChanged).toHaveBeenCalled();
  });
});

describe("ThreadTabs keys", () => {
  afterEach(() => {
    cleanup();
    openThread.mockReset();
    restoreThread.mockReset();
  });

  const press = (code: string, extra: Partial<KeyboardEventInit> = {}) =>
    fireEvent.keyDown(document, { code, key: code === "BracketRight" ? "}" : "{", metaKey: true, shiftKey: true, ...extra });

  it("⌘⇧] and ⌘⇧[ move to the next and previous tab, wrapping around", () => {
    const onSelect = vi.fn();
    render(<ThreadTabs slug="multi" threads={threads} current={4} onSelect={onSelect} onToast={() => {}} />);
    press("BracketRight");
    expect(onSelect).toHaveBeenLastCalledWith(7);
    press("BracketLeft");
    expect(onSelect).toHaveBeenLastCalledWith(0);
    cleanup();
    onSelect.mockClear();
    render(<ThreadTabs slug="multi" threads={threads} current={7} onSelect={onSelect} onToast={() => {}} />);
    press("BracketRight");
    expect(onSelect).toHaveBeenLastCalledWith(0);
    cleanup();
    onSelect.mockClear();
    render(<ThreadTabs slug="multi" threads={threads} current={0} onSelect={onSelect} onToast={() => {}} />);
    press("BracketLeft");
    expect(onSelect).toHaveBeenLastCalledWith(7);
  });

  it("⌘T opens a thread from the current tab under the default name, no box to fill, and selects it", async () => {
    openThread.mockResolvedValue({ ok: true, thread: { id: 9, name: "thread 4", live: true } });
    const onSelect = vi.fn();
    const onChanged = vi.fn();
    render(<ThreadTabs slug="multi" threads={threads} current={4} onSelect={onSelect} onChanged={onChanged} onToast={() => {}} />);
    fireEvent.keyDown(document, { key: "t", metaKey: true });
    await waitFor(() => expect(openThread).toHaveBeenCalledWith("multi", 4, "thread 4"));
    await waitFor(() => expect(onSelect).toHaveBeenCalledWith(9));
    expect(onChanged).toHaveBeenCalled();
    expect(screen.queryByPlaceholderText(/name of the thread/i)).toBeNull();
  });

  it("⌘⇧T brings back the tab closed last and selects it", async () => {
    restoreThread.mockResolvedValue({ ok: true, thread: { id: 9, name: "Tests", live: true } });
    const onSelect = vi.fn();
    const onChanged = vi.fn();
    render(<ThreadTabs slug="multi" threads={threads} current={4} onSelect={onSelect} onChanged={onChanged} onToast={() => {}} />);
    fireEvent.keyDown(document, { key: "T", metaKey: true, shiftKey: true });
    await waitFor(() => expect(restoreThread).toHaveBeenCalledWith("multi"));
    await waitFor(() => expect(onSelect).toHaveBeenCalledWith(9));
    expect(onChanged).toHaveBeenCalled();
    expect(openThread).not.toHaveBeenCalled();
  });

  it("⌘⇧T tells you when there is nothing to bring back, and ignores ⌥⌘⇧T and ⇧T", async () => {
    restoreThread.mockRejectedValue(new Error("job multi has no closed tab to bring back"));
    const onToast = vi.fn();
    render(<ThreadTabs slug="multi" threads={threads} current={0} onSelect={() => {}} onToast={onToast} />);
    fireEvent.keyDown(document, { key: "T", metaKey: true, shiftKey: true, altKey: true });
    fireEvent.keyDown(document, { key: "T", shiftKey: true });
    expect(restoreThread).not.toHaveBeenCalled();
    fireEvent.keyDown(document, { key: "T", metaKey: true, shiftKey: true });
    await waitFor(() => expect(onToast).toHaveBeenCalledWith("restore tab: job multi has no closed tab to bring back"));
  });

  it("⌘T tells you when the daemon refuses, and ignores ⌘⇧T, ⌥⌘T and plain T", async () => {
    openThread.mockRejectedValue(new Error("dir is gone"));
    restoreThread.mockResolvedValue({ ok: true, thread: { id: 9, name: "Tests", live: true } });
    const onToast = vi.fn();
    render(<ThreadTabs slug="multi" threads={threads} current={0} onSelect={() => {}} onToast={onToast} />);
    fireEvent.keyDown(document, { key: "T", metaKey: true, shiftKey: true });
    fireEvent.keyDown(document, { key: "t", metaKey: true, altKey: true });
    fireEvent.keyDown(document, { key: "t" });
    expect(openThread).not.toHaveBeenCalled();
    fireEvent.keyDown(document, { key: "t", metaKey: true });
    await waitFor(() => expect(onToast).toHaveBeenCalledWith("new thread: dir is gone"));
  });

  it("leaves the brackets alone without both ⌘ and ⇧, and does nothing with one tab", () => {
    const onSelect = vi.fn();
    render(<ThreadTabs slug="multi" threads={threads} current={0} onSelect={onSelect} onToast={() => {}} />);
    press("BracketRight", { shiftKey: false });
    press("BracketRight", { metaKey: false });
    press("BracketRight", { altKey: true });
    expect(onSelect).not.toHaveBeenCalled();
    cleanup();
    render(<ThreadTabs slug="multi" threads={threads.slice(0, 1)} current={0} onSelect={onSelect} onToast={() => {}} />);
    press("BracketRight");
    expect(onSelect).not.toHaveBeenCalled();
  });
});

describe("ThreadTabs drag", () => {
  afterEach(() => {
    cleanup();
    orderThreads.mockReset();
    vi.restoreAllMocks();
  });

  // jsdom lays nothing out: each tab is 100px wide, side by side, in the order shown
  const layOut = () =>
    vi.spyOn(HTMLElement.prototype, "getBoundingClientRect").mockImplementation(function (this: HTMLElement) {
      const tabs = [...document.querySelectorAll('[role="tab"]')];
      const at = tabs.indexOf(this);
      const left = at < 0 ? 0 : at * 100;
      const width = at < 0 ? 0 : 100;
      return { x: left, y: 0, left, top: 0, right: left + width, bottom: 20, width, height: 20, toJSON: () => ({}) } as DOMRect;
    });

  const drag = async (tab: HTMLElement, fromX: number, toX: number) => {
    fireEvent.mouseDown(tab, { button: 0, clientX: fromX, clientY: 10 });
    for (const x of [fromX + 10, (fromX + toX) / 2, toX]) {
      await act(async () => {
        fireEvent.mouseMove(document, { clientX: x, clientY: 10 });
      });
    }
    await act(async () => {
      fireEvent.mouseUp(document, { clientX: toX, clientY: 10 });
    });
  };

  it("moves a tab where it is dragged, the first tab too, and saves the order", async () => {
    layOut();
    orderThreads.mockResolvedValue({ ok: true });
    const onChanged = vi.fn();
    const onSelect = vi.fn();
    render(<ThreadTabs slug="multi" threads={threads} current={4} onSelect={onSelect} onChanged={onChanged} onToast={() => {}} />);
    await drag(screen.getByRole("tab", { name: /Multi/ }), 50, 250);
    await waitFor(() => expect(orderThreads).toHaveBeenCalledWith("multi", [4, 7, 0]));
    await waitFor(() => expect(onChanged).toHaveBeenCalled());
    expect(onSelect).not.toHaveBeenCalled();
  });

  it("shows the new order at once, and puts it back with the reason when the daemon refuses", async () => {
    layOut();
    let refuse: (e: Error) => void = () => {};
    orderThreads.mockReturnValue(new Promise((_, reject) => (refuse = reject)));
    const onToast = vi.fn();
    render(<ThreadTabs slug="multi" threads={threads} current={4} onSelect={() => {}} onToast={onToast} />);
    const names = () => screen.getAllByRole("tab").map((t) => t.querySelector(".thread-name")?.textContent);
    await drag(screen.getByRole("tab", { name: /Docs/ }), 250, 50);
    await waitFor(() => expect(orderThreads).toHaveBeenCalledWith("multi", [7, 0, 4]));
    expect(names()).toEqual(["Docs", "Multi", "CI"]);
    await act(async () => refuse(new Error("edited elsewhere")));
    await waitFor(() => expect(onToast).toHaveBeenCalledWith("move tab: edited elsewhere"));
    expect(names()).toEqual(["Multi", "CI", "Docs"]);
  });

  it("a click without a drag still selects the tab", async () => {
    // a drag just dropped swallows the click that ends it, for a moment
    await new Promise((r) => setTimeout(r, 60));
    layOut();
    const onSelect = vi.fn();
    render(<ThreadTabs slug="multi" threads={threads} current={0} onSelect={onSelect} onToast={() => {}} />);
    const tab = screen.getByRole("tab", { name: /Docs/ });
    fireEvent.mouseDown(tab, { button: 0, clientX: 250, clientY: 10 });
    fireEvent.mouseUp(document, { clientX: 250, clientY: 10 });
    fireEvent.click(tab);
    expect(onSelect).toHaveBeenCalledWith(7);
    expect(orderThreads).not.toHaveBeenCalled();
  });
});
