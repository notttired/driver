export interface BoardBarProps {
  /** the jobs on the board (the archive drawer's aside) */
  total: number;
  /** open the ⌘K palette */
  onSearch: () => void;
}

/** The strip above the columns: the way into the palette, and the count. */
export function BoardBar({ total, onSearch }: BoardBarProps) {
  return (
    <div className="boardbar" id="boardbar">
      <button className="bb-search" id="job-search" type="button" title="jump to a pane — ⌘K" onClick={onSearch}>
        <span className="bb-glyph" aria-hidden="true">
          ⌕
        </span>
        search
        <span className="bb-hint">⌘K</span>
      </button>
      <span className="bb-read">{`${total} jobs`}</span>
    </div>
  );
}
