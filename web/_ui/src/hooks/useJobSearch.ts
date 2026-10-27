import { useMemo, useState } from "react";

import { COLUMNS, type Column } from "../lib/columns.ts";
import { jobKey } from "../lib/jobKey.ts";
import { matchJob, parseTerms } from "../lib/jobSearch.ts";
import type { WebJob } from "../lib/types.ts";
import type { BoardModel } from "./useBoard.ts";

export interface JobSearchModel {
  query: string;
  setQuery(query: string): void;
  terms: readonly string[];
  /** the board's buckets, filtered — the board's OWN arrays (same identity)
   *  while the search is idle, so nothing downstream re-renders for nothing */
  byColumn: Record<Column, WebJob[]>;
  archived: WebJob[];
  /** keys of the visible jobs while a search is active */
  visibleKeys: ReadonlySet<string>;
  matched: number;
  total: number;
  active: boolean;
}

const NO_KEYS: ReadonlySet<string> = new Set();

export function useJobSearch(board: BoardModel): JobSearchModel {
  const [query, setQuery] = useState("");
  const { byColumn, archived } = board;
  const terms = useMemo(() => parseTerms(query), [query]);

  return useMemo(() => {
    let total = archived.length;
    for (const col of COLUMNS) total += byColumn[col].length;

    if (terms.length === 0) {
      return {
        query,
        setQuery,
        terms,
        byColumn,
        archived,
        visibleKeys: NO_KEYS,
        matched: total,
        total,
        active: false,
      };
    }

    const visibleKeys = new Set<string>();
    const keep = (job: WebJob): boolean => {
      if (!matchJob(job, terms)) return false;
      visibleKeys.add(jobKey(job));
      return true;
    };

    const cols = {} as Record<Column, WebJob[]>;
    let matched = 0;
    for (const col of COLUMNS) {
      cols[col] = byColumn[col].filter(keep);
      matched += cols[col].length;
    }
    const arch = archived.filter(keep);
    matched += arch.length;

    return {
      query,
      setQuery,
      terms,
      byColumn: cols,
      archived: arch,
      visibleKeys,
      matched,
      total,
      active: true,
    };
  }, [byColumn, archived, terms, query]);
}
