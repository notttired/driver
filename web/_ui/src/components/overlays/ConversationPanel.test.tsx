import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { WebConversation } from "../../lib/types.ts";

const { fetchConversation } = vi.hoisted(() => ({
  fetchConversation: vi.fn<(slug: string, signal?: AbortSignal) => Promise<WebConversation>>(),
}));
vi.mock("../../api/client.ts", () => ({ fetchConversation }));

import { ConversationPanel } from "./ConversationPanel.tsx";

describe("ConversationPanel", () => {
  afterEach(() => {
    cleanup();
    fetchConversation.mockReset();
  });

  it("lists your messages and pi's final answers, links the URLs, and shows the turn in progress", async () => {
    fetchConversation.mockResolvedValue({
      rev: "1",
      pending: true,
      messages: [
        { role: "user", text: "What is in fruit.txt?", ts: 1793181600 },
        { role: "assistant", text: "One word: pineapple. See https://example.com/x).", ts: 1793181660 },
        { role: "user", text: "Rename it.", ts: 1793181700 },
        { role: "assistant", text: "", end: "aborted", ts: 1793181710 },
        { role: "user", text: "Again.", ts: 1793181720 },
      ],
    });
    render(<ConversationPanel slug="demo" opened pi={{ state: "working", ts: 1 }} />);
    expect(await screen.findByText("What is in fruit.txt?")).toBeInTheDocument();
    expect(screen.getByText(/One word: pineapple/)).toBeInTheDocument();
    const a = screen.getByRole("link", { name: "https://example.com/x" });
    expect(a).toHaveAttribute("href", "https://example.com/x");
    expect(a).toHaveAttribute("target", "_blank");
    expect(screen.getByText("— interrupted")).toBeInTheDocument();
    expect(screen.getByText("working…")).toBeInTheDocument();
    expect(screen.getByText("3 turns")).toBeInTheDocument();
    expect(screen.getAllByText("you")).toHaveLength(3);
    expect(fetchConversation).toHaveBeenCalledWith("demo", expect.anything());
  });

  it("says when pi stopped without answering, and reads nothing before the job is opened", async () => {
    fetchConversation.mockResolvedValue({ rev: "2", pending: true, messages: [{ role: "user", text: "Hi" }] });
    render(<ConversationPanel slug="demo" opened pi={{ state: "aborted", ts: 1 }} />);
    expect(await screen.findByText("stopped before answering")).toBeInTheDocument();
    cleanup();

    fetchConversation.mockClear();
    render(<ConversationPanel slug="demo" opened={false} />);
    expect(screen.getByText(/the record starts with the first message/)).toBeInTheDocument();
    await new Promise((r) => setTimeout(r, 0));
    expect(fetchConversation).not.toHaveBeenCalled();
  });

  it("shows the empty hint for an opened job with nothing said yet", async () => {
    fetchConversation.mockResolvedValue({ rev: "0", pending: false, messages: [] });
    render(<ConversationPanel slug="demo" opened pi={{ state: "ready", ts: 1 }} />);
    expect(await screen.findByText(/nothing yet/)).toBeInTheDocument();
  });
});
