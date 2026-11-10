import { useEffect, useRef, useState, type KeyboardEvent } from "react";

import type { WebLane } from "../../lib/types.ts";

/** One line of text taken on Enter, dropped on Escape; blur is Enter when
 *  something was typed. */
function NameInput({
  label,
  initial,
  placeholder,
  onDone,
}: {
  label: string;
  initial: string;
  placeholder?: string;
  onDone: (value: string | null) => void;
}) {
  const [value, setValue] = useState(initial);
  const ref = useRef<HTMLInputElement>(null);
  const done = useRef(false);
  useEffect(() => {
    ref.current?.focus();
    ref.current?.select();
  }, []);
  const finish = (take: boolean) => {
    if (done.current) return;
    done.current = true;
    const v = value.trim();
    onDone(take && v ? v : null);
  };
  const onKeyDown = (e: KeyboardEvent<HTMLInputElement>) => {
    if (e.key === "Enter") {
      e.preventDefault();
      finish(true);
    } else if (e.key === "Escape") {
      e.preventDefault();
      e.stopPropagation();
      finish(false);
    }
  };
  return (
    <input
      ref={ref}
      className="lane-input"
      aria-label={label}
      placeholder={placeholder}
      maxLength={40}
      value={value}
      onChange={(e) => setValue(e.target.value)}
      onKeyDown={onKeyDown}
      onBlur={() => finish(true)}
    />
  );
}

/** A lane's head: its name (click to rename) and the ✕ that removes it. */
export function LaneTitle({ lane, onRename }: { lane: WebLane; onRename: (id: number, name: string) => void }) {
  const [editing, setEditing] = useState(false);
  if (editing) {
    return (
      <NameInput
        label="column name"
        initial={lane.name}
        onDone={(v) => {
          setEditing(false);
          if (v !== null && v !== lane.name) onRename(lane.id, v);
        }}
      />
    );
  }
  return (
    <button className="lane-name" title="rename this column · drag to move it" data-drag onClick={() => setEditing(true)}>
      {lane.name}
    </button>
  );
}

export function LaneRemove({ lane, count, onRemove }: { lane: WebLane; count: number; onRemove: (id: number) => void }) {
  const cards = count === 1 ? "1 card goes" : `${count} cards go`;
  return (
    <button
      className="lane-remove"
      aria-label={`remove column ${lane.name}`}
      title={count ? `remove this column; its ${cards} back to the columns their states name` : "remove this column"}
      onClick={() => {
        if (count && !window.confirm(`Remove ${lane.name}? Its ${cards} back to the columns their states name.`)) return;
        onRemove(lane.id);
      }}
    >
      ✕
    </button>
  );
}

/** The + after the last column: click, name the new column, Enter. */
export function AddLane({ onAdd }: { onAdd: (name: string) => void }) {
  const [open, setOpen] = useState(false);
  return (
    <section className="col col-add" data-col="add">
      {open ? (
        <div className="col-head">
          <NameInput
            label="new column name"
            initial=""
            placeholder="column name"
            onDone={(v) => {
              setOpen(false);
              if (v !== null) onAdd(v);
            }}
          />
        </div>
      ) : (
        <button className="btn-add-lane" aria-label="add a column" title="add a column" onClick={() => setOpen(true)}>
          +
        </button>
      )}
    </section>
  );
}
