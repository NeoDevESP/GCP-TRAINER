"use client";

// Spanish is the primary language of the interface; English is a translation.
// Texts are written in Spanish inside t("…"); lib/en.ts maps them to English.
// `npm run check:i18n` (run before every build) fails when a t("…") string has
// no English entry.

import { useCallback, useEffect, useState } from "react";
import { en } from "./en";

export type Lang = "es" | "en";
const KEY = "gcplab.lang";

export function getLang(): Lang {
  try {
    const v = localStorage.getItem(KEY);
    if (v === "en" || v === "es") return v;
  } catch {
    /* storage unavailable */
  }
  return "es";
}

export function storeLang(l: Lang) {
  try {
    localStorage.setItem(KEY, l);
  } catch {
    /* storage unavailable: the choice lasts for this page only */
  }
}

/** k marks a Spanish string that is translated later with t(variable). */
export const k = (s: string) => s;

/** translate outside React (pure helper). */
export function translate(lang: Lang, text: string, vars?: Record<string, string | number>): string {
  let s = lang === "en" ? en[text] ?? text : text;
  if (vars) for (const [k, v] of Object.entries(vars)) s = s.split(`{${k}}`).join(String(v));
  return s;
}

/**
 * useI18n returns the current language and t(). The first render is Spanish
 * (the static export is prerendered in Spanish); the stored choice is applied
 * right after mount, so there is no hydration mismatch.
 */
export function useI18n() {
  const [lang, setLangState] = useState<Lang>("es");
  useEffect(() => {
    const l = getLang();
    setLangState(l);
    document.documentElement.lang = l;
  }, []);
  const t = useCallback((text: string, vars?: Record<string, string | number>) => translate(lang, text, vars), [lang]);
  return { lang, t };
}

/** changeLang stores the language, saves it on the account and reloads. */
export async function changeLang(l: Lang, save: (l: Lang) => Promise<unknown>) {
  storeLang(l);
  try {
    await save(l);
  } catch {
    /* not signed in: the browser preference is enough */
  }
  window.location.reload();
}
