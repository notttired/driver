import { useEffect, useRef, useState } from "react";

import { closeThread, openThread, renameThread } from "../../api/client.ts";
import { threadStatusToken } from "../../lib/status.ts";
import type { WebThread } from "../../lib/types.ts";

export interface ThreadTabsProps {
  slug: string;
  /** the job's tabs, its own pi first (WebJob.threads) */
  threads: WebThread[];
  /** the tab shown: a thread id, 0 for the job's own pi */
  current: number;
  onSelect: (id: number) => void;
  /** a thread was opened, closed or renamed: the board should refetch
   *  (awaited before a new thread's tab is selected, so the tab exists) */
  onChanged?: () => Promise<unknown> | void;
  onToast: (message: string) => void;
}

/** "thread N" for the Nth tab, skipping names in use. */
export function suggestThreadName(threads: readonly Pick<WebThread, "name">[]): string {
  const taken = new Set(threads.map((t) => t.name.toLowerCase()));
  for (let n = threads.length + 1; ; n++) {
    const name = `thread ${n}`;
    if (!taken.has(name)) return name;
  }
}

/**
 * The tabs above a job's pane: one per pi — the job's own first, then every
 * open thread — each with a dot for its state. `+` opens a thread from the
 * current tab (a copy of its conversation under a name you type); ✕ on a
 * thread's tab closes its pi (the conversation stays with the job); a
 * double-click renames it.
 */
export function ThreadTabs({ slug, threads, current, onSelect, onChanged, onToast }: ThreadTabsProps) {
  const [adding, setAdding] = useState(false);
  const [name, setName] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const [renaming, setRenaming] = useState<{ id: number; value: string } | null>(null);
  const inFlight = useRef(false);

  // another job's tabs must not carry a half-typed name along
  useEffect(() => {
    setAdding(false);
    setError("");
    setRenaming(null);
  }, [slug]);

  // ⌘⇧] / ⌘⇧[: the next / previous tab, round the end. On the document, so
  // the pane (which leaves ⌘-combinations to the page) and the sidebar both
  // hear it; by key code, so the brackets are the brackets on any layout.
  useEffect(() => {
    if (threads.length < 2) return;
    const onKeyDown = (e: KeyboardEvent) => {
      if (!e.metaKey || !e.shiftKey || e.altKey || e.ctrlKey) return;
      const step = e.code === "BracketRight" ? 1 : e.code === "BracketLeft" ? -1 : 0;
      if (!step) return;
      e.preventDefault();
      const at = Math.max(0, threads.findIndex((t) => t.id === current));
      onSelect(threads[(at + step + threads.length) % threads.length]!.id);
    };
    document.addEventListener("keydown", onKeyDown);
    return () => document.removeEventListener("keydown", onKeyDown);
  }, [threads, current, onSelect]);

  const begin = () => {
    setName(suggestThreadName(threads));
    setError("");
    setAdding(true);
  };
  const submit = () => {
    const trimmed = name.trim();
    if (!trimmed || inFlight.current) return;
    inFlight.current = true;
    setBusy(true);
    setError("");
    void (async () => {
      try {
        const res = await openThread(slug, current, trimmed);
        setAdding(false);
        await onChanged?.();
        onSelect(res.thread.id);
      } catch (e: unknown) {
        setError(e instanceof Error ? e.message : String(e));
      } finally {
        inFlight.current = false;
        setBusy(false);
      }
    })();
  };
  const close = (t: WebThread) => {
    if (!window.confirm(`Close ${t.name}? Its pi is closed; what it said stays with the job.`)) return;
    void closeThread(slug, t.id)
      .then(() => {
        if (current === t.id) onSelect(0);
        onChanged?.();
      })
      .catch((e: unknown) => onToast(`${t.name}: ${e instanceof Error ? e.message : String(e)}`));
  };
  const rename = () => {
    if (!renaming) return;
    const value = renaming.value.trim();
    const was = threads.find((t) => t.id === renaming.id)?.name;
    setRenaming(null);
    if (!value || value === was) return;
    void renameThread(slug, renaming.id, value)
      .then(() => onChanged?.())
      .catch((e: unknown) => onToast(`rename: ${e instanceof Error ? e.message : String(e)}`));
  };

  return (
    <div className="thread-tabs" data-no-drag>
      <div className="thread-row" role="tablist" aria-label="threads">
        {threads.map((t) => {
          const tok = threadStatusToken(t);
          const selected = t.id === current;
          return (
            <button
              key={t.id}
              role="tab"
              type="button"
              className={"thread-tab" + (selected ? " selected" : "") + (t.live ? "" : " dead")}
              aria-selected={selected}
              title={`${t.name} — ${tok.text}${tok.detail ? ` ${tok.detail}` : ""}${t.id ? "\ndouble-click to rename · ✕ closes its pi" : ""}${threads.length > 1 ? "\n⌘⇧] / ⌘⇧[ next / previous tab" : ""}`}
              onClick={() => onSelect(t.id)}
              onDoubleClick={() => {
                if (t.id) setRenaming({ id: t.id, value: t.name });
              }}
            >
              <span className={`thread-dot status-${tok.variant}`} aria-hidden="true" />
              {renaming?.id === t.id ? (
                <input
                  className="thread-rename"
                  autoFocus
                  value={renaming.value}
                  spellCheck={false}
                  onClick={(e) => e.stopPropagation()}
                  onChange={(e) => setRenaming({ id: t.id, value: e.target.value })}
                  onBlur={rename}
                  onKeyDown={(e) => {
                    if (e.key === "Enter") {
                      e.preventDefault();
                      rename();
                    } else if (e.key === "Escape") {
                      e.preventDefault();
                      e.stopPropagation();
                      setRenaming(null);
                    }
                  }}
                />
              ) : (
                <span className="thread-name">{t.name}</span>
              )}
              {t.id ? (
                <span
                  className="thread-close"
                  role="button"
                  aria-label={`close ${t.name}`}
                  onClick={(e) => {
                    e.stopPropagation();
                    close(t);
                  }}
                >
                  ✕
                </span>
              ) : null}
            </button>
          );
        })}
        <button
          type="button"
          className="thread-add"
          title="new thread — another pi on this job, starting from a copy of this tab's conversation"
          hidden={adding}
          onClick={begin}
        >
          +
        </button>
      </div>
      {adding ? (
        <div className="rename-row thread-new-row">
          <div className="rename-line">
            <input
              className="rename-input"
              placeholder="name of the thread"
              spellCheck={false}
              autoFocus
              value={name}
              disabled={busy}
              onChange={(e) => {
                setName(e.target.value);
                setError("");
              }}
              onKeyDown={(e) => {
                if (e.key === "Enter") {
                  e.preventDefault();
                  submit();
                } else if (e.key === "Escape") {
                  e.preventDefault();
                  e.stopPropagation();
                  setAdding(false);
                }
              }}
            />
            <button className="btn primary" disabled={!name.trim() || busy} onClick={submit}>
              {busy ? <span className="submit-spinner" aria-hidden="true" /> : null}+ thread
            </button>
            <button className="btn ghost" disabled={busy} onClick={() => setAdding(false)}>
              cancel
            </button>
          </div>
          {error ? (
            <div className="rename-hint err">✗ {error}</div>
          ) : (
            <div className="rename-hint">
              {busy ? "opening…" : <>→ another pi on this job, starting from a copy of this tab's conversation, in a tab of its own</>}
            </div>
          )}
        </div>
      ) : null}
    </div>
  );
}
