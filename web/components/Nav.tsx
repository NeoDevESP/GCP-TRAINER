"use client";

import { useEffect, useState } from "react";
import { api, setToken } from "@/lib/api";
import { changeLang, k, useI18n, type Lang } from "@/lib/i18n";
import type { User } from "@/lib/types";

const links = [
  ["/dashboard", k("Panel")],
  ["/catalog", k("Laboratorios")],
  ["/incidents", k("Incidentes")],
  ["/company", k("Empresa")],
  ["/skills", k("Grafo de habilidades")],
  ["/leaderboard", k("Ligas")],
];

export function LangSwitch() {
  const { lang, t } = useI18n();
  const pick = (l: Lang) => changeLang(l, (x) => api("/api/me/lang", { body: { lang: x } }));
  return (
    <div className="row" role="group" aria-label={t("Idioma")} style={{ gap: 4 }}>
      {(["es", "en"] as Lang[]).map((l) => (
        <button
          key={l}
          className={`btn ${lang === l ? "" : "secondary"}`}
          style={{ padding: "3px 8px", fontSize: 12 }}
          aria-pressed={lang === l}
          onClick={() => lang !== l && pick(l)}
        >
          {l === "es" ? "ES" : "EN"}
        </button>
      ))}
    </div>
  );
}

export default function Nav({ active }: { active?: string }) {
  const { t } = useI18n();
  const [user, setUser] = useState<User | null>(null);
  useEffect(() => {
    api<{ user: User }>("/api/me")
      .then((p) => setUser(p.user))
      .catch(() => setUser(null));
  }, []);
  return (
    <nav className="nav" aria-label={t("Principal")}>
      <a className="skip-link" href="#main">
        {t("Saltar al contenido")}
      </a>
      <a className="brand" href="/dashboard">
        Cloud <span>Mastery</span>
      </a>
      {links.map(([href, label]) => (
        <a key={href} href={href} className={active === href ? "active" : ""} aria-current={active === href ? "page" : undefined}>
          {t(label)}
        </a>
      ))}
      {user && (user.role === "admin" || user.role === "instructor") && (
        <a href="/admin" className={active === "/admin" ? "active" : ""} aria-current={active === "/admin" ? "page" : undefined}>
          {t("Instructor")}
        </a>
      )}
      <div className="spacer" />
      <LangSwitch />
      {user && <span className="muted small">{user.name}</span>}
      <button
        className="btn secondary"
        onClick={() => {
          setToken("");
          window.location.href = "/";
        }}
      >
        {t("Cerrar sesión")}
      </button>
    </nav>
  );
}
