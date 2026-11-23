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

/** Close a tab's pi, no questions asked: its conversation stays with the
 *  job, so nothing is lost. The first tab (id 0) closes too — the daemon
 *  hands its place to the next thread. The tab in view falls back to the
 *  job's own when it was this one; the board refetches. Shared by the tab's
 *  ✕ and ⌘W. */
export function closeThreadNow(
  slug: string,
  t: WebThread,
  opts: { current: number; onSelect: (id: number) => void; onChanged?: () => Promise<unknown> | void; onToast: (message: string) => void },
): void {
  void closeThread(slug, t.id)
    .then(() => {
      if (opts.current === t.id) opts.onSelect(0);
      void opts.onChanged?.();
    })
    .catch((e: unknown) => opts.onToast(`${t.name}: ${e instanceof Error ? e.message : String(e)}`));
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
 * current tab (a copy of its conversation under a name you type); ✕ closes
 * a tab's pi (the conversation stays with the job; closing the first tab
 * hands its place to the next), never the last one; a double-click renames
 * a thread.
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
  // open a thread from the current tab as `named`; the new tab is selected
  // once the board knows it. The row shows a refusal; a key reports it as
  // a toast.
  const open = async (named: string, onError: (message: string) => void) => {
    if (inFlight.current) return;
    inFlight.current = true;
    setBusy(true);
    setError("");
    try {
      const res = await openThread(slug, current, named);
      setAdding(false);
      await onChanged?.();
      onSelect(res.thread.id);
    } catch (e: unknown) {
      onError(e instanceof Error ? e.message : String(e));
    } finally {
      inFlight.current = false;
      setBusy(false);
    }
  };
  const submit = () => {
    const trimmed = name.trim();
    if (trimmed) void open(trimmed, setError);
  };

  // ⌘T: a thread from the current tab under the default name, no box to
  // fill — the tab can be renamed later
  const openRef = useRef(open);
  openRef.current = open;
  const threadsRef = useRef(threads);
  threadsRef.current = threads;
  useEffect(() => {
    const onKeyDown = (e: KeyboardEvent) => {
      if (!e.metaKey || e.shiftKey || e.altKey || e.ctrlKey || e.key.toLowerCase() !== "t") return;
      e.preventDefault();
      void openRef.current(suggestThreadName(threadsRef.current), (message) => onToast(`new thread: ${message}`));
    };
    document.addEventListener("keydown", onKeyDown);
    return () => document.removeEventListener("keydown", onKeyDown);
  }, [onToast]);
  const close = (t: WebThread) => closeThreadNow(slug, t, { current, onSelect, onChanged, onToast });
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
              className={"thread-tab" + (selected ? " selected" : "") + (t.live ? "" : " dead") + (t.attention ? " attention" : "")}
              aria-selected={selected}
              title={`${t.name} — ${tok.text}${tok.detail ? ` ${tok.detail}` : ""}${t.attention ? ", stopped since you last looked at it" : ""}${t.id ? "\ndouble-click to rename" : ""}${threads.length > 1 ? " · ✕ or ⌘W closes its pi\n⌘⇧] / ⌘⇧[ next / previous tab" : ""}`}
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
              {threads.length > 1 ? (
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
          title="new thread (⌘T for one named by default) — another pi on this job, starting from a copy of this tab's conversation"
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
