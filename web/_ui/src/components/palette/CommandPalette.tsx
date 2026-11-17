import { useEffect, useMemo, useRef, useState } from "react";

import { ESC_PRIORITY, useEscapeLayer } from "../../hooks/useEscapeStack.ts";
import { searchPalette, type PaletteItem } from "../../lib/palette.ts";
import type { WebJob, WebLane } from "../../lib/types.ts";

export interface PalettePick {
  /** the board key of the job to open */
  key: string;
  /** the tab to open it on: a thread id, 0 for the job's own pi */
  thread: number;
}

export interface CommandPaletteProps {
  open: boolean;
  jobs: readonly WebJob[];
  lanes: readonly WebLane[];
  onPick: (pick: PalettePick) => void;
  /** ⌘-Enter: open the job's directory in the editor instead */
  onOpenDir: (slug: string) => void;
  onClose: () => void;
}

/**
 * ⌘K: a popup over everything with one box to type in and, beneath it, every
 * pane on the board — each job and each of its threads — with its status,
 * narrowing to what the typed words are found in (lib/palette.ts: names,
 * slugs, directories, columns, states, links, models, tools, thread names).
 * ↑↓ move, Enter opens the pane, ⌘-Enter opens the job's directory in the
 * editor, Escape or the scrim closes. Mounted only while open, so opening is
 * a fresh box every time.
 */
export function CommandPalette({ open, jobs, lanes, onPick, onOpenDir, onClose }: CommandPaletteProps) {
  useEscapeLayer(open, ESC_PRIORITY.palette, onClose);
  if (!open) return null;
  return <Palette jobs={jobs} lanes={lanes} onPick={onPick} onOpenDir={onOpenDir} onClose={onClose} />;
}

function Palette({ jobs, lanes, onPick, onOpenDir, onClose }: Omit<CommandPaletteProps, "open">) {
  const [query, setQuery] = useState("");
  const [cursor, setCursor] = useState(0);
  const inputRef = useRef<HTMLInputElement | null>(null);
  const listRef = useRef<HTMLOListElement | null>(null);
  const items = useMemo(() => searchPalette(jobs, lanes, query), [jobs, lanes, query]);
  const total = useMemo(() => searchPalette(jobs, lanes, "", Infinity).length, [jobs, lanes]);
  const at = Math.min(cursor, Math.max(items.length - 1, 0));
  const current = items[at];

  useEffect(() => inputRef.current?.focus(), []);
  // the selected row stays in view as the arrows move it
  useEffect(() => {
    const el = listRef.current?.querySelector<HTMLElement>('[aria-selected="true"]');
    if (el && typeof el.scrollIntoView === "function") el.scrollIntoView({ block: "nearest" });
  }, [at, items]);

  const pick = (item: PaletteItem) => {
    onPick({ key: item.jobKey, thread: item.thread });
    onClose();
  };
  const onKeyDown = (e: React.KeyboardEvent<HTMLInputElement>) => {
    switch (e.key) {
      case "ArrowDown":
        e.preventDefault();
        if (items.length) setCursor((at + 1) % items.length);
        break;
      case "ArrowUp":
        e.preventDefault();
        if (items.length) setCursor((at - 1 + items.length) % items.length);
        break;
      case "Enter":
        e.preventDefault();
        if (!current) return;
        if (e.metaKey) {
          onOpenDir(current.slug);
          onClose();
        } else pick(current);
        break;
    }
  };

  return (
    <div
      className="overlay palette-overlay"
      id="palette"
      onClick={(e) => {
        if (e.target === e.currentTarget) onClose();
      }}
    >
      <div className="palette" role="dialog" aria-label="search">
        <div className="pal-box">
          <span className="pal-glyph" aria-hidden="true">
            ⌕
          </span>
          <input
            ref={inputRef}
            className="pal-input"
            type="text"
            role="combobox"
            aria-expanded="true"
            aria-controls="palette-list"
            aria-activedescendant={current ? `pal-${current.key}` : undefined}
            placeholder="jump to a pane — by name, directory, column, status, link, thread…"
            autoComplete="off"
            spellCheck={false}
            value={query}
            onChange={(e) => {
              setQuery(e.target.value);
              setCursor(0);
            }}
            onKeyDown={onKeyDown}
          />
        </div>
        {items.length ? (
          <ol className="pal-list" id="palette-list" role="listbox" ref={listRef}>
            {items.map((item, i) => (
              <li
                key={item.key}
                id={`pal-${item.key}`}
                role="option"
                aria-selected={i === at}
                className={"pal-row" + (i === at ? " selected" : "")}
                onMouseMove={() => {
                  if (i !== at) setCursor(i);
                }}
                onClick={() => pick(item)}
              >
                <span className={`thread-dot status-${item.token.variant}`} aria-hidden="true" />
                <span className="pal-main">
                  <span className="pal-title">{item.title}</span>
                  {item.subtitle ? <span className="pal-sub">{item.subtitle}</span> : null}
                  {item.why ? <span className="pal-why">{item.why}</span> : null}
                </span>
                <span className="pal-col">{item.column}</span>
                <span className={`status status-${item.token.variant}`} title={item.token.title}>
                  {item.token.text}
                  {item.token.detail ? <i className="tool">{item.token.detail}</i> : null}
                </span>
              </li>
            ))}
          </ol>
        ) : (
          <div className="pal-empty">nothing matches “{query}”</div>
        )}
        <div className="pal-foot">
          <span>↑↓ move</span>
          <span>⏎ open</span>
          <span>⌘⏎ open dir in editor</span>
          <span>esc close</span>
          <span className="pal-count">{query.trim() ? `${items.length} of ${total} panes` : `${total} panes`}</span>
        </div>
      </div>
    </div>
  );
}
