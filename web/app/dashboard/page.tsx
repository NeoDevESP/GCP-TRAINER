"use client";

import { useEffect, useState } from "react";
import Nav from "@/components/Nav";
import Bar from "@/components/Bar";
import { api } from "@/lib/api";
import { useAuth } from "@/components/useAuth";
import { useI18n } from "@/lib/i18n";
import { dimension, label, recKind } from "@/lib/labels";
import type { Profile, Recommendation } from "@/lib/types";

async function startRec(r: Recommendation) {
  if (r.gen) {
    const s = await api<{ id: string }>("/api/incidents", { body: r.gen });
    window.location.href = `/lab?session=${s.id}`;
  } else if (r.labId) {
    window.location.href = `/lab?id=${encodeURIComponent(r.labId)}`;
  }
}

function RecList({ items }: { items: Recommendation[] | null }) {
  const { t } = useI18n();
  const [err, setErr] = useState("");
  if (!items?.length) return <p className="muted">{t("No hay nada en cola: elige cualquier laboratorio del catálogo.")}</p>;
  return (
    <div className="col">
      {items.map((r, i) => (
        <div key={i} className="row" style={{ justifyContent: "space-between", borderBottom: "1px solid var(--border)", paddingBottom: 8 }}>
          <div style={{ minWidth: 0, flex: 1 }}>
            <div>
              <strong>{r.title}</strong> <span className="pill">{label(recKind, r.kind, t)}</span>
            </div>
            <div className="muted small">{r.reason}</div>
          </div>
          <button className="btn secondary" onClick={() => startRec(r).catch((e) => setErr(e.message))}>
            {t("Empezar")}
          </button>
        </div>
      ))}
      {err && <p className="error small">{err}</p>}
    </div>
  );
}

