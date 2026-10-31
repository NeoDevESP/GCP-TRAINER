"use client";

// Zentra final exam (internal L2 certification): six cases drawn at random
// from weeks 3-8, solved without tutor or hints, plus a theory test. The
// result is a report by area with strengths, weak spots and what to review
// (guide symptoms, cases to repeat, study topics), and a printable
// certificate when passed. The exam lives in the learner's study runs.

import { useEffect, useMemo, useState } from "react";
import Nav from "@/components/Nav";
import { Choices } from "@/components/Exam";
import { useAuth } from "@/components/useAuth";
import { LEVELS } from "@/components/learn";
import { api } from "@/lib/api";
import { k, useI18n } from "@/lib/i18n";
import { fmtTime, isRight, loadRun, questionByKey, saveRun, shuffle, syncStudy, withKeys, type Question, type Topic } from "@/lib/study";
import type { LabSummary, Stage } from "@/lib/types";

const KEY = "cm-final";
const PASS = 70;
const QUIZ_N = 20;
const AREAS: Record<string, string> = {
  access: k("Accesos e identidad"),
  network: k("Red"),
  compute: k("Cómputo y costes"),
  data: k("Datos y bases de datos"),
  platform: k("GKE y Cloud Run"),
  secops: k("Seguridad y operaciones"),
};
const BRANCH_AREA: Record<string, string> = { iam: "access", foundations: "access", networking: "network", compute: "compute", finops: "compute", architecture: "compute", storage: "data", databases: "data", data: "data", gke: "platform", devops: "platform", security: "secops", observability: "secops" };
const TOPIC_AREA: Record<string, string> = {
  "gcp-console": "access", iam: "access", "support-identity": "access",
  networking: "network", "support-network": "network",
  compute: "compute", "support-compute": "compute",
  storage: "data", databases: "data", data: "data", "support-data": "data",
  gke: "platform",
  security: "secops", observability: "secops", support: "secops", "support-craft": "secops", "support-method": "secops", "support-governance": "secops",
};
// Guide (runbook) areas behind each exam area.
const GUIDE_AREAS: Record<string, string[]> = { access: ["access"], network: ["network"], compute: ["compute"], data: ["data"], platform: ["gke", "serverless"], secops: ["security"] };

interface QuizItem {
  key: string;
  order: number[];
}
interface Final {
  started: number;
  ids: string[];
  quiz: QuizItem[];
  answers: number[][];
}
interface CaseResult {
  id: string;
  score: number;
  helped: boolean;
}
interface FinalResult {
  at: number;
  seconds: number;
  total: number;
  casesPct: number;
  quizPct: number;
  passed: boolean;
  cases: CaseResult[];
  areas: Record<string, number>;
  weakTopics: string[];
}
interface FinalDoc {
  current?: Final | null;
  history: FinalResult[];
}
interface Attempt {
  labId: string;
  started: string;
  finished?: string;
  score: number;
  status?: string;
  sessionId?: string;
  tutor?: boolean;
  solutionShown?: boolean;
  hintsUsed?: number[];
}
interface Runbook {
  id: string;
  area: string;
  symptom: string;
}

const examHref = (id: string, running?: string) => (running ? `/lab?session=${running}&id=${encodeURIComponent(id)}&exam=1` : `/lab?id=${encodeURIComponent(id)}&exam=1`);

// rebuild turns a stored quiz item back into a question with its option order.
function rebuild(byTopic: Record<string, Topic>, it: QuizItem): (Question & { topic: string }) | null {
  const q = questionByKey(byTopic, it.key);
  if (!q) return null;
  return { ...q, options: it.order.map((i) => q.options[i]), answers: q.answers.map((a) => it.order.indexOf(a)).sort(), topic: it.key.split(":")[0] };
}

