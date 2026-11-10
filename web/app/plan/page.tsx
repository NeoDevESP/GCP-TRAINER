"use client";

// The study plan. The curriculum is a list of modules in order (lib/plan.ts); how
// many days it takes depends on the time the learner chooses per day: pending
// activities are packed into sessions with their real duration (lab minutes,
// concepts and questions of each topic, the exam, shifts). Each activity is
// essential, recommended or optional. Labs, topics, shifts and exams tick
// themselves from the learner's progress; manual tasks are ticked by hand.
// Preferences and ticks are saved in the account (study run "cm-plan").

import { useEffect, useMemo, useState } from "react";
import Nav from "@/components/Nav";
import { useAuth } from "@/components/useAuth";
import { labHref, useProgress } from "@/components/learn";
import { api } from "@/lib/api";
import { k, useI18n } from "@/lib/i18n";
import { DAYS, DEFAULT_PREFS, planLocal, planSchedule, type Item, type PlanItem, type PlanPrefs } from "@/lib/plan";
import { saveRun, syncStudy, type ExamRecord, type Progress, type Topic } from "@/lib/study";
import type { Priority } from "@/lib/model";
import type { LabSummary } from "@/lib/types";

const PRIO: Record<Priority, string> = { essential: k("esencial"), recommended: k("recomendada"), optional: k("opcional") };
const BUDGETS = [30, 60, 90, 120, 180, 240];
const fmtMin = (m: number) => (m >= 60 ? `${Math.floor(m / 60)} h${m % 60 ? ` ${m % 60} min` : ""}` : `${m} min`);
// module titles were written as "Día N · …": the number is now the module
const moduleTitle = (s: string) => s.replace(/^[^·]*·\s*/, "");

