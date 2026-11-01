"use client";

import { useEffect, useState } from "react";
import Nav from "@/components/Nav";
import Bar from "@/components/Bar";
import { api } from "@/lib/api";
import { useAuth } from "@/components/useAuth";
import { useI18n } from "@/lib/i18n";
import { dimension, label, recKind } from "@/lib/labels";
import type { LabSummary, Profile, Recommendation, Stage } from "@/lib/types";
import { RouteLine, StepGuide, TrackBadge, labHref, nextLab, useProgress } from "@/components/learn";
import { k } from "@/lib/i18n";
import { planLocal, planSchedule } from "@/lib/plan";
import { dueKeys, loadReview, loadRun, syncStudy } from "@/lib/study";
import type { SkillScore } from "@/lib/types";

// the five competencies of a skill: [measured key, column name, field]
const COMPS: [string, string, keyof SkillScore][] = [
  ["comprehension", k("Comprensión"), "comprehension"],
  ["execution", k("Ejecución"), "correctness"],
  ["autonomy", k("Autonomía"), "independence"],
  ["retention", k("Retención"), "retention"],
  ["transfer", k("Transferencia"), "transfer"],
];

const greeting = () => {
  const h = new Date().getHours();
  return h < 6 ? k("Buenas noches, {name}") : h < 13 ? k("Buenos días, {name}") : h < 21 ? k("Buenas tardes, {name}") : k("Buenas noches, {name}");
};

