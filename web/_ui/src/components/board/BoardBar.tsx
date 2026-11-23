export type View = "board" | "plans";

export interface BoardBarProps {
  /** the jobs on the board (the archive drawer's aside) */
  total: number;
  /** open the ⌘K palette */
  onSearch: () => void;
  /** which view is up: the board's columns, or the plans */
  view: View;
  onView: (view: View) => void;
}

/** The strip above the columns: the way into the palette, the switch
 *  between the board and the plans, and the count. */
export function BoardBar({ total, onSearch, view, onView }: BoardBarProps) {
  return (
    <div className="boardbar" id="boardbar">
      <button className="bb-search" id="job-search" type="button" title="jump to a pane — ⌘K" onClick={onSearch}>
        <span className="bb-glyph" aria-hidden="true">
          ⌕
        </span>
        search
        <span className="bb-hint">⌘K</span>
      </button>
      <div className="bb-views" role="group" aria-label="view">
        {(["board", "plans"] as const).map((v) => (
          <button key={v} type="button" className={view === v ? "on" : ""} aria-pressed={view === v} onClick={() => onView(v)}>
            {v.toUpperCase()}
          </button>
        ))}
      </div>
      <span className="bb-read">{`${total} jobs`}</span>
    </div>
  );
}
