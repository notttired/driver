import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { JobForm } from "../../lib/types.ts";

const { forkJob } = vi.hoisted(() => ({
  forkJob: vi.fn<(body: { slug: string; to: string; name: string }) => Promise<JobForm>>(),
}));
vi.mock("../../api/client.ts", () => ({ forkJob }));

import { ForkButton, ForkRow, suggestForkName, useForkControl, type ForkControlState } from "./ForkControl.tsx";

const form = (slug: string, name: string): JobForm => ({
  slug,
  name,
  dir: "/work",
  model: "",
  color: "",
  notes: "",
  state: "working",
  editable: false,
  archived: false,
  mtime: 1,
  links: [],
});

function Harness({ onForked, before }: { onForked: (slug: string, name: string) => void; before?: () => Promise<void> }) {
  const state = useForkControl("abacus", "Abacus fork", onForked, { beforeSubmit: before });
  return (
    <>
      <ForkButton state={state} visible />
      <ForkRow state={state} column="working" />
    </>
  );
}

const stateStub = (over: Partial<ForkControlState>): ForkControlState => ({
  slug: "abacus",
  open: true,
  value: "Abacus fork",
  preview: "abacus-fork",
  error: "",
  busy: false,
  begin: () => {},
  cancel: () => {},
  setValue: () => {},
  submit: () => {},
  ...over,
});

describe("suggestForkName", () => {
  it("adds ' fork', counts up past taken slugs and does not stack forks", () => {
    const taken = (s: string) => s === "abacus-fork" || s === "abacus-fork-2";
    expect(suggestForkName("Abacus", () => false)).toBe("Abacus fork");
    expect(suggestForkName("Abacus", taken)).toBe("Abacus fork 3");
    expect(suggestForkName("Abacus fork", () => false)).toBe("Abacus fork");
    expect(suggestForkName("Abacus fork 2", () => false)).toBe("Abacus fork");
    expect(suggestForkName("  ", () => false)).toBe("job fork");
  });
});

describe("ForkRow", () => {
  afterEach(cleanup);

  it("previews the copy's slug and column, and shows the server's refusal", () => {
    const { rerender } = render(<ForkRow state={stateStub({})} column="blocked" />);
    expect(screen.getByText("abacus-fork")).toHaveClass("slug");
    expect(screen.getByText(/beside it in BLOCKED/)).toBeInTheDocument();
    rerender(<ForkRow state={stateStub({ error: 'slug "abacus-fork" is taken' })} column="blocked" />);
    expect(screen.getByText(/✗ slug "abacus-fork" is taken/)).toHaveClass("err");
    rerender(<ForkRow state={stateStub({ value: "!!!", preview: "" })} column="blocked" />);
    expect(screen.getByText(/needs at least one letter or digit/)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "⑂ fork" })).toBeDisabled();
    rerender(<ForkRow state={stateStub({ open: false })} column="blocked" />);
    expect(screen.queryByPlaceholderText("name of the copy")).toBeNull();
  });

  it("submits on Enter and cancels on Escape", () => {
    const submit = vi.fn();
    const cancel = vi.fn();
    render(<ForkRow state={stateStub({ submit, cancel })} column="draft" />);
    const input = screen.getByPlaceholderText("name of the copy");
    fireEvent.keyDown(input, { key: "Enter" });
    fireEvent.keyDown(input, { key: "Escape" });
    expect(submit).toHaveBeenCalledTimes(1);
    expect(cancel).toHaveBeenCalledTimes(1);
  });
});

describe("useForkControl", () => {
  afterEach(() => {
    cleanup();
    forkJob.mockReset();
  });

  it("opens on the suggested name, flushes, posts slug/to/name and reports the copy", async () => {
    const onForked = vi.fn();
    const before = vi.fn(() => Promise.resolve());
    forkJob.mockResolvedValue(form("abacus-fork", "Abacus fork"));
    render(<Harness onForked={onForked} before={before} />);
    expect(screen.queryByPlaceholderText("name of the copy")).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "⑂ fork" }));
    const input = screen.getByPlaceholderText("name of the copy") as HTMLInputElement;
    expect(input.value).toBe("Abacus fork");
    // the head button hides while the row is open
    expect(screen.getAllByRole("button", { name: "⑂ fork" })).toHaveLength(1);
    fireEvent.change(input, { target: { value: " Abacus  v2 " } });
    expect(screen.getByText("abacus-v2")).toHaveClass("slug");
    fireEvent.keyDown(input, { key: "Enter" });
    await waitFor(() => expect(onForked).toHaveBeenCalledWith("abacus-fork", "Abacus fork"));
    expect(before).toHaveBeenCalledTimes(1);
    expect(forkJob).toHaveBeenCalledWith({ slug: "abacus", to: "abacus-v2", name: "Abacus  v2" });
    expect(screen.queryByPlaceholderText("name of the copy")).toBeNull();
  });

  it("keeps the row open with the server's reason when the fork is refused", async () => {
    const onForked = vi.fn();
    forkJob.mockRejectedValue(new Error('slug "abacus-fork" is taken'));
    render(<Harness onForked={onForked} />);
    fireEvent.click(screen.getByRole("button", { name: "⑂ fork" }));
    fireEvent.keyDown(screen.getByPlaceholderText("name of the copy"), { key: "Enter" });
    expect(await screen.findByText(/is taken/)).toHaveClass("err");
    expect(onForked).not.toHaveBeenCalled();
    expect(screen.getByPlaceholderText("name of the copy")).toBeInTheDocument();
  });
});
