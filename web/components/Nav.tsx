"use client";

// Nav is the application shell of the learning pages: a slim dark rail with
// the main sections, the language switch and the account menu (a bottom tab
// bar on phones). The lab workspace does not use it: there the Google Cloud
// console replica has its own top bar.

import { useEffect, useRef, useState, type ReactNode } from "react";
import { api, setToken } from "@/lib/api";
import { changeLang, k, useI18n, type Lang } from "@/lib/i18n";
import type { User } from "@/lib/types";

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
  teacher: S(<><circle cx="12" cy="7" r="3.5" /><path d="M5 21v-1a7 7 0 0 1 14 0v1" /></>),
};

const links: [string, string, string][] = [
  ["/dashboard", k("Inicio"), "home"],
  ["/catalog", k("Rutas"), "map"],
  ["/incidents", k("Incidencias"), "alert"],
  ["/company", k("Empresa"), "company"],
  ["/skills", k("Habilidades"), "graph"],
  ["/leaderboard", k("Ligas"), "trophy"],
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

export default function Nav({ active }: { active?: string }) {
  const { t } = useI18n();
  const [user, setUser] = useState<User | null>(null);
  const [account, setAccount] = useState(false);
  const acc = useRef<HTMLDivElement>(null);
  useEffect(() => {
    api<{ user: User }>("/api/me")
      .then((p) => setUser(p.user))
      .catch(() => setUser(null));
    document.body.classList.add("cm");
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
  const items = [...links, ...(user && (user.role === "admin" || user.role === "instructor") ? [["/admin", k("Instructor"), "teacher"] as [string, string, string]] : [])];
  const initial = (user?.name ?? "?").slice(0, 1).toUpperCase();
  return (
    <>
      <a className="skip-link" href="#main">
        {t("Saltar al contenido")}
      </a>
      <nav className="cm-rail" aria-label={t("Principal")}>
        <a className="cm-logo" href="/dashboard" aria-label="Cloud Mastery">
          <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
            <path d="M7 18a5 5 0 0 1-.9-9.9A6 6 0 0 1 17.6 7 4.5 4.5 0 0 1 17 18z" />
          </svg>
        </a>
        <div className="cm-rail-links">
          {items.map(([href, label, icon]) => (
            <a key={href} href={href} className={active === href ? "on" : ""} aria-current={active === href ? "page" : undefined}>
              {ICONS[icon]}
              <span>{t(label)}</span>
            </a>
          ))}
        </div>
        <div className="cm-rail-foot">
          <LangSwitch />
          <div className="cm-acc" ref={acc}>
            <button type="button" className="cm-avatar" aria-label={t("Cuenta: {name}", { name: user?.name ?? "" })} aria-expanded={account} onClick={() => setAccount(!account)}>
              {initial}
            </button>
            {account && (
              <div className="cm-pop" role="dialog" aria-label={t("Cuenta")}>
                <div className="cm-avatar big" aria-hidden="true">{initial}</div>
                <div className="gpop-name">{user?.name}</div>
                <div className="gpop-mail">{user?.email}</div>
                <button
                  type="button"
                  className="btn secondary"
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
    </>
  );
}
