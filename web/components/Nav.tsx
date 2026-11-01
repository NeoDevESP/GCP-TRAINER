"use client";

// Nav is the application shell of the learning pages. Five destinations —
// Inicio, Aprender, Practicar, Trabajo, Perfil — in a slim dark rail (a
// bottom tab bar on phones); the pages of the current destination appear as
// tabs above the content, with global search (Ctrl+K or /) and keyboard
// shortcuts (? lists them). The account menu holds language, theme and
// density. The lab workspace does not use it: there the Google Cloud console
// replica has its own top bar.

import { useEffect, useRef, useState, type ReactNode } from "react";
import CommandSearch, { type SearchItem } from "@/components/CommandSearch";
import { getDensity, getTheme, setDensity, setTheme, type Density, type Theme } from "@/lib/prefs";
import { api, setToken } from "@/lib/api";
import { changeLang, k, useI18n, type Lang } from "@/lib/i18n";
import type { User } from "@/lib/types";
import SaveStatus from "@/components/SaveStatus";

const S = (d: ReactNode) => (
  <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
    {d}
  </svg>
);

const ICONS: Record<string, ReactNode> = {
  home: S(<path d="M3 11l9-7 9 7v9a1 1 0 0 1-1 1h-5v-6H9v6H4a1 1 0 0 1-1-1z" />),
  map: S(<><path d="M9 4L3 6v14l6-2 6 2 6-2V4l-6 2z" /><path d="M9 4v14" /><path d="M15 6v14" /></>),
  alert: S(<><path d="M12 3l9 16H3z" /><path d="M12 10v4" /><path d="M12 17h.01" /></>),
  company: S(<><path d="M4 21V5a1 1 0 0 1 1-1h8a1 1 0 0 1 1 1v16" /><path d="M14 9h5a1 1 0 0 1 1 1v11" /><path d="M8 8h2M8 12h2M8 16h2" /><path d="M2 21h20" /></>),
  graph: S(<><circle cx="6" cy="6" r="2.5" /><circle cx="18" cy="8" r="2.5" /><circle cx="9" cy="18" r="2.5" /><path d="M8 7.5l7.7 0M7 8.3l1.4 7.4M16.4 10l-5.6 6.4" /></>),
  trophy: S(<><path d="M8 4h8v5a4 4 0 0 1-8 0z" /><path d="M8 6H5a3 3 0 0 0 3 4M16 6h3a3 3 0 0 1-3 4" /><path d="M12 13v4M8 21h8M9 17h6v4H9z" /></>),
  book: S(<><path d="M4 5a2 2 0 0 1 2-2h13v16H6a2 2 0 0 0-2 2z" /><path d="M4 21V5" /><path d="M9 7h6" /></>),
  bank: S(<><path d="M3 10l9-6 9 6" /><path d="M5 10v8M9.5 10v8M14.5 10v8M19 10v8" /><path d="M3 20h18" /></>),
  calendar: S(<><rect x="3" y="5" width="18" height="16" rx="2" /><path d="M3 10h18M8 3v4M16 3v4" /></>),
  headset: S(<><path d="M4 14v-2a8 8 0 0 1 16 0v2" /><rect x="3" y="14" width="4" height="6" rx="1.5" /><rect x="17" y="14" width="4" height="6" rx="1.5" /><path d="M19 20a3 3 0 0 1-3 2h-3" /></>),
  lifebuoy: S(<><circle cx="12" cy="12" r="9" /><circle cx="12" cy="12" r="3.5" /><path d="M5.6 5.6l3.9 3.9M14.5 14.5l3.9 3.9M18.4 5.6l-3.9 3.9M9.5 14.5l-3.9 3.9" /></>),
  teacher: S(<><circle cx="12" cy="7" r="3.5" /><path d="M5 21v-1a7 7 0 0 1 14 0v1" /></>),
  practice: S(<><rect x="3" y="4" width="18" height="14" rx="2" /><path d="M7 9l3 2.5L7 14M12 14h5" /><path d="M8 21h8" /></>),
  search: S(<><circle cx="11" cy="11" r="6.5" /><path d="M20 20l-4.2-4.2" /></>),
};

// The five destinations and their pages. The first page is where the
// destination opens; the key is the "g" shortcut that goes there.
type Page = [href: string, label: string];
interface Dest {
  id: string;
  label: string;
  icon: string;
  key: string;
  pages: Page[];
}
const DESTS: Dest[] = [
  { id: "home", label: k("Inicio"), icon: "home", key: "h", pages: [["/dashboard", k("Inicio")]] },
  { id: "learn", label: k("Aprender"), icon: "book", key: "a", pages: [["/learn", k("Lecciones")], ["/plan", k("Plan de estudio")], ["/guide", k("Guía de diagnóstico")]] },
  { id: "practice", label: k("Practicar"), icon: "practice", key: "p", pages: [["/catalog", k("Rutas y laboratorios")], ["/zentra", k("Zentra")], ["/incidents", k("Incidencias")]] },
  { id: "work", label: k("Trabajo"), icon: "headset", key: "t", pages: [["/shift", k("Turno de soporte")], ["/company", k("Empresa")]] },
  { id: "profile", label: k("Perfil"), icon: "teacher", key: "u", pages: [["/skills", k("Habilidades")], ["/leaderboard", k("Ligas")]] },
];

