import { useCallback, useEffect, useRef, useState } from "react";

import { renameJob } from "../../api/client.ts";
import { slugify } from "../../lib/slug.ts";

export interface RenameControlState {
  slug: string;
  open: boolean;
  value: string;
  /** slugified preview of `value` — what the server will accept */
  preview: string;
  error: string;
  busy: boolean;
  begin: () => void;
  cancel: () => void;
  setValue: (v: string) => void;
  submit: () => void;
}

/**
 * The rename flow's state. The box holds the job's name (its slug when it has
 * none); what is typed becomes the name on the card, and its slugified form
 * the slug. Lives ABOVE the keyed overlay body: a successful
 * rename re-points the open slug, which remounts the body in the same commit
 * the response lands in — state held inside it would be gone before render.
 */
export function useRenameControl(
  slug: string | null,
  /** the job's typed name ("" when it goes by its slug) — what the box opens on */
  name: string,
  onRenamed: ((slug: string) => void) | undefined,
  opts: { open?: boolean; beforeSubmit?: () => void | Promise<void> } = {},
): RenameControlState {
  const { open: overlayOpen = true, beforeSubmit } = opts;
  const current = slug ?? "";
  const currentName = name || current;
  const [open, setOpen] = useState(false);
  const [value, setValueRaw] = useState(currentName);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  // a closed overlay must not carry a half-typed rename into the next job
  useEffect(() => {
    if (overlayOpen) return;
    setOpen(false);
    setError("");
  }, [overlayOpen]);
  useEffect(() => {
    setOpen(false);
    setError("");
  }, [current]);

  const beforeRef = useRef(beforeSubmit);
  beforeRef.current = beforeSubmit;
  const inFlight = useRef(false);

  const begin = useCallback(() => {
    setValueRaw(currentName);
    setError("");
    setOpen(true);
  }, [currentName]);

  const cancel = useCallback(() => {
    setOpen(false);
    setError("");
  }, []);

  const setValue = useCallback((v: string) => {
    setValueRaw(v);
    setError("");
  }, []);

  const submit = useCallback(() => {
    if (!current || inFlight.current) return;
    const to = slugify(value);
    const newName = value.trim();
    if (!to) return;
    if (to === current && newName === currentName) {
      setOpen(false);
      return;
    }
    inFlight.current = true;
    setBusy(true);
    setError("");
    void (async () => {
      try {
        await beforeRef.current?.();
        const res = await renameJob(current, to, newName);
        setOpen(false);
        onRenamed?.(res.slug);
      } catch (e: unknown) {
        setError(e instanceof Error ? e.message : String(e));
      } finally {
        inFlight.current = false;
        setBusy(false);
      }
    })();
  }, [current, currentName, onRenamed, value]);

  return {
    slug: current,
    open,
    value,
    preview: slugify(value),
    error,
    busy,
    begin,
    cancel,
    setValue,
    submit,
  };
}

export function RenameButton({ state, visible }: { state: RenameControlState; visible: boolean }) {
  return (
    <button
      className="btn ghost"
      id="job-rename"
      hidden={!visible || state.open}
      title="rename this job — the name on its card; the slug (and a live tmux session) follow"
      onClick={state.begin}
    >
      ✎ rename
    </button>
  );
}

export function RenameRow({ state }: { state: RenameControlState }) {
  if (!state.open) return null;
  return (
    <div className="rename-row" id="job-rename-row">
      <div className="rename-line">
        <input
          className="rename-input"
          placeholder="new name"
          spellCheck={false}
          autoFocus
          value={state.value}
          onChange={(e) => state.setValue(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "Enter") {
              e.preventDefault();
              state.submit();
            } else if (e.key === "Escape") {
              e.preventDefault();
              e.stopPropagation();
              state.cancel();
            }
          }}
        />
        <button className="btn" disabled={!state.preview || state.busy} onClick={state.submit}>
          rename
        </button>
        <button className="btn ghost" disabled={state.busy} onClick={state.cancel}>
          cancel
        </button>
      </div>
      {state.error ? (
        <div className="rename-hint err">✗ {state.error}</div>
      ) : (
        <div className="rename-hint">
          {state.preview ? (
            <>
              → slug <span className="slug">{state.preview}</span>
            </>
          ) : (
            "→ a name needs at least one letter or digit"
          )}
        </div>
      )}
    </div>
  );
}
