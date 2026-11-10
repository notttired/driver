import { useCallback, useState } from "react";

import type { Column } from "../lib/columns.ts";
import { readLS, writeLS } from "./useLocalStorage.ts";

export const COLLAPSED_KEY = "driver-collapsed-columns";

function read(): Column[] {
  try {
    const parsed: unknown = JSON.parse(readLS(COLLAPSED_KEY) ?? "[]");
    return Array.isArray(parsed) ? parsed.filter((c): c is Column => typeof c === "string") : [];
  } catch {
    return [];
  }
}

/** Which columns are folded to a strip — a view choice this browser keeps. */
export function useCollapsedColumns(): {
  isCollapsed: (col: Column) => boolean;
  toggle: (col: Column) => void;
} {
  const [collapsed, setCollapsed] = useState<Column[]>(read);
  const isCollapsed = useCallback((col: Column) => collapsed.includes(col), [collapsed]);
  const toggle = useCallback((col: Column) => {
    setCollapsed((prev) => {
      const next = prev.includes(col) ? prev.filter((c) => c !== col) : [...prev, col];
      writeLS(COLLAPSED_KEY, JSON.stringify(next));
      return next;
    });
  }, []);
  return { isCollapsed, toggle };
}
