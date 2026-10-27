import { useCallback, useState } from "react";

export function readLS(key: string): string | null {
  try {
    return localStorage.getItem(key);
  } catch {
    return null;
  }
}

export function writeLS(key: string, value: string | null): void {
  try {
    if (value === null) localStorage.removeItem(key);
    else localStorage.setItem(key, value);
  } catch {
    // storage unavailable (private mode, quota) — the in-memory value still wins
  }
}

export function useLocalStorageFlag(key: string, initial: boolean): [boolean, (v: boolean) => void] {
  const [value, setValue] = useState(() => {
    const raw = readLS(key);
    return raw === null ? initial : raw === "1";
  });
  const set = useCallback(
    (v: boolean) => {
      setValue(v);
      writeLS(key, v ? "1" : "0");
    },
    [key],
  );
  return [value, set];
}
