"use client";

// Exam is a practice exam that behaves like the real one: a fixed number of
// questions spread by the official domain weights, a countdown, no feedback
// until you hand it in, marking questions for review and jumping between
// them. The run survives a page reload; results are broken down by domain.

import { useEffect, useMemo, useState, type ReactNode } from "react";
import { useI18n } from "@/lib/i18n";
import { addHistory, buildExam, fmtTime, isRight, loadRun, saveRun, type ExamInfo, type ExamQuestion, type Topic } from "@/lib/study";

// Choices renders the options of a question as radio buttons, or as
// checkboxes limited to N picks for a "choose N" question.
export function Choices({ name, options, sel, onChange, multi, disabled, mark }: { name: string; options: string[]; sel: number[]; onChange: (s: number[]) => void; multi?: boolean; disabled?: boolean; mark?: (i: number) => string }) {
  return (
    <>
      {options.map((o, i) => (
        <label key={i} className={"ln-opt" + (mark ? mark(i) : sel.includes(i) ? " on" : "")}>
          <input
            type={multi ? "checkbox" : "radio"}
            name={name}
            checked={sel.includes(i)}
            disabled={disabled}
            onChange={() => onChange(multi ? (sel.includes(i) ? sel.filter((x) => x !== i) : [...sel, i]) : [i])}
          />
          <span>{o}</span>
        </label>
      ))}
    </>
  );
}

interface Run {
  qs: ExamQuestion[];
  ans: number[][];
  flag: boolean[];
  cur: number;
  started: number;
  done?: number;
}

const key = (cert: string, full: boolean) => `cm-exam-run-${cert}-${full ? "full" : "quick"}`;

