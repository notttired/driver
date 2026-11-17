export type SideView = "conversation" | "notes";

export interface SideSwitchProps {
  view: SideView;
  onSwitch: (view: SideView) => void;
}

/**
 * The sidebar's head: CONVERSATION and NOTES as two tabs over one space —
 * the conversation by default, the notes when you ask. Both panels stay
 * mounted (the notes keep their unsaved typing, the record its scroll);
 * the one not shown is hidden.
 */
export function SideSwitch({ view, onSwitch }: SideSwitchProps) {
  const tab = (v: SideView, label: string) => (
    <button
      key={v}
      type="button"
      role="tab"
      className={"side-tab" + (view === v ? " selected" : "")}
      aria-selected={view === v}
      onClick={() => onSwitch(v)}
    >
      {label}
    </button>
  );
  return (
    <div className="side-switch" role="tablist" aria-label="sidebar">
      {tab("conversation", "CONVERSATION")}
      {tab("notes", "NOTES")}
    </div>
  );
}
