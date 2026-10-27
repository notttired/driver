import { useCallback, useEffect, useMemo, useRef } from "react";

export interface Debounced<A extends unknown[]> {
  call: (...args: A) => void;
  /** run a pending call now */
  flush: () => void;
  /** drop a pending call */
  cancel: () => void;
}

/** Trailing-edge debounce that flushes (never drops) a pending call on unmount. */
export function useDebouncedCallback<A extends unknown[]>(
  fn: (...args: A) => void,
  ms: number,
): Debounced<A> {
  const fnRef = useRef(fn);
  fnRef.current = fn;
  const timer = useRef<number | null>(null);
  const pending = useRef<A | null>(null);

  const cancel = useCallback(() => {
    if (timer.current !== null) window.clearTimeout(timer.current);
    timer.current = null;
    pending.current = null;
  }, []);

  const flush = useCallback(() => {
    if (timer.current === null) return;
    window.clearTimeout(timer.current);
    timer.current = null;
    const a = pending.current;
    pending.current = null;
    if (a) fnRef.current(...a);
  }, []);

  const call = useCallback(
    (...args: A) => {
      pending.current = args;
      if (timer.current !== null) window.clearTimeout(timer.current);
      timer.current = window.setTimeout(() => {
        timer.current = null;
        const a = pending.current;
        pending.current = null;
        if (a) fnRef.current(...a);
      }, ms);
    },
    [ms],
  );

  const flushRef = useRef(flush);
  flushRef.current = flush;
  useEffect(() => () => flushRef.current(), []);

  // one stable object, so callers can list it in effect deps without the
  // effect re-running (and re-scheduling a save) on every render
  return useMemo(() => ({ call, flush, cancel }), [call, flush, cancel]);
}

export interface DraftSync {
  flush: () => void;
  cancel: () => void;
  /** tell the sync what the server now holds, so an echo is not re-saved */
  markSynced: (v: string) => void;
}

/**
 * Autosave a text value: the first value seen for a slug is the LOADED one
 * and is not echoed back; every later change is debounced into `save`.
 */
export function useDebouncedDraft(
  slug: string | null,
  value: string,
  save: (slug: string, text: string) => void,
  ms = 600,
): DraftSync {
  const { call, flush, cancel } = useDebouncedCallback(save, ms);
  const lastRef = useRef<string | null>(null);
  const slugRef = useRef<string | null>(null);

  useEffect(() => {
    if (!slug) return;
    if (slugRef.current !== slug) {
      slugRef.current = slug;
      lastRef.current = value;
      return;
    }
    if (lastRef.current === value) return;
    lastRef.current = value;
    call(slug, value);
  }, [slug, value, call]);

  const markSynced = useCallback((v: string) => {
    lastRef.current = v;
  }, []);

  return { flush, cancel, markSynced };
}