export function LangSwitch() {
  const { lang, t } = useI18n();
  const pick = (l: Lang) => changeLang(l, (x) => api("/api/me/lang", { body: { lang: x } }));
  return (
    <div className="lang-switch" role="group" aria-label={t("Idioma")}>
      {(["es", "en"] as Lang[]).map((l) => (
        <button key={l} type="button" className={lang === l ? "on" : ""} aria-pressed={lang === l} onClick={() => lang !== l && pick(l)}>
          {l === "es" ? "ES" : "EN"}
        </button>
      ))}
    </div>
  );
}

const typing = (el: EventTarget | null) => {
  const e = el as HTMLElement | null;
  return !!e && (e.isContentEditable || /^(INPUT|TEXTAREA|SELECT)$/.test(e.tagName) || !!e.closest?.(".xterm, .monaco-editor"));
};

const SHORTCUTS: [string, string][] = [
  ["Ctrl K  /  /", k("Buscar")],
  ["g h", k("Ir a Inicio")],
  ["g a", k("Ir a Aprender")],
  ["g p", k("Ir a Practicar")],
  ["g t", k("Ir a Trabajo")],
  ["g u", k("Ir a Perfil")],
  ["?", k("Ver los atajos")],
  ["Esc", k("Cerrar un diálogo")],
];

