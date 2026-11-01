"use client";

// Onboarding: goal, time per day and a six-question diagnosis. The result
// places the learner (lib/model.ts placement), sets the study plan (daily
// time, Week 0 optional or not) and opens the first activity of their level.
// Everything is saved on the account (study runs "cm-onboarding" and
// "cm-plan"), so the plan and the dashboard use it on every device.

import { useEffect, useState } from "react";
import Nav from "@/components/Nav";
import { useAuth } from "@/components/useAuth";
import { k, useI18n } from "@/lib/i18n";
import { DEFAULT_PREFS, type PlanPrefs } from "@/lib/plan";
import { PLACEMENT_START, placement, type Placement } from "@/lib/model";
import { loadRun, saveRun, syncStudy } from "@/lib/study";

const GOALS: [string, string][] = [
  ["job", k("Trabajar en soporte cloud (por ejemplo, en un banco)")],
  ["ace", k("Aprobar la certificación Associate Cloud Engineer")],
  ["explore", k("Aprender Google Cloud a mi ritmo")],
];
const TIMES = [30, 60, 90, 120, 180];

// [question, options, index of the right one]
const QUESTIONS: [string, string[], number][] = [
  [k("¿Qué es un proyecto de Google Cloud?"), [k("Una carpeta de archivos en Cloud Storage"), k("El contenedor donde viven los recursos, los permisos y la facturación"), k("Un tipo de máquina virtual"), k("Una cuenta de usuario")], 1],
  [k("Una VM está en la zona europe-west1-b. ¿Qué significa?"), [k("Que está en una zona de la región europe-west1"), k("Que se replica en toda Europa"), k("Que es la segunda copia de la VM"), k("Que solo se puede usar desde Bélgica")], 0],
  [k("Una persona recibe 403 PERMISSION_DENIED al leer un bucket. ¿Qué miras primero?"), [k("Si el bucket existe"), k("La red de la VM"), k("Qué identidad hace la llamada y qué roles tiene sobre ese bucket"), k("Reinicio el servicio")], 2],
  [k("Una VM sin IP pública no puede descargar paquetes de internet. ¿Qué suele faltar?"), [k("Un disco más grande"), k("Una cuenta de servicio nueva"), k("Más CPU"), k("Cloud NAT para esa red y región")], 3],
  [k("El balanceador devuelve 502 y todos los backends salen UNHEALTHY. ¿Causa típica?"), [k("El cortafuegos no deja pasar las sondas de salud de Google"), k("Falta un certificado SSL"), k("El bucket es privado"), k("La facturación está desactivada")], 0],
  [k("Antes de cambiar algo en producción en un banco, ¿qué haces?"), [k("Lo cambio rápido y miro si funciona"), k("Reinicio todo para empezar limpio"), k("Reúno evidencias, propongo el cambio mínimo y lo valido después"), k("Doy permisos de propietario para no bloquearme")], 2],
];

const LEVEL: Record<Placement, [string, string]> = {
  zero: [k("Empiezas desde cero"), k("Haremos primero la Semana 0: qué es la nube, la consola, la terminal, redes y permisos, con un taller guiado para cada cosa.")],
  basics: [k("Ya tienes una base"), k("La Semana 0 queda como opcional. Empiezas por el oficio de soporte: leer la cola, priorizar y escribir bien.")],
  intermediate: [k("Vas con ventaja"), k("Te saltas lo básico y entras directo en los casos de Zentra, el banco digital ficticio.")],
};

interface Onboarding {
  goal: string;
  minutes: number;
  correct: number;
  total: number;
  level: Placement;
  at: number;
}

