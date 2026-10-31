"use client";

// The 10-day plan (about 4 hours a day) to start working in GCP platform
// support: each day mixes ACE labs, bank support cases, study topics, shifts
// and the practice exam. Labs, topics, shifts and exams tick themselves from
// the learner's progress; the few manual tasks are ticked by hand and saved
// in the account with the rest of the study progress.

import { useEffect, useMemo, useState } from "react";
import Nav from "@/components/Nav";
import { useAuth } from "@/components/useAuth";
import { labHref, useProgress } from "@/components/learn";
import { api } from "@/lib/api";
import { useI18n } from "@/lib/i18n";
import { itemDone, key, planLocal, planStatus, type Item } from "@/lib/plan";
import { saveRun, syncStudy, type ExamRecord, type Progress, type Topic } from "@/lib/study";
import type { LabSummary } from "@/lib/types";

export default function PlanPage() {
  useAuth();
  const { t } = useI18n();
  const { state, running } = useProgress();
  const [labs, setLabs] = useState<Record<string, LabSummary>>({});
  const [topics, setTopics] = useState<Record<string, Topic>>({});
  const [prog, setProg] = useState<Progress>({});
  const [hist, setHist] = useState<ExamRecord[]>([]);
  const [shifts, setShifts] = useState(0);
  const [checked, setChecked] = useState<Record<string, boolean>>({});
  const [open, setOpen] = useState<number | null>(null);
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
        setReady(true);
      });
  }, []);

  const done = (it: Item) => itemDone(it, { labs, state, prog, hist, shifts, checked });
  const toggle = (task: string) => {
    const c = { ...checked, [task]: !checked[task] };
    setChecked(c);
    saveRun("cm-plan", { checked: c });
  };

  const { days, counts, total, done: allDone, today } = useMemo(() => planStatus({ labs, state, prog, hist, shifts, checked }), [labs, state, prog, hist, shifts, checked]);
  const shown = open ?? today;

  const label = (it: Item) => {
    if ("lab" in it) {
      const kind = it.lab.startsWith("zentra-t") ? t("Taller desde cero") : it.lab.startsWith("zentra-s") ? t("Taller del oficio") : it.lab.startsWith("zentra-") ? (labs[it.lab]?.type === "guided" ? t("Tutorial Zentra") : t("Caso Zentra")) : it.lab.startsWith("bank-") ? t("Caso Banco Atlas") : t("Laboratorio ACE");
      return { text: labs[it.lab]?.title ?? it.lab, kind, href: labHref(it.lab, running[it.lab]) };
    }
    if ("topic" in it) return { text: topics[it.topic]?.name ?? it.topic, kind: t("Tema · test ≥ 70"), href: `/learn?topic=${it.topic}` };
    if ("exam" in it) return { text: t("Simulacro completo del {cert}", { cert: it.exam }), kind: t("Examen"), href: `/learn?cert=${it.exam}&exam=full` };
    if ("shift" in it) return { text: t("Turno de soporte n.º {n}", { n: it.shift }), kind: t("Turno"), href: "/shift" };
    return { text: t(it.text), kind: t("Tarea"), href: it.href ?? "" };
  };

  return (
    <>
      <Nav active="/plan" />
      <main id="main" className="page">
        <header className="cm-head">
          <div>
            <h1>{t("Plan de 10 días")}</h1>
            <p className="muted" style={{ margin: "6px 0 0", maxWidth: 700 }}>
              {t("Unas 4 horas al día para llegar con soltura a un puesto de soporte GCP: cada día mezcla laboratorios del ACE, casos reales de soporte en banca y un tema de estudio. Las casillas se marcan solas al superar cada cosa.")}
            </p>
          </div>
          <div className="pl-total" aria-label={t("Progreso del plan")}>
            <strong>{total ? Math.round((100 * allDone) / total) : 0} %</strong>
            <span className="muted">{t("{done} de {total} hechos", { done: allDone, total })}</span>
          </div>
        </header>
        {!ready ? (
          <p className="muted">{t("Cargando…")}</p>
        ) : (
          <div className="pl-grid">
            <ol className="pl-days" aria-label={t("Días")}>
              {days.map((d, i) => (
                <li key={i}>
                  <button type="button" className={`pl-day${i === shown ? " active" : ""}${counts[i] === d.items.length ? " done" : ""}`} aria-current={i === shown ? "step" : undefined} onClick={() => setOpen(i)}>
                    <span className="pl-num">{counts[i] === d.items.length ? "✓" : i + 1}</span>
                    <span className="pl-dtitle">{t(d.title)}</span>
                    <span className="pl-meter" aria-hidden="true"><span style={{ width: `${(100 * counts[i]) / Math.max(1, d.items.length)}%` }} /></span>
                  </button>
                </li>
              ))}
            </ol>
            <section className="card pl-detail">
              <h2>{t(days[shown].title)}</h2>
              <p className="muted">{t(days[shown].goal)} · {t("≈ 4 h")}{shown === today ? " · " + t("toca hoy") : ""}</p>
              <ul className="pl-items">
                {days[shown].items.map((it) => {
                  const lb = label(it);
                  const ok = done(it);
                  return (
                    <li key={key(it)} className={ok ? "done" : ""}>
                      {"task" in it ? (
                        <span className="pl-check">
                          <input type="checkbox" id={"pl-" + it.task} checked={ok} onChange={() => toggle(it.task)} />
                          <label htmlFor={"pl-" + it.task}>{lb.text}</label>
                          {lb.href && <a href={lb.href} className="pl-open">{t("Abrir")}</a>}
                        </span>
                      ) : (
                        <a href={lb.href} className="pl-check">
                          <span className="pl-box" aria-hidden="true">{ok ? "✓" : ""}</span>
                          <span>{lb.text}</span>
                          <span className="sr-only">{ok ? t("hecho") : t("pendiente")}</span>
                        </a>
                      )}
                      <span className="pl-kind">{lb.kind}</span>
                    </li>
                  );
                })}
              </ul>
            </section>
          </div>
        )}
      </main>
    </>
  );
}
