"use client";

import { useEffect, useMemo, useState } from "react";
import Nav from "@/components/Nav";
import { api, qs } from "@/lib/api";
import { useAuth } from "@/components/useAuth";
import { useI18n } from "@/lib/i18n";
import { label, labLevel } from "@/lib/labels";
import type { LabSummary } from "@/lib/types";
import { Icon } from "@/components/console/icons";
import { PathMap, TrackIcon, TypeLegend, TypeTag, labHref, nextLab, useProgress } from "@/components/learn";

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

  return (
    <>
      <Nav active="/catalog" />
      <main id="main" className="page">
        <h1>{t("Catálogo de laboratorios")}</h1>
        {err && <p className="error">{err}</p>}

        <div className="row" style={{ marginBottom: 16 }}>
          <input placeholder={t("Buscar laboratorios o habilidades…")} aria-label={t("Buscar")} value={q} onChange={(e) => setQ(e.target.value)} style={{ maxWidth: 360 }} />
          <div className="lt-chips" role="group" aria-label={t("Nivel")}>
            <button type="button" className="lt-chip" aria-pressed={!flat && !track} onClick={() => { setAll(false); setLevel(""); setQ(""); open(""); }}>
              <Icon name="project" size={16} /> {t("Por rutas")}
            </button>
            <button type="button" className="lt-chip" aria-pressed={all && !level && !q} onClick={() => { setAll(true); setLevel(""); }}>
              {t("Todos ({n})", { n: cat?.labs.length ?? 0 })}
            </button>
            {LEVELS.map((lv) => (
              <button key={lv} type="button" className="lt-chip" aria-pressed={level === lv} onClick={() => setLevel(level === lv ? "" : lv)}>
                {label(labLevel, lv, t)}
              </button>
            ))}
          </div>
        </div>

        {flat ? (
          <>
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
            <div className="lt-hero" style={{ marginTop: 16 }}>
              <TrackIcon id={cur.id} index={curIndex} size={64} />
              <div style={{ flex: 1 }}>
                <h2 style={{ marginTop: 0 }}>{cur.title}</h2>
                <p>{cur.description}</p>
                <div className="lt-route-foot">
                  <span>{t("{n} de {total} superados", { n: cur.labs.filter((id) => state[id] === "done").length, total: cur.labs.length })}</span>
                  <span className="lt-bar" aria-hidden="true"><span style={{ width: `${(cur.labs.filter((id) => state[id] === "done").length / Math.max(1, cur.labs.length)) * 100}%` }} /></span>
                </div>
              </div>
            </div>
            <div className="grid two" style={{ marginTop: 16, alignItems: "start" }}>
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
            <p className="muted" style={{ marginTop: 0 }}>{t("Elige una ruta y sigue sus pasos en orden. Si no sabes por dónde empezar, prueba «Bases» o el programa de 30 días de Associate Cloud Engineer.")}</p>
            <div className="lt-routes">
              {(cat?.tracks ?? []).map((tr, i) => {
                const done = tr.labs.filter((id) => state[id] === "done").length;
                const nx = nextLab(tr.labs, state);
                return (
                  <button key={tr.id} type="button" className="lt-route" onClick={() => open(tr.id)}>
                    <span className="lt-route-head">
                      <TrackIcon id={tr.id} index={i} />
                      <h3>{tr.title}</h3>
                    </span>
                    <p>{tr.description}</p>
                    <span className="lt-route-foot">
                      <span>{t("{n} laboratorios", { n: tr.labs.length })}</span>
                      <span className="lt-bar" aria-hidden="true"><span style={{ width: `${(done / Math.max(1, tr.labs.length)) * 100}%` }} /></span>
                      <span>{done}/{tr.labs.length}</span>
                    </span>
                    {done > 0 && nx && byId[nx] && <span className="small muted">{t("Siguiente:")} {byId[nx].title}</span>}
                  </button>
                );
              })}
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
