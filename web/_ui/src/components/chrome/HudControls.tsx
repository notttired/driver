import { useState } from "react";

const FALLBACK_ACCENTS = ["violet", "cyan", "amber", "crimson", "green"] as const;

/** Cycles the accent hue; the pre-paint IIFE owns the attribute and storage. */
export function AccentToggle() {
  const [accent, setAccent] = useState(() => window.driverHud?.getAccent() ?? "violet");
  const accents = window.driverHud?.accents ?? FALLBACK_ACCENTS;
  return (
    <button
      className="theme-btn"
      id="accent-toggle"
      title="accent colour"
      onClick={() => {
        const i = accents.indexOf(accent);
        const next = accents[(i + 1) % accents.length] ?? "violet";
        window.driverHud?.setAccent(next);
        setAccent(next);
      }}
    >
      ◈ {accent.toUpperCase()}
    </button>
  );
}