export default function FinalPage() {
  useAuth();
  const { t, lang } = useI18n();
  const [labs, setLabs] = useState<Record<string, LabSummary>>({});
  const [stages, setStages] = useState<Stage[]>([]);
  const [topics, setTopics] = useState<Record<string, Topic>>({});
  const [guide, setGuide] = useState<Runbook[]>([]);
  const [atts, setAtts] = useState<Attempt[]>([]);
  const [doc, setDoc] = useState<FinalDoc>({ history: [] });
  const [name, setName] = useState("");
  const [now, setNow] = useState(Date.now());
  const [ready, setReady] = useState(false);
  const [err, setErr] = useState("");

  useEffect(() => {
    Promise.all([
      api<{ labs: LabSummary[]; tracks: { id: string; stages?: Stage[] }[] }>("/api/catalog").then((r) => {
        setLabs(Object.fromEntries(r.labs.map((l) => [l.id, l])));
        setStages(r.tracks.find((x) => x.id === "zentra")?.stages ?? []);
      }),
      api<{ topics: Topic[] }>("/api/learn").then((r) => setTopics(Object.fromEntries(withKeys(r.topics).map((x) => [x.id, x])))),
      api<{ runbooks: Runbook[] }>("/api/runbooks").then((r) => setGuide(r.runbooks)).catch(() => {}),
      api<Attempt[]>("/api/me/attempts").then((a) => setAtts(a ?? [])).catch(() => {}),
      api<{ user: { name: string } }>("/api/me").then((p) => setName(p.user.name)).catch(() => {}),
      syncStudy().catch(() => {}),
    ])
      .catch((e) => setErr(e.message))
      .finally(() => {
        setDoc(loadRun<FinalDoc>(KEY) ?? { history: [] });
        setReady(true);
      });
    const iv = setInterval(() => setNow(Date.now()), 1000);
    return () => clearInterval(iv);
  }, [lang]);

  const cur = doc.current ?? null;
  const save = (d: FinalDoc, flush = false) => {
    setDoc(d);
    return saveRun(KEY, d, flush);
  };

  // Best attempt of each exam case since the exam started; tutor or shown
  // solution means 0, hints are already discounted from the score.
  const results = useMemo(() => {
    const r: Record<string, CaseResult & { running?: string }> = {};
    if (!cur) return r;
    for (const id of cur.ids) r[id] = { id, score: 0, helped: false };
    for (const a of atts) {
      if (!cur.ids.includes(a.labId) || Date.parse(a.started) < cur.started) continue;
      if (a.status === "running" && a.sessionId) r[a.labId].running = a.sessionId;
      if (!a.finished) continue;
      const helped = !!(a.tutor || a.solutionShown);
      const score = helped ? 0 : a.score;
      if (score >= r[a.labId].score) r[a.labId] = { ...r[a.labId], score, helped: helped || (a.hintsUsed?.length ?? 0) > 0 };
    }
    return r;
  }, [atts, cur]);

  const quiz = useMemo(() => (cur ? cur.quiz.map((it) => rebuild(topics, it)) : []), [cur, topics]);

  const start = () => {
    // one case per week from week 3 (weeks 0-2 are workshops and tutorials), not the week test
    const ids = stages.filter((s) => !["w0", "oficio", "w1"].includes(s.id)).map((s) => shuffle(s.labs.slice(0, -1).filter((id) => labs[id] && labs[id].type !== "guided"))[0]).filter(Boolean);
    const pool = Object.keys(TOPIC_AREA)
      .filter((id) => topics[id])
      .map((id) => shuffle([...topics[id].quiz, ...topics[id].scenarios]));
    const picked: Question[] = [];
    for (let round = 0; picked.length < QUIZ_N && round < 10; round++) for (const qs of shuffle(pool)) if (qs[round] && picked.length < QUIZ_N) picked.push(qs[round]);
    const items = picked.map((q) => ({ key: q.key!, order: shuffle(q.options.map((_, i) => i)) }));
    save({ ...doc, current: { started: Date.now(), ids, quiz: items, answers: items.map(() => []) } }, true);
  };
  const answer = (i: number, sel: number[]) => {
    if (!cur) return;
    const answers = cur.answers.map((a, j) => (j === i ? sel : a));
    save({ ...doc, current: { ...cur, answers } });
  };
  const finish = () => {
    if (!cur) return;
    const pending = cur.ids.filter((id) => !(atts.some((a) => a.labId === id && a.finished && Date.parse(a.started) >= cur.started))).length;
    const blank = cur.answers.filter((a) => !a.length).length;
    if ((pending || blank) && !confirm(t("Quedan {c} casos sin entregar y {q} preguntas sin responder; contarán como 0. ¿Terminar el examen?", { c: pending, q: blank }))) return;
    const cases = cur.ids.map((id) => ({ id, score: results[id]?.score ?? 0, helped: !!results[id]?.helped }));
    const casesPct = cases.reduce((a, c) => a + c.score, 0) / Math.max(1, cases.length);
    const area: Record<string, { got: number; max: number }> = {};
    const add = (a: string, got: number, max: number) => {
      area[a] = area[a] ?? { got: 0, max: 0 };
      area[a].got += got;
      area[a].max += max;
    };
    for (const c of cases) add(BRANCH_AREA[labs[c.id]?.branch] ?? "secops", c.score, 100);
    let right = 0;
    const missedTopics: Record<string, number> = {};
    quiz.forEach((q, i) => {
      if (!q) return;
      const ok = isRight(q, cur.answers[i] ?? []);
      if (ok) right++;
      else missedTopics[q.topic] = (missedTopics[q.topic] ?? 0) + 1;
      add(TOPIC_AREA[q.topic] ?? "secops", ok ? 100 : 0, 100);
    });
    const quizPct = (100 * right) / Math.max(1, quiz.length);
    const total = Math.round(0.7 * casesPct + 0.3 * quizPct);
    const areas = Object.fromEntries(Object.entries(area).map(([a, v]) => [a, Math.round((100 * v.got) / Math.max(1, v.max))]));
    const res: FinalResult = {
      at: Date.now(), seconds: Math.round((Date.now() - cur.started) / 1000), total, casesPct: Math.round(casesPct), quizPct: Math.round(quizPct),
      passed: total >= PASS && cases.every((c) => c.score >= 50), cases, areas,
      weakTopics: Object.entries(missedTopics).sort((a, b) => b[1] - a[1]).map(([x]) => x).slice(0, 4),
    };
    save({ current: null, history: [res, ...doc.history].slice(0, 10) }, true);
  };

  const last = doc.history[0];
  const stageOf = (id: string) => stages.find((s) => s.labs.includes(id));

  return (
    <>
      <Nav active="/zentra" />
      <main id="main" className="page">
        <header className="cm-head">
          <div>
            <h1>{t("Examen final de Zentra")}</h1>
            <p className="muted" style={{ margin: "6px 0 0", maxWidth: 720 }}>
              {t("La certificación interna de Soporte N2: seis casos al azar de las semanas 3 a 8, sin profesor ni pistas, y un test de teoría con preguntas del oficio de soporte. Al terminar recibes un informe por áreas con lo que dominas y lo que debes repasar.")}
            </p>
          </div>
        </header>
        {err && <p className="error">{err}</p>}
        {!ready ? (
          <p className="muted">{t("Cargando…")}</p>
        ) : !cur ? (
          <>
            <section className="card fx-rules">
              <h2>{t("Cómo funciona")}</h2>
              <ul>
                <li>{t("Seis casos, uno de cada semana de la 3 a la 8, elegidos al azar. Se abren en modo examen: sin profesor ni pistas.")}</li>
                <li>{t("Veinte preguntas de teoría de los temas de soporte y de Google Cloud.")}</li>
                <li>{t("Nota: 70 % los casos y 30 % el test. Apruebas con {n} o más y ningún caso por debajo de 50.", { n: PASS })}</li>
                <li>{t("Un caso hecho con el profesor o viendo la solución cuenta como 0. Tiempo orientativo: 4 horas; puedes parar y seguir otro día.")}</li>
              </ul>
              <button type="button" className="cm-pillbtn ink sm" onClick={start} disabled={stages.length < 2}>{t("Empezar el examen")}</button>
            </section>
            {last && <Report res={last} labs={labs} guide={guide} topics={topics} name={name} stageOf={stageOf} />}
          </>
        ) : (
          <>
            <section className="card sh-bar">
              <div>
                <span className="muted">{t("Tiempo de examen")}</span>
                <strong className="sh-clock">{fmtTime(Math.floor((now - cur.started) / 1000))}</strong>
              </div>
              <div>
                <span className="muted">{t("Casos entregados")}</span>
                <strong>{cur.ids.filter((id) => atts.some((a) => a.labId === id && a.finished && Date.parse(a.started) >= cur.started)).length} / {cur.ids.length}</strong>
              </div>
              <div>
                <span className="muted">{t("Preguntas respondidas")}</span>
                <strong>{cur.answers.filter((a) => a.length).length} / {cur.quiz.length}</strong>
              </div>
              <button type="button" className="btn" onClick={finish}>{t("Terminar y ver el informe")}</button>
            </section>
            <section className="cm-section" aria-labelledby="fx-cases">
              <div className="cm-section-head">
                <h2 id="fx-cases">{t("Parte 1 · Casos")}</h2>
                <button type="button" className="btn secondary" onClick={() => api<Attempt[]>("/api/me/attempts").then((a) => setAtts(a ?? [])).catch(() => {})}>{t("Actualizar notas")}</button>
              </div>
              <ol className="sh-queue">
                {cur.ids.map((id) => {
                  const l = labs[id];
                  const r = results[id];
                  const st = stageOf(id);
                  const done = atts.some((a) => a.labId === id && a.finished && Date.parse(a.started) >= cur.started);
                  return (
                    <li key={id} className={`card sh-ticket${done ? " done" : ""}`}>
                      {l?.ticket && <span className={`sh-prio p${Number(l.ticket.priority.replace("P", "")) || 4}`}>{l.ticket.priority}</span>}
                      <div className="sh-body">
                        <div className="muted sh-id">{st?.title} · {st ? t(LEVELS[st.level] ?? st.level) : ""}</div>
                        <div className="sh-title">{l?.title ?? id}</div>
                      </div>
                      <div className="sh-sla">{done ? <span className={r.score >= 50 ? "sh-ok" : "sh-late"}>{t("Nota {n}", { n: r.score })}{r.helped ? " · " + t("con ayuda") : ""}</span> : <span className="muted">{t("Sin entregar")}</span>}</div>
                      <a className={done ? "btn secondary" : "btn"} href={examHref(id, r?.running)}>{done ? t("Repetir") : r?.running ? t("Continuar") : t("Abrir")}</a>
                    </li>
                  );
                })}
              </ol>
            </section>
            <section className="cm-section" aria-labelledby="fx-quiz">
              <div className="cm-section-head">
                <h2 id="fx-quiz">{t("Parte 2 · Teoría")}</h2>
                <span className="muted small">{t("Tus respuestas se guardan solas. No verás si aciertas hasta el informe.")}</span>
              </div>
              <ol className="fx-quiz">
                {quiz.map((q, i) =>
                  q ? (
                    <li key={cur.quiz[i].key} className="card">
                      <fieldset className="ln-q">
                        <legend>{q.q}</legend>
                        {q.answers.length > 1 && <p className="ex-multi">{t("Elige {n} respuestas.", { n: q.answers.length })}</p>}
                        <Choices name={"fx" + i} multi={q.answers.length > 1} options={q.options} sel={cur.answers[i] ?? []} onChange={(s) => answer(i, q.answers.length > 1 ? s.slice(-q.answers.length) : s)} />
                      </fieldset>
                    </li>
                  ) : null,
                )}
              </ol>
            </section>
          </>
        )}
      </main>
    </>
  );
}

