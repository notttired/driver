import { useEffect, useRef, useState } from "react";

import { ApiError, fetchDirs, saveJobForm } from "../../api/client.ts";
import { useDebouncedCallback } from "../../hooks/useDebouncedDraft.ts";
import type { JobForm, ModelsResponse } from "../../lib/types.ts";

export const FORM_AUTOSAVE_MS = 600;
const DIRS_DEBOUNCE_MS = 200;

export interface LaunchFormProps {
  form: JobForm;
  models: ModelsResponse | null;
  /** a save landed; the overlay keeps the lock token current */
  onSaved: (mtime: number) => void;
  /** "edited elsewhere" — the overlay refetches and remounts the form */
  onReload: () => void;
  /** lets the overlay flush a pending save before a rename or a start */
  registerFlush?: (fn: (() => Promise<void>) | null) => void;
}

type SaveState =
  | { kind: "idle" }
  | { kind: "dirty" }
  | { kind: "saving" }
  | { kind: "saved" }
  | { kind: "conflict"; message: string }
  | { kind: "error"; message: string };

/**
 * A not-started job's settings: the directory pi opens in (autocomplete from
 * /api/dirs) and the model (datalist from /api/models) — what START hands to
 * pi. Every change autosaves 600ms later under the form's optimistic lock; a
 * 409 is "edited elsewhere" with a reload affordance. There is no spec: the
 * job is briefed in its pane once pi is open.
 */
export function LaunchForm({ form, models, onSaved, onReload, registerFlush }: LaunchFormProps) {
  const [dir, setDir] = useState(form.dir);
  const [model, setModel] = useState(form.model);
  const [save, setSave] = useState<SaveState>({ kind: "idle" });
  const [dirs, setDirs] = useState<string[]>([]);
  const mtime = useRef(form.mtime);
  const inflight = useRef<Promise<void> | null>(null);
  const seeded = useRef(false);

  const doSave = (d: string, m: string) => {
    setSave({ kind: "saving" });
    const p = saveJobForm({ slug: form.slug, dir: d, model: m, mtime: mtime.current })
      .then((res) => {
        mtime.current = res.mtime;
        onSaved(res.mtime);
        setSave({ kind: "saved" });
      })
      .catch((e: unknown) => {
        const message = e instanceof Error ? e.message : String(e);
        if (e instanceof ApiError && e.status === 409) setSave({ kind: "conflict", message });
        else setSave({ kind: "error", message });
      })
      .finally(() => {
        if (inflight.current === p) inflight.current = null;
      });
    inflight.current = p;
  };
  const debounced = useDebouncedCallback(doSave, FORM_AUTOSAVE_MS);

  useEffect(() => {
    if (!seeded.current) {
      seeded.current = true;
      return;
    }
    setSave((s) => (s.kind === "conflict" ? s : { kind: "dirty" }));
    debounced.call(dir, model);
  }, [dir, model, debounced]);

  useEffect(() => {
    if (!registerFlush) return;
    registerFlush(async () => {
      debounced.flush();
      await inflight.current;
    });
    return () => registerFlush(null);
  }, [registerFlush, debounced]);

  // directory autocomplete follows the typed prefix
  const dirLookup = useDebouncedCallback((q: string) => {
    fetchDirs(q)
      .then((r) => setDirs(r.dirs))
      .catch(() => setDirs([]));
  }, DIRS_DEBOUNCE_MS);
  useEffect(() => {
    dirLookup.call(dir);
  }, [dir, dirLookup]);

  const conflict = save.kind === "conflict";

  return (
    <form className="editor-form launch-form" onSubmit={(e) => e.preventDefault()} data-no-drag>
      <div className="f-row">
        <label className="f-field grow">
          DIRECTORY
          <input
            id="editor-dir"
            list="dir-options"
            autoComplete="off"
            spellCheck={false}
            placeholder="~/src/…"
            value={dir}
            disabled={conflict}
            onChange={(e) => setDir(e.target.value)}
          />
          <datalist id="dir-options">
            {dirs.map((d) => (
              <option key={d} value={d} />
            ))}
          </datalist>
        </label>
        <label className="f-field">
          MODEL
          <input
            id="editor-model"
            list="model-options"
            autoComplete="off"
            spellCheck={false}
            placeholder={models?.default ? `default · ${models.default}` : "default"}
            value={model}
            disabled={conflict}
            onChange={(e) => setModel(e.target.value)}
          />
          <datalist id="model-options">
            {(models?.models ?? []).map((m) => (
              <option key={`${m.provider}/${m.id}`} value={`${m.provider}/${m.id}`} />
            ))}
          </datalist>
        </label>
      </div>
      <div className="editor-foot">
        <span className={"editor-hint" + (save.kind === "error" || conflict ? " err" : "")} id="editor-hint">
          {save.kind === "idle"
            ? "autosaves · START (or a drag to WORKING) opens pi here"
            : save.kind === "dirty"
              ? "…"
              : save.kind === "saving"
                ? "saving…"
                : save.kind === "saved"
                  ? "saved"
                  : `✗ ${save.message}`}
        </span>
        {conflict ? (
          <button type="button" className="btn" id="editor-reload" onClick={onReload}>
            ↻ reload
          </button>
        ) : null}
      </div>
      <p className="launch-note">
        Not started. START opens a tmux session running pi in the directory above and shows its pane here; whatever
        you type first is pi&apos;s first prompt. The job stays in DRAFT until pi starts on that message.
      </p>
    </form>
  );
}
