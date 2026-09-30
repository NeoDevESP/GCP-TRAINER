"use client";

// Learn: everything to know before a certification exam. Each certification
// shows its official exam structure (sections, weights, objectives, format)
// and the study topics in order of weight; each topic has concept cards,
// three games (flashcards, matching, "which service?") and a quiz; and a
// practice exam reproduces the real one (see components/Exam.tsx).
// Progress is a per-browser convenience kept in localStorage.

import { useEffect, useMemo, useRef, useState } from "react";
import Nav from "@/components/Nav";
import Exam, { Choices } from "@/components/Exam";
import { api, qs } from "@/lib/api";
import { useAuth } from "@/components/useAuth";
import { k, useI18n } from "@/lib/i18n";
import type { LabSummary } from "@/lib/types";
import { TypeTag, labHref, useProgress } from "@/components/learn";
import { fmtTime, isRight, loadHistory, loadProgress, mix, readiness, saveProgress, shuffle, topicWeights, type Cert, type Concept, type ExamInfo, type ExamRecord, type Progress, type Question, type Topic, type TopicProgress } from "@/lib/study";

const TABS: [string, string][] = [
  ["concepts", k("Conceptos")],
  ["cards", k("Tarjetas")],
  ["match", k("Emparejar")],
  ["which", k("¿Qué servicio uso?")],
  ["quiz", k("Cuestionario")],
];

function Ring({ value, label, sub, dark = true }: { value: number; label: string; sub: string; dark?: boolean }) {
  return (
    <div className={"cm-ring" + (dark ? "" : " ln-ring")} role="img" aria-label={label}>
      <svg width="132" height="132" viewBox="0 0 132 132" aria-hidden="true">
        <circle cx="66" cy="66" r="56" fill="none" stroke={dark ? "#2A2D35" : "var(--cm-sunk)"} strokeWidth="12" />
        <circle cx="66" cy="66" r="56" fill="none" stroke="#3DBA7E" strokeWidth="12" strokeLinecap="round" strokeDasharray={`${Math.max(0.001, value * 352)} 352`} transform="rotate(-90 66 66)" />
      </svg>
      <div aria-hidden="true">
        <strong>{Math.round(value * 100)}%</strong>
        <span>{sub}</span>
      </div>
    </div>
  );
}

