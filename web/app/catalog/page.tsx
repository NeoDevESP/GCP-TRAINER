"use client";

import { useEffect, useMemo, useState } from "react";
import Nav from "@/components/Nav";
import { api } from "@/lib/api";
import { useAuth } from "@/components/useAuth";
import { useI18n } from "@/lib/i18n";
import { label, labLevel, labType } from "@/lib/labels";
import type { LabSummary } from "@/lib/types";

interface Catalog {
  labs: LabSummary[];
  tracks: { id: string; title: string; labs: string[] }[];
}

export default function CatalogPage() {
  useAuth();
  const { t, lang } = useI18n();
  const [cat, setCat] = useState<Catalog | null>(null);
  const [track, setTrack] = useState("");
  const [q, setQ] = useState("");
  const [err, setErr] = useState("");
  useEffect(() => {
    api<Catalog>("/api/catalog").then(setCat).catch((e) => setErr(e.message));
  }, [lang]);

  const trackTitle = useMemo(() => Object.fromEntries((cat?.tracks ?? []).map((tr) => [tr.id, tr.title])), [cat]);

  const labs = useMemo(() => {
    const inTrack = track ? new Set(cat?.tracks.find((tr) => tr.id === track)?.labs ?? []) : null;
    return (cat?.labs ?? []).filter(
      (l) =>
        (!inTrack || inTrack.has(l.id)) &&
        (!q || (l.title + " " + l.summary + " " + l.skills.join(" ")).toLowerCase().includes(q.toLowerCase())),
    );
  }, [cat, track, q]);

  return (
    <>
      <Nav active="/catalog" />
      <main id="main" className="page">
        <h1>{t("Catálogo de laboratorios")}</h1>
        {err && <p className="error">{err}</p>}
        <div className="row" style={{ marginBottom: 16 }}>
          <select value={track} onChange={(e) => setTrack(e.target.value)} style={{ width: 280 }} aria-label={t("Ruta")}>
            <option value="">{t("Todas las rutas")}</option>
            {(cat?.tracks ?? []).map((tr) => (
              <option key={tr.id} value={tr.id}>
                {tr.title}
              </option>
            ))}
          </select>
          <input placeholder={t("Buscar laboratorios o habilidades…")} aria-label={t("Buscar")} value={q} onChange={(e) => setQ(e.target.value)} style={{ maxWidth: 360 }} />
          <span className="muted small">{t("{n} laboratorios", { n: labs.length })}</span>
        </div>
        <div className="grid three">
          {labs.map((l) => (
            <a key={l.id} href={`/lab?id=${encodeURIComponent(l.id)}`} className="card" style={{ color: "inherit", textDecoration: "none" }}>
              <div className="row small muted">
                <span>
                  {trackTitle[l.track] ?? l.track}
                  {l.day ? ` · ${t("día {n}", { n: l.day })}` : ""}
                </span>
                <span className="pill">{label(labType, l.type, t)}</span>
                <span className="pill">{label(labLevel, l.level, t)}</span>
              </div>
              <h3 style={{ margin: "6px 0" }}>{l.title}</h3>
              <p className="small muted" style={{ margin: 0 }}>{l.summary}</p>
              <div className="row small muted" style={{ marginTop: 8 }}>
                <span aria-label={t("dificultad {n} de 5", { n: l.difficulty })}>{"★".repeat(Math.max(1, Math.min(5, l.difficulty)))}</span>
                <span>{l.minutes} min</span>
                <span>{l.fidelity?.join("/")}</span>
              </div>
            </a>
          ))}
        </div>
      </main>
    </>
  );
}
