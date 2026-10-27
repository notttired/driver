import { useState } from "react";

import { addJobLink, fetchTabs, removeJobLink } from "../../api/client.ts";
import { isHttpUrl, linkLabel } from "../../lib/links.ts";
import type { BrowserTab, TabsResponse, WebLink } from "../../lib/types.ts";
import { LinkChip } from "../board/chips/LinkChips.tsx";

export interface LinksPanelProps {
  slug: string;
  links: WebLink[];
  /** the server owns the list; refetch after any write */
  onChanged: () => void;
}

const errorText = (e: unknown) => (e instanceof Error ? e.message : String(e));

function urlHost(url: string): string {
  try {
    return new URL(url).host.replace(/^www\./, "");
  } catch {
    return "";
  }
}

/** A tab's title makes a decent label; keep it short enough for a chip. */
const tabLabel = (t: BrowserTab) => t.title.trim().slice(0, 80) || undefined;

/**
 * The sidebar's link list: add by URL (label optional), remove, PR status,
 * and a picker over the browser's open tabs (GET /api/tabs) so a page you
 * are looking at can be attached with one click.
 */
export function LinksPanel({ slug, links, onChanged }: LinksPanelProps) {
  const [url, setUrl] = useState("");
  const [label, setLabel] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [picker, setPicker] = useState(false);
  const [tabs, setTabs] = useState<TabsResponse | null>(null);
  const [tabsBusy, setTabsBusy] = useState(false);
  const [tabsError, setTabsError] = useState("");

  const canAdd = !busy && isHttpUrl(url.trim());

  const attach = async (href: string, name?: string) => {
    setBusy(true);
    setError("");
    try {
      await addJobLink(slug, href, name);
      onChanged();
      return true;
    } catch (e: unknown) {
      setError(errorText(e));
      return false;
    } finally {
      setBusy(false);
    }
  };

  const add = async () => {
    if (!canAdd) return;
    if (await attach(url.trim(), label.trim() || undefined)) {
      setUrl("");
      setLabel("");
    }
  };

  const remove = async (id: number) => {
    setBusy(true);
    setError("");
    try {
      await removeJobLink(slug, id);
      onChanged();
    } catch (e: unknown) {
      setError(errorText(e));
    } finally {
      setBusy(false);
    }
  };

  const loadTabs = async () => {
    setTabsBusy(true);
    setTabsError("");
    try {
      setTabs(await fetchTabs());
    } catch (e: unknown) {
      setTabsError(errorText(e));
    } finally {
      setTabsBusy(false);
    }
  };

  const togglePicker = () => {
    const next = !picker;
    setPicker(next);
    if (next) void loadTabs();
  };

  const linked = new Set(links.map((l) => l.url));

  return (
    <section className="side-panel links-panel">
      <div className="side-head">
        <b>LINKS</b>
        <span className="side-hint">{links.length ? String(links.length) : ""}</span>
      </div>
      {links.length ? (
        <ul className="link-list">
          {links.map((l) => (
            <li key={l.id} className="link-row">
              <LinkChip link={l} />
              <span className="link-text" title={l.url}>
                {l.url.replace(/^https?:\/\//, "")}
              </span>
              {l.kind === "github_pr" && l.status ? <span className={`pr-status pr-${l.status}`}>{l.status}</span> : null}
              <button
                className="btn ghost link-remove"
                title="remove this link"
                aria-label={`remove link ${linkLabel(l)}`}
                disabled={busy}
                onClick={() => void remove(l.id)}
              >
                ✕
              </button>
            </li>
          ))}
        </ul>
      ) : (
        <div className="link-empty">no links yet — PRs the daemon spots land here</div>
      )}
      <div className="link-add">
        <input
          className="link-input"
          type="url"
          placeholder="https://…"
          aria-label="link url"
          spellCheck={false}
          value={url}
          onChange={(e) => {
            setUrl(e.target.value);
            setError("");
          }}
          onKeyDown={(e) => {
            if (e.key === "Enter") {
              e.preventDefault();
              void add();
            }
          }}
        />
        <input
          className="link-input label"
          type="text"
          placeholder="label (optional)"
          aria-label="link label"
          spellCheck={false}
          value={label}
          onChange={(e) => setLabel(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "Enter") {
              e.preventDefault();
              void add();
            }
          }}
        />
        <button className="btn" disabled={!canAdd} onClick={() => void add()}>
          + add
        </button>
        <button
          className={"btn" + (picker ? " on" : "")}
          id="tabs-toggle"
          title="pick from the browser's open tabs"
          aria-expanded={picker}
          onClick={togglePicker}
        >
          ⊞ tabs
        </button>
      </div>
      {error ? <div className="side-hint err">✗ {error}</div> : null}
      {picker ? (
        <div className="tab-picker" id="tab-picker">
          <div className="side-head">
            <b>OPEN TABS</b>
            <span className="side-hint">
              {tabsBusy ? "asking…" : tabs ? `${tabs.browser}${tabs.running ? ` · ${tabs.tabs.length}` : ""}` : ""}
            </span>
            <button className="btn ghost" title="refresh the tab list" aria-label="refresh tabs" disabled={tabsBusy} onClick={() => void loadTabs()}>
              ↻
            </button>
          </div>
          {tabsError ? <div className="side-hint err">✗ {tabsError}</div> : null}
          {tabs && !tabs.running ? <div className="link-empty">{tabs.browser} is not running</div> : null}
          {tabs?.running && tabs.tabs.length === 0 ? <div className="link-empty">no http(s) tabs open</div> : null}
          {tabs?.tabs.length ? (
            <ul className="tab-list">
              {tabs.tabs.map((t) => {
                const already = linked.has(t.url);
                return (
                  <li key={`${t.window}:${t.index}`}>
                    <button
                      className={"tab-row" + (already ? " linked" : "") + (t.active ? " active" : "")}
                      title={t.url}
                      disabled={busy || already}
                      onClick={() => void attach(t.url, tabLabel(t))}
                    >
                      <span className="tab-dot" aria-hidden="true">
                        {t.active ? "●" : "○"}
                      </span>
                      <span className="tab-title">{t.title || t.url}</span>
                      <span className="tab-host">{urlHost(t.url)}</span>
                      {already ? <span className="tab-linked">linked</span> : null}
                    </button>
                  </li>
                );
              })}
            </ul>
          ) : null}
        </div>
      ) : null}
    </section>
  );
}
