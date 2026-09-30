"use client";

// The tutor: a private teacher that walks the learner through the lab one
// step at a time — what to do, why, where it is in the console (with "show
// me where"), which fields to fill in and the equivalent command. It notices
// when a step is done (in the console or in Cloud Shell) and moves on.

import { useCallback, useEffect, useState } from "react";
import { api } from "@/lib/api";
import { k, useI18n } from "@/lib/i18n";
import { Icon } from "@/components/console/icons";
import Markdown from "@/components/Markdown";

export type TutorStep = {
  n: number;
  phase: string;
  title: string;
  why: string;
  concept?: string;
  note?: string;
  where: { page?: string; path: string; action?: string; target?: string };
  fields?: { label: string; value: string }[];
  command: string;
  match?: string;
  manual?: boolean;
};

const PHASES: [string, string, string][] = [
  ["investigate", k("Investiga"), "search"],
  ["fix", k("Arregla"), "edit"],
  ["verify", k("Comprueba"), "check"],
  ["communicate", k("Comunica"), "mail"],
];

const phaseName = (p: string, build: boolean) => {
  if (p === "setup") return k("Prepara");
  if (p === "fix" && build) return k("Construye");
  return PHASES.find((x) => x[0] === p)?.[1] ?? p;
};

export default function Tutor({
  sessionId,
  tick,
  auto,
  labType,
  run,
  paste,
  onPanel,
  onCheck,
}: {
  sessionId: string;
  tick: number;
  auto?: boolean;
  labType?: string;
  run: (cmd: string, note: string) => void;
  paste: (cmd: string) => void;
  onPanel: (id: "desk" | "guide") => void;
  onCheck: () => void;
}) {
  const { t } = useI18n();
  const [on, setOn] = useState<boolean | null>(null);
  const [plan, setPlan] = useState<{ intro?: string; steps: TutorStep[]; done: number[]; type?: string } | null>(null);
  const [manual, setManual] = useState<number[]>([]);
  const [cur, setCur] = useState(1);
  const [pinned, setPinned] = useState(false);
  const [err, setErr] = useState("");
  const store = `gcplab.tutor.${sessionId}`;

  const load = useCallback(async () => {
    try {
      const p = await api<any>(`/api/sessions/${sessionId}/views/tutor`);
      setPlan({ intro: p.intro, steps: p.steps ?? [], done: p.done ?? [], type: p.type });
      setOn(true);
    } catch {
      setOn(false);
    }
  }, [sessionId]);

  useEffect(() => {
    try {
      setManual(JSON.parse(localStorage.getItem(store) || "[]"));
    } catch {
      /* storage unavailable */
    }
  }, [store]);
  useEffect(() => {
    load();
  }, [load, tick]);

  const activate = useCallback(async () => {
    setErr("");
    try {
      await api(`/api/sessions/${sessionId}/tutor`, { body: {} });
      await load();
    } catch (e: any) {
      setErr(e.message);
    }
  }, [sessionId, load]);
  useEffect(() => {
    if (auto && on === false) activate();
  }, [auto, on, activate]);

  const steps = plan?.steps ?? [];
  const isDone = (n: number) => !!plan?.done.includes(n) || manual.includes(n);
  const firstOpen = steps.find((s) => !isDone(s.n))?.n ?? 0;
  // follow the learner unless they jumped to a step on purpose
  useEffect(() => {
    if (!pinned && firstOpen) setCur(firstOpen);
  }, [firstOpen, pinned]);

  useEffect(() => {
    document.getElementById("tu-title")?.closest(".tu-card")?.scrollIntoView({ block: "nearest", behavior: "smooth" });
  }, [cur]);

  const markDone = (n: number) => {
    const next = [...new Set([...manual, n])];
    setManual(next);
    setPinned(false);
    try {
      localStorage.setItem(store, JSON.stringify(next));
    } catch {
      /* storage unavailable */
    }
  };
  const show = (s: TutorStep) => {
    if (s.where.action === "panel") {
      onPanel("desk");
      return;
    }
    window.dispatchEvent(new CustomEvent("cc:show", { detail: s.where }));
  };

  if (on === null) return <p className="muted">{t("Cargando…")}</p>;

  if (!on) {
    return (
      <div className="tu-off">
        <div className="tu-hero">
          <span className="tu-avatar" aria-hidden="true"><Icon name="school" size={26} /></span>
          <div>
            <h3>{t("Tu profesor particular")}</h3>
            <p>{t("Te acompaño paso a paso hasta resolverlo, como en tu primer día con un compañero experto al lado.")}</p>
          </div>
        </div>
        <ul className="tu-list-benefits">
          <li>{t("Qué hacer en cada momento y por qué se hace así.")}</li>
          <li>{t("Dónde está en la consola, con un botón que te lleva y te lo señala.")}</li>
          <li>{t("Qué campos rellenar y el comando equivalente de Cloud Shell.")}</li>
          <li>{t("Detecto cuándo lo has hecho y pasamos al siguiente paso.")}</li>
        </ul>
        <p className="small muted" style={{ margin: 0 }}>{t("Es un modo de aprendizaje: el laboratorio cuenta como guiado (la mitad de XP y no suma autonomía). Cuando cojas soltura, repítelo sin profesor.")}</p>
        <button type="button" className="cc-btn primary" onClick={activate}>
          <Icon name="school" size={18} /> {t("Activar el profesor")}
        </button>
        {err && <p className="error small">{err}</p>}
      </div>
    );
  }

  const build = !["incident", "boss"].includes(plan?.type ?? labType ?? "");
  const step = steps.find((s) => s.n === cur);
  const doneN = steps.filter((s) => isDone(s.n)).length;
  const allDone = steps.length > 0 && doneN === steps.length;
  const phaseState = (p: string) => {
    const ps = steps.filter((s) => (p === "investigate" ? s.phase === "investigate" || s.phase === "setup" : s.phase === p));
    if (!ps.length) return "";
    if (ps.every((s) => isDone(s.n))) return "done";
    return step && (step.phase === p || (p === "investigate" && step.phase === "setup")) ? "on" : "";
  };

  return (
    <div>
      {plan?.intro && (
        <div className="tu-hero" style={{ marginBottom: 10 }}>
          <span className="tu-avatar" aria-hidden="true"><Icon name="school" size={22} /></span>
          <p style={{ color: "var(--text)" }}>{plan.intro}</p>
        </div>
      )}
      <div className="lt-check-head">
        <span>{t("{n} de {total} pasos", { n: doneN, total: steps.length })}</span>
        <span className="lt-bar" aria-hidden="true"><span style={{ width: `${steps.length ? (doneN / steps.length) * 100 : 0}%` }} /></span>
      </div>
      <div className="tu-phases" aria-label={t("Fases")}>
        {PHASES.filter(([p]) => steps.some((s) => s.phase === p || (p === "investigate" && s.phase === "setup"))).map(([p, name]) => (
          <span key={p} className={`tu-phase ${phaseState(p)}`}>{p === "fix" && build ? t("Construye") : t(name)}</span>
        ))}
      </div>

      {allDone && !pinned ? (
        <div className="tu-card tu-end">
          <span className="tu-avatar" aria-hidden="true"><Icon name="check" size={26} /></span>
          <h3>{t("¡Lo has hecho todo!")}</h3>
          <p className="muted">{t("Pulsa Comprobar para ver la nota y, cuando quieras, entrega el laboratorio. La próxima vez intenta hacerlo sin mí.")}</p>
          <button type="button" className="cc-btn primary" onClick={onCheck}><Icon name="check" size={18} /> {t("Comprobar")}</button>
        </div>
      ) : step ? (
        <section className={`tu-card${isDone(step.n) ? " done" : ""}`} aria-live="polite" aria-labelledby="tu-title">
          <div className="tu-kicker">
            <span>{t("Paso {n} de {total}", { n: step.n, total: steps.length })} · {t(phaseName(step.phase, build))}</span>
            {isDone(step.n) && <span style={{ color: "var(--ok)" }}>✓ {t("hecho")}</span>}
          </div>
          <h3 id="tu-title">{step.title}</h3>
          <p className="tu-why">{step.why}</p>
          {step.note && (
            <p className="tu-note"><Icon name="bulb" size={18} /> <span>{step.note}</span></p>
          )}
          {step.concept && <p className="tu-concept"><strong>{t("Concepto:")}</strong> {step.concept}</p>}

          <div className="tu-sec">{t("Dónde está")}</div>
          <div className="tu-where">
            <span>{step.where.path}{step.where.target && step.where.action !== "shell" ? ` › ${step.where.target}` : ""}</span>
            <button type="button" className="cc-btn" onClick={() => show(step)}>
              <Icon name="pointer" size={16} /> {t("Muéstrame dónde")}
            </button>
          </div>

          {step.fields?.length ? (
            <>
              <div className="tu-sec">{t("Qué rellenar")}</div>
              <table className="tu-fields">
                <tbody>
                  {step.fields.map((f, i) => (
                    <tr key={i}><td>{f.label}</td><td>{f.value}</td></tr>
                  ))}
                </tbody>
              </table>
            </>
          ) : null}

          <div className="tu-sec">{step.where.action === "shell" || !step.where.page ? t("En Cloud Shell") : t("O en Cloud Shell, el comando equivalente")}</div>
          <pre className="tu-cmd">{step.command}</pre>
          <div className="tu-actions">
            <button type="button" className="cc-btn" onClick={() => paste(step.command)}><Icon name="copy" size={16} /> {t("Pegar en Cloud Shell")}</button>
            {!step.command.includes("\n") && (
              <button type="button" className="cc-btn" onClick={() => run(step.command, step.title)}><Icon name="play" size={16} /> {t("Hazlo por mí")}</button>
            )}
          </div>

          {isDone(step.n) ? (
            <div className="tu-okmsg">
              <Icon name="check" size={18} /> {t("¡Bien hecho!")}
              {steps.some((s) => s.n > step.n) && (
                <button type="button" className="cc-btn" style={{ marginLeft: "auto" }} onClick={() => { setPinned(false); setCur(steps.find((s) => s.n > step.n && !isDone(s.n))?.n ?? step.n + 1); }}>
                  {t("Siguiente paso")} →
                </button>
              )}
            </div>
          ) : (
            <div className="tu-wait">
              {step.manual || !step.match ? (
                <button type="button" className="cc-btn primary" onClick={() => markDone(step.n)}><Icon name="check" size={16} /> {t("Ya lo he hecho")}</button>
              ) : (
                <>
                  <span className="dot" aria-hidden="true" /> {t("Te espero: en cuanto lo hagas, lo detecto solo.")}
                  <button type="button" className="cc-link" style={{ marginLeft: "auto" }} onClick={() => markDone(step.n)}>{t("Saltar este paso")}</button>
                </>
              )}
            </div>
          )}
        </section>
      ) : null}

      <details style={{ marginTop: 12 }}>
        <summary className="small">{t("Ver todos los pasos")}</summary>
        <ol className="tu-steps">
          {steps.map((s) => (
            <li key={s.n} className={isDone(s.n) ? "ok" : s.n === cur ? "cur" : ""}>
              <button type="button" onClick={() => { setCur(s.n); setPinned(true); }}>
                <span className="tu-n" aria-hidden="true">{isDone(s.n) ? "✓" : s.n}</span>
                <span>{s.title}<span className="sr-only"> ({isDone(s.n) ? t("hecho") : t("pendiente")})</span></span>
              </button>
            </li>
          ))}
        </ol>
      </details>
      {plan && !steps.length && <Markdown text={t("Este laboratorio no tiene pasos guiados.")} />}
    </div>
  );
}