function Report({ res, labs, guide, topics, name, stageOf }: { res: FinalResult; labs: Record<string, LabSummary>; guide: Runbook[]; topics: Record<string, Topic>; name: string; stageOf: (id: string) => Stage | undefined }) {
  const { t, lang } = useI18n();
  const weak = Object.entries(res.areas).filter(([, v]) => v < 70).map(([a]) => a);
  const strong = Object.entries(res.areas).filter(([, v]) => v >= 80).map(([a]) => a);
  const repeat = res.cases.filter((c) => c.score < 70);
  const runbooks = guide.filter((r) => weak.some((a) => (GUIDE_AREAS[a] ?? []).includes(r.area))).slice(0, 6);
  const date = new Date(res.at).toLocaleDateString(lang === "en" ? "en-GB" : "es-ES", { day: "numeric", month: "long", year: "numeric" });
  return (
    <section className="cm-section fx-report" aria-labelledby="fx-report">
      <div className="cm-section-head">
        <h2 id="fx-report">{t("Tu último informe")}</h2>
        {res.passed && <button type="button" className="btn secondary" onClick={() => window.print()}>{t("Imprimir el certificado")}</button>}
      </div>
      <div className={`card fx-score ${res.passed ? "ok" : "ko"}`}>
        <strong className="fx-total">{res.total}</strong>
        <div>
          <div className="fx-verdict">{res.passed ? t("Aprobado: Soporte N2 de Zentra") : t("Aún no: repasa lo de abajo y vuelve a intentarlo")}</div>
          <div className="muted small">{t("Casos {a} · Teoría {b} · {time}", { a: res.casesPct, b: res.quizPct, time: fmtTime(res.seconds) })}</div>
        </div>
      </div>
      <div className="fx-areas">
        {Object.keys(AREAS).filter((a) => res.areas[a] != null).map((a) => (
          <div key={a} className="fx-area">
            <span>{t(AREAS[a])}</span>
            <span className="fx-meter" role="img" aria-label={t("{area}: {n} %", { area: t(AREAS[a]), n: res.areas[a] })}>
              <span className={res.areas[a] >= 80 ? "ok" : res.areas[a] >= 70 ? "" : "ko"} style={{ width: `${res.areas[a]}%` }} />
            </span>
            <strong>{res.areas[a]} %</strong>
          </div>
        ))}
      </div>
      <div className="grid two" style={{ alignItems: "start" }}>
        <div className="card">
          <h3>{t("Lo que dominas")}</h3>
          {strong.length ? <ul>{strong.map((a) => <li key={a}>{t(AREAS[a])}</li>)}</ul> : <p className="muted">{t("Todavía ninguna área por encima del 80 %.")}</p>}
          <h3>{t("Casos del examen")}</h3>
          <ul className="fx-cases">
            {res.cases.map((c) => (
              <li key={c.id}>
                <span>{labs[c.id]?.title ?? c.id}</span>
                <span className="muted small">{stageOf(c.id)?.title}</span>
                <strong className={c.score >= 70 ? "sh-ok" : "sh-late"}>{c.score}{c.helped ? " *" : ""}</strong>
              </li>
            ))}
          </ul>
          {res.cases.some((c) => c.helped) && <p className="muted small">{t("* con pistas o con ayuda del profesor")}</p>}
        </div>
        <div className="card">
          <h3>{t("Qué repasar")}</h3>
          {!weak.length && !repeat.length && !res.weakTopics.length ? (
            <p className="muted">{t("Nada pendiente. ¡Buen trabajo!")}</p>
          ) : (
            <>
              {repeat.length > 0 && (
                <>
                  <p className="small"><strong>{t("Repite estos casos")}</strong></p>
                  <ul>{repeat.map((c) => <li key={c.id}><a href={`/lab?id=${encodeURIComponent(c.id)}`}>{labs[c.id]?.title ?? c.id}</a></li>)}</ul>
                </>
              )}
              {runbooks.length > 0 && (
                <>
                  <p className="small"><strong>{t("Léete estos síntomas de la guía")}</strong></p>
                  <ul>{runbooks.map((r) => <li key={r.id}><a href={`/guide?id=${r.id}`}>{r.symptom}</a></li>)}</ul>
                </>
              )}
              {res.weakTopics.length > 0 && (
                <>
                  <p className="small"><strong>{t("Y estos temas de teoría")}</strong></p>
                  <ul>{res.weakTopics.map((id) => topics[id] && <li key={id}><a href={`/learn?topic=${id}`}>{topics[id].name}</a></li>)}</ul>
                </>
              )}
            </>
          )}
        </div>
      </div>
      {res.passed && (
        <div className="fx-cert" aria-label={t("Certificado")}>
          <div className="fx-cert-kick">Cloud Mastery · Zentra</div>
          <div className="fx-cert-title">{t("Certificado de Soporte N2")}</div>
          <div className="fx-cert-name">{name}</div>
          <p>{t("ha superado el examen final de la ruta Zentra con una nota de {n} sobre 100: seis incidencias y peticiones reales de Google Cloud resueltas sin ayuda y la prueba de teoría.", { n: res.total })}</p>
          <div className="muted small">{date} · {t("Banco ficticio creado para aprender")}</div>
        </div>
      )}
    </section>
  );
}
