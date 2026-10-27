import { useEffect, useRef, useState } from "react";

import { saveJobNotes } from "../../api/client.ts";
import { useDebouncedDraft } from "../../hooks/useDebouncedDraft.ts";

export const NOTES_AUTOSAVE_MS = 600;

export interface NotesPanelProps {
  slug: string;
  initial: string;
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
 *  from the overlay, so a different job is a fresh mount. */
export function NotesPanel({ slug, initial, onSaved, registerFlush }: NotesPanelProps) {
  const [text, setText] = useState(initial);
  const [save, setSave] = useState<SaveState>({ kind: "idle" });
  const inflight = useRef<Promise<void> | null>(null);

  const sync = useDebouncedDraft(
    slug,
    text,
    (s, t) => {
      setSave({ kind: "saving" });
      const p = saveJobNotes(s, t)
        .then(() => {
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

  useEffect(() => {
    if (!registerFlush) return;
    registerFlush(async () => {
      sync.flush();
      await inflight.current;
    });
    return () => registerFlush(null);
  }, [registerFlush, sync]);

  return (
    <section className="side-panel notes-panel">
      <div className="side-head">
        <b>NOTES</b>
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
        placeholder="notes to self — context, what to check next (never shown to pi)"
        spellCheck={false}
        value={text}
        onChange={(e) => setText(e.target.value)}
      />
    </section>
  );
}