export default function Nav({ active }: { active?: string }) {
  const { t } = useI18n();
  const [user, setUser] = useState<User | null>(null);
  const [account, setAccount] = useState(false);
  const [search, setSearch] = useState(false);
  const [help, setHelp] = useState(false);
  const [theme, setThemeState] = useState<Theme>("system");
  const [density, setDensityState] = useState<Density>("comfortable");
  const acc = useRef<HTMLDivElement>(null);
  useEffect(() => {
    api<{ user: User }>("/api/me")
      .then((p) => setUser(p.user))
      .catch(() => setUser(null));
    document.body.classList.add("cm");
    setThemeState(getTheme());
    setDensityState(getDensity());
    return () => document.body.classList.remove("cm");
  }, []);
  useEffect(() => {
    if (!account) return;
    const close = (e: MouseEvent | KeyboardEvent) => {
      if (e instanceof KeyboardEvent ? e.key === "Escape" : !acc.current?.contains(e.target as Node)) setAccount(false);
    };
    window.addEventListener("mousedown", close);
    window.addEventListener("keydown", close);
    return () => {
      window.removeEventListener("mousedown", close);
      window.removeEventListener("keydown", close);
    };
  }, [account]);
  const dests = DESTS.map((d) => (d.id === "profile" && user && (user.role === "admin" || user.role === "instructor") ? { ...d, pages: [...d.pages, ["/admin", k("Instructor")] as Page] } : d));
  // keyboard: Ctrl/⌘+K or / search, ? help, g+letter goes to a destination
  useEffect(() => {
    let g = 0;
    const onKey = (e: KeyboardEvent) => {
      if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === "k") {
        e.preventDefault();
        setSearch(true);
        return;
      }
      if (e.ctrlKey || e.metaKey || e.altKey || typing(e.target)) return;
      if (e.key === "/") {
        e.preventDefault();
        setSearch(true);
      } else if (e.key === "?") {
        setHelp(true);
      } else if (e.key === "Escape") {
        setHelp(false);
      } else if (e.key === "g") {
        g = Date.now();
      } else if (Date.now() - g < 1200) {
        const d = DESTS.find((x) => x.key === e.key);
        if (d) window.location.href = d.pages[0][0];
        g = 0;
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, []);
  const cur = dests.find((d) => d.pages.some(([h]) => h === active));
  const pages: SearchItem[] = dests.flatMap((d) => d.pages.map(([href, label]) => ({ label: t(label), hint: t(d.label), href })));
  const initial = (user?.name ?? "?").slice(0, 1).toUpperCase();
  const pickTheme = (v: Theme) => {
    setTheme(v);
    setThemeState(v);
  };
  const pickDensity = (v: Density) => {
    setDensity(v);
    setDensityState(v);
  };
  return (
    <>
      <a className="skip-link" href="#main">
        {t("Saltar al contenido")}
      </a>
      <SaveStatus />
      <nav className="cm-rail" aria-label={t("Principal")}>
        <a className="cm-logo" href="/dashboard" aria-label="Cloud Mastery">
          <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
            <path d="M7 18a5 5 0 0 1-.9-9.9A6 6 0 0 1 17.6 7 4.5 4.5 0 0 1 17 18z" />
          </svg>
        </a>
        <div className="cm-rail-links">
          {dests.map((d) => (
            <a key={d.id} href={d.pages[0][0]} className={cur?.id === d.id ? "on" : ""} aria-current={cur?.id === d.id ? "page" : undefined}>
              {ICONS[d.icon]}
              <span>{t(d.label)}</span>
            </a>
          ))}
        </div>
        <div className="cm-rail-foot">
          <div className="cm-acc" ref={acc}>
            <button type="button" className="cm-avatar" aria-label={t("Cuenta y ajustes: {name}", { name: user?.name ?? "" })} aria-expanded={account} aria-haspopup="dialog" onClick={() => setAccount(!account)}>
              {initial}
            </button>
            {account && (
              <div className="cm-pop" role="dialog" aria-label={t("Cuenta y ajustes")}>
                <div className="cm-avatar big" aria-hidden="true">{initial}</div>
                <div className="gpop-name">{user?.name}</div>
                <div className="gpop-mail">{user?.email}</div>
                <div className="cm-pref">
                  <span className="cm-pref-label" id="pf-lang">{t("Idioma")}</span>
                  <LangSwitch />
                </div>
                <div className="cm-pref" role="radiogroup" aria-labelledby="pf-theme">
                  <span className="cm-pref-label" id="pf-theme">{t("Tema")}</span>
                  <div className="seg">
                    {([["system", k("Sistema")], ["light", k("Claro")], ["dark", k("Oscuro")]] as [Theme, string][]).map(([v, l]) => (
                      <button key={v} type="button" role="radio" aria-checked={theme === v} className={theme === v ? "on" : ""} onClick={() => pickTheme(v)}>
                        {t(l)}
                      </button>
                    ))}
                  </div>
                </div>
                <div className="cm-pref" role="radiogroup" aria-labelledby="pf-density">
                  <span className="cm-pref-label" id="pf-density">{t("Densidad")}</span>
                  <div className="seg">
                    {([["comfortable", k("Cómoda")], ["compact", k("Compacta")]] as [Density, string][]).map(([v, l]) => (
                      <button key={v} type="button" role="radio" aria-checked={density === v} className={density === v ? "on" : ""} onClick={() => pickDensity(v)}>
                        {t(l)}
                      </button>
                    ))}
                  </div>
                </div>
                <button type="button" className="btn secondary" style={{ width: "100%", marginBottom: 8 }} onClick={() => { setAccount(false); setHelp(true); }}>
                  {t("Atajos de teclado")}
                </button>
                <button
                  type="button"
                  className="btn secondary"
                  style={{ width: "100%" }}
                  onClick={() => {
                    setToken("");
                    window.location.href = "/";
                  }}
                >
                  {t("Cerrar sesión")}
                </button>
              </div>
            )}
          </div>
        </div>
      </nav>
      <div className="cm-subnav">
        {cur && cur.pages.length > 1 ? (
          <nav aria-label={t(cur.label)} className="cm-tabs">
            {cur.pages.map(([href, label]) => (
              <a key={href} href={href} className={active === href ? "on" : ""} aria-current={active === href ? "page" : undefined}>
                {t(label)}
              </a>
            ))}
          </nav>
        ) : (
          <span />
        )}
        <button type="button" className="cm-search-btn" onClick={() => setSearch(true)} aria-keyshortcuts="Control+K /">
          {ICONS.search}
          <span className="cm-search-text">{t("Buscar")}</span>
          <kbd aria-hidden="true">Ctrl K</kbd>
        </button>
      </div>
      <CommandSearch open={search} onClose={() => setSearch(false)} pages={pages} />
      {help && (
        <div className="cs-backdrop" onMouseDown={(e) => e.target === e.currentTarget && setHelp(false)}>
          <div className="cs-dialog cs-help" role="dialog" aria-modal="true" aria-labelledby="kb-title" onKeyDown={(e) => e.key === "Escape" && setHelp(false)}>
            <h2 id="kb-title">{t("Atajos de teclado")}</h2>
            <dl>
              {SHORTCUTS.map(([keys, what]) => (
                <div key={keys}>
                  <dt><kbd>{keys}</kbd></dt>
                  <dd>{t(what)}</dd>
                </div>
              ))}
            </dl>
            <button type="button" className="btn" autoFocus onClick={() => setHelp(false)}>{t("Cerrar")}</button>
          </div>
        </div>
      )}
    </>
  );
}
