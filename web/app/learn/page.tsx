"use client";

// Learn: what to know before a quiz, per certification. Each topic has
// concept cards, three games (flashcards, matching, "which service?") and a
// quiz; a certification also has a mixed practice exam weighted like the
// real one. Progress is a per-browser convenience kept in localStorage.

import { useEffect, useMemo, useRef, useState } from "react";
import Nav from "@/components/Nav";
import { api, qs } from "@/lib/api";
import { useAuth } from "@/components/useAuth";
import { k, useI18n } from "@/lib/i18n";
import type { LabSummary } from "@/lib/types";
import { TypeTag, labHref, useProgress } from "@/components/learn";

interface Concept {
  id: string;
  term: string;
  what: string;
  points: string[];
  tip: string;
}
interface Question {
  q: string;
  options: string[];
  answer: number;
  why: string;
}
interface Topic {
  id: string;
  name: string;
  intro: string;
  concepts: Concept[];
  scenarios: Question[];
  quiz: Question[];
}
interface Cert {
  id: string;
  name: string;
  level: string;
  weights: Record<string, number>;
  guide: string;
}

interface TopicProgress {
  seen?: string[];
  known?: string[];
  best?: number;
}
type Progress = Record<string, TopicProgress>;

const STORE = "cm-learn";
function loadProgress(): Progress {
  try {
    return JSON.parse(localStorage.getItem(STORE) || "{}") as Progress;
  } catch {
    return {};
  }
}
function saveProgress(p: Progress) {
  try {
    localStorage.setItem(STORE, JSON.stringify(p));
  } catch {
    /* private mode: progress is a convenience only */
  }
}

function shuffle<T>(a: T[]): T[] {
  const b = [...a];
  for (let i = b.length - 1; i > 0; i--) {
    const j = Math.floor(Math.random() * (i + 1));
    [b[i], b[j]] = [b[j], b[i]];
  }
  return b;
}

// Shuffled copy of a question whose answer index follows its option.
function mix(q: Question): Question {
  const order = shuffle(q.options.map((_, i) => i));
  return { ...q, options: order.map((i) => q.options[i]), answer: order.indexOf(q.answer) };
}

// Topic readiness 0..1: concepts read (40 %) + best quiz score (60 %).
function readiness(t: Topic, p?: TopicProgress) {
  const seen = (p?.seen ?? []).filter((id) => t.concepts.some((c) => c.id === id)).length;
  return 0.4 * (seen / Math.max(1, t.concepts.length)) + 0.6 * ((p?.best ?? 0) / 100);
}

const TABS: [string, string][] = [
  ["concepts", k("Conceptos")],
  ["cards", k("Tarjetas")],
  ["match", k("Emparejar")],
  ["which", k("¿Qué servicio uso?")],
  ["quiz", k("Cuestionario")],
];

