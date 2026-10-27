import { useRef, useState } from "react";
import type { CSSProperties } from "react";

import { useMenuDismiss } from "../../hooks/useMenuDismiss.ts";
import { isJobColor, JOB_COLORS } from "../../lib/format.ts";

interface SwatchStripProps {
  value: string | undefined;
  onPick: (color: string) => void;
  /** the panel-head size */
  compact?: boolean;
  disabled?: boolean;
}

const NO_COLOR = "NO COLOR";

/** The job-colour dropdown. "" is "no colour" and is a real choice. */
export function SwatchStrip({ value, onPick, compact, disabled }: SwatchStripProps) {
  const [open, setOpen] = useState(false);
  const rootRef = useRef<HTMLSpanElement | null>(null);
  const want = isJobColor(value) ? value : "";

  useMenuDismiss(open, setOpen, rootRef);

  const label = (val: string) => (val ? val.toUpperCase() : NO_COLOR);
  const dot = (val: string) => (
    <span
      className={val ? "dot" : "dot none"}
      style={
        val
          ? ({ background: `var(--job-${val})`, "--sw": `var(--job-${val})` } as CSSProperties)
          : undefined
      }
    />
  );

  const row = (val: string) => {
    const sel = want === val;
    return (
      <span
        key={val || "__none__"}
        className={sel ? "colorsel-row sel" : "colorsel-row"}
        role="option"
        aria-selected={sel}
        title={val || "no color"}
        onClick={() => {
          onPick(val);
          setOpen(false);
        }}
      >
        {dot(val)}
        {label(val)}
        {sel ? <span className="tick">✓</span> : null}
      </span>
    );
  };

  return (
    <span
      ref={rootRef}
      className={compact ? "colorsel compact" : "colorsel"}
      onClick={(ev) => ev.stopPropagation()}
    >
      <button
        type="button"
        className="colorsel-btn"
        aria-haspopup="listbox"
        aria-expanded={open}
        disabled={disabled}
        title="job colour"
        onClick={() => setOpen((o) => !o)}
      >
        {dot(want)}
        {label(want)} <span className="caret">▾</span>
      </button>
      {open ? (
        <span className="colorsel-menu" role="listbox">
          {row("")}
          {JOB_COLORS.map((c) => row(c))}
        </span>
      ) : null}
    </span>
  );
}