export default function LearnPage() {
  useAuth();
  const { t, lang } = useI18n();
  const [topics, setTopics] = useState<Topic[] | null>(null);
  const [exams, setExams] = useState<ExamInfo[]>([]);
  const [certs, setCerts] = useState<Cert[]>([]);
  const [labs, setLabs] = useState<LabSummary[]>([]);
  const [cert, setCert] = useState("ACE");
  const [topic, setTopic] = useState("");
  const [tab, setTab] = useState("concepts");
  const [mode, setMode] = useState("");
  const [prog, setProg] = useState<Progress>({});
  const [hist, setHist] = useState<ExamRecord[]>([]);
  const [round, setRound] = useState(0);
  const [err, setErr] = useState("");
  const { state, running } = useProgress();

  useEffect(() => {
    setProg(loadProgress());
    setHist(loadHistory());
    const skill = qs("skill");
    if (qs("topic")) setTopic(qs("topic"));
    else if (skill) setTopic("@" + skill.split(".")[0]);
    if (qs("cert")) setCert(qs("cert"));
    if (qs("tab")) setTab(qs("tab"));
    if (qs("exam")) setMode(qs("exam") === "quick" ? "quick" : "full");
  }, []);
  useEffect(() => {
    api<{ topics: Topic[]; exams: ExamInfo[] }>("/api/learn").then((r) => { setTopics(r.topics); setExams(r.exams); }).catch((e) => setErr(e.message));
    api<{ certs: Cert[]; labs: LabSummary[] }>("/api/catalog").then((r) => {
      setCerts(r.certs);
      setLabs(r.labs);
    }).catch(() => {});
  }, [lang]);
  // ?skill=networking.vpc names a branch: open the topic of that branch.
  useEffect(() => {
    if (topics && topic.startsWith("@")) setTopic(topics.find((x) => x.id === topic.slice(1))?.id ?? topics.find((x) => x.branch === topic.slice(1))?.id ?? "");
  }, [topics, topic]);

  const update = (id: string, f: (p: TopicProgress) => TopicProgress) => {
    setProg((old) => {
      const next = { ...old, [id]: f(old[id] ?? {}) };
      saveProgress(next);
      return next;
    });
  };
  const go = (tp: string, tb = "concepts", md = "", c = cert) => {
    setTopic(tp);
    setTab(tb);
    setMode(md);
    setCert(c);
    setHist(loadHistory());
    setRound((r) => r + 1);
    const q = md ? `?cert=${c}&exam=${md}` : tp ? `?topic=${tp}&cert=${c}` : `?cert=${c}`;
    window.history.replaceState(null, "", "/learn" + q);
    window.scrollTo(0, 0);
  };

  const byId = useMemo(() => Object.fromEntries((topics ?? []).map((x) => [x.id, x])), [topics]);
  const cur = certs.find((c) => c.id === cert);
  const ex = exams.find((e) => e.cert === cert);
  const certTopics = useMemo(() => {
    if (!ex) return [];
    const w = topicWeights(ex);
    return Object.entries(w)
      .filter(([id]) => byId[id])
      .sort((a, b) => b[1] - a[1])
      .map(([id, v]) => ({ t: byId[id], pct: Math.round(v), w: v }));
  }, [ex, byId]);
  const certReady = certTopics.length ? certTopics.reduce((a, x) => a + readiness(x.t, prog[x.t.id]) * x.w, 0) / certTopics.reduce((a, x) => a + x.w, 0) : 0;
  const certHist = hist.filter((h) => h.cert === cert);

  const tp = byId[topic];
  const tpLabs = useMemo(() => (tp ? labs.filter((l) => l.skills.some((s) => s.split(".")[0] === tp.branch)).slice(0, 6) : []), [tp, labs]);
  const tpCerts = useMemo(() => (tp ? exams.filter((e) => e.domains.some((d) => d.topics.includes(tp.id))).map((e) => e.cert) : []), [tp, exams]);
  const topicLink = (id: string) => byId[id] ? <button type="button" className="res-skill" style={{ border: 0, cursor: "pointer" }} onClick={() => go(id)}>{byId[id].name}</button> : null;

  return (
    <>
      <Nav active="/learn" />
      <main id="main" className="page">
        {err && <p className="error">{err}</p>}
        {!topics ? (
          <p className="muted">{t("Cargando…")}</p>
        ) : mode && ex && cur ? (
          <>
            <button type="button" className="btn secondary" onClick={() => go("")}>← {t("Volver a {cert}", { cert: cur.id })}</button>
            <header className="cm-head" style={{ marginTop: 16, marginBottom: 20 }}>
              <div>
                <div className="muted">{cur.name}</div>
                <h1>{mode === "full" ? t("Simulacro de examen") : t("Simulacro rápido")}</h1>
              </div>
            </header>
            <Exam key={cert + mode + round} ex={ex} certName={cur.id} byId={byId} full={mode === "full"} onExit={() => go("")} topicLink={topicLink} />
          </>
        ) : tp ? (
          <>
            <button type="button" className="btn secondary" onClick={() => go("")}>← {t("Volver a {cert}", { cert })}</button>
            <section className="cm-dark cm-feature ln-hero" style={{ marginTop: 16 }}>
              <div style={{ flex: 1, minWidth: 0 }}>
                <div className="cm-kick">{t("Tema")}</div>
                <h2>{tp.name}</h2>
                <p style={{ margin: 0 }}>{tp.intro}</p>
                {tpCerts.length > 0 && (
                  <div className="row" style={{ gap: 6, marginTop: 12 }}>
                    <span className="cm-sub small">{t("Entra en:")}</span>
                    {tpCerts.map((c) => <span key={c} className="cm-dark-chip">{c}</span>)}
                  </div>
                )}
              </div>
              <div className="ln-hero-side">
                <div className="cm-sub">{t("Preparación {n} %", { n: Math.round(readiness(tp, prog[tp.id]) * 100) })}</div>
                <span className="cm-prog" style={{ width: 220 }}>
                  <div><span style={{ width: `${readiness(tp, prog[tp.id]) * 100}%` }} /></div>
                </span>
                {prog[tp.id]?.best != null && <div className="cm-sub">{t("Mejor nota: {n} %", { n: prog[tp.id]?.best ?? 0 })}</div>}
                <div className="cm-sub small">{t("{a} conceptos · {b} preguntas", { a: tp.concepts.length, b: tp.quiz.length + tp.scenarios.length })}</div>
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
                <Quiz key={tp.id + "q" + round} questions={shuffle([...tp.quiz, ...tp.scenarios]).slice(0, 12)} onDone={(pct) => update(tp.id, (p) => ({ ...p, best: Math.max(p.best ?? 0, pct) }))} restart={() => go(tp.id, "concepts")} restartLabel={t("Repasar los conceptos")} next={() => go(tp.id, "quiz")} nextLabel={t("Otro cuestionario")} />
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
                <p className="muted" style={{ margin: "6px 0 0", maxWidth: 680 }}>{t("Todo lo que debes saber antes del examen, certificación a certificación: cómo es el examen, qué secciones tiene, los conceptos de cada tema, juegos para fijarlos y simulacros como el real.")}</p>
              </div>
            </header>
            <div className="cm-levels" role="group" aria-label={t("Certificación")} style={{ marginBottom: 20 }}>
              {certs.map((c) => (
                <button key={c.id} type="button" aria-pressed={cert === c.id} title={c.name} onClick={() => go("", "concepts", "", c.id)}>
                  {c.id}
                </button>
              ))}
            </div>
            {cur && ex && (
              <>
                <section className="cm-dark cm-feature" style={{ marginBottom: 16 }}>
                  <Ring value={certReady} label={t("Preparación {n} %", { n: Math.round(certReady * 100) })} sub={t("preparado")} />
                  <div style={{ flex: 1, minWidth: 0 }}>
                    <div className="cm-kick">{cur.level === "professional" ? t("Certificación Professional") : t("Certificación Associate")}</div>
                    <h2>{cur.name}</h2>
                    <p style={{ margin: 0 }}>{ex.format}</p>
                  </div>
                  <div className="ln-hero-side">
                    <button type="button" className="cm-pillbtn light sm" onClick={() => go("", "concepts", "full")}>{t("Simulacro completo")}</button>
                    <button type="button" className="cm-pillbtn dark sm" onClick={() => go("", "concepts", "quick")}>{t("Simulacro rápido (20)")}</button>
                    {cur.guide && <a className="cm-sub" href={cur.guide} target="_blank" rel="noreferrer" style={{ color: "#C9D6FF" }}>{t("Guía oficial del examen")} ↗</a>}
                  </div>
                </section>
                <dl className="ln-facts">
                  <div><dt>{t("Duración")}</dt><dd>{t("{n} minutos", { n: ex.minutes })}</dd></div>
                  <div><dt>{t("Preguntas")}</dt><dd>{ex.questions}</dd></div>
                  <div><dt>{t("Precio")}</dt><dd>{ex.fee}</dd></div>
                  <div><dt>{t("Validez")}</dt><dd>{ex.validity}</dd></div>
                  <div className="wide"><dt>{t("Experiencia recomendada")}</dt><dd>{ex.experience}</dd></div>
                </dl>
                <p className="small muted" style={{ marginTop: 8 }}>{t("Datos tomados de la guía oficial. Google la revisa de vez en cuando: confírmalos en el enlace antes de reservar el examen.")}</p>

                <section className="cm-section">
                  <div className="cm-section-head">
                    <h2>{t("Secciones del examen")}</h2>
                    <span className="small muted">{t("Lo que evalúa cada parte y cuánto pesa")}</span>
                  </div>
                  <ol className="ln-domains">
                    {ex.domains.map((d, i) => (
                      <li key={i} className="ln-domain">
                        <div className="ln-domain-head">
                          <span className="ln-num" aria-hidden="true">{i + 1}</span>
                          <h3>{d.name}</h3>
                          <span className="ln-weight">{t("{n} %", { n: d.weight })}</span>
                        </div>
                        <span className="cm-prog"><div><span style={{ width: `${d.weight}%`, background: "var(--cm-accent)" }} /></div></span>
                        <ul>
                          {d.objectives.map((o, j) => <li key={j}>{o}</li>)}
                        </ul>
                        <div className="row" style={{ gap: 6 }}>
                          <span className="small muted">{t("Estudia:")}</span>
                          {d.topics.map((id) => byId[id] && (
                            <button key={id} type="button" className="res-skill" style={{ border: 0, cursor: "pointer" }} onClick={() => go(id)}>
                              {byId[id].name} · {Math.round(readiness(byId[id], prog[id]) * 100)}%
                            </button>
                          ))}
                        </div>
                      </li>
                    ))}
                  </ol>
                </section>

                <section className="cm-section">
                  <div className="cm-section-head">
                    <h2>{t("Temas, de más a menos importante")}</h2>
                  </div>
                  <div className="cm-grid3">
                    {certTopics.map(({ t: x, pct }, i) => {
                      const r = readiness(x, prog[x.id]);
                      return (
                        <button key={x.id} type="button" className="cm-card" onClick={() => go(x.id)}>
                          <span className="row" style={{ justifyContent: "space-between", width: "100%" }}>
                            <span className="ln-num" aria-hidden="true">{i + 1}</span>
                            <span className="cm-lvl">{t("≈{n} % del examen", { n: pct })}</span>
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
                </section>

                <section className="cm-section">
                  <div className="cm-section-head">
                    <h2>{t("Tus simulacros")}</h2>
                  </div>
                  {certHist.length === 0 ? (
                    <p className="muted" style={{ margin: 0 }}>{t("Aún no has hecho ningún simulacro de esta certificación. Cuando tengas los temas principales, prueba el rápido.")}</p>
                  ) : (
                    <div className="ln-hist-wrap">
                      <table className="ln-hist">
                        <thead>
                          <tr><th scope="col">{t("Fecha")}</th><th scope="col">{t("Tipo")}</th><th scope="col">{t("Nota")}</th><th scope="col">{t("Tiempo")}</th><th scope="col">{t("Sección más floja")}</th></tr>
                        </thead>
                        <tbody>
                          {certHist.slice(0, 10).map((h) => {
                            const weak = [...h.domains].filter((d) => d.total).sort((a, b) => a.ok / a.total - b.ok / b.total)[0];
                            const pct = Math.round((h.score / Math.max(1, h.total)) * 100);
                            return (
                              <tr key={h.at}>
                                <td>{new Date(h.at).toLocaleDateString(lang === "en" ? "en-GB" : "es-ES")}</td>
                                <td>{h.full ? t("Completo") : t("Rápido")}</td>
                                <td><span className={"pill " + (pct >= 70 ? "ok" : "warn")}>{pct}%</span></td>
                                <td>{fmtTime(h.seconds)}</td>
                                <td>{weak ? weak.name : "—"}</td>
                              </tr>
                            );
                          })}
                        </tbody>
                      </table>
                    </div>
                  )}
                </section>
              </>
            )}
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
  const [sel, setSel] = useState<number[]>([]);
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
                <span>✓ {x.answers.map((a) => x.options[a]).join(" · ")}</span>
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
  const multi = q.answers.length > 1;
  const right = isRight(q, sel);
  const check = () => {
    if (!sel.length) return;
    setChecked(true);
    const a = [...answers, right];
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
        {multi && <p className="ex-multi">{t("Elige {n} respuestas.", { n: q.answers.length })}</p>}
        <Choices
          name={"q" + i}
          multi={multi}
          options={q.options}
          sel={sel}
          disabled={checked}
          onChange={(s) => setSel(multi ? s.slice(-q.answers.length) : s)}
          mark={checked ? (n) => (q.answers.includes(n) ? " ok" : sel.includes(n) ? " bad" : "") : undefined}
        />
      </fieldset>
      <div aria-live="polite">
        {checked && (
          <div className={"ln-feedback " + (right ? "ok" : "bad")}>
            <strong>{right ? t("¡Correcto!") : t("No es correcto")}</strong>
            {!right && <span>✓ {q.answers.map((a) => q.options[a]).join(" · ")}</span>}
            <span>{q.why}</span>
          </div>
        )}
      </div>
      <div className="row">
        {!checked ? (
          <button type="button" className="cm-pillbtn primary sm" disabled={sel.length !== q.answers.length} onClick={check}>{t("Comprobar")}</button>
        ) : (
          <button type="button" className="cm-pillbtn primary sm" onClick={() => { setI(i + 1); setSel([]); setChecked(false); }}>
            {i === qs0.length - 1 ? t("Ver resultado") : t("Siguiente pregunta")} →
          </button>
        )}
      </div>
    </div>
  );
}
