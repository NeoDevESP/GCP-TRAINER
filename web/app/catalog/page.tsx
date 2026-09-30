"use client";

import { useEffect, useMemo, useState } from "react";
import Nav from "@/components/Nav";
import { api, qs } from "@/lib/api";
import { useAuth } from "@/components/useAuth";
import { k, useI18n } from "@/lib/i18n";
import { label, labLevel } from "@/lib/labels";
import type { LabSummary } from "@/lib/types";
import { Icon } from "@/components/console/icons";
import { PathMap, TrackBadge, TypeLegend, TypeTag, labHref, nextLab, useProgress } from "@/components/learn";

interface Track {
  id: string;
  title: string;
  description?: string;
  level?: string;
  labs: string[];
}

interface Catalog {
  labs: LabSummary[];
  tracks: Track[];
}

const LEVELS = ["basic", "intermediate", "advanced", "professional"];
const TIERS: [string, string][] = [
  ["", k("Todas")],
  ["foundational", k("Empiezo de cero")],
  ["associate", k("Ya trabajo con cloud")],
  ["professional", k("Profesional")],
];

function LabCard({ l, trackTitle, st, running }: { l: LabSummary; trackTitle?: string; st?: string; running?: string }) {
  const { t } = useI18n();
  return (
    <a href={labHref(l.id, running)} className="card" style={{ color: "inherit", textDecoration: "none", display: "flex", flexDirection: "column", gap: 6 }}>
      <div className="row small muted">
        <TypeTag type={l.type} />
        <span className="pill">{label(labLevel, l.level, t)}</span>
        {st === "done" && <span className="pill ok">{t("superado")}</span>}
        {st === "doing" && <span className="pill warn">{t("empezado")}</span>}
      </div>
      <h3 style={{ margin: 0 }}>{l.title}</h3>
      <p className="small muted" style={{ margin: 0, flex: 1 }}>{l.summary}</p>
      <div className="row small muted">
        {trackTitle && <span>{trackTitle}</span>}
        <span>{t("{n} min", { n: l.minutes })}</span>
        <span className="lp-stars" aria-label={t("dificultad {n} de 5", { n: l.difficulty })}>{"●".repeat(Math.max(1, Math.min(5, l.difficulty)))}</span>
      </div>
    </a>
  );
}

