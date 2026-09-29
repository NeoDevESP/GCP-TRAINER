"use client";

// Nav is the application shell, laid out like the Google Cloud console: a
// top bar (menu, product name, search, language, help and account) and a
// collapsible navigation sidebar.

import { useEffect, useRef, useState } from "react";
import { api, setToken } from "@/lib/api";
import { changeLang, k, useI18n, type Lang } from "@/lib/i18n";
import type { User } from "@/lib/types";
import { Icon } from "@/components/console/icons";

const links: [string, string, string][] = [
  ["/dashboard", k("Panel"), "home"],
  ["/catalog", k("Laboratorios"), "compute"],
  ["/incidents", k("Incidentes"), "bell"],
  ["/company", k("Empresa"), "project"],
  ["/skills", k("Grafo de habilidades"), "pubsub"],
  ["/leaderboard", k("Ligas"), "bigquery"],
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

function sideOpen(): boolean {
  // On narrow screens the menu overlays the page: start closed there.
  if (typeof window !== "undefined" && window.matchMedia?.("(max-width: 900px)").matches) return false;
  try {
    return localStorage.getItem("gcplab.side") !== "closed";
  } catch {
    return true;
  }
}

export default function Nav({ active }: { active?: string }) {
  const { t } = useI18n();
  const [user, setUser] = useState<User | null>(null);
  const [open, setOpen] = useState(true);
  const [account, setAccount] = useState(false);
  const [q, setQ] = useState("");
  const acc = useRef<HTMLDivElement>(null);
  useEffect(() => {
    api<{ user: User }>("/api/me")
      .then((p) => setUser(p.user))
      .catch(() => setUser(null));
    setOpen(sideOpen());
  }, []);
  useEffect(() => {
    document.body.dataset.side = open ? "open" : "closed";
    return () => {
      delete document.body.dataset.side;
    };
  }, [open]);
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
  const toggle = () => {
    const next = !open;
    setOpen(next);
    try {
      localStorage.setItem("gcplab.side", next ? "open" : "closed");
    } catch {
      /* storage unavailable */
    }
  };
  const items = [...links, ...(user && (user.role === "admin" || user.role === "instructor") ? [["/admin", k("Instructor"), "person"] as [string, string, string]] : [])];
  return (
    <>
      <a className="skip-link" href="#main">
        {t("Saltar al contenido")}
      </a>
      <header className="gtop">
        <button type="button" className="gicon-btn" aria-label={t("Menú de navegación")} aria-expanded={open} aria-controls="gside" onClick={toggle}>
          <Icon name="menu" />
        </button>
        <a className="gbrand" href="/dashboard">
          <span className="gbrand-mark" aria-hidden="true" />
          Cloud Mastery
        </a>
        <form
          className="gsearch"
          role="search"
          onSubmit={(e) => {
            e.preventDefault();
            window.location.href = `/catalog?q=${encodeURIComponent(q)}`;
          }}
        >
          <Icon name="search" />
          <input value={q} onChange={(e) => setQ(e.target.value)} placeholder={t("Buscar laboratorios, habilidades y servicios")} aria-label={t("Buscar laboratorios")} />
        </form>
        <div className="gtop-actions">
          <LangSwitch />
          <a className="gicon-btn" href="/catalog" title={t("Laboratorios")} aria-label={t("Laboratorios")}>
            <Icon name="shell" />
          </a>
          <div className="gpopwrap" ref={acc}>
            <button type="button" className="gavatar" aria-label={t("Cuenta: {name}", { name: user?.name ?? "" })} aria-expanded={account} onClick={() => setAccount(!account)}>
              {(user?.name ?? "?").slice(0, 1).toUpperCase()}
            </button>
            {account && (
              <div className="gpop" role="dialog" aria-label={t("Cuenta")}>
                <div className="gavatar big" aria-hidden="true">{(user?.name ?? "?").slice(0, 1).toUpperCase()}</div>
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
      </header>
      <nav id="gside" className="gside" aria-label={t("Principal")} hidden={!open}>
        {items.map(([href, label, icon]) => (
          <a key={href} href={href} className={active === href ? "active" : ""} aria-current={active === href ? "page" : undefined}>
            <Icon name={icon} />
            <span>{t(label)}</span>
          </a>
        ))}
      </nav>
    </>
  );
}