export default function PlanPage() {
  useAuth();
  const { t, lang } = useI18n();
  const { state, running } = useProgress();
  const [labs, setLabs] = useState<Record<string, LabSummary>>({});
  const [topics, setTopics] = useState<Record<string, Topic>>({});
  const [prog, setProg] = useState<Progress>({});
  const [hist, setHist] = useState<ExamRecord[]>([]);
  const [shifts, setShifts] = useState(0);
  const [checked, setChecked] = useState<Record<string, boolean>>({});
  const [prefs, setPrefs] = useState<PlanPrefs>(DEFAULT_PREFS);
  const [open, setOpen] = useState(0);
  const [ready, setReady] = useState(false);

  useEffect(() => {
    Promise.all([
      api<{ labs: LabSummary[] }>("/api/catalog").then((r) => setLabs(Object.fromEntries(r.labs.map((l) => [l.id, l])))),
      api<{ topics: Topic[] }>("/api/learn").then((r) => setTopics(Object.fromEntries(r.topics.map((x) => [x.id, x])))),
      syncStudy().then((s) => {
        setProg(s.progress);
        setHist(s.history);
      }),
    ])
      .catch(() => {})
      .finally(() => {
        const l = planLocal();
        setChecked(l.checked);
        setShifts(l.shifts);
        setPrefs(l.prefs);
        setReady(true);
      });
  }, []);

  const persist = (c: Record<string, boolean>, p: PlanPrefs) => saveRun("cm-plan", { checked: c, prefs: p });
  const toggle = (task: string) => {
    const c = { ...checked, [task]: !checked[task] };
    setChecked(c);
    persist(c, prefs);
  };
  const setPref = (p: Partial<PlanPrefs>) => {
    const next = { ...prefs, ...p };
    setPrefs(next);
    setOpen(0);
    persist(checked, next);
  };

  const plan = useMemo(() => planSchedule({ labs, state, prog, hist, shifts, checked }, topics, prefs), [labs, state, prog, hist, shifts, checked, topics, prefs]);
  const shown = Math.min(open, Math.max(0, plan.sessions.length - 1));
  const finish = new Date(Date.now() + Math.max(0, plan.sessions.length - 1) * 86400000);

  const label = (it: Item) => {
    if ("lab" in it) {
      const kind = it.lab.startsWith("zentra-t") ? t("Taller desde cero") : it.lab.startsWith("zentra-s") ? t("Taller del oficio") : it.lab.startsWith("zentra-") ? (labs[it.lab]?.type === "guided" ? t("Tutorial Zentra") : t("Caso Zentra")) : it.lab.startsWith("bank-") ? t("Caso Banco Atlas") : t("Laboratorio ACE");
      return { text: labs[it.lab]?.title ?? it.lab, kind, href: labHref(it.lab, running[it.lab]) };
    }
    if ("topic" in it) return { text: topics[it.topic]?.name ?? it.topic, kind: t("Tema · test ≥ 70"), href: `/learn?topic=${it.topic}` };
    if ("exam" in it) return { text: t("Simulacro completo del {cert}", { cert: it.exam }), kind: t("Examen · aprobar ≥ 70 %"), href: `/learn?cert=${it.exam}&exam=full` };
    if ("shift" in it) return { text: t("Turno de soporte n.º {n}", { n: it.shift }), kind: t("Turno"), href: "/shift" };
    return { text: t(it.text), kind: t("Tarea"), href: it.href ?? "" };
  };

  const row = (e: PlanItem) => {
    const it = e.item;
    const lb = label(it);
    return (
      <li key={e.key} className={e.done ? "done" : ""}>
        {"task" in it ? (
          <span className="pl-check">
            <input type="checkbox" id={"pl-" + it.task} checked={e.done} onChange={() => toggle(it.task)} />
            <label htmlFor={"pl-" + it.task}>{lb.text}</label>
            {lb.href && <a href={lb.href} className="pl-open">{t("Abrir")}</a>}
          </span>
        ) : (
          <a href={lb.href} className="pl-check">
            <span className="pl-box" aria-hidden="true">{e.done ? "✓" : ""}</span>
            <span>{lb.text}</span>
            <span className="sr-only">{e.done ? t("hecho") : t("pendiente")}</span>
          </a>
        )}
        <span className="pl-kind">
          {lb.kind} · {fmtMin(e.minutes)} · <span className={`pl-prio ${e.priority}`}>{t(PRIO[e.priority])}</span>
        </span>
      </li>
    );
  };

  const session = plan.sessions[shown];

  return (
    <>
      <Nav active="/plan" />
      <main id="main" className="page">
        <header className="cm-head">
          <div>
            <h1>{t("Tu plan de estudio")}</h1>
            <p className="muted" style={{ margin: "6px 0 0", maxWidth: 700 }}>
              {t("Módulos en orden, de cero al puesto de soporte. Tú eliges cuánto tiempo tienes al día y el plan reparte lo pendiente con la duración real de cada actividad. Las casillas se marcan solas al superar cada cosa.")}
            </p>
          </div>
          <div className="pl-total" aria-label={t("Progreso del plan")}>
            <strong>{plan.total ? Math.round((100 * plan.done) / plan.total) : 0} %</strong>
            <span className="muted">{t("{done} de {total} hechos", { done: plan.done, total: plan.total })}</span>
          </div>
        </header>
        {!ready ? (
          <p className="muted">{t("Cargando…")}</p>
        ) : (
          <>
            <section className="card pl-prefs" aria-labelledby="pl-prefs-title">
              <h2 id="pl-prefs-title" className="sr-only">{t("Preferencias del plan")}</h2>
              <div className="pl-pref">
                <label htmlFor="pl-budget">{t("Tiempo al día")}</label>
                <select id="pl-budget" value={prefs.minutesPerDay} onChange={(e) => setPref({ minutesPerDay: Number(e.target.value) })}>
                  {BUDGETS.map((m) => <option key={m} value={m}>{fmtMin(m)}</option>)}
                </select>
              </div>
              <label className="pl-toggle"><input type="checkbox" checked={prefs.recommended} onChange={(e) => setPref({ recommended: e.target.checked })} /> {t("Incluir actividades recomendadas")}</label>
              <label className="pl-toggle"><input type="checkbox" checked={prefs.optional} onChange={(e) => setPref({ optional: e.target.checked })} /> {t("Incluir opcionales")}</label>
              <label className="pl-toggle"><input type="checkbox" checked={prefs.skipBasics} onChange={(e) => setPref({ skipBasics: e.target.checked })} /> {t("Ya sé lo básico (la Semana 0 pasa a opcional)")}</label>
              <p className="pl-summary" role="status">
                {plan.sessions.length
                  ? t("Te quedan {h} de estudio: {n} sesiones de hasta {m}. A este ritmo terminas hacia el {date}.", { h: fmtMin(plan.pendingMin), n: plan.sessions.length, m: fmtMin(prefs.minutesPerDay), date: finish.toLocaleDateString(lang === "en" ? "en-GB" : "es-ES", { day: "numeric", month: "long", year: "numeric" }) })
                  : t("¡No te queda nada pendiente en el plan!")}
              </p>
            </section>
            {plan.sessions.length > 0 && (
              <div className="pl-grid">
                <ol className="pl-days" aria-label={t("Sesiones")}>
                  {plan.sessions.slice(0, 30).map((d, i) => (
                    <li key={i}>
                      <button type="button" className={`pl-day${i === shown ? " active" : ""}`} aria-current={i === shown ? "step" : undefined} onClick={() => setOpen(i)}>
                        <span className="pl-num">{i + 1}</span>
                        <span className="pl-dtitle">
                          {i === 0 ? t("Hoy") : t("Sesión {n}", { n: i + 1 })} · {fmtMin(d.minutes)}
                          <span className="pl-dsub">{moduleTitle(t(DAYS[d.entries[0].module].title))}</span>
                        </span>
                      </button>
                    </li>
                  ))}
                  {plan.sessions.length > 30 && <li className="muted small">{t("y {n} sesiones más", { n: plan.sessions.length - 30 })}</li>}
                </ol>
                {session && (
                  <section className="card pl-detail">
                    <h2>{shown === 0 ? t("La sesión de hoy") : t("Sesión {n}", { n: shown + 1 })}</h2>
                    <p className="muted">
                      {t("{n} actividades · {m}", { n: session.entries.length, m: fmtMin(session.minutes) })}
                      {session.long ? " · " + t("una actividad larga: hazla en dos ratos") : ""}
                    </p>
                    {Array.from(new Set(session.entries.map((e) => e.module))).map((m) => (
                      <div key={m}>
                        <h3 className="pl-module">{t("Módulo {n}", { n: m + 1 })} · {moduleTitle(t(DAYS[m].title))}</h3>
                        <p className="muted small" style={{ marginTop: 0 }}>{t(DAYS[m].goal)}</p>
                        <ul className="pl-items">{session.entries.filter((e) => e.module === m).map(row)}</ul>
                      </div>
                    ))}
                  </section>
                )}
              </div>
            )}
            <details className="card" style={{ marginTop: 16 }}>
              <summary><strong>{t("Todo el plan por módulos")}</strong></summary>
              {DAYS.map((d, m) => {
                const items = plan.entries.filter((e) => e.module === m);
                const min = items.reduce((a, e) => a + e.minutes, 0);
                return (
                  <div key={m}>
                    <h3 className="pl-module">{t("Módulo {n}", { n: m + 1 })} · {moduleTitle(t(d.title))} <span className="muted small">· {fmtMin(min)} · {items.filter((e) => e.done).length}/{items.length}</span></h3>
                    <ul className="pl-items">{items.map(row)}</ul>
                  </div>
                );
              })}
            </details>
          </>
        )}
      </main>
    </>
  );
}
