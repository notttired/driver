// The four board columns, in display order. A leaf module on purpose: both
// the format helpers and the drag table import it, and neither may import the
// other.
export const COLUMNS = ["draft", "working", "blocked", "done"] as const;
export type Column = (typeof COLUMNS)[number];
