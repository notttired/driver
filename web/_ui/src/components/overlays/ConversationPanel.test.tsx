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

  it("renders pi's answers as markdown and leaves yours as typed", async () => {
    fetchConversation.mockResolvedValue({
      rev: "3",
      pending: false,
      messages: [
        { role: "user", text: "**not bold** <keep>", ts: 1793181600 },
        {
          role: "assistant",
          ts: 1793181660,
          text: [
            "## Done",
            "",
            "- **bold** item",
            "- `code`, [docs](https://x.io/d) and [bad](javascript:alert(1))",
            "",
            "```sh",
            "ls -la",
            "```",
            "",
            "| a | b |",
            "|---|---|",
            "| 1 | 2 |",
            "",
            "Use <slug> here",
            "next line",
          ].join("\n"),
        },
      ],
    });
    const { container } = render(<ConversationPanel slug="demo" opened pi={{ state: "ready", ts: 1 }} />);
    expect(await screen.findByRole("heading", { level: 2, name: "Done" })).toBeInTheDocument();
    expect(container.querySelectorAll(".convo-md li")).toHaveLength(2);
    expect(container.querySelector(".convo-md strong")?.textContent).toBe("bold");
    expect(container.querySelector(".convo-md pre code")?.textContent).toBe("ls -la\n");
    const docs = screen.getByRole("link", { name: "docs" });
    expect(docs).toHaveAttribute("href", "https://x.io/d");
    expect(docs).toHaveAttribute("target", "_blank");
    // an unsafe target is not a link at all, just its text
    expect(screen.queryByRole("link", { name: "bad" })).toBeNull();
    expect(screen.getByText(/and bad$/)).toBeInTheDocument();
    expect(screen.getByRole("table")).toBeInTheDocument();
    expect(screen.getByRole("cell", { name: "2" })).toBeInTheDocument();
    // raw HTML is text, not markup; a single newline is a line break
    expect(screen.getByText(/Use <slug> here/)).toBeInTheDocument();
    expect(container.querySelectorAll(".convo-md br")).toHaveLength(1);
    // your own rows are verbatim
    expect(screen.getByText("**not bold** <keep>")).toBeInTheDocument();
    expect(container.querySelectorAll(".convo-msg.you strong")).toHaveLength(0);
  });
});
