// Theme: light / dark / system, persisted in localStorage. "system" follows the
// OS preference live via matchMedia.
import { useEffect, useState } from "react";

export type Theme = "light" | "dark" | "system";
const KEY = "plato.theme";

export function getTheme(): Theme {
  const v = localStorage.getItem(KEY);
  return v === "light" || v === "dark" || v === "system" ? v : "system";
}

function systemDark(): boolean {
  return window.matchMedia("(prefers-color-scheme: dark)").matches;
}

function apply(theme: Theme) {
  const dark = theme === "dark" || (theme === "system" && systemDark());
  document.documentElement.setAttribute("data-theme", dark ? "dark" : "light");
}

// useTheme returns the current theme and a setter, applying it to <html>.
export function useTheme(): [Theme, (t: Theme) => void] {
  const [theme, setThemeState] = useState<Theme>(getTheme());

  useEffect(() => {
    apply(theme);
    // When in system mode, react to OS preference changes.
    if (theme !== "system") return;
    const mq = window.matchMedia("(prefers-color-scheme: dark)");
    const onChange = () => apply("system");
    mq.addEventListener("change", onChange);
    return () => mq.removeEventListener("change", onChange);
  }, [theme]);

  const setTheme = (t: Theme) => {
    localStorage.setItem(KEY, t);
    setThemeState(t);
  };
  return [theme, setTheme];
}