export default function StartPage() {
  useAuth();
  const { t } = useI18n();
  const [step, setStep] = useState(0);
  const [goal, setGoal] = useState("job");
  const [minutes, setMinutes] = useState(60);
  const [answers, setAnswers] = useState<Record<number, number>>({});
  const [done, setDone] = useState<Onboarding | null>(null);

  useEffect(() => {
    syncStudy()
      .catch(() => {})
      .finally(() => {
        const prev = loadRun<Onboarding>("cm-onboarding");
        if (prev?.level) setDone(prev);
      });
  }, []);

  const finish = () => {
    const correct = QUESTIONS.filter(([, , right], i) => answers[i] === right).length;
    const level = placement(correct, QUESTIONS.length);
    const result: Onboarding = { goal, minutes, correct, total: QUESTIONS.length, level, at: Date.now() };
    const plan = loadRun<{ checked?: Record<string, boolean>; prefs?: Partial<PlanPrefs> }>("cm-plan") ?? {};
    const prefs: PlanPrefs = { ...DEFAULT_PREFS, ...(plan.prefs ?? {}), minutesPerDay: minutes, skipBasics: PLACEMENT_START[level].skipBasics };
    saveRun("cm-plan", { checked: plan.checked ?? {}, prefs });
    saveRun("cm-onboarding", result);
    setDone(result);
    setStep(0);
    window.scrollTo({ top: 0 });
  };

  return (
    <>
      <Nav active="/dashboard" />
      <main id="main" className="page" style={{ maxWidth: 860 }}>
        {done && step === 0 ? (
          <section aria-labelledby="ob-result">
            <p className="muted" style={{ margin: 0 }}>{t("Tu punto de partida")}</p>
            <h1 id="ob-result">{t(LEVEL[done.level][0])}</h1>
            <p style={{ fontSize: 17, lineHeight: "26px" }}>{t(LEVEL[done.level][1])}</p>
            <div className="card" style={{ marginTop: 16 }}>
              <p style={{ marginTop: 0 }}>
                {t("Diagnóstico: {c} de {n} respuestas correctas.", { c: done.correct, n: done.total })}{" "}
                {t("Tu plan reparte el trabajo en sesiones de hasta {m} min al día.", { m: done.minutes })}
              </p>
              <div className="row" style={{ gap: 12, flexWrap: "wrap" }}>
                <a className="btn" href={`/lab?id=${PLACEMENT_START[done.level].lab}`}>{t("Empezar ahora")}</a>
                <a className="btn secondary" href="/plan">{t("Ver mi plan")}</a>
                <button type="button" className="btn secondary" onClick={() => { setDone(null); setStep(1); setAnswers({}); }}>{t("Repetir el diagnóstico")}</button>
              </div>
            </div>
          </section>
        ) : (
          <>
            <p className="muted" style={{ margin: 0 }}>{t("Paso {n} de 2", { n: step <= 1 ? 1 : 2 })}</p>
            <h1>{step <= 1 ? t("Bienvenida: ajustemos tu ruta") : t("Diagnóstico rápido")}</h1>
            {step <= 1 ? (
              <form
                className="card ob-form"
                onSubmit={(e) => {
                  e.preventDefault();
                  setStep(2);
                  window.scrollTo({ top: 0 });
                }}
              >
                <fieldset>
                  <legend>{t("¿Qué quieres conseguir?")}</legend>
                  {GOALS.map(([id, label]) => (
                    <label key={id} className="ob-opt">
                      <input type="radio" name="goal" value={id} checked={goal === id} onChange={() => setGoal(id)} /> {t(label)}
                    </label>
                  ))}
                </fieldset>
                <fieldset>
                  <legend>{t("¿Cuánto tiempo tienes al día?")}</legend>
                  <div className="ob-times">
                    {TIMES.map((m) => (
                      <label key={m} className={`ob-chip${minutes === m ? " on" : ""}`}>
                        <input type="radio" name="minutes" value={m} checked={minutes === m} onChange={() => setMinutes(m)} className="sr-only" />
                        {m < 60 ? `${m} min` : `${m / 60} h`.replace(".5", ",5")}
                      </label>
                    ))}
                  </div>
                </fieldset>
                <button className="btn" type="submit">{t("Siguiente: diagnóstico")}</button>
              </form>
            ) : (
              <form
                className="ob-form"
                onSubmit={(e) => {
                  e.preventDefault();
                  finish();
                }}
              >
                <p className="muted">{t("Seis preguntas para saber por dónde empezar. Si no lo sabes, elige «No lo sé»: adivinar te pondría en un nivel que aún no te toca.")}</p>
                {QUESTIONS.map(([q, opts], i) => (
                  <fieldset key={i} className="card">
                    <legend>{i + 1}. {t(q)}</legend>
                    {[...opts, k("No lo sé")].map((o, j) => (
                      <label key={j} className="ob-opt">
                        <input type="radio" name={`q${i}`} required checked={answers[i] === (j < opts.length ? j : -1)} onChange={() => setAnswers({ ...answers, [i]: j < opts.length ? j : -1 })} /> {t(o)}
                      </label>
                    ))}
                  </fieldset>
                ))}
                <div className="row" style={{ gap: 12 }}>
                  <button type="button" className="btn secondary" onClick={() => setStep(1)}>{t("Atrás")}</button>
                  <button className="btn" type="submit">{t("Ver mi punto de partida")}</button>
                </div>
              </form>
            )}
          </>
        )}
      </main>
    </>
  );
}