const typeName = (ty: string) => ({ guided: k("Guiado"), challenge: k("Reto"), incident: k("Incidente"), capstone: k("Proyecto final"), boss: k("Jefe final"), "case-study": k("Caso práctico"), interview: k("Entrevista") })[ty] ?? ty;

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
  const [track, setTrackState] = useState("zentra");
  const [tracks, setTracks] = useState<{ id: string; title: string; description?: string; labs: string[]; stages?: Stage[] }[]>([]);
  const [labs, setLabs] = useState<Record<string, LabSummary>>({});
  const [q, setQ] = useState("");
  const { state, running: runningLab } = useProgress();
  const [study, setStudy] = useState<ReturnType<typeof planLocal> & { due: number; lastShift?: number } | null>(null);
  useEffect(() => {
    syncStudy()
      .catch(() => {})
      .finally(() => setStudy({ ...planLocal(), due: dueKeys(loadReview()).length, lastShift: loadRun<{ history: { score: number }[] }>("cm-shift")?.history?.[0]?.score }));
  }, []);
  useEffect(() => {
    try {
      const saved = localStorage.getItem("gcplab.track");
      if (saved) setTrackState(saved);
    } catch {
      /* storage unavailable */
    }
  }, []);
  const setTrack = (id: string) => {
    setTrackState(id);
    try {
      localStorage.setItem("gcplab.track", id);
    } catch {
      /* storage unavailable */
    }
  };
  useEffect(() => {
    api<{ tracks: { id: string; title: string; description?: string; labs: string[]; stages?: Stage[] }[]; labs: LabSummary[] }>("/api/catalog")
      .then((c) => {
        setTracks(c.tracks ?? []);
        setLabs(Object.fromEntries((c.labs ?? []).map((l) => [l.id, l])));
      })
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
  const cur = tracks.find((x) => x.id === track) ?? tracks[0];
  const pathLabs = (cur?.labs ?? []).map((id) => labs[id]).filter(Boolean) as LabSummary[];
  const doneN = pathLabs.filter((l) => state[l.id] === "done").length;
  const nextId = nextLab(pathLabs.map((l) => l.id), state);
  const next = nextId ? labs[nextId] : undefined;
  const plan = study && Object.keys(labs).length ? planSchedule({ labs, state, ...study }, {}, study.prefs) : null;
  const pday = plan?.sessions[0];
  const zIds = (tracks.find((x) => x.id === "zentra")?.labs ?? []).filter((id) => labs[id]);
  const zNext = nextLab(zIds, state);
  const zStages = tracks.find((x) => x.id === "zentra")?.stages ?? [];
  const finalDone = !!study && !!loadRun<{ history: { passed: boolean }[] }>("cm-final")?.history?.some((h) => h.passed);
  const zDone = zIds.filter((id) => state[id] === "done").length;
  return (
    <>
      <Nav active="/dashboard" />
      <main id="main" className="page">
        <header className="cm-head">
          <div>
            <div className="muted">
              {p.streak > 0 ? t("{n} días seguidos practicando", { n: p.streak }) : t("Hoy es un buen día para empezar")} · {t("Nivel")} {p.level} · {p.xp} XP
            </div>
            <h1>{t(greeting(), { name: p.user.name.split(" ")[0] })}</h1>
          </div>
          <form
            className="cm-search"
            role="search"
            onSubmit={(e) => {
              e.preventDefault();
              window.location.href = `/catalog?q=${encodeURIComponent(q)}`;
            }}
          >
            <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" aria-hidden="true"><circle cx="11" cy="11" r="7" /><path d="M20 20l-3.5-3.5" /></svg>
            <input type="search" value={q} onChange={(e) => setQ(e.target.value)} placeholder={t("Busca un laboratorio o un servicio")} aria-label={t("Buscar laboratorios")} />
          </form>
        </header>

        {zStages.length > 0 && (
          <section className="cm-section yp" aria-labelledby="yp-title" style={{ marginTop: 0 }}>
            <div className="cm-section-head">
              <div>
                <h2 id="yp-title">{zDone === 0 ? t("Empieza aquí, desde cero") : t("Tu camino")}</h2>
                <p className="muted small" style={{ margin: "4px 0 0", maxWidth: 760 }}>
                  {zDone === 0
                    ? t("No necesitas saber nada de la nube. Sigue el camino en orden: la Semana 0 te enseña lo básico con talleres guiados y la Semana 1, el oficio de soporte (tickets, prioridades, escalados, P1, cambios y postmortem); después, ocho semanas de casos reales en Zentra, de básico a profesional (la última, los tickets más habituales en banca), y el examen final.")
                    : t("Sigue en orden: cada semana termina con una prueba y la siguiente sube de nivel.")}
                </p>
              </div>
              {zNext && (
                <a className="cm-pillbtn ink sm" href={labHref(zNext, runningLab[zNext])}>
                  {zDone === 0 ? t("Empezar el primer taller") : t("Siguiente paso")} →
                </a>
              )}
            </div>
            <ol className="yp-steps">
              {zStages.map((s, i) => {
                const done = s.labs.filter((id) => state[id] === "done").length;
                const st = done === s.labs.length ? "done" : zNext && s.labs.includes(zNext) ? "current" : "todo";
                return (
                  <li key={s.id} className={`yp-step ${st}`}>
                    <a href={`/catalog?track=zentra#${s.id}`} aria-current={st === "current" ? "step" : undefined}>
                      <span className="yp-dot" aria-hidden="true">{st === "done" ? "✓" : i}</span>
                      <span className="yp-name">{s.title.replace(/^[^·]*·\s*/, "")}</span>
                      <span className="yp-count">{t("{done}/{total}", { done, total: s.labs.length })}</span>
                    </a>
                  </li>
                );
              })}
              <li className={`yp-step ${finalDone ? "done" : !zNext ? "current" : "todo"}`}>
                <a href="/final">
                  <span className="yp-dot" aria-hidden="true">{finalDone ? "✓" : "★"}</span>
                  <span className="yp-name">{t("Examen final")}</span>
                  <span className="yp-count">{t("Soporte N2")}</span>
                </a>
              </li>
            </ol>
            {zNext && labs[zNext] && <p className="small yp-next">{t("Ahora toca: {title}", { title: labs[zNext].title })}</p>}
          </section>
        )}

        {plan && (pday || plan.done === plan.total) && (
          <section className="cm-section" aria-labelledby="today-title" style={{ marginTop: 0 }}>
            <div className="cm-section-head">
              <h2 id="today-title">{t("Hoy toca")}</h2>
            </div>
            <ul className="db-today">
              <li>
                <a className="cm-card" href="/plan">
                  <span className="db-kick">{t("Tu plan de estudio")}</span>
                  <strong>{pday ? t("La sesión de hoy") : t("¡Plan completado!")}</strong>
                  {pday && <span className="muted small">{t("{n} actividades · {m}", { n: pday.entries.length, m: pday.minutes + " min" })}</span>}
                  <span className="db-meter" aria-hidden="true"><span style={{ width: `${(100 * plan.done) / Math.max(1, plan.total)}%` }} /></span>
                </a>
              </li>
              <li>
                <a className="cm-card" href={study!.due ? "/learn?review=1" : "/learn"}>
                  <span className="db-kick">{t("Repaso")}</span>
                  <strong>{study!.due ? t("{n} preguntas para repasar hoy", { n: study!.due }) : t("Repaso al día")}</strong>
                  <span className="muted small">{study!.due ? t("Las que fallaste, justo cuando toca repasarlas.") : t("Haz un cuestionario: lo que falles volverá aquí.")}</span>
                </a>
              </li>
              {zIds.length > 0 && (
                <li>
                  <a className="cm-card" href={zNext ? labHref(zNext, runningLab[zNext]) : "/zentra"}>
                    <span className="db-kick">{t("Zentra · {done} de {total}", { done: zDone, total: zIds.length })}</span>
                    <strong>{zNext ? labs[zNext]?.title : t("Ruta de Zentra completada")}</strong>
                    <span className="muted small">{zNext && labs[zNext]?.ticket ? `${labs[zNext].ticket!.priority} · ${labs[zNext].ticket!.id}` : t("Tu primer mes en Soporte GCP")}</span>
                  </a>
                </li>
              )}
              <li>
                <a className="cm-card" href="/shift">
                  <span className="db-kick">{t("Turno de soporte")}</span>
                  <strong>{study!.shifts ? t("{n} turnos hechos", { n: study!.shifts }) : t("Tu primer turno")}</strong>
                  <span className="muted small">{study!.lastShift != null ? t("Última nota: {n}", { n: study!.lastShift }) : t("Varios tickets a la vez con su SLA")}</span>
                </a>
              </li>
            </ul>
          </section>
        )}

        {cur && (
          <div className="cm-hero-row">
            <section className="cm-dark cm-hero" aria-labelledby="next-title">
              <div className="cm-ring" role="img" aria-label={t("{n} de {total} superados", { n: doneN, total: pathLabs.length })}>
                <svg width="132" height="132" viewBox="0 0 132 132" aria-hidden="true">
                  <circle cx="66" cy="66" r="56" fill="none" stroke="#2A2D35" strokeWidth="12" />
                  <circle cx="66" cy="66" r="56" fill="none" stroke="#2B59E0" strokeWidth="12" strokeLinecap="round" strokeDasharray={`${Math.max(0.001, (doneN / Math.max(1, pathLabs.length)) * 352)} 352`} transform="rotate(-90 66 66)" />
                </svg>
                <div aria-hidden="true">
                  <strong>{doneN}/{pathLabs.length}</strong>
                  <span>{t("pasos")}</span>
                </div>
              </div>
              <div style={{ display: "flex", flexDirection: "column", gap: 12, flex: 1, minWidth: 0 }}>
                <div className="row" style={{ gap: 8 }}>
                  <span className="cm-dark-chip accent">{cur.title}</span>
                  {next && <span className="cm-dark-chip">{t(typeName(next.type))} · {t("{n} min", { n: next.minutes })}</span>}
                </div>
                {next ? (
                  <>
                    <div className="cm-kick">{state[next.id] === "doing" ? t("Continúa donde lo dejaste") : doneN === 0 ? t("Empieza aquí") : t("Tu siguiente paso")}</div>
                    <h2 id="next-title">{next.title}</h2>
                    <p>{next.summary}</p>
                    <div className="row" style={{ gap: 12, marginTop: 8 }}>
                      <a className="cm-pillbtn primary" href={labHref(next.id, runningLab[next.id])}>
                        <svg width="18" height="18" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true"><path d="M8 5v14l11-7z" /></svg>
                        {runningLab[next.id] ? t("Continuar") : t("Empezar")}
                      </a>
                      {next.type !== "boss" && (
                        <a className="cm-pillbtn dark" href={labHref(next.id, runningLab[next.id]) + "&tutor=1"}>
                          <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="#F2B64C" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true"><path d="M2 9l10-5 10 5-10 5z" /><path d="M6 11v5c3 2 9 2 12 0v-5" /></svg>
                          {t("Con profesor")}
                        </a>
                      )}
                    </div>
                  </>
                ) : (
                  <>
                    <h2 id="next-title">{t("¡Ruta completada!")}</h2>
                    <p>{t("Has superado todos los laboratorios de esta ruta. Elige otra abajo.")}</p>
                  </>
                )}
              </div>
            </section>
            <aside className="cm-amber cm-tutor" aria-labelledby="tutor-title">
              <div className="row" style={{ gap: 12 }}>
                <span className="cm-tutor-dot" aria-hidden="true">
                  <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round"><path d="M2 9l10-5 10 5-10 5z" /><path d="M6 11v5c3 2 9 2 12 0v-5" /></svg>
                </span>
                <h3 id="tutor-title" style={{ margin: 0 }}>{t("Tu profesor")}</h3>
              </div>
              <p>{t("¿Algo nuevo para ti? Hazlo conmigo: te enseño dónde está cada cosa en la consola y por qué se hace así.")}</p>
              <a className="cm-pillbtn ink sm" href="/incidents" style={{ marginTop: "auto" }}>{t("Incidencia asistida")}</a>
            </aside>
          </div>
        )}

        {cur && (
          <section className="cm-section" aria-labelledby="route-title">
            <div className="cm-section-head">
              <h2 id="route-title">{t("Tu ruta · {name}", { name: cur.title })}</h2>
              <div className="row" style={{ gap: 12 }}>
                <label htmlFor="track" className="sr-only">{t("Cambiar de ruta")}</label>
                <select id="track" value={cur.id} onChange={(e) => setTrack(e.target.value)}>
                  {tracks.map((x) => <option key={x.id} value={x.id}>{x.title}</option>)}
                </select>
                <a className="cm-link" href={`/catalog?track=${encodeURIComponent(cur.id)}`} style={{ fontWeight: 500 }}>{t("Ver las {n} etapas", { n: pathLabs.length })}</a>
              </div>
            </div>
            <RouteLine labs={pathLabs} state={state} running={runningLab} />
          </section>
        )}

        <section className="cm-tiles" aria-label={t("Explora otras rutas")}>
          {tracks
            .map((x, i) => ({ x, i, done: x.labs.filter((id) => state[id] === "done").length }))
            .filter((r) => r.x.id !== cur?.id)
            .sort((a, b) => (b.done > 0 ? 1 : 0) - (a.done > 0 ? 1 : 0) || a.i - b.i)
            .slice(0, 4)
            .map(({ x, i, done }) => (
              <a key={x.id} className="cm-tile" href={`/catalog?track=${encodeURIComponent(x.id)}`}>
                <TrackBadge id={x.id} index={i} />
                <span className="cm-tile-title">{x.title}</span>
                <span className="cm-prog">
                  <div><span style={{ width: `${(done / Math.max(1, x.labs.length)) * 100}%` }} /></div>
                  {done}/{x.labs.length}
                </span>
              </a>
            ))}
        </section>

        <div className="grid two" style={{ marginTop: 24, alignItems: "start" }}>
          <div className="card">
            <h3>{t("Recomendado para ti")}</h3>
            <RecList items={[...(p.adaptive ?? []), ...(p.recommendations ?? [])].filter((r) => r.labId !== next?.id).slice(0, 3)} />
          </div>
          <div className="card">
            <h3>{t("Cómo funciona un laboratorio")}</h3>
            <StepGuide stack />
          </div>
        </div>

        <details className="lt-more lt-section">
          <summary>{t("Estadísticas detalladas: nivel, competencias, certificaciones y más")}</summary>
        <div className="row" style={{ justifyContent: "flex-end", marginBottom: 12 }}>
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
                    <td><Bar value={d.score} label={label(dimension, d.dimension, t)} /></td>
                    <td className="muted small" style={{ width: 110 }}>{t("{pct}% · {n} evid.", { pct: Math.round(d.score), n: d.evidence })}</td>
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
              <>
              <p className="small muted" style={{ marginTop: 0 }}>{t("Cinco competencias por habilidad: entenderla (preguntas y explicaciones), ejecutarla, hacerlo sin ayuda, recordarla con el tiempo y aplicarla en un caso nuevo. «—» es que aún no hay pruebas, no que esté mal.")}</p>
              <div className="table-scroll">
              <table className="comp-table">
                <thead><tr><th>{t("Habilidad")}</th><th>{t("Dominio")}</th>{COMPS.map(([, name]) => <th key={name} className="num">{t(name)}</th>)}</tr></thead>
                <tbody>
                  {[...p.skills].sort((a, b) => b.mastery - a.mastery).map((s) => (
                    <tr key={s.skill}>
                      <td>{s.skill} {s.masteryBadge && <span className="pill ok">{t("dominada")}</span>}</td>
                      <td style={{ minWidth: 110 }}><Bar value={s.mastery} label={s.skill} /> <span className="small muted">{Math.round(s.mastery)}</span></td>
                      {COMPS.map(([key, name, field]) => (
                        <td key={key} className="num small">{!s.measured || s.measured.includes(key) ? Math.round(Number(s[field] ?? 0)) : <span className="muted" title={t("sin datos aún")}>—</span>}</td>
                      ))}
                    </tr>
                  ))}
                </tbody>
              </table>
              </div>
              </>
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
        </details>
      </main>
    </>
  );
}