export default function Dashboard() {
  useAuth();
  const { t, lang } = useI18n();
  const [p, setP] = useState<Profile | null>(null);
  const [attempts, setAttempts] = useState<any[]>([]);
  const [err, setErr] = useState("");
  const [track, setTrack] = useState("ace-30");
  const [tracks, setTracks] = useState<{ id: string; title: string }[]>([]);
  useEffect(() => {
    api<{ tracks: { id: string; title: string }[] }>("/api/catalog")
      .then((c) => setTracks(c.tracks ?? []))
      .catch(() => {});
  }, [lang]);

  useEffect(() => {
    api<Profile>(`/api/me?track=${track}`).then(setP).catch((e) => setErr(e.message));
  }, [track, lang]);
  useEffect(() => {
    api<any[]>("/api/me/attempts").then((a) => setAttempts(a ?? [])).catch(() => {});
  }, []);

  const downloadTranscript = async () => {
    const tr = await api("/api/me/transcript");
    const blob = new Blob([JSON.stringify(tr, null, 2)], { type: "application/json" });
    const a = document.createElement("a");
    a.href = URL.createObjectURL(blob);
    a.download = "cloud-mastery-expediente.json";
    a.click();
  };

  if (err) return (<><Nav active="/dashboard" /><main id="main" className="page"><p className="error">{err}</p></main></>);
  if (!p) return (<><Nav active="/dashboard" /><main id="main" className="page"><p className="muted">{t("Cargando…")}</p></main></>);

  const running = attempts.filter((a) => a.status === "running");
  const date = (d: string) => new Date(d).toLocaleDateString(lang === "en" ? "en-GB" : "es-ES");
  return (
    <>
      <Nav active="/dashboard" />
      <main id="main" className="page">
        <div className="row" style={{ justifyContent: "space-between" }}>
          <h1>{t("Hola de nuevo, {name}", { name: p.user.name })}</h1>
          <button className="btn secondary" onClick={downloadTranscript}>
            {t("Descargar expediente firmado")}
          </button>
        </div>
        <div className="grid three">
          <div className="card">
            <div className="muted small">{t("Nivel")}</div>
            <div className="kpi">{p.level}</div>
            <Bar value={p.levelProgress * 100} />
            <div className="muted small">{t("{xp} XP · {bonus} de bonificación · racha de {streak} días", { xp: p.xp, bonus: p.bonusXp, streak: p.streak })}</div>
          </div>
          <div className="card">
            <div className="muted small">{t("Etapa profesional")}</div>
            <div className="kpi">{p.career.stage}</div>
            {p.career.next && (
              <div className="small muted">
                {t("Siguiente:")} <strong>{p.career.next}</strong>
                {p.career.nextNeeds?.length ? <ul>{p.career.nextNeeds.map((n) => <li key={n}>{n}</li>)}</ul> : null}
              </div>
            )}
          </div>
          <div className="card">
            <div className="muted small">{t("Autonomía")}</div>
            <div className="kpi">{p.student.autonomy.stage}</div>
            <Bar value={p.student.autonomy.progress * 100} />
            <div className="small muted">{p.student.autonomy.why}</div>
          </div>
        </div>

        {running.length > 0 && (
          <div className="card" style={{ marginTop: 16 }}>
            <h3>{t("Laboratorios en curso")}</h3>
            {running.map((a) => (
              <div key={a.id} className="row">
                <a href={`/lab?session=${a.sessionId ?? ""}&id=${encodeURIComponent(a.labId)}`}>{a.labId}</a>
                <span className="muted small">{t("empezado el {date}", { date: new Date(a.started).toLocaleString(lang === "en" ? "en-GB" : "es-ES") })}</span>
              </div>
            ))}
          </div>
        )}

        <div className="grid two" style={{ marginTop: 16 }}>
          <div className="card">
            <h2>{t("Plan adaptativo")}</h2>
            <RecList items={p.adaptive} />
            <h3>{t("Recomendaciones de la ruta")}</h3>
            <div className="row small">
              <label htmlFor="track" style={{ margin: 0 }}>
                {t("Ruta")}
              </label>
              <select id="track" value={track} onChange={(e) => setTrack(e.target.value)} style={{ width: "auto" }}>
                {(tracks.length ? tracks : [{ id: "ace-30", title: "Associate Cloud Engineer — 30 días" }]).map((tr) => (
                  <option key={tr.id} value={tr.id}>
                    {tr.title}
                  </option>
                ))}
              </select>
            </div>
            <RecList items={p.recommendations} />
          </div>
          <div className="card">
            <h2>{t("Dimensiones de competencia")}</h2>
            <table>
              <tbody>
                {p.student.dimensions.map((d) => (
                  <tr key={d.dimension}>
                    <td style={{ width: 170 }}>{label(dimension, d.dimension, t)}</td>
                    <td><Bar value={d.score * 100} label={label(dimension, d.dimension, t)} /></td>
                    <td className="muted small" style={{ width: 110 }}>{t("{pct}% · {n} evid.", { pct: Math.round(d.score * 100), n: d.evidence })}</td>
                  </tr>
                ))}
              </tbody>
            </table>
            {p.student.weakestDimensions?.length ? (
              <p className="small muted">{t("Céntrate ahora en: {list}", { list: p.student.weakestDimensions.map((d) => label(dimension, d, t)).join(", ") })}</p>
            ) : null}
          </div>
        </div>

        <div className="grid two" style={{ marginTop: 16 }}>
          <div className="card">
            <h2>{t("Preparación para certificaciones")}</h2>
            {p.readiness.map((r) => (
              <div key={r.cert} style={{ marginBottom: 10 }}>
                <div className="row" style={{ justifyContent: "space-between" }}>
                  <strong>{r.name}</strong>
                  <span>{Math.round(r.percent)}%</span>
                </div>
                <Bar value={r.percent} label={r.name} />
                <div className="small muted">{r.explanation}</div>
              </div>
            ))}
          </div>
          <div className="card">
            <h2>{t("Retención")}</h2>
            {p.student.retention?.length ? (
              <table>
                <thead><tr><th>{t("Habilidad")}</th><th>{t("Recuerdo")}</th><th>{t("Repaso")}</th></tr></thead>
                <tbody>
                  {p.student.retention.slice(0, 12).map((r) => (
                    <tr key={r.skill}>
                      <td>{r.skill}</td>
                      <td><Bar value={r.retention * 100} label={r.skill} /></td>
                      <td>{r.due ? <span className="pill warn">{t("toca repasar")}</span> : <span className="muted small">{date(r.dueAt)}</span>}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            ) : (
              <p className="muted">{t("Supera laboratorios sin pistas para empezar a construir memoria duradera.")}</p>
            )}
            {p.student.recurringErrors?.length ? (
              <>
                <h3>{t("Errores recurrentes")}</h3>
                <ul className="small">
                  {p.student.recurringErrors.map((e) => (
                    <li key={e.pattern}>{e.pattern} ×{e.count} <span className="muted">({e.labs.join(", ")})</span></li>
                  ))}
                </ul>
              </>
            ) : null}
          </div>
        </div>

        <div className="grid two" style={{ marginTop: 16 }}>
          <div className="card">
            <h2>{t("Habilidades")}</h2>
            {p.skills?.length ? (
              <table>
                <thead><tr><th>{t("Habilidad")}</th><th>{t("Dominio")}</th><th></th></tr></thead>
                <tbody>
                  {[...p.skills].sort((a, b) => b.mastery - a.mastery).map((s) => (
                    <tr key={s.skill}>
                      <td>{s.skill} {s.masteryBadge && <span className="pill ok">{t("dominada")}</span>}</td>
                      <td style={{ width: "40%" }}><Bar value={s.mastery} label={s.skill} /></td>
                      <td className="small muted">{Math.round(s.mastery)}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            ) : (
              <p className="muted">{t("Todavía no hay habilidades medidas.")}</p>
            )}
          </div>
          <div className="card">
            <h2>{t("Especializaciones")}</h2>
            {p.career.specializations?.map((s) => (
              <div key={s.id} style={{ marginBottom: 10 }}>
                <div className="row" style={{ justifyContent: "space-between" }}>
                  <strong>{s.name}</strong>
                  {s.done ? <span className="pill ok">{t("completada")}</span> : !s.unlocked ? <span className="pill">{t("bloqueada")}</span> : <span>{Math.round(s.percent)}%</span>}
                </div>
                <Bar value={s.percent} label={s.name} />
                {s.missing?.length ? <div className="small muted">{t("Falta: {list}", { list: s.missing.join(", ") })}</div> : null}
              </div>
            ))}
            <h3>{t("Insignias")}</h3>
            <div className="row">
              {p.badges?.length ? p.badges.map((b) => <span key={b.id} className="pill ok" title={b.reason}>{b.name}</span>) : <span className="muted">{t("Aún ninguna.")}</span>}
            </div>
          </div>
        </div>
      </main>
    </>
  );
}
