/** The job overlay's main/sidebar split, as the main pane's share in percent. */
export const DEFAULT_SPLIT = 50;
export const MIN_SPLIT = 25;
export const MAX_SPLIT = 75;
export const SPLIT_KEY = "driver-job-split";

export function clampSplit(pct: number): number {
  if (!Number.isFinite(pct)) return DEFAULT_SPLIT;
  return Math.min(MAX_SPLIT, Math.max(MIN_SPLIT, Math.round(pct)));
}

/** Where a pointer at clientX lands as a split, given the body's box. */
export function splitFromPointer(clientX: number, left: number, width: number): number {
  if (width <= 0) return DEFAULT_SPLIT;
  return clampSplit(((clientX - left) / width) * 100);
}
