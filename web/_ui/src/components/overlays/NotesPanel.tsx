import { useEffect, useRef, useState, type ReactNode } from "react";

import { fetchJobNotes, saveJobNotes } from "../../api/client.ts";
import { useDebouncedDraft } from "../../hooks/useDebouncedDraft.ts";
import { usePoll } from "../../hooks/usePoll.ts";

export const NOTES_AUTOSAVE_MS = 600;
export const NOTES_POLL_MS = 3000;

export interface NotesPanelProps {
  slug: string;
  initial: string;
  /** what heads the panel in place of its title (the sidebar's switch) */
  head?: ReactNode;
  /** another panel has the sidebar: stay mounted, out of sight */
  hidden?: boolean;
  /** the board's has_notes mark follows a save */
  onSaved?: () => void;
  /** lets the overlay flush a pending save before a rename / submit */
  registerFlush?: (fn: (() => Promise<void>) | null) => void;
}

type SaveState =
  | { kind: "idle" }
  | { kind: "saving" }
  | { kind: "saved" }
  | { kind: "error"; message: string };

/** Free-form notes, autosaved 600ms after the last keystroke. Keyed by slug
 *  from the overlay, so a different job is a fresh mount. Polled, so what pi
 *  writes to them (its driver_notes tool) shows up while you have nothing
 *  unsaved; what you are typing wins over it. */
export function NotesPanel({ slug, initial, head, hidden, onSaved, registerFlush }: NotesPanelProps) {
  const [text, setText] = useState(initial);
  const [save, setSave] = useState<SaveState>({ kind: "idle" });
  const inflight = useRef<Promise<void> | null>(null);
  const typed = useRef(initial);
  typed.current = text;
  const held = useRef(initial);
  const saves = useRef(0);

  const sync = useDebouncedDraft(
    slug,
    text,
    (s, t) => {
      saves.current++;
      setSave({ kind: "saving" });
      const p = saveJobNotes(s, t)
        .then(() => {
          if (inflight.current === p) held.current = t;
          setSave({ kind: "saved" });
          onSaved?.();
        })
        .catch((e: unknown) =>
          setSave({ kind: "error", message: e instanceof Error ? e.message : String(e) }),
        )
        .finally(() => {
          if (inflight.current === p) inflight.current = null;
        });
      inflight.current = p;
    },
    NOTES_AUTOSAVE_MS,
  );

  usePoll(async (signal) => {
    const before = saves.current;
    const { text: t } = await fetchJobNotes(slug, signal);
    if (saves.current !== before || inflight.current || t === held.current || typed.current !== held.current) return;
    held.current = t;
    sync.cancel();
    sync.markSynced(t);
    setText(t);
  }, NOTES_POLL_MS);

  useEffect(() => {
    if (!registerFlush) return;
    registerFlush(async () => {
      sync.flush();
      await inflight.current;
    });
    return () => registerFlush(null);
  }, [registerFlush, sync]);

  return (
    <section className="side-panel notes-panel" hidden={hidden}>
      <div className="side-head">
        {head ?? <b>NOTES</b>}
        <span className={"side-hint" + (save.kind === "error" ? " err" : "")}>
          {save.kind === "saving"
            ? "saving…"
            : save.kind === "saved"
              ? "saved"
              : save.kind === "error"
                ? `✗ ${save.message}`
                : ""}
        </span>
      </div>
      <textarea
        className="notes-text"
        id="job-notes"
        placeholder="notes to self — context, what to check next (pi reads or changes them only when you ask)"
        spellCheck={false}
        value={text}
        onChange={(e) => setText(e.target.value)}
      />
    </section>
  );
}
