import { useCallback, useMemo, useRef, useState } from "react";

import { fetchBoard } from "../api/client.ts";
import type { Column } from "../lib/columns.ts";
import { bucketJobs } from "../lib/format.ts";
import { jobKey } from "../lib/jobKey.ts";
import type { WebBoard, WebJob } from "../lib/types.ts";
import { usePoll } from "./usePoll.ts";

export const BOARD_POLL_MS = 3000;

export interface BoardModel {
  board: WebBoard | undefined;
  /** the poll's latest failure — the chrome's "unreachable" signal */
  error: Error | undefined;
  /** every job by jobKey(), archived ones included */
  byKey: Map<string, WebJob>;
  byColumn: Record<Column, WebJob[]>;
  archived: WebJob[];
  refetch: () => Promise<void>;
  /** the poll pauses while a card is in the air */
  setDragging: (dragging: boolean) => void;
  /**
   * The ONE write path: await the request, then refetch whether it succeeded
   * or not (pessimistic — the board is what the server says). Resolves to the
   * error (its message is the server's own sentence) or null; the same
   * sentence is parked in `lastError` for the chrome's status corner.
   */
  mutate: (fn: () => Promise<unknown>) => Promise<Error | null>;
  lastError: string | null;
}

export function useBoard(): BoardModel {
  const draggingRef = useRef(false);
  const [lastError, setLastError] = useState<string | null>(null);

  const setDragging = useCallback((d: boolean) => {
    draggingRef.current = d;
  }, []);

  const { data, error, refetch } = usePoll<WebBoard>(
    (signal) => fetchBoard(signal),
    BOARD_POLL_MS,
    { pauseWhen: () => draggingRef.current, pauseWhenHidden: true },
  );

  const mutate = useCallback(
    async (fn: () => Promise<unknown>): Promise<Error | null> => {
      try {
        await fn();
        setLastError(null);
        await refetch();
        return null;
      } catch (e) {
        const err = e instanceof Error ? e : new Error(String(e));
        setLastError(err.message);
        await refetch();
        return err;
      }
    },
    [refetch],
  );

  const refetchBoard = useCallback(async () => {
    await refetch();
  }, [refetch]);

  const { byKey, byColumn, archived } = useMemo(() => {
    const key = new Map<string, WebJob>();
    for (const j of data?.jobs ?? []) key.set(jobKey(j), j);
    const { byColumn: cols, archived: arch } = bucketJobs(data?.jobs ?? []);
    return { byKey: key, byColumn: cols, archived: arch };
  }, [data]);

  return {
    board: data,
    error,
    byKey,
    byColumn,
    archived,
    refetch: refetchBoard,
    setDragging,
    mutate,
    lastError,
  };
}
