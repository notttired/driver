import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

const { fetchSettings, probeFolders, probeBrowser, openSystemPane } = vi.hoisted(() => ({
  fetchSettings: vi.fn(),
  probeFolders: vi.fn(),
  probeBrowser: vi.fn(),
  openSystemPane: vi.fn(),
}));
vi.mock("../../api/client.ts", () => ({ fetchSettings, probeFolders, probeBrowser, openSystemPane }));

import { SettingsOverlay } from "./SettingsOverlay.tsx";

describe("SettingsOverlay", () => {
  afterEach(() => {
    cleanup();
    fetchSettings.mockReset();
    probeFolders.mockReset();
    probeBrowser.mockReset();
    openSystemPane.mockReset();
  });

  it("shows the install, asks for folder access on request and reports each folder", async () => {
    fetchSettings.mockResolvedValue({ version: "abc123", binary: "/Users/me/bin/driver", home: "/Users/me/.driver", browser: "Google Chrome", serve: "http://127.0.0.1:7781" });
    probeFolders.mockResolvedValue({ folders: [{ name: "Desktop", state: "ok" }, { name: "Documents", state: "denied" }, { name: "Downloads", state: "ok" }] });
    openSystemPane.mockResolvedValue({ ok: true });
    render(<SettingsOverlay open onClose={() => {}} onToast={() => {}} />);
    expect(await screen.findByText("/Users/me/bin/driver")).toBeInTheDocument();
    expect(screen.getByText(/abc123/)).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: /ask now/i }));
    await waitFor(() => expect(probeFolders).toHaveBeenCalled());
    expect(await screen.findByText("Documents")).toBeInTheDocument();
    expect(screen.getByText("Documents").closest("li")).toHaveTextContent(/denied/);
    expect(screen.getByText("Desktop").closest("li")).toHaveTextContent(/ok/);
    fireEvent.click(screen.getByRole("button", { name: /full disk access/i }));
    await waitFor(() => expect(openSystemPane).toHaveBeenCalledWith("full-disk-access"));
    fireEvent.click(screen.getByRole("button", { name: /show the binary/i }));
    await waitFor(() => expect(openSystemPane).toHaveBeenCalledWith("binary"));
  });

  it("tests the browser read and explains what is missing", async () => {
    fetchSettings.mockResolvedValue({ version: "v", binary: "/b", home: "/h", browser: "Google Chrome", serve: "u" });
    probeBrowser.mockResolvedValueOnce({ ok: false, error: "macOS has not let driver read Google Chrome — allow it under System Settings" });
    render(<SettingsOverlay open onClose={() => {}} onToast={() => {}} />);
    await screen.findByText("/b");
    fireEvent.click(screen.getByRole("button", { name: /test/i }));
    expect(await screen.findByText(/has not let driver read/)).toBeInTheDocument();
    probeBrowser.mockResolvedValueOnce({ ok: true, title: "A page", url: "https://x.io", text: false });
    fireEvent.click(screen.getByRole("button", { name: /test/i }));
    expect(await screen.findByText(/A page/)).toBeInTheDocument();
    expect(screen.getByText(/A page/)).toHaveTextContent(/URL and title only/);
    probeBrowser.mockResolvedValueOnce({ ok: true, title: "A page", url: "https://x.io", text: true });
    fireEvent.click(screen.getByRole("button", { name: /test/i }));
    await waitFor(() => expect(screen.getByText(/page text came through/)).toBeInTheDocument());
  });

  it("renders nothing while closed", () => {
    render(<SettingsOverlay open={false} onClose={() => {}} onToast={() => {}} />);
    expect(screen.queryByText(/SETTINGS/)).toBeNull();
  });
});
