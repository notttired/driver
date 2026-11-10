// The four state columns, in display order, and the key of a lane (a column
// of your own). A leaf module on purpose: both the format helpers and the
// drag table import it, and neither may import the other.
export const COLUMNS = ["draft", "working", "blocked", "done"] as const;
export type StateColumn = (typeof COLUMNS)[number];
/** the built-in column a job sits in while its pi watches over the shipped work */
export const WATCHING = "watching";
export type LaneColumn = `lane:${number}`;
export type Column = StateColumn | typeof WATCHING | LaneColumn;

/** a column a card is parked in, away from the one its state names */
export const isParking = (column: string): boolean => column === WATCHING || laneId(column) !== null;

export const laneColumn = (id: number): LaneColumn => `lane:${id}`;

/** the lane id a column key names, or null for a state column */
export function laneId(column: string): number | null {
  if (!column.startsWith("lane:")) return null;
  const id = Number(column.slice(5));
  return Number.isInteger(id) && id > 0 ? id : null;
}

export function isStateColumn(column: string): column is StateColumn {
  return (COLUMNS as readonly string[]).includes(column);
}

/** The board's column order: the saved order, less what is gone, then every
 *  column it leaves out — the state columns in their own order, then lanes. */
export function columnOrder(saved: readonly string[], laneIds: readonly number[]): Column[] {
  const known = new Set<string>([...COLUMNS, WATCHING, ...laneIds.map(laneColumn)]);
  const out: Column[] = [];
  for (const k of saved) {
    if (known.has(k) && !out.includes(k as Column)) out.push(k as Column);
  }
  for (const k of known) {
    if (!out.includes(k as Column)) out.push(k as Column);
  }
  return out;
}