export default function CatalogPage() {
  useAuth();
  const { t, lang } = useI18n();
  const [cat, setCat] = useState<Catalog | null>(null);
  const [track, setTrack] = useState("");
  const [all, setAll] = useState(false);
  const [level, setLevel] = useState("");
  const [tier, setTier] = useState("");
  const [q, setQ] = useState("");
  const [err, setErr] = useState("");
  const { state, running } = useProgress();
  useEffect(() => {
    setQ(qs("q"));
    setTrack(qs("track"));
  }, []);
  useEffect(() => {
    api<Catalog>("/api/catalog").then(setCat).catch((e) => setErr(e.message));
  }, [lang]);

  const byId = useMemo(() => Object.fromEntries((cat?.labs ?? []).map((l) => [l.id, l])), [cat]);
  const trackTitle = useMemo(() => Object.fromEntries((cat?.tracks ?? []).map((tr) => [tr.id, tr.title])), [cat]);
  const open = (id: string) => {
    setTrack(id);
    setAll(false);
    window.history.replaceState(null, "", id ? `/catalog?track=${encodeURIComponent(id)}` : "/catalog");
    window.scrollTo(0, 0);
  };

  const filtered = useMemo(
    () =>
      (cat?.labs ?? []).filter(
        (l) =>
          (!level || l.level === level) &&
          (!q || (l.title + " " + l.summary + " " + l.skills.join(" ")).toLowerCase().includes(q.toLowerCase())),
      ),
    [cat, q, level],
  );

  const cur = cat?.tracks.find((tr) => tr.id === track);
  const curIndex = cat?.tracks.findIndex((tr) => tr.id === track) ?? 0;
  const flat = all || !!q || !!level;

  const levelOf: Record<string, string> = { foundational: "foundational", associate: "associate", professional: "professional" };
  const routes = (cat?.tracks ?? []).map((tr, i) => ({ tr, i, done: tr.labs.filter((id) => state[id] === "done").length }));
  const shownRoutes = routes.filter((r) => !tier || levelOf[r.tr.level ?? ""] === tier);
  const feature = routes.find((r) => r.done > 0 && r.done < r.tr.labs.length) ?? routes.find((r) => r.tr.id === "ace-30") ?? routes[0];

  return (
    <>
      <Nav active="/catalog" />
      <main id="main" className="page">
        {err && <p className="error">{err}</p>}

        {flat ? (
          <>
            <header className="cm-head">
              <div>
                <button type="button" className="cc-link" onClick={() => { setAll(false); setLevel(""); setQ(""); open(""); }} style={{ padding: 0, border: 0, background: "none", color: "var(--cm-accent-2)", font: "500 14px var(--cm-sans)", cursor: "pointer" }}>← {t("Todas las rutas")}</button>
                <h1>{q ? t("Resultados de «{q}»", { q }) : t("Todos los laboratorios")}</h1>
              </div>
              <label className="cm-search">
                <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" aria-hidden="true"><circle cx="11" cy="11" r="7" /><path d="M20 20l-3.5-3.5" /></svg>
                <input type="search" placeholder={t("Buscar laboratorios o habilidades…")} aria-label={t("Buscar")} value={q} onChange={(e) => setQ(e.target.value)} />
              </label>
            </header>
            <div className="cm-levels" role="group" aria-label={t("Nivel del laboratorio")} style={{ marginBottom: 20 }}>
              <button type="button" aria-pressed={!level} onClick={() => setLevel("")}>{t("Todos ({n})", { n: cat?.labs.length ?? 0 })}</button>
              {LEVELS.map((lv) => (
                <button key={lv} type="button" aria-pressed={level === lv} onClick={() => setLevel(level === lv ? "" : lv)}>
                  {label(labLevel, lv, t)}
                </button>
              ))}
            </div>
            <p className="muted small">{t("{n} laboratorios", { n: filtered.length })}</p>
            <div className="grid three">
              {filtered.map((l) => <LabCard key={l.id} l={l} trackTitle={trackTitle[l.track] ?? l.track} st={state[l.id]} running={running[l.id]} />)}
            </div>
          </>
        ) : cur ? (
          <>
            <button type="button" className="btn secondary" onClick={() => open("")}>
              <Icon name="back" size={18} /> {t("Todas las rutas")}
            </button>
            <section className="cm-dark cm-feature" style={{ marginTop: 16 }}>
              <TrackBadge id={cur.id} index={curIndex} size={72} />
              <div style={{ flex: 1, minWidth: 0 }}>
                <div className="cm-kick">{t("{n} laboratorios", { n: cur.labs.length })}</div>
                <h2>{cur.title}</h2>
                <p style={{ margin: 0 }}>{cur.description}</p>
              </div>
              <div style={{ display: "flex", flexDirection: "column", alignItems: "flex-end", gap: 10, minWidth: 220 }}>
                <div className="cm-sub">{t("{n} de {total} superados", { n: cur.labs.filter((id) => state[id] === "done").length, total: cur.labs.length })}</div>
                <span className="cm-prog" style={{ width: 220 }}>
                  <div><span style={{ width: `${(cur.labs.filter((id) => state[id] === "done").length / Math.max(1, cur.labs.length)) * 100}%` }} /></div>
                </span>
                {(() => {
                  const nx = nextLab(cur.labs, state);
                  return nx ? <a className="cm-pillbtn light sm" href={labHref(nx, running[nx])}>{t("Continuar ruta")}</a> : null;
                })()}
              </div>
            </section>
            <div className="grid two" style={{ marginTop: 20, alignItems: "start" }}>
              <div className="card">
                <h3>{t("Pasos de la ruta")}</h3>
                <PathMap labs={cur.labs.map((id) => byId[id]).filter(Boolean)} state={state} running={running} />
              </div>
              <div className="card">
                <h3>{t("Tipos de laboratorio")}</h3>
                <TypeLegend />
              </div>
            </div>
          </>
        ) : (
          <>
            <header className="cm-head">
              <div>
                <h1>{t("¿Qué quieres aprender?")}</h1>
                <p className="muted" style={{ margin: "6px 0 0" }}>{t("Elige una ruta y sigue sus pasos en orden. Cada paso es un laboratorio real en la consola.")}</p>
              </div>
              <label className="cm-search">
                <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" aria-hidden="true"><circle cx="11" cy="11" r="7" /><path d="M20 20l-3.5-3.5" /></svg>
                <input type="search" placeholder={t("Buscar laboratorios o habilidades…")} aria-label={t("Buscar")} value={q} onChange={(e) => setQ(e.target.value)} />
              </label>
            </header>
            <div className="row" style={{ justifyContent: "space-between", marginBottom: 20 }}>
              <div className="cm-levels" role="group" aria-label={t("Tu nivel")}>
                {TIERS.map(([id, name]) => (
                  <button key={id} type="button" aria-pressed={tier === id} onClick={() => setTier(id)}>{t(name)}</button>
                ))}
              </div>
              <button type="button" className="btn secondary" onClick={() => setAll(true)}>{t("Ver todos los laboratorios ({n})", { n: cat?.labs.length ?? 0 })}</button>
            </div>

            {feature && !tier && (
              <button type="button" className="cm-dark cm-feature" onClick={() => open(feature.tr.id)} style={{ width: "100%", border: 0, textAlign: "left", font: "inherit", cursor: "pointer", marginBottom: 20 }}>
                <TrackBadge id={feature.tr.id} index={feature.i} size={72} />
                <span style={{ display: "flex", flexDirection: "column", gap: 6, flex: 1, minWidth: 0 }}>
                  <span className="cm-kick" style={{ color: "#C9D6FF", fontWeight: 500 }}>{feature.done > 0 ? t("Tu ruta en curso") : t("Recomendada para empezar")}</span>
                  <span style={{ font: "700 28px/34px var(--cm-display)" }}>{feature.tr.title}</span>
                  <span className="cm-sub" style={{ fontSize: 15 }}>{feature.tr.description}</span>
                </span>
                <span style={{ display: "flex", flexDirection: "column", alignItems: "flex-end", gap: 10, flex: "none" }}>
                  <span className="cm-sub">{t("{n} de {total} completados", { n: feature.done, total: feature.tr.labs.length })}</span>
                  <span className="cm-prog" style={{ width: 220 }}><div><span style={{ width: `${(feature.done / Math.max(1, feature.tr.labs.length)) * 100}%` }} /></div></span>
                  <span className="cm-pillbtn light sm">{feature.done > 0 ? t("Continuar ruta") : t("Ver la ruta")}</span>
                </span>
              </button>
            )}

            <div className="cm-grid3">
              {shownRoutes.filter((r) => tier || r.tr.id !== feature?.tr.id).map(({ tr, i, done }) => (
                <button key={tr.id} type="button" className="cm-card" onClick={() => open(tr.id)}>
                  <span className="row" style={{ justifyContent: "space-between", width: "100%" }}>
                    <TrackBadge id={tr.id} index={i} size={52} />
                    <span className="cm-lvl">{t(TIERS.find(([id]) => id === levelOf[tr.level ?? ""])?.[1] ?? "")}</span>
                  </span>
                  <h3>{tr.title}</h3>
                  <p>{tr.description}</p>
                  <span className="cm-prog" style={{ width: "100%" }}>
                    <div><span style={{ width: `${(done / Math.max(1, tr.labs.length)) * 100}%` }} /></div>
                    {t("{n} de {total}", { n: done, total: tr.labs.length })}
                  </span>
                </button>
              ))}
            </div>
            <div className="card lt-section">
              <h3>{t("Tipos de laboratorio")}</h3>
              <TypeLegend />
            </div>
          </>
        )}
      </main>
    </>
  );
}
