import { useCallback, useEffect, useRef, useState } from "react";

import { forkJob } from "../../api/client.ts";
import { slugify } from "../../lib/slug.ts";

export interface ForkControlState {
  slug: string;
  open: boolean;
  value: string;
  /** slugified preview of `value` — the copy's slug */
  preview: string;
  error: string;
  busy: boolean;
  begin: () => void;
  cancel: () => void;
  setValue: (v: string) => void;
  submit: () => void;
}

/**
 * A name for the copy that no job on the board has yet: "<base> fork", then
 * "<base> fork 2", "<base> fork 3", … `taken` says whether a slug is in use.
 */
export function suggestForkName(base: string, taken: (slug: string) => boolean): string {
  const root = base.trim().replace(/\s+fork(\s+\d+)?$/i, "") || "job";
  let name = `${root} fork`;
  for (let n = 2; taken(slugify(name)) && n < 1000; n++) name = `${root} fork ${n}`;
  return name;
}

/**
 * The fork flow's state, the rename control's twin: the box opens on a
 * suggested name for the copy; what is typed becomes the copy's name and its
 * slugified form the copy's slug. Lives ABOVE the keyed overlay body like the
 * rename control — a successful fork re-points the overlay at the copy, which
 * remounts the body.
 */
export function useForkControl(
  slug: string | null,
  /** the name the box opens on (suggestForkName) */
  suggested: string,
  onForked: ((slug: string, name: string) => void) | undefined,
  opts: { open?: boolean; beforeSubmit?: () => void | Promise<void> } = {},
): ForkControlState {
  const { open: overlayOpen = true, beforeSubmit } = opts;
  const current = slug ?? "";
  const [open, setOpen] = useState(false);
  const [value, setValueRaw] = useState(suggested);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  // a closed overlay, or another job, must not carry a half-typed fork along
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
    setValueRaw(suggested);
    setError("");
    setOpen(true);
  }, [suggested]);

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
    const name = value.trim();
    if (!to) return;
    inFlight.current = true;
    setBusy(true);
    setError("");
    void (async () => {
      try {
        await beforeRef.current?.();
        const form = await forkJob({ slug: current, to, name });
        setOpen(false);
        onForked?.(form.slug, form.name || form.slug);
      } catch (e: unknown) {
        setError(e instanceof Error ? e.message : String(e));
      } finally {
        inFlight.current = false;
        setBusy(false);
      }
    })();
  }, [current, onForked, value]);

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

export function ForkButton({ state, visible }: { state: ForkControlState; visible: boolean }) {
  return (
    <button
      className="btn ghost"
      id="job-fork"
      hidden={!visible || state.open}
      title="fork this job — a copy with its conversation, notes and links, under a new name, in the same column"
      onClick={state.begin}
    >
      ⑂ fork
    </button>
  );
}

export function ForkRow({ state, column }: { state: ForkControlState; column: string }) {
  if (!state.open) return null;
  return (
    <div className="rename-row fork-row" id="job-fork-row">
      <div className="rename-line">
        <input
          className="rename-input"
          placeholder="name of the copy"
          spellCheck={false}
          autoFocus
          value={state.value}
          disabled={state.busy}
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
        <button className="btn primary" disabled={!state.preview || state.busy} onClick={state.submit}>
          {state.busy ? <span className="submit-spinner" aria-hidden="true" /> : null}⑂ fork
        </button>
        <button className="btn ghost" disabled={state.busy} onClick={state.cancel}>
          cancel
        </button>
      </div>
      {state.error ? (
        <div className="rename-hint err">✗ {state.error}</div>
      ) : (
        <div className="rename-hint">
          {state.busy ? (
            "copying…"
          ) : state.preview ? (
            <>
              → a copy of this job — conversation, notes, links, directory, model — as slug{" "}
              <span className="slug">{state.preview}</span>, beside it in {column.toUpperCase()}
            </>
          ) : (
            "→ a name needs at least one letter or digit"
          )}
        </div>
      )}
    </div>
  );
}
