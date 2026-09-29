"use client";

// Visual learning aids around the Google Cloud console: the progress of each
// lab, route icons, the path of a route as a map of steps, the three-step
// "how a lab works" guide, a plain-words legend of lab types and the
// first-visit tour of the lab workspace. None of this changes the console.

import { useEffect, useRef, useState } from "react";
import { api } from "@/lib/api";
import { k, useI18n } from "@/lib/i18n";
import { Icon } from "@/components/console/icons";
import type { LabSummary } from "@/lib/types";

export type LabState = "done" | "doing" | "new";

/** useProgress returns the state of every lab the user has touched. */
export function useProgress() {
  const [state, setState] = useState<Record<string, LabState>>({});
  const [running, setRunning] = useState<Record<string, string>>({});
  useEffect(() => {
    api<any[]>("/api/me/attempts")
      .then((as) => {
        const s: Record<string, LabState> = {};
        const r: Record<string, string> = {};
        for (const a of as ?? []) {
          if (a.passed) s[a.labId] = "done";
          else if (!s[a.labId]) s[a.labId] = "doing";
          if (a.status === "running" && a.sessionId) r[a.labId] = a.sessionId;
        }
        setState(s);
        setRunning(r);
      })
      .catch(() => {});
  }, []);
  return { state, running };
}

const TRACK_ICON: Record<string, string> = {
  foundations: "book",
  linux: "shell",
  "ace-30": "project",
  capstones: "check",
  architect: "network",
  devops: "refresh",
  sre: "monitoring",
  gke: "gke",
  security: "shield",
  network: "network",
  data: "bigquery",
  serverless: "run",
  ml: "bulb",
  finops: "billing",
  career: "person",
};

/** TrackIcon is the round coloured icon of a route. */
export function TrackIcon({ id, index = 0, size = 40 }: { id: string; index?: number; size?: number }) {
  return (
    <span className={`lt-icon lt-c${index % 6}`} style={{ width: size, height: size }} aria-hidden="true">
      <Icon name={TRACK_ICON[id] ?? "folder"} size={Math.round(size * 0.55)} />
    </span>
  );
}

export const labHref = (id: string, running?: string) => (running ? `/lab?session=${running}&id=${encodeURIComponent(id)}` : `/lab?id=${encodeURIComponent(id)}`);

/** nextLab is the first lab of a route not passed yet. */
export function nextLab(ids: string[], state: Record<string, LabState>) {
  return ids.find((id) => state[id] !== "done");
}

/** PathMap draws a route as a line of numbered steps: done, current, pending. */
export function PathMap({ labs, state, running, compact }: { labs: LabSummary[]; state: Record<string, LabState>; running?: Record<string, string>; compact?: boolean }) {
  const { t } = useI18n();
  const current = nextLab(labs.map((l) => l.id), state);
  return (
    <ol className={`lp-path${compact ? " compact" : ""}`} aria-label={t("Pasos de la ruta")}>
      {labs.map((l, i) => {
        const st = state[l.id] === "done" ? "done" : l.id === current ? "current" : state[l.id] === "doing" ? "doing" : "todo";
        const word = st === "done" ? t("superado") : st === "current" ? t("siguiente") : st === "doing" ? t("empezado") : t("pendiente");
        return (
          <li key={l.id} className={`lp-step ${st}`}>
            <a href={labHref(l.id, running?.[l.id])} aria-label={`${i + 1}. ${l.title} (${word})`}>
              <span className="lp-dot" aria-hidden="true">{st === "done" ? <Icon name="check" size={18} /> : i + 1}</span>
              <span className="lp-step-text">
                <span className="lp-step-title">{l.title}</span>
                {!compact && (
                  <span className="lp-step-meta">
                    <TypeTag type={l.type} /> <span>{l.minutes} min</span> <span className="lp-stars" aria-hidden="true">{"●".repeat(Math.max(1, Math.min(5, l.difficulty)))}</span>
                  </span>
                )}
              </span>
              {st === "current" && <span className="lp-go">{t("Empezar")} →</span>}
            </a>
          </li>
        );
      })}
    </ol>
  );
}

const TYPES: Record<string, { es: string; icon: string; tone: string }> = {
  guided: { es: k("Guiado"), icon: "book", tone: "g" },
  challenge: { es: k("Reto"), icon: "bulb", tone: "c" },
  incident: { es: k("Incidente"), icon: "bell", tone: "i" },
  capstone: { es: k("Proyecto final"), icon: "check", tone: "p" },
  boss: { es: k("Jefe final"), icon: "shield", tone: "i" },
  "case-study": { es: k("Caso práctico"), icon: "project", tone: "c" },
  interview: { es: k("Entrevista"), icon: "person", tone: "p" },
};

/** TypeTag shows the kind of lab with an icon and a colour. */
export function TypeTag({ type }: { type: string }) {
  const { t } = useI18n();
  const d = TYPES[type] ?? { es: type, icon: "info", tone: "c" };
  return (
    <span className={`lt-type t-${d.tone}`}>
      <Icon name={d.icon} size={14} /> {t(d.es)}
    </span>
  );
}

/** TypeLegend explains the kinds of labs in plain words. */
export function TypeLegend() {
  const { t } = useI18n();
  return (
    <div className="lt-legend">
      <div><TypeTag type="guided" /> <span>{t("Te dice qué construir paso a paso. Ideal para empezar.")}</span></div>
      <div><TypeTag type="challenge" /> <span>{t("Te da el objetivo, tú decides cómo. Las pistas cuestan puntos.")}</span></div>
      <div><TypeTag type="incident" /> <span>{t("Algo se ha roto en producción: investiga, arréglalo y explica la causa.")}</span></div>
      <div><TypeTag type="capstone" /> <span>{t("Proyecto que junta todo lo aprendido en la ruta.")}</span></div>
    </div>
  );
}

