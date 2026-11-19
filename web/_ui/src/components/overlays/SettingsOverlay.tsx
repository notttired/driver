import { useEffect, useState } from "react";

import { fetchSettings, openSystemPane, probeBrowser, probeFolders } from "../../api/client.ts";
import { ESC_PRIORITY } from "../../hooks/useEscapeStack.ts";
import type { BrowserProbeResult, FolderProbe, SettingsInfo } from "../../lib/types.ts";
import { Overlay } from "./Overlay.tsx";

export interface SettingsOverlayProps {
  open: boolean;
  onClose: () => void;
  onToast: (message: string) => void;
}

/**
 * ⚙: what this install needs from macOS, asked for from here rather than
 * met mid-task. Every pi a job runs is driver's child to macOS, so the
 * permissions are driver's: the protected folders (asked for by reading
 * them from the daemon, which puts the prompts up now, in driver's name,
 * or granted for good with Full Disk Access) and the browser (Automation,
 * plus the browser's own switch for JavaScript over Apple events, which
 * ⌘⇧V's page text needs). Plus where things are.
 */
export function SettingsOverlay({ open, onClose, onToast }: SettingsOverlayProps) {
  return (
    <Overlay id="settings-overlay" open={open} priority={ESC_PRIORITY.job} onClose={onClose} panelClassName="new-panel settings-panel">
      <SettingsBody onClose={onClose} onToast={onToast} />
    </Overlay>
  );
}

function SettingsBody({ onClose, onToast }: Omit<SettingsOverlayProps, "open">) {
  const [info, setInfo] = useState<SettingsInfo | null>(null);
  const [folders, setFolders] = useState<FolderProbe[] | null>(null);
  const [asking, setAsking] = useState(false);
  const [browser, setBrowser] = useState<BrowserProbeResult | null>(null);
  const [testing, setTesting] = useState(false);

  useEffect(() => {
    const ac = new AbortController();
    fetchSettings(ac.signal)
      .then(setInfo)
      .catch((e: unknown) => {
        if (!ac.signal.aborted) onToast(`settings: ${e instanceof Error ? e.message : String(e)}`);
      });
    return () => ac.abort();
  }, [onToast]);

  const fail = (what: string) => (e: unknown) => onToast(`${what}: ${e instanceof Error ? e.message : String(e)}`);
  const open = (pane: Parameters<typeof openSystemPane>[0]) => () => void openSystemPane(pane).catch(fail("open"));
  const askFolders = () => {
    setAsking(true);
    probeFolders()
      .then((r) => setFolders(r.folders))
      .catch(fail("folders"))
      .finally(() => setAsking(false));
  };
  const testBrowser = () => {
    setTesting(true);
    probeBrowser()
      .then(setBrowser)
      .catch(fail("browser"))
      .finally(() => setTesting(false));
  };
  const browserName = info?.browser ?? "the browser";

  return (
    <>
      <div className="panel-head">
        <span className="panel-title">SETTINGS</span>
        <span className="spacer" />
        <button className="btn ghost" title="close (Esc)" onClick={onClose}>
          ✕
        </button>
      </div>
      <div className="settings-body">
        <section className="settings-section">
          <h3>Paste context — ⌘⇧V</h3>
          <p>
            In a job, ⌘⇧V puts your front {browserName} tab into the pane: its URL and title, and the text you have selected on
            it — or the whole page's text when nothing is selected. Nothing is sent; add your question and press Enter.
          </p>
          <p>Two things have to be allowed once:</p>
          <ol>
            <li>
              macOS: driver may control {browserName} — <b>Automation</b>. The first read asks; or allow it in the pane below.
            </li>
            <li>
              {browserName}: <b>View › Developer › Allow JavaScript from Apple Events</b>, for the page's text. Without it ⌘⇧V
              still pastes the URL and title.
            </li>
          </ol>
          <div className="settings-actions">
            <button className="btn primary" disabled={testing} onClick={testBrowser}>
              {testing ? <span className="submit-spinner" aria-hidden="true" /> : null}⚡ test
            </button>
            <button className="btn" onClick={open("automation")}>
              open Automation
            </button>
          </div>
          {browser ? (
            browser.ok ? (
              <p className="settings-result ok">
                ✓ read “{browser.title}”
                {browser.text
                  ? browser.selected
                    ? " — the selected text came through"
                    : " — the page text came through"
                  : ` — URL and title only: turn on Allow JavaScript from Apple Events in ${browserName}'s View › Developer menu for the text`}
              </p>
            ) : (
              <p className="settings-result err">✗ {browser.error}</p>
            )
          ) : null}
        </section>

        <section className="settings-section">
          <h3>Your Desktop, Documents and Downloads</h3>
          <p>
            Every pi a job runs is driver's child to macOS, so the first time one reads a protected folder, macOS asks whether
            driver may — a dialog over whatever you were doing. Ask now instead: the daemon reads each folder and macOS puts
            up its questions here, once per folder. Or grant <b>Full Disk Access</b> once and cover them all; the binary is
            signed so the grant holds across installs.
          </p>
          <div className="settings-actions">
            <button className="btn primary" disabled={asking} onClick={askFolders}>
              {asking ? <span className="submit-spinner" aria-hidden="true" /> : null}⚑ ask now
            </button>
            <button className="btn" onClick={open("full-disk-access")}>
              open Full Disk Access
            </button>
            <button className="btn" onClick={open("binary")}>
              show the binary
            </button>
          </div>
          {folders ? (
            <ul className="settings-folders">
              {folders.map((f) => (
                <li key={f.name} className={`state-${f.state}`}>
                  <span className="settings-folder">{f.name}</span>
                  <span className="settings-state">{f.state === "ok" ? "✓ ok" : f.state === "denied" ? "✗ denied — allow it when asked, or grant Full Disk Access" : "— no such folder"}</span>
                </li>
              ))}
            </ul>
          ) : null}
        </section>

        <section className="settings-section settings-about">
          <h3>This install</h3>
          {info ? (
            <dl>
              <dt>version</dt>
              <dd>{info.version}</dd>
              <dt>binary</dt>
              <dd>{info.binary}</dd>
              <dt>home</dt>
              <dd>{info.home}</dd>
              <dt>browser</dt>
              <dd>{info.browser}</dd>
              <dt>serving</dt>
              <dd>{info.serve}</dd>
            </dl>
          ) : (
            <p className="settings-result">loading…</p>
          )}
        </section>
      </div>
    </>
  );
}
