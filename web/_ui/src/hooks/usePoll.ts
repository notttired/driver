import { useCallback, useEffect, useRef, useState } from "react";

export interface PollOptions {
  enabled?: boolean;
  /** skip ticks while the tab is hidden; refetch the moment it is shown */
  pauseWhenHidden?: boolean;
  /** skip a tick while this says so (the board: while a card is in the air) */
  pauseWhen?: () => boolean;
}

export interface PollResult<T> {
  data: T | undefined;
  error: Error | undefined;
  failStreak: number;
  /** Run one fetch now. Resolves true when a payload landed (or the run was
   *  superseded by a newer one that will), false when paused or failed. */
  refetch: () => Promise<boolean>;
}

export function usePoll<T>(
  fetcher: (signal: AbortSignal) => Promise<T>,
  intervalMs: number,
  opts: PollOptions = {},
): PollResult<T> {
  const { enabled = true, pauseWhenHidden = true, pauseWhen } = opts;

  const [data, setData] = useState<T | undefined>(undefined);
  const [error, setError] = useState<Error | undefined>(undefined);
  const [failStreak, setFailStreak] = useState(0);

  const fetcherRef = useRef(fetcher);
  fetcherRef.current = fetcher;
  const pauseWhenRef = useRef(pauseWhen);
  pauseWhenRef.current = pauseWhen;

  const aliveRef = useRef(true);
  const inFlightRef = useRef<AbortController | null>(null);

  const run = useCallback(async (): Promise<boolean> => {
    if (pauseWhenRef.current?.()) return false;
    if (pauseWhenHidden && typeof document !== "undefined" && document.hidden) {
      return false;
    }
    inFlightRef.current?.abort();
    const ac = new AbortController();
    inFlightRef.current = ac;
    try {
      const next = await fetcherRef.current(ac.signal);
      if (!aliveRef.current || ac.signal.aborted) return true;
      setData(next);
      setError(undefined);
      setFailStreak(0);
      return true;
    } catch (e) {
      if (ac.signal.aborted) return true;
      if (!aliveRef.current) return true;
      setError(e instanceof Error ? e : new Error(String(e)));
      setFailStreak((n) => n + 1);
      return false;
    }
  }, [pauseWhenHidden]);

  useEffect(() => {
    aliveRef.current = true;
    return () => {
      aliveRef.current = false;
      inFlightRef.current?.abort();
    };
  }, []);

  useEffect(() => {
    if (!enabled) return;
    void run();
    const id = window.setInterval(() => void run(), intervalMs);
    return () => window.clearInterval(id);
  }, [enabled, intervalMs, run]);

  useEffect(() => {
    if (!enabled || !pauseWhenHidden) return;
    const onVis = () => {
      if (!document.hidden) void run();
    };
    document.addEventListener("visibilitychange", onVis);
    return () => document.removeEventListener("visibilitychange", onVis);
  }, [enabled, pauseWhenHidden, run]);

  return { data, error, failStreak, refetch: run };
}