export default function Exam({ ex, certName, byId, full, onExit, topicLink }: { ex: ExamInfo; certName: string; byId: Record<string, Topic>; full: boolean; onExit: () => void; topicLink: (id: string) => ReactNode }) {
  const { t } = useI18n();
  const n = full ? 50 : 20;
  const minutes = full ? ex.minutes : Math.round((ex.minutes * n) / 50);
  const k = key(ex.cert, full);
  const [run, setRun] = useState<Run | null>(null);
  const [saved, setSaved] = useState<Run | null>(null);
  const [now, setNow] = useState(Date.now());
  const [confirm, setConfirm] = useState(false);
  const [filter, setFilter] = useState<"all" | "bad" | "flag">("bad");
  const [announce, setAnnounce] = useState("");

  useEffect(() => {
    const r = loadRun<Run>(k);
    if (r && !r.done) setSaved(r);
  }, [k]);
  useEffect(() => {
    if (!run || run.done) return;
    const id = setInterval(() => setNow(Date.now()), 1000);
    return () => clearInterval(id);
  }, [run]);

  const left = run ? Math.max(0, Math.round((run.started + minutes * 60000 - now) / 1000)) : minutes * 60;
  const update = (f: (r: Run) => Run) =>
    setRun((r) => {
      if (!r) return r;
      const next = f(r);
      saveRun(k, next.done ? null : next);
      return next;
    });

  const finish = () => {
    setConfirm(false);
    update((r) => {
      const done = Date.now();
      const doms = ex.domains.map((d, i) => ({ name: d.name, ok: r.qs.filter((q, j) => q.domain === i && isRight(q, r.ans[j])).length, total: r.qs.filter((q) => q.domain === i).length }));
      addHistory({ cert: ex.cert, at: done, score: r.qs.filter((q, j) => isRight(q, r.ans[j])).length, total: r.qs.length, full, seconds: Math.round((done - r.started) / 1000), domains: doms });
      return { ...r, done };
    });
    window.scrollTo(0, 0);
  };
  useEffect(() => {
    if (!run || run.done) return;
    if (left === 0) finish();
    else if (left === 600 || left === 300 || left === 60) setAnnounce(t("Quedan {n} minutos", { n: left / 60 }));
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [left]);

  const start = () => {
    const qs = buildExam(ex, byId, n);
    const r: Run = { qs, ans: qs.map(() => []), flag: qs.map(() => false), cur: 0, started: Date.now() };
    saveRun(k, r);
    setSaved(null);
    setRun(r);
    setNow(Date.now());
  };

  const result = useMemo(() => {
    if (!run?.done) return null;
    const ok = run.qs.map((q, j) => isRight(q, run.ans[j]));
    return { ok, score: ok.filter(Boolean).length };
  }, [run]);

  if (!run)
    return (
      <div className="ex-intro">
        <h2>{full ? t("Simulacro completo") : t("Simulacro rápido")}</h2>
        <ul className="ex-rules">
          <li><strong>{t("{n} preguntas", { n })}</strong> {t("repartidas según el peso oficial de cada sección.")}</li>
          <li><strong>{t("{n} minutos", { n: minutes })}</strong> {t("de tiempo. Al llegar a cero se entrega solo.")}</li>
          <li>{t("No verás si aciertas hasta entregar, igual que en el examen real.")}</li>
          <li>{t("Algunas preguntas piden elegir varias respuestas: lo indica el enunciado.")}</li>
          <li>{t("Puedes marcar preguntas para revisarlas y moverte libremente entre ellas.")}</li>
          <li>{t("Si cierras la página, el simulacro sigue contando y puedes continuarlo.")}</li>
        </ul>
        <div className="row">
          {saved ? (
            <>
              <button type="button" className="cm-pillbtn primary sm" onClick={() => { setRun(saved); setSaved(null); setNow(Date.now()); }}>{t("Continuar el simulacro empezado")}</button>
              <button type="button" className="btn secondary" onClick={start}>{t("Empezar uno nuevo")}</button>
            </>
          ) : (
            <button type="button" className="cm-pillbtn primary sm" onClick={start}>{t("Empezar el simulacro")}</button>
          )}
          <button type="button" className="btn secondary" onClick={onExit}>{t("Volver")}</button>
        </div>
      </div>
    );

  if (result) {
    const pct = Math.round((result.score / run.qs.length) * 100);
    const pass = pct >= 70;
    const used = Math.round(((run.done ?? 0) - run.started) / 1000);
    const doms = ex.domains.map((d, i) => {
      const idx = run.qs.map((q, j) => [q, j] as const).filter(([q]) => q.domain === i);
      const ok = idx.filter(([, j]) => result.ok[j]).length;
      return { d, ok, total: idx.length, pct: idx.length ? Math.round((ok / idx.length) * 100) : 0 };
    });
    const weak = doms.filter((x) => x.total && x.pct < 70);
    const shown = run.qs.map((q, j) => [q, j] as const).filter(([, j]) => filter === "all" || (filter === "bad" ? !result.ok[j] : run.flag[j]));
    return (
      <div className="res">
        <section className="cm-dark res-hero">
          <div className="cm-ring" role="img" aria-label={t("{n} de {total} correctas", { n: result.score, total: run.qs.length })}>
            <svg width="132" height="132" viewBox="0 0 132 132" aria-hidden="true">
              <circle cx="66" cy="66" r="56" fill="none" stroke="#2A2D35" strokeWidth="12" />
              <circle cx="66" cy="66" r="56" fill="none" stroke={pass ? "#3DBA7E" : "#F2B64C"} strokeWidth="12" strokeLinecap="round" strokeDasharray={`${Math.max(0.001, (pct / 100) * 352)} 352`} transform="rotate(-90 66 66)" />
            </svg>
            <div aria-hidden="true">
              <strong>{pct}%</strong>
              <span>{t("{n} de {total}", { n: result.score, total: run.qs.length })}</span>
            </div>
          </div>
          <div style={{ display: "flex", flexDirection: "column", gap: 10, flex: 1, minWidth: 0 }}>
            <div className="row" style={{ gap: 8 }}>
              <span className="cm-dark-chip">{certName}</span>
              <span className="cm-dark-chip">{t("Tiempo usado: {t}", { t: fmtTime(used) })}</span>
            </div>
            <h1 style={{ margin: 0, color: "#FFFFFF" }}>{pass ? t("Por encima de la referencia") : t("Por debajo de la referencia")}</h1>
            <p style={{ margin: 0 }}>{t("Google no publica la nota de corte. Usamos un 70 % como referencia orientativa: repite simulacros hasta superarla con margen en todas las secciones.")}</p>
          </div>
        </section>
        <section className="res-card">
          <h2>{t("Resultado por sección")}</h2>
          <ul className="ex-doms">
            {doms.map((x, i) => (
              <li key={i}>
                <div className="row" style={{ justifyContent: "space-between" }}>
                  <strong>{x.d.name}</strong>
                  <span className="small muted">{t("{a} de {b} · {w} % del examen", { a: x.ok, b: x.total, w: x.d.weight })}</span>
                </div>
                <span className="cm-prog">
                  <div><span style={{ width: `${x.pct}%`, background: x.pct >= 70 ? "var(--cm-ok)" : "var(--cm-amber)" }} /></div>
                  {x.pct}%
                </span>
              </li>
            ))}
          </ul>
          {weak.length > 0 && (
            <div className="ln-tip" style={{ marginTop: 14 }}>
              <strong>{t("Qué repasar")}</strong>
              <span className="row" style={{ gap: 6 }}>
                {Array.from(new Set(weak.flatMap((x) => x.d.topics))).map((id) => <span key={id}>{topicLink(id)}</span>)}
              </span>
            </div>
          )}
        </section>
        <section className="res-card">
          <div className="row" style={{ justifyContent: "space-between", marginBottom: 12 }}>
            <h2 style={{ margin: 0 }}>{t("Revisión de preguntas")}</h2>
            <div className="cm-levels" role="group" aria-label={t("Filtrar")}>
              <button type="button" aria-pressed={filter === "bad"} onClick={() => setFilter("bad")}>{t("Falladas ({n})", { n: result.ok.filter((x) => !x).length })}</button>
              <button type="button" aria-pressed={filter === "flag"} onClick={() => setFilter("flag")}>{t("Marcadas ({n})", { n: run.flag.filter(Boolean).length })}</button>
              <button type="button" aria-pressed={filter === "all"} onClick={() => setFilter("all")}>{t("Todas")}</button>
            </div>
          </div>
          {shown.length === 0 ? (
            <p className="muted">{t("Nada que mostrar aquí.")}</p>
          ) : (
            <ol className="ex-review">
              {shown.map(([q, j]) => (
                <li key={j} className={result.ok[j] ? "ok" : "bad"}>
                  <span className="small muted">{t("Pregunta {n}", { n: j + 1 })} · {ex.domains[q.domain]?.name}</span>
                  <strong>{q.q}</strong>
                  <span>{t("Tu respuesta")}: {run.ans[j].length ? run.ans[j].map((a) => q.options[a]).join(" · ") : t("sin responder")}</span>
                  {!result.ok[j] && <span>✓ {q.answers.map((a) => q.options[a]).join(" · ")}</span>}
                  <span className="muted">{q.why}</span>
                </li>
              ))}
            </ol>
          )}
        </section>
        <div className="row">
          <button type="button" className="cm-pillbtn primary sm" onClick={() => { setRun(null); }}>{t("Hacer otro simulacro")}</button>
          <button type="button" className="btn secondary" onClick={onExit}>{t("Volver a la certificación")}</button>
        </div>
      </div>
    );
  }

  const q = run.qs[run.cur];
  const sel = run.ans[run.cur];
  const multi = q.answers.length > 1;
  const answered = run.ans.filter((a) => a.length).length;
  return (
    <div className="ex">
      <div className="ex-bar">
        <span className="ex-title">{certName}</span>
        <span className={"ex-timer" + (left < 300 ? " low" : "")} role="timer" aria-label={t("Tiempo restante")}>⏱ {fmtTime(left)}</span>
        <span className="small">{t("{a} de {b} respondidas", { a: answered, b: run.qs.length })}</span>
        <button type="button" className="cm-pillbtn primary sm" onClick={() => setConfirm(true)}>{t("Terminar y entregar")}</button>
      </div>
      <p className="sr-only" aria-live="assertive">{announce}</p>
      {confirm && (
        <div className="ex-confirm" role="alertdialog" aria-labelledby="ex-c-t" aria-describedby="ex-c-d">
          <h2 id="ex-c-t">{t("¿Entregar el simulacro?")}</h2>
          <p id="ex-c-d">
            {t("Sin responder: {a}. Marcadas para revisar: {b}. Después no podrás cambiar las respuestas.", { a: run.qs.length - answered, b: run.flag.filter(Boolean).length })}
          </p>
          <div className="row">
            <button type="button" className="cm-pillbtn primary sm" onClick={finish} autoFocus>{t("Entregar")}</button>
            <button type="button" className="btn secondary" onClick={() => setConfirm(false)}>{t("Seguir revisando")}</button>
          </div>
        </div>
      )}
      <div className="ex-body">
        <div className="ex-main">
          <div className="small muted">{t("Pregunta {n} de {total}", { n: run.cur + 1, total: run.qs.length })}</div>
          <fieldset className="ln-q">
            <legend>{q.q}</legend>
            {multi && <p className="ex-multi">{t("Elige {n} respuestas.", { n: q.answers.length })}</p>}
            <Choices
              name={"ex" + run.cur}
              multi={multi}
              options={q.options}
              sel={sel}
              onChange={(s) => update((r) => ({ ...r, ans: r.ans.map((a, j) => (j === r.cur ? (multi ? s.slice(-q.answers.length) : s) : a)) }))}
            />
          </fieldset>
          <div className="row">
            <button type="button" className="btn secondary" disabled={run.cur === 0} onClick={() => update((r) => ({ ...r, cur: r.cur - 1 }))}>← {t("Anterior")}</button>
            <button type="button" className="btn secondary" aria-pressed={run.flag[run.cur]} onClick={() => update((r) => ({ ...r, flag: r.flag.map((f, j) => (j === r.cur ? !f : f)) }))}>
              {run.flag[run.cur] ? "★ " + t("Marcada para revisar") : "☆ " + t("Marcar para revisar")}
            </button>
            {run.cur < run.qs.length - 1 ? (
              <button type="button" className="cm-pillbtn primary sm" onClick={() => update((r) => ({ ...r, cur: r.cur + 1 }))}>{t("Siguiente")} →</button>
            ) : (
              <button type="button" className="cm-pillbtn primary sm" onClick={() => setConfirm(true)}>{t("Revisar y entregar")}</button>
            )}
          </div>
        </div>
        <nav className="ex-grid" aria-label={t("Ir a una pregunta")}>
          <div className="ex-legend small muted">
            <span><i className="ex-dot done" /> {t("Respondida")}</span>
            <span><i className="ex-dot flag" /> {t("Marcada")}</span>
          </div>
          <div className="ex-nums">
            {run.qs.map((_, j) => (
              <button
                key={j}
                type="button"
                className={"ex-num" + (run.ans[j].length ? " done" : "") + (run.flag[j] ? " flag" : "") + (j === run.cur ? " cur" : "")}
                aria-current={j === run.cur ? "step" : undefined}
                aria-label={t("Pregunta {n}", { n: j + 1 }) + (run.ans[j].length ? ", " + t("respondida") : "") + (run.flag[j] ? ", " + t("marcada") : "")}
                onClick={() => update((r) => ({ ...r, cur: j }))}
              >
                {j + 1}
              </button>
            ))}
          </div>
        </nav>
      </div>
    </div>
  );
}