/** StepGuide is the three steps of every lab. */
export function StepGuide({ compact, stack }: { compact?: boolean; stack?: boolean }) {
  const { t } = useI18n();
  const steps: [string, string, string][] = [
    ["book", t("Lee la misión"), t("Qué pide el cliente y qué hay que conseguir.")],
    ["compute", t("Hazlo en Google Cloud"), t("Con clics en la consola o con comandos en Cloud Shell: cuentan igual.")],
    ["check", t("Pulsa Comprobar"), t("Verás qué está bien y qué falta. Puedes comprobar las veces que quieras.")],
  ];
  return (
    <ol className={`lt-steps${compact ? " compact" : ""}${stack ? " stack" : ""}`} aria-label={t("Cómo funciona un laboratorio")}>
      {steps.map(([icon, title, text], i) => (
        <li key={title}>
          <span className="lt-steps-n" aria-hidden="true">{i + 1}</span>
          {!compact && <span className="lt-steps-icon" aria-hidden="true"><Icon name={icon} size={22} /></span>}
          <span>
            <strong>{title}</strong>
            {!compact && <span className="lt-steps-text">{text}</span>}
          </span>
        </li>
      ))}
    </ol>
  );
}

/** Checklist lets the student tick the objectives as they go (saved in the browser). */
export function Checklist({ items, storeKey }: { items: string[]; storeKey: string }) {
  const { t } = useI18n();
  const [done, setDone] = useState<Record<number, boolean>>({});
  useEffect(() => {
    try {
      setDone(JSON.parse(localStorage.getItem(storeKey) || "{}"));
    } catch {
      /* storage unavailable */
    }
  }, [storeKey]);
  const toggle = (i: number) => {
    const next = { ...done, [i]: !done[i] };
    setDone(next);
    try {
      localStorage.setItem(storeKey, JSON.stringify(next));
    } catch {
      /* storage unavailable */
    }
  };
  const n = items.filter((_, i) => done[i]).length;
  return (
    <div className="lt-check">
      <div className="lt-check-head">
        <span>{t("{n} de {total} hechos", { n, total: items.length })}</span>
        <span className="lt-bar" aria-hidden="true"><span style={{ width: `${items.length ? (n / items.length) * 100 : 0}%` }} /></span>
      </div>
      <ul>
        {items.map((o, i) => (
          <li key={o} className={done[i] ? "on" : ""}>
            <label>
              <input type="checkbox" checked={!!done[i]} onChange={() => toggle(i)} /> <span>{o}</span>
            </label>
          </li>
        ))}
      </ul>
      <p className="lt-check-note">{t("Marca lo que ya hayas hecho. La nota real la da el botón Comprobar.")}</p>
    </div>
  );
}

/** Tour is the first-visit introduction to the lab workspace. */
export function Tour() {
  const { t } = useI18n();
  const [step, setStep] = useState(-1);
  const box = useRef<HTMLDivElement>(null);
  useEffect(() => {
    try {
      if (!localStorage.getItem("gcplab.tour")) setStep(0);
    } catch {
      /* storage unavailable: skip the tour */
    }
  }, []);
  useEffect(() => {
    if (step >= 0) box.current?.querySelector<HTMLButtonElement>(".lt-tour-next")?.focus();
  }, [step]);
  if (step < 0) return null;
  const slides: [string, string, string][] = [
    ["compute", t("Esta es la consola de Google Cloud"), t("Arriba tienes la misma consola que usarás en el trabajo: menú de productos, buscador y proyecto. Cada botón hace lo mismo que en Google Cloud.")],
    ["shell", t("Abajo, Cloud Shell"), t("Cada clic en la consola escribe aquí su comando gcloud, así aprendes la terminal sin darte cuenta. También puedes escribir tú los comandos.")],
    ["book", t("A la derecha, tu misión"), t("Lee el ticket y los objetivos, pide una pista si te atascas y pulsa Comprobar para ver tu nota. No puedes romper nada: es un entorno de prácticas.")],
  ];
  const close = () => {
    try {
      localStorage.setItem("gcplab.tour", "seen");
    } catch {
      /* storage unavailable */
    }
    setStep(-1);
  };
  const [icon, title, text] = slides[step];
  const last = step === slides.length - 1;
  return (
    <div className="lt-tour" role="dialog" aria-modal="true" aria-labelledby="lt-tour-title" onKeyDown={(e) => e.key === "Escape" && close()}>
      <div className="lt-tour-box" ref={box}>
        <div className={`lt-tour-art z${step}`} aria-hidden="true">
          <div className="a-console"><Icon name="compute" size={20} /></div>
          <div className="a-panel"><Icon name="book" size={18} /></div>
          <div className="a-shell"><Icon name="shell" size={18} /></div>
        </div>
        <div className="lt-tour-body">
          <div className="lt-tour-kicker"><Icon name={icon} size={18} /> {t("Paso {n} de {total}", { n: step + 1, total: slides.length })}</div>
          <h2 id="lt-tour-title">{title}</h2>
          <p>{text}</p>
          <div className="lt-tour-dots" aria-hidden="true">{slides.map((_, i) => <span key={i} className={i === step ? "on" : ""} />)}</div>
          <div className="row" style={{ justifyContent: "flex-end" }}>
            <button type="button" className="btn secondary" onClick={close}>{t("Saltar")}</button>
            <button type="button" className="btn lt-tour-next" onClick={() => (last ? close() : setStep(step + 1))}>{last ? t("¡A practicar!") : t("Siguiente")}</button>
          </div>
        </div>
      </div>
    </div>
  );
}
