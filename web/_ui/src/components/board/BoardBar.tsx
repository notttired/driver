import { useEffect, useRef } from "react";

import type { JobSearchModel } from "../../hooks/useJobSearch.ts";

export interface BoardBarProps {
  search: JobSearchModel;
  /** while one is open, ctrl-K belongs to the pane: only ⌘K takes the search */
  overlayOpen?: boolean;
  onShortcut?: () => void;
}

export function BoardBar({ search, overlayOpen, onShortcut }: BoardBarProps) {
  const { query, setQuery, matched, total, active } = search;
  const inputRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    const onKeyDown = (e: KeyboardEvent) => {
      if (e.key !== "k" && e.key !== "K") return;
      if (!e.metaKey && !(e.ctrlKey && !overlayOpen)) return;
      e.preventDefault();
      onShortcut?.();
      inputRef.current?.focus();
      inputRef.current?.select();
    };
    document.addEventListener("keydown", onKeyDown);
    return () => document.removeEventListener("keydown", onKeyDown);
  }, [overlayOpen, onShortcut]);

  return (
    <div className="boardbar" id="boardbar">
      <span className="bb-search">
        <input
          ref={inputRef}
          className="bb-input"
          id="job-search"
          type="text"
          placeholder="search jobs…"
          aria-label="search jobs"
          autoComplete="off"
          spellCheck={false}
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          onKeyDown={(e) => {
            if (e.key !== "Escape") return;
            e.stopPropagation();
            setQuery("");
          }}
        />
        {active ? (
          <button
            className="bb-clear"
            id="job-search-clear"
            type="button"
            title="clear the search"
            aria-label="clear the search"
            onClick={() => setQuery("")}
          >
            ✕
          </button>
        ) : null}
      </span>
      {active ? null : <span className="bb-hint">⌘K</span>}
      <span className="bb-read">
        {active ? (
          <>
            <b>{matched}</b> of {total} jobs
          </>
        ) : (
          `${total} jobs`
        )}
      </span>
    </div>
  );
}
