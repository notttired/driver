import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

const { fetchJobNotes, saveJobNotes } = vi.hoisted(() => ({
  fetchJobNotes: vi.fn<(slug: string, signal?: AbortSignal) => Promise<{ text: string }>>(),
  saveJobNotes: vi.fn<(slug: string, text: string) => Promise<{ ok: true }>>(),
}));
vi.mock("../../api/client.ts", () => ({ fetchJobNotes, saveJobNotes }));

import { NotesPanel } from "./NotesPanel.tsx";

const deferred = <T,>() => {
  let resolve!: (v: T) => void;
  const promise = new Promise<T>((r) => (resolve = r));
  return { promise, resolve };
};

describe("NotesPanel", () => {
  afterEach(() => {
    cleanup();
    fetchJobNotes.mockReset();
    saveJobNotes.mockReset();
  });

  it("shows what pi wrote to the notes while they are open, without saving it back", async () => {
    fetchJobNotes.mockResolvedValue({ text: "mine\n\nfrom pi" });
    render(<NotesPanel slug="demo" initial="mine" />);
    await waitFor(() => expect(screen.getByRole("textbox")).toHaveValue("mine\n\nfrom pi"));
    expect(fetchJobNotes).toHaveBeenCalledWith("demo", expect.anything());
    await new Promise((r) => setTimeout(r, 700));
    expect(saveJobNotes).not.toHaveBeenCalled();

    saveJobNotes.mockResolvedValue({ ok: true });
    fireEvent.change(screen.getByRole("textbox"), { target: { value: "mine\n\nfrom pi!" } });
    await waitFor(() => expect(saveJobNotes).toHaveBeenCalledWith("demo", "mine\n\nfrom pi!"));
  });

  it("keeps what you are typing over a change that arrives meanwhile", async () => {
    const poll = deferred<{ text: string }>();
    fetchJobNotes.mockReturnValue(poll.promise);
    saveJobNotes.mockResolvedValue({ ok: true });
    render(<NotesPanel slug="demo" initial="mine" />);
    fireEvent.change(screen.getByRole("textbox"), { target: { value: "mine, more" } });
    poll.resolve({ text: "mine\n\nfrom pi" });
    await waitFor(() => expect(saveJobNotes).toHaveBeenCalledWith("demo", "mine, more"));
    expect(screen.getByRole("textbox")).toHaveValue("mine, more");
  });
});
