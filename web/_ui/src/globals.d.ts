// The two pre-paint IIFEs in index.html publish these; the chrome's toggles
// are their only readers.
export {};

export type ThemePref = "light" | "dark" | "system";

declare global {
  interface Window {
    driverTheme?: {
      get(): ThemePref;
      set(pref: ThemePref): void;
    };
    driverHud?: {
      accents: readonly string[];
      getAccent(): string;
      setAccent(accent: string): void;
    };
  }
}
