// Display preferences kept in this browser: theme (system, light, dark) and
// density (comfortable, compact). They are applied as data-theme and
// data-density on <html>; app/layout.tsx applies them before the first paint
// (PREFS_BOOT) so the page never flashes the wrong theme.

export type Theme = "system" | "light" | "dark";
export type Density = "comfortable" | "compact";
const TK = "cm-theme";
const DK = "cm-density";

function read(key: string) {
  try {
    return localStorage.getItem(key) ?? "";
  } catch {
    return "";
  }
}
function write(key: string, v: string) {
  try {
    localStorage.setItem(key, v);
  } catch {
    /* storage unavailable: the choice lasts for this page */
  }
}

export const getTheme = (): Theme => (["light", "dark"].includes(read(TK)) ? (read(TK) as Theme) : "system");
export const getDensity = (): Density => (read(DK) === "compact" ? "compact" : "comfortable");

export function applyPrefs() {
  const el = document.documentElement;
  const t = getTheme();
  if (t === "system") delete el.dataset.theme;
  else el.dataset.theme = t;
  el.dataset.density = getDensity();
}
export function setTheme(t: Theme) {
  write(TK, t);
  applyPrefs();
  window.dispatchEvent(new Event("cm-prefs"));
}
export function setDensity(d: Density) {
  write(DK, d);
  applyPrefs();
  window.dispatchEvent(new Event("cm-prefs"));
}

/** isDark: the dark palette is showing (manual choice or system preference). */
export function isDark(): boolean {
  if (typeof window === "undefined") return false;
  const t = getTheme();
  if (t !== "system") return t === "dark";
  return window.matchMedia?.("(prefers-color-scheme: dark)").matches ?? false;
}

/** PREFS_BOOT runs inline in <head>: language, theme and density before paint. */
export const PREFS_BOOT = `try{var d=document.documentElement,s=localStorage;var l=s.getItem("gcplab.lang");if(l==="en"||l==="es")d.lang=l;var t=s.getItem("cm-theme");if(t==="light"||t==="dark")d.dataset.theme=t;d.dataset.density=s.getItem("cm-density")==="compact"?"compact":"comfortable"}catch(e){}`;