export default function LearnPage() {
  useAuth();
  const { t, lang } = useI18n();
  const [topics, setTopics] = useState<Topic[] | null>(null);
  const [certs, setCerts] = useState<Cert[]>([]);
  const [labs, setLabs] = useState<LabSummary[]>([]);
  const [cert, setCert] = useState("ACE");
  const [topic, setTopic] = useState("");
  const [tab, setTab] = useState("concepts");
  const [exam, setExam] = useState(false);
  const [prog, setProg] = useState<Progress>({});
  const [round, setRound] = useState(0);
  const [err, setErr] = useState("");
  const { state, running } = useProgress();

  useEffect(() => {
    setProg(loadProgress());
    const skill = qs("skill");
    const tp = qs("topic") || (skill ? skill.split(".")[0] : "");
    if (tp) setTopic(tp);
    if (qs("cert")) setCert(qs("cert"));
    if (qs("tab")) setTab(qs("tab"));
    if (qs("exam")) setExam(true);
  }, []);
  useEffect(() => {
    api<{ topics: Topic[] }>("/api/learn").then((r) => setTopics(r.topics)).catch((e) => setErr(e.message));
    api<{ certs: Cert[]; labs: LabSummary[] }>("/api/catalog").then((r) => {
      setCerts(r.certs);
      setLabs(r.labs);
    }).catch(() => {});
  }, [lang]);

  const update = (id: string, f: (p: TopicProgress) => TopicProgress) => {
    setProg((old) => {
      const next = { ...old, [id]: f(old[id] ?? {}) };
      saveProgress(next);
      return next;
    });
  };
  const go = (tp: string, tb = "concepts", ex = false) => {
    setTopic(tp);
    setTab(tb);
    setExam(ex);
    setRound((r) => r + 1);
    const q = ex ? `?cert=${cert}&exam=1` : tp ? `?topic=${tp}&cert=${cert}` : `?cert=${cert}`;
    window.history.replaceState(null, "", "/learn" + q);
    window.scrollTo(0, 0);
  };

  const byId = useMemo(() => Object.fromEntries((topics ?? []).map((x) => [x.id, x])), [topics]);
  const cur = certs.find((c) => c.id === cert);
  const certTopics = useMemo(() => {
    if (!cur || !topics) return [];
    const total = Object.values(cur.weights).reduce((a, b) => a + b, 0) || 1;
    return Object.entries(cur.weights)
      .filter(([b]) => byId[b])
      .sort((a, b) => b[1] - a[1])
      .map(([b, w]) => ({ t: byId[b], pct: Math.round((w / total) * 100), w }));
  }, [cur, topics, byId]);
  const certReady = certTopics.length ? certTopics.reduce((a, x) => a + readiness(x.t, prog[x.t.id]) * x.w, 0) / certTopics.reduce((a, x) => a + x.w, 0) : 0;

  const tp = byId[topic];
  const tpLabs = useMemo(() => (tp ? labs.filter((l) => l.skills.some((s) => s.split(".")[0] === tp.id)).slice(0, 6) : []), [tp, labs]);

  // A practice exam: 20 questions picked in proportion to the blueprint weights.
  const examQs = useMemo(() => {
    if (!exam || !certTopics.length) return [];
    const total = certTopics.reduce((a, x) => a + x.w, 0);
    const out: Question[] = [];
    for (const x of certTopics) {
      const n = Math.max(1, Math.round((20 * x.w) / total));
      out.push(...shuffle([...x.t.quiz, ...x.t.scenarios]).slice(0, n));
    }
    return shuffle(out).slice(0, 20);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [exam, cert, topics, round]);

  return (
    <>
      <Nav active="/learn" />
      <main id="main" className="page">
        {err && <p className="error">{err}</p>}
        {!topics ? (
          <p className="muted">{t("Cargando…")}</p>
        ) : exam && cur ? (
          <>
            <button type="button" className="btn secondary" onClick={() => go("")}>← {t("Volver a {cert}", { cert: cur.id })}</button>
            <header className="cm-head" style={{ marginTop: 16 }}>
              <div>
                <div className="muted">{cur.name}</div>
                <h1>{t("Examen de práctica")}</h1>
                <p className="muted" style={{ margin: "6px 0 0" }}>{t("{n} preguntas repartidas según el peso de cada tema en el examen real.", { n: examQs.length })}</p>
              </div>
            </header>
            <Quiz key={"exam" + cert + round} questions={examQs} onDone={() => {}} restart={() => go("", "concepts", true)} />
          </>
        ) : tp ? (
          <>
            <button type="button" className="btn secondary" onClick={() => go("")}>← {t("Todos los temas")}</button>
            <section className="cm-dark cm-feature ln-hero" style={{ marginTop: 16 }}>
              <div style={{ flex: 1, minWidth: 0 }}>
                <div className="cm-kick">{t("Tema")}</div>
                <h2>{tp.name}</h2>
                <p style={{ margin: 0 }}>{tp.intro}</p>
              </div>
              <div className="ln-hero-side">
                <div className="cm-sub">{t("Preparación {n} %", { n: Math.round(readiness(tp, prog[tp.id]) * 100) })}</div>
                <span className="cm-prog" style={{ width: 220 }}>
                  <div><span style={{ width: `${readiness(tp, prog[tp.id]) * 100}%` }} /></div>
                </span>
                {prog[tp.id]?.best != null && <div className="cm-sub">{t("Mejor nota: {n} %", { n: prog[tp.id]?.best ?? 0 })}</div>}
              </div>
            </section>
            <div className="ln-tabs" role="tablist" aria-label={t("Modo de estudio")}>
              {TABS.map(([id, name], i) => (
                <button key={id} type="button" role="tab" id={"tab-" + id} aria-selected={tab === id} aria-controls="ln-panel" onClick={() => go(tp.id, id)}>
                  <span className="ln-step" aria-hidden="true">{i + 1}</span>
                  {t(name)}
                </button>
              ))}
            </div>
            <section id="ln-panel" role="tabpanel" aria-labelledby={"tab-" + tab} className="ln-panel">
              {tab === "concepts" && (
                <Concepts
                  topic={tp}
                  seen={prog[tp.id]?.seen ?? []}
                  toggle={(id) => update(tp.id, (p) => ({ ...p, seen: (p.seen ?? []).includes(id) ? (p.seen ?? []).filter((x) => x !== id) : [...(p.seen ?? []), id] }))}
                  next={() => go(tp.id, "cards")}
                />
              )}
              {tab === "cards" && <Cards key={tp.id + round} concepts={tp.concepts} known={prog[tp.id]?.known ?? []} mark={(id, ok) => update(tp.id, (p) => ({ ...p, known: ok ? Array.from(new Set([...(p.known ?? []), id])) : (p.known ?? []).filter((x) => x !== id) }))} next={() => go(tp.id, "match")} />}
              {tab === "match" && <Match key={tp.id + round} concepts={tp.concepts} next={() => go(tp.id, "which")} />}
              {tab === "which" && <Quiz key={tp.id + "w" + round} questions={tp.scenarios} short onDone={() => {}} restart={() => go(tp.id, "which")} next={() => go(tp.id, "quiz")} nextLabel={t("Ir al cuestionario")} />}
              {tab === "quiz" && (
                <Quiz key={tp.id + "q" + round} questions={[...tp.quiz, ...shuffle(tp.scenarios).slice(0, 2)]} onDone={(pct) => update(tp.id, (p) => ({ ...p, best: Math.max(p.best ?? 0, pct) }))} restart={() => go(tp.id, "concepts")} restartLabel={t("Repasar los conceptos")} />
              )}
            </section>
            {tpLabs.length > 0 && (
              <section className="cm-section">
                <div className="cm-section-head">
                  <h2>{t("Practícalo en la consola")}</h2>
                </div>
                <div className="cm-grid3">
                  {tpLabs.map((l) => (
                    <a key={l.id} className="cm-card" href={labHref(l.id, running[l.id])}>
                      <span className="row small"><TypeTag type={l.type} />{state[l.id] === "done" && <span className="pill ok">{t("superado")}</span>}</span>
                      <h3 style={{ fontSize: 17, lineHeight: "22px" }}>{l.title}</h3>
                      <p>{l.summary}</p>
                    </a>
                  ))}
                </div>
              </section>
            )}
          </>
        ) : (
          <>
            <header className="cm-head">
              <div>
                <h1>{t("Aprende")}</h1>
                <p className="muted" style={{ margin: "6px 0 0", maxWidth: 640 }}>{t("Todo lo que debes saber antes del cuestionario, tema a tema. Lee los conceptos, juega para fijarlos y comprueba si estás listo.")}</p>
              </div>
            </header>
            <div className="cm-levels" role="group" aria-label={t("Certificación")} style={{ marginBottom: 20 }}>
              {certs.map((c) => (
                <button key={c.id} type="button" aria-pressed={cert === c.id} title={c.name} onClick={() => { setCert(c.id); window.history.replaceState(null, "", `/learn?cert=${c.id}`); }}>
                  {c.id}
                </button>
              ))}
            </div>
            {cur && (
              <section className="cm-dark cm-feature" style={{ marginBottom: 24 }}>
                <div className="cm-ring" role="img" aria-label={t("Preparación {n} %", { n: Math.round(certReady * 100) })}>
                  <svg width="132" height="132" viewBox="0 0 132 132" aria-hidden="true">
                    <circle cx="66" cy="66" r="56" fill="none" stroke="#2A2D35" strokeWidth="12" />
                    <circle cx="66" cy="66" r="56" fill="none" stroke="#3DBA7E" strokeWidth="12" strokeLinecap="round" strokeDasharray={`${Math.max(0.001, certReady * 352)} 352`} transform="rotate(-90 66 66)" />
                  </svg>
                  <div aria-hidden="true">
                    <strong>{Math.round(certReady * 100)}%</strong>
                    <span>{t("preparado")}</span>
                  </div>
                </div>
                <div style={{ flex: 1, minWidth: 0 }}>
                  <div className="cm-kick">{t("Certificación")}</div>
                  <h2>{cur.name}</h2>
                  <p style={{ margin: 0 }}>{t("Estudia los temas en este orden: primero los que más pesan en el examen. Cuando los tengas, haz el examen de práctica.")}</p>
                </div>
                <div className="ln-hero-side">
                  <button type="button" className="cm-pillbtn light sm" onClick={() => go("", "concepts", true)}>{t("Examen de práctica")}</button>
                  {cur.guide && <a className="cm-sub" href={cur.guide} target="_blank" rel="noreferrer" style={{ color: "#C9D6FF" }}>{t("Guía oficial del examen")} ↗</a>}
                </div>
              </section>
            )}
            <div className="cm-grid3">
              {certTopics.map(({ t: x, pct }, i) => {
                const r = readiness(x, prog[x.id]);
                return (
                  <button key={x.id} type="button" className="cm-card" onClick={() => go(x.id)}>
                    <span className="row" style={{ justifyContent: "space-between", width: "100%" }}>
                      <span className="ln-num" aria-hidden="true">{i + 1}</span>
                      <span className="cm-lvl">{t("{n} % del examen", { n: pct })}</span>
                    </span>
                    <h3>{x.name}</h3>
                    <p>{x.intro}</p>
                    <span className="small muted">{t("{a} conceptos · {b} preguntas", { a: x.concepts.length, b: x.quiz.length + x.scenarios.length })}</span>
                    <span className="cm-prog" style={{ width: "100%" }}>
                      <div><span style={{ width: `${r * 100}%` }} /></div>
                      {Math.round(r * 100)}%
                    </span>
                  </button>
                );
              })}
            </div>
          </>
        )}
      </main>
    </>
  );
}

function Concepts({ topic, seen, toggle, next }: { topic: Topic; seen: string[]; toggle: (id: string) => void; next: () => void }) {
  const { t } = useI18n();
  const n = topic.concepts.filter((c) => seen.includes(c.id)).length;
  return (
    <>
      <p className="muted" style={{ marginTop: 0 }}>{t("Lee cada concepto y márcalo cuando lo entiendas. {n} de {total} entendidos.", { n, total: topic.concepts.length })}</p>
      <div className="ln-concepts">
        {topic.concepts.map((c) => {
          const ok = seen.includes(c.id);
          return (
            <article key={c.id} className={"ln-concept" + (ok ? " ok" : "")}>
              <h3>{c.term}</h3>
              <p className="ln-what">{c.what}</p>
              <ul>
                {c.points.map((p, i) => <li key={i}>{p}</li>)}
              </ul>
              <div className="ln-tip">
                <strong>{t("En el examen")}</strong>
                <span>{c.tip}</span>
              </div>
              <button type="button" className={"btn " + (ok ? "secondary" : "")} aria-pressed={ok} onClick={() => toggle(c.id)}>
                {ok ? "✓ " + t("Entendido") : t("Lo he entendido")}
              </button>
            </article>
          );
        })}
      </div>
      <div className="row" style={{ marginTop: 20 }}>
        <button type="button" className="cm-pillbtn primary sm" onClick={next}>{t("Siguiente: repasar con tarjetas")} →</button>
      </div>
    </>
  );
}

function Cards({ concepts, known, mark, next }: { concepts: Concept[]; known: string[]; mark: (id: string, ok: boolean) => void; next: () => void }) {
  const { t } = useI18n();
  const [deck, setDeck] = useState<Concept[]>(() => shuffle(concepts));
  const [flip, setFlip] = useState(false);
  const [done, setDone] = useState(0);
  const c = deck[0];
  const answer = (ok: boolean) => {
    mark(c.id, ok);
    setFlip(false);
    if (ok) {
      setDone((d) => d + 1);
      setDeck((d) => d.slice(1));
    } else {
      setDeck((d) => [...d.slice(1), d[0]]);
    }
  };
  if (!c)
    return (
      <div className="ln-end">
        <div className="ln-big">🎉</div>
        <h2>{t("¡Te sabes todas las tarjetas!")}</h2>
        <div className="row">
          <button type="button" className="btn secondary" onClick={() => { setDeck(shuffle(concepts)); setDone(0); }}>{t("Otra vez")}</button>
          <button type="button" className="cm-pillbtn primary sm" onClick={next}>{t("Siguiente: emparejar")} →</button>
        </div>
      </div>
    );
  return (
    <div className="ln-cards">
      <p className="muted" style={{ margin: 0 }} aria-live="polite">{t("{n} de {total} dominadas · Pulsa la tarjeta para darle la vuelta.", { n: done, total: concepts.length })}</p>
      <button type="button" className={"ln-card" + (flip ? " flip" : "")} onClick={() => setFlip(!flip)} aria-label={flip ? c.what : t("{term}. Pulsa para ver la respuesta", { term: c.term })}>
        {flip ? (
          <>
            <span className="ln-card-kick">{c.term}</span>
            <span className="ln-card-back">{c.what}</span>
            <span className="ln-card-tip">{c.tip}</span>
          </>
        ) : (
          <>
            <span className="ln-card-kick">{t("¿Qué es…?")}</span>
            <span className="ln-card-front">{c.term}</span>
            {known.includes(c.id) && <span className="ln-card-tip">{t("Ya la sabías la última vez")}</span>}
          </>
        )}
      </button>
      <div className="row" style={{ justifyContent: "center" }}>
        <button type="button" className="btn secondary" disabled={!flip} onClick={() => answer(false)}>{t("Repasar otra vez")}</button>
        <button type="button" className="btn" disabled={!flip} onClick={() => answer(true)}>{t("Me la sé")}</button>
      </div>
    </div>
  );
}

function Match({ concepts, next }: { concepts: Concept[]; next: () => void }) {
  const { t } = useI18n();
  const deal = () => shuffle(concepts).slice(0, 5);
  const [set, setSet] = useState<Concept[]>(deal);
  const [defs, setDefs] = useState<Concept[]>(() => shuffle(set));
  const [pick, setPick] = useState("");
  const [ok, setOk] = useState<string[]>([]);
  const [wrong, setWrong] = useState("");
  const [miss, setMiss] = useState(0);
  const [msg, setMsg] = useState("");
  const timer = useRef<ReturnType<typeof setTimeout> | null>(null);
  useEffect(() => () => { if (timer.current) clearTimeout(timer.current); }, []);
  const again = () => {
    const s = deal();
    setSet(s);
    setDefs(shuffle(s));
    setOk([]);
    setPick("");
    setMiss(0);
    setMsg("");
  };
  const choose = (id: string) => {
    if (!pick || ok.includes(id)) return;
    if (id === pick) {
      setOk([...ok, id]);
      setMsg(t("¡Correcto!"));
    } else {
      setMiss(miss + 1);
      setWrong(id);
      setMsg(t("No es esa. Prueba otra."));
      if (timer.current) clearTimeout(timer.current);
      timer.current = setTimeout(() => setWrong(""), 700);
    }
    setPick("");
  };
  const finished = ok.length === set.length;
  return (
    <div>
      <p className="muted" style={{ marginTop: 0 }}>{t("Elige un concepto a la izquierda y después su definición a la derecha.")}</p>
      <p className="sr-only" aria-live="polite">{msg}</p>
      <div className="ln-match">
        <div className="ln-col" role="group" aria-label={t("Conceptos")}>
          {set.map((c) => (
            <button key={c.id} type="button" className={"ln-chip" + (ok.includes(c.id) ? " ok" : pick === c.id ? " on" : "")} disabled={ok.includes(c.id)} aria-pressed={pick === c.id} onClick={() => setPick(c.id)}>
              {c.term}
            </button>
          ))}
        </div>
        <div className="ln-col" role="group" aria-label={t("Definiciones")}>
          {defs.map((c) => (
            <button key={c.id} type="button" className={"ln-chip def" + (ok.includes(c.id) ? " ok" : wrong === c.id ? " bad" : "")} disabled={ok.includes(c.id) || !pick} onClick={() => choose(c.id)}>
              {c.what}
            </button>
          ))}
        </div>
      </div>
      {finished && (
        <div className="ln-end">
          <h2>{miss === 0 ? t("¡Perfecto, sin fallos!") : t("Hecho con {n} fallos", { n: miss })}</h2>
          <div className="row">
            <button type="button" className="btn secondary" onClick={again}>{t("Otra ronda")}</button>
            <button type="button" className="cm-pillbtn primary sm" onClick={next}>{t("Siguiente: ¿qué servicio uso?")} →</button>
          </div>
        </div>
      )}
    </div>
  );
}

function Quiz({ questions, onDone, restart, restartLabel, next, nextLabel, short }: { questions: Question[]; onDone: (pct: number) => void; restart: () => void; restartLabel?: string; next?: () => void; nextLabel?: string; short?: boolean }) {
  const { t } = useI18n();
  const [qs0] = useState(() => shuffle(questions).map(mix));
  const [i, setI] = useState(0);
  const [sel, setSel] = useState<number | null>(null);
  const [checked, setChecked] = useState(false);
  const [answers, setAnswers] = useState<boolean[]>([]);
  const q = qs0[i];
  if (!qs0.length) return <p className="muted">{t("No hay preguntas todavía.")}</p>;
  if (!q) {
    const good = answers.filter(Boolean).length;
    const pct = Math.round((good / qs0.length) * 100);
    const pass = pct >= 80;
    return (
      <div className="ln-end">
        <div className="cm-ring ln-ring" role="img" aria-label={t("{n} de {total} correctas", { n: good, total: qs0.length })}>
          <svg width="132" height="132" viewBox="0 0 132 132" aria-hidden="true">
            <circle cx="66" cy="66" r="56" fill="none" stroke="var(--cm-sunk)" strokeWidth="12" />
            <circle cx="66" cy="66" r="56" fill="none" stroke={pass ? "var(--cm-ok)" : "var(--cm-amber)"} strokeWidth="12" strokeLinecap="round" strokeDasharray={`${Math.max(0.001, (pct / 100) * 352)} 352`} transform="rotate(-90 66 66)" />
          </svg>
          <div aria-hidden="true">
            <strong>{pct}%</strong>
            <span>{t("{n} de {total}", { n: good, total: qs0.length })}</span>
          </div>
        </div>
        <h2>{short ? t("¡Hecho!") : pass ? t("¡Listo para el cuestionario!") : pct >= 50 ? t("Casi: repasa lo que has fallado") : t("Aún no: vuelve a los conceptos y repite")}</h2>
        {!short && <p className="muted" style={{ margin: 0 }}>{pass ? t("Has superado el 80 %. Ya puedes pasar a los laboratorios de este tema.") : t("Necesitas un 80 %. Mira abajo las que fallaste y por qué.")}</p>}
        {answers.some((a) => !a) && (
          <ul className="ln-review">
            {qs0.map((x, n) => !answers[n] && (
              <li key={n}>
                <strong>{x.q}</strong>
                <span>✓ {x.options[x.answer]}</span>
                <span className="muted">{x.why}</span>
              </li>
            ))}
          </ul>
        )}
        <div className="row">
          <button type="button" className="btn secondary" onClick={restart}>{restartLabel ?? t("Otra vez")}</button>
          {next && <button type="button" className="cm-pillbtn primary sm" onClick={next}>{nextLabel ?? t("Siguiente")} →</button>}
        </div>
      </div>
    );
  }
  const right = sel === q.answer;
  const check = () => {
    if (sel == null) return;
    setChecked(true);
    const a = [...answers, sel === q.answer];
    setAnswers(a);
    if (i === qs0.length - 1) onDone(Math.round((a.filter(Boolean).length / qs0.length) * 100));
  };
  return (
    <div className="ln-quiz">
      <div className="cm-prog">
        <div><span style={{ width: `${(i / qs0.length) * 100}%`, background: "var(--cm-accent)" }} /></div>
        {t("Pregunta {n} de {total}", { n: i + 1, total: qs0.length })}
      </div>
      <fieldset className="ln-q">
        <legend>{q.q}</legend>
        {q.options.map((o, n) => (
          <label key={n} className={"ln-opt" + (checked && n === q.answer ? " ok" : checked && n === sel ? " bad" : sel === n ? " on" : "")}>
            <input type="radio" name={"q" + i} checked={sel === n} disabled={checked} onChange={() => setSel(n)} />
            <span>{o}</span>
          </label>
        ))}
      </fieldset>
      <div aria-live="polite">
        {checked && (
          <div className={"ln-feedback " + (right ? "ok" : "bad")}>
            <strong>{right ? t("¡Correcto!") : t("No es correcto")}</strong>
            <span>{q.why}</span>
          </div>
        )}
      </div>
      <div className="row">
        {!checked ? (
          <button type="button" className="cm-pillbtn primary sm" disabled={sel == null} onClick={check}>{t("Comprobar")}</button>
        ) : (
          <button type="button" className="cm-pillbtn primary sm" onClick={() => { setI(i + 1); setSel(null); setChecked(false); }}>
            {i === qs0.length - 1 ? t("Ver resultado") : t("Siguiente pregunta")} →
          </button>
        )}
      </div>
    </div>
  );
}
