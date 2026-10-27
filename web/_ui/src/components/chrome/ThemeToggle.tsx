import { useState } from "react";

import type { ThemePref } from "../../globals.d.ts";

const THEME_CYCLE: Record<ThemePref, ThemePref> = {
  system: "light",
  light: "dark",
  dark: "system",
};

export function ThemeToggle() {
  const [pref, setPref] = useState<ThemePref>(() => window.driverTheme?.get() ?? "system");
  return (
    <button
      className="theme-btn"
      id="theme-toggle"
      title="theme: system → light → dark"
      onClick={() => {
        const next = THEME_CYCLE[pref];
        window.driverTheme?.set(next);
        setPref(next);
      }}
    >
      ◐ {pref.toUpperCase()}
    </button>
  );
}
