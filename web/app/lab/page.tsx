"use client";

import { Checklist, StepGuide, Tour, TypeTag } from "@/components/learn";
import Tutor from "@/components/Tutor";
import React, { Suspense, useCallback, useEffect, useRef, useState } from "react";
import Nav from "@/components/Nav";
import Terminal, { type TerminalHandle } from "@/components/Terminal";
import CloudConsole from "@/components/console/CloudConsole";
import CodeEditor from "@/components/CodeEditor";
import Markdown from "@/components/Markdown";
import Bar from "@/components/Bar";
import { api, qs } from "@/lib/api";
import { useAuth } from "@/components/useAuth";
import { k, useI18n } from "@/lib/i18n";
import { constraint, evidenceField, factor, hintKind, label, labLevel, labMode, role, ticketStatus, validator } from "@/lib/labels";
import { Icon } from "@/components/console/icons";
import type { SessionInfo } from "@/lib/types";


function Briefing({ labId, onStarted }: { labId: string; onStarted: (s: SessionInfo, tutor?: boolean) => void }) {
  const { t, lang } = useI18n();
  const [lab, setLab] = useState<any>(null);
  const [fidelity, setFidelity] = useState("");
  const [err, setErr] = useState("");
  const [busy, setBusy] = useState(false);
  useEffect(() => {
    api(`/api/labs/${encodeURIComponent(labId)}`)
      .then((l) => {
        setLab(l);
        setFidelity(l.defaultFidelity || "F0");
      })
      .catch((e) => setErr(e.message));
  }, [labId, lang]);
  const start = async (tutor?: boolean) => {
    setBusy(true);
    setErr("");
    try {
      const s = await api<SessionInfo>(`/api/labs/${encodeURIComponent(labId)}/start`, { body: { fidelity } });
      onStarted(s, tutor);
    } catch (e: any) {
      setErr(e.message);
    } finally {
      setBusy(false);
    }
  };
  if (err && !lab) return <p className="error">{err}</p>;
  if (!lab) return <p className="muted">{t("Cargando…")}</p>;
  return (
    <div className="card" style={{ maxWidth: 900, margin: "0 auto" }}>
      <div className="row small muted">
        <span>{lab.track}{lab.day ? ` · ${t("día {n}", { n: lab.day })}` : ""}</span>
        <TypeTag type={lab.type} />
        <span className="pill">{label(labLevel, lab.level, t)}</span>
        <span>{lab.minutes} min</span>
      </div>
      <h1>{lab.title}</h1>
      <StepGuide />
      <h3>{t("Tu misión")}</h3>
      <Markdown text={String(lab.story || lab.summary).replace(/\{\{\s*\.(\w+)\s*\}\}/g, "‹$1›")} />
      {lab.objectives?.length ? (
        <>
          <h3>{t("Objetivos")}</h3>
          <ul>{lab.objectives.map((o: string) => <li key={o}>{o}</li>)}</ul>
        </>
      ) : null}
      {lab.constraints?.length ? (
        <>
          <h3>{t("Restricciones")}</h3>
          <ul>{lab.constraints.map((o: string) => <li key={o}>{constraint(o, t)}</li>)}</ul>
        </>
      ) : null}
      {lab.rubric?.length ? (
        <details>
          <summary><strong>{t("Cómo se te evaluará")}</strong></summary>
          <table>
            <tbody>
              {lab.rubric.map((r: any) => (
                <tr key={r.name}>
                  <td>{r.name} {r.critical && <span className="pill bad">{t("crítico")}</span>}</td>
                  <td className="muted small">{label(validator, r.validator, t)}</td>
                  <td>{t("{n} ptos.", { n: r.points })}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </details>
      ) : null}
      <div className="row" style={{ marginTop: 20 }}>
        {qs("tutor") === "1" && lab.type !== "boss" ? (
          <>
            <button className="btn" onClick={() => start(true)} disabled={busy} style={{ height: 44, padding: "0 24px" }}>
              <Icon name="school" size={18} /> {busy ? t("Preparando el entorno…") : t("Empezar con el profesor")}
            </button>
            <button className="btn secondary" onClick={() => start()} disabled={busy} style={{ height: 44 }}>
              {t("Empezar sin profesor")}
            </button>
          </>
        ) : (
          <>
            <button className="btn" onClick={() => start()} disabled={busy} style={{ height: 44, padding: "0 24px" }}>
              {busy ? t("Preparando el entorno…") : t("Empezar laboratorio")}
            </button>
            {lab.type !== "boss" && (
              <button className="btn secondary" onClick={() => start(true)} disabled={busy} style={{ height: 44 }}>
                <Icon name="school" size={18} /> {t("Empezar con el profesor")}
              </button>
            )}
          </>
        )}
        <span className="muted small">{t("No puedes romper nada: es un proyecto de prácticas que se crea solo para ti.")}</span>
      </div>
      <details style={{ marginTop: 12 }}>
        <summary className="small muted">{t("Opciones avanzadas")}</summary>
      <div className="row" style={{ marginTop: 8 }}>
        <label htmlFor="fid" style={{ margin: 0 }}>{t("Fidelidad")}</label>
        <select id="fid" value={fidelity} onChange={(e) => setFidelity(e.target.value)} style={{ width: 200 }}>
          {(lab.fidelity ?? ["F0"]).map((f: string) => (
            <option key={f} value={f}>
              {f === "F0" ? t("F0 — simulador") : f === "F1" ? t("F1 — emuladores") : t("F2 — proyecto real de GCP")}
            </option>
          ))}
        </select>
      </div>
      </details>
      {err && <p className="error">{err}</p>}
    </div>
  );
}

// shellWord quotes text as one single-quoted shell word (newlines become spaces).
const shellWord = (text: string) => "'" + text.replace(/\s*\n\s*/g, " ").trim().replace(/'/g, "'\\''") + "'";

type DeskAction = "update" | "comment" | "resolve" | "escalate";
const DESK_ACTIONS: [DeskAction, string, string][] = [
  ["update", k("Responder al solicitante"), k("Lo lee quien abrió el ticket: qué pasa o pasaba, qué has hecho y qué tiene que hacer. Sin jerga.")],
  ["comment", k("Nota interna"), k("Solo para el equipo: causa técnica, evidencias, quién hizo qué.")],
  ["resolve", k("Resolver y cerrar"), k("Qué fallaba, qué has cambiado y cómo lo has comprobado.")],
  ["escalate", k("Escalar"), k("A quién y con todo lo que necesita: qué, dónde, desde cuándo, impacto y qué has hecho ya.")],
];

/** Desk is the ticket tool of the lab: read the ticket, ask the people involved and work the ticket. */
function Desk({ data, sessionId, onChange }: { data: any; sessionId: string; onChange: () => void }) {
  const { t: tr } = useI18n();
  const [who, setWho] = useState("");
  const [question, setQuestion] = useState("");
  const [action, setAction] = useState<DeskAction>("update");
  const [team, setTeam] = useState("");
  const [text, setText] = useState("");
  const [msg, setMsg] = useState<{ ok: boolean; text: string } | null>(null);
  const [busy, setBusy] = useState(false);
  if (!data?.ticket && !data?.actors?.length) return <p className="muted">{tr("Este laboratorio no tiene ticket. Usa la terminal y los objetivos de la izquierda.")}</p>;
  const t = data.ticket;
  const actor = who || data.actors?.[0]?.role || "";
  const run = async (line: string, done: () => void) => {
    setBusy(true);
    setMsg(null);
    try {
      const r = await api<{ output: string; exit: number }>(`/api/sessions/${sessionId}/exec`, { body: { line } });
      setMsg({ ok: r.exit === 0, text: r.output.trim() });
      if (r.exit === 0) done();
      onChange();
    } catch (e: any) {
      setMsg({ ok: false, text: e.message });
    } finally {
      setBusy(false);
    }
  };
  const ask = () => question.trim() && run(`ask ${actor} ${shellWord(question)}`, () => setQuestion(""));
  const write = () => {
    if (!text.trim() || (action === "escalate" && !team.trim())) return;
    const line = action === "escalate" ? `ticket escalate ${shellWord(team.trim().split(/\s+/)[0])} ${shellWord(text)}` : `ticket ${action} ${shellWord(text)}`;
    run(line, () => setText(""));
  };
  const closed = t && (t.status === "RESOLVED" || t.status === "ESCALATED");
  return (
    <div className="col">
      {t && (
        <div className="card">
          <div className="row">
            <strong>{t.id}</strong>
            <span className={`pill ${t.priority === "P1" ? "bad" : t.priority === "P2" ? "warn" : ""}`}>{t.priority}</span>
            <span className="pill">{t.kind}</span>
            <span className="pill">{label(ticketStatus, t.status, tr)}</span>
            {t.sla && <span className="muted small">SLA {t.sla}</span>}
          </div>
          <h3>{t.summary}</h3>
          {t.impact && <p className="small"><strong>{tr("Impacto:")}</strong> {t.impact}</p>}
          <p className="small muted">{tr("Abierto por {who}", { who: t.reporter })}{t.service ? ` · ${tr("servicio {name}", { name: t.service })}` : ""}</p>
          {t.comments?.map((c: any, i: number) => (
            <div key={i} className={"small dk-comment" + (c.from === "student" ? (c.public ? " pub" : " int") : "")}>
              <strong>{c.from === "student" ? tr("Tú") : c.from}</strong> <span className="muted">{c.at}{c.from === "student" ? ` · ${c.public ? tr("respuesta pública") : tr("nota interna")}` : ""}</span>
              <div style={{ whiteSpace: "pre-wrap" }}>{c.text}</div>
            </div>
          ))}
          {t.attachments?.map((a: any) => (
            <details key={a.name}>
              <summary className="small">📎 {a.name}</summary>
              <pre>{a.content}</pre>
            </details>
          ))}
        </div>
      )}
      {data.actors?.length ? (
        <div className="card">
          <h3>{tr("Preguntar a una persona")}</h3>
          <p className="small muted" style={{ marginTop: 0 }}>{tr("Casi ningún ticket llega completo. Pregunta lo que falta: qué recurso, qué error exacto, desde cuándo, qué ha cambiado, quién aprueba.")}</p>
          <div className="dk-people">
            {data.actors.map((a: any) => (
              <label key={a.role} className={"dk-person" + (actor === a.role ? " on" : "")}>
                <input type="radio" name="dk-who" checked={actor === a.role} onChange={() => setWho(a.role)} />
                <span>
                  <strong>{a.name}</strong>
                  <span className="small muted">{a.persona}</span>
                </span>
              </label>
            ))}
          </div>
          <label className="sr-only" htmlFor="dk-q">{tr("Tu pregunta")}</label>
          <textarea id="dk-q" rows={2} value={question} placeholder={tr("Escribe tu pregunta…")} onChange={(e) => setQuestion(e.target.value)} onKeyDown={(e) => { if (e.key === "Enter" && !e.shiftKey) { e.preventDefault(); ask(); } }} />
          <div className="row" style={{ marginTop: 8 }}>
            <button type="button" className="btn" disabled={busy || !question.trim()} onClick={ask}>{tr("Preguntar")}</button>
          </div>
        </div>
      ) : null}
      {data.questions?.length ? (
        <div className="card">
          <h3>{tr("Conversaciones")}</h3>
          {data.questions.map((q: any, i: number) => (
            <div key={i} className="small" style={{ marginBottom: 8 }}>
              <strong>{tr("Tú → {who}:", { who: q.actor })}</strong> {q.question}
              <div className="muted">{q.answer}</div>
            </div>
          ))}
        </div>
      ) : null}
      {t && (
        <div className="card">
          <h3>{tr("Trabajar el ticket")}</h3>
          {closed && <p className="small pill ok" style={{ display: "inline-block" }}>{t.status === "RESOLVED" ? tr("Ticket resuelto") : tr("Ticket escalado a {team}", { team: t.escalatedTo })}</p>}
          <div className="dk-actions" role="radiogroup" aria-label={tr("Qué quieres hacer")}>
            {DESK_ACTIONS.map(([id, name]) => (
              <label key={id} className={"dk-action" + (action === id ? " on" : "")}>
                <input type="radio" name="dk-action" checked={action === id} onChange={() => setAction(id)} />
                {tr(name)}
              </label>
            ))}
          </div>
          <p className="small muted">{tr(DESK_ACTIONS.find(([id]) => id === action)![2])}</p>
          {action === "escalate" && (
            <>
              <label className="small" htmlFor="dk-team">{tr("Equipo")}</label>
              <input id="dk-team" value={team} placeholder={tr("p. ej. csirt, redes, google")} onChange={(e) => setTeam(e.target.value)} />
            </>
          )}
          <label className="sr-only" htmlFor="dk-text">{tr("Texto")}</label>
          <textarea id="dk-text" rows={4} value={text} onChange={(e) => setText(e.target.value)} placeholder={tr("Escribe aquí…")} />
          <div className="row" style={{ marginTop: 8 }}>
            <button type="button" className="btn" disabled={busy || !text.trim() || (action === "escalate" && !team.trim())} onClick={write}>
              {action === "update" ? tr("Enviar respuesta") : action === "comment" ? tr("Guardar nota") : action === "resolve" ? tr("Resolver") : tr("Escalar")}
            </button>
          </div>
          <p className="small muted">{tr("También desde la terminal:")} <code>ticket update|comment|resolve &quot;…&quot;</code>, <code>ticket escalate EQUIPO &quot;…&quot;</code>, <code>ask QUIÉN &quot;…&quot;</code>.</p>
        </div>
      )}
      <div aria-live="polite">
        {msg && <p className={"small " + (msg.ok ? "" : "error")} style={{ whiteSpace: "pre-wrap" }}>{msg.text}</p>}
      </div>
    </div>
  );
}

/** ShellEditor is the Cloud Shell Editor: an explorer of the shell's files and a code editor. */
function ShellEditor({ sessionId, tick }: { sessionId: string; tick: number }) {
  const { t } = useI18n();
  const [files, setFiles] = useState<Record<string, string>>({});
  const [sel, setSel] = useState("");
  const [draft, setDraft] = useState("");
  const [dirty, setDirty] = useState(false);
  const [newName, setNewName] = useState("");
  const [msg, setMsg] = useState("");
  const dirtyRef = useRef(false);
  dirtyRef.current = dirty;
  useEffect(() => {
    api<Record<string, string>>(`/api/sessions/${sessionId}/views/files`)
      .then((f) => {
        setFiles(f ?? {});
        if (!dirtyRef.current && sel && f?.[sel] !== undefined) setDraft(f[sel]);
      })
      .catch(() => {});
  }, [sessionId, tick, sel]);
  const open = (p: string) => {
    setSel(p);
    setDraft(files[p] ?? "");
    setDirty(false);
  };
  const save = async () => {
    await api(`/api/sessions/${sessionId}/files`, { method: "PUT", body: { path: sel, content: draft } });
    setFiles({ ...files, [sel]: draft });
    setDirty(false);
    setMsg(t("Guardado {file}", { file: sel }));
    setTimeout(() => setMsg(""), 2000);
  };
  return (
    <div className="se">
      <div className="se-explorer">
        <div className="se-title">{t("Explorador")}</div>
        <div className="se-root"><Icon name="folder" size={16} /> ~</div>
        <ul>
          {Object.keys(files).sort().map((p) => (
            <li key={p}>
              <button type="button" className={p === sel ? "on" : ""} onClick={() => open(p)}>
                <Icon name="file" size={16} /> <span>{p}</span>
              </button>
            </li>
          ))}
        </ul>
        <form onSubmit={(e) => { e.preventDefault(); const n = newName.trim(); if (n) { setFiles({ ...files, [n]: files[n] ?? "" }); open(n); setNewName(""); } }}>
          <input placeholder={t("archivo nuevo (p. ej. main.tf)")} aria-label={t("Nombre del archivo nuevo")} value={newName} onChange={(e) => setNewName(e.target.value)} />
        </form>
      </div>
      <div className="se-main">
        {sel ? (
          <>
            <div className="se-tabs">
              <span className="se-tab">{sel}{dirty ? " ●" : ""}</span>
              <span className="se-spacer" />
              <span className="se-msg">{msg}</span>
              <button type="button" className="se-btn" onClick={save} disabled={!dirty}>
                <Icon name="save" size={16} /> {t("Guardar")}
              </button>
            </div>
            <div className="se-code">
              <CodeEditor path={sel} value={draft} onChange={(v) => { setDraft(v); setDirty(true); }} />
            </div>
          </>
        ) : (
          <p className="se-empty">
            {t("Elige un archivo o crea uno. Los archivos se comparten con la terminal")} (<code>cat</code>, <code>terraform apply</code>, <code>kubectl apply -f</code>, <code>docker build</code>…).
          </p>
        )}
      </div>
    </div>
  );
}

function Result({ out, session }: { out: any; session: SessionInfo }) {
  const { t, lang } = useI18n();
  const r = out.result;
  const [skillNames, setSkillNames] = useState<Record<string, string>>({});
  useEffect(() => {
    api<any>("/api/catalog")
      .then((c) => {
        const m: Record<string, string> = {};
        for (const b of c.branches ?? []) for (const sk of b.skills ?? []) m[sk.id] = sk.name;
        setSkillNames(m);
      })
      .catch(() => {});
  }, [lang]);
  const pct = r.max ? r.score / r.max : 0;
  const good = r.items.filter((it: any) => it.earned >= it.points);
  const missing = r.items.flatMap((it: any) => (it.checks ?? []).filter((c: any) => !c.pass).map((c: any) => ({ ...c, item: it.name, critical: it.critical })));
  const verdict = r.passed ? t("¡Superado!") : r.criticalFailed ? t("Aún no: falta algo esencial") : pct >= 0.6 ? t("Casi: te falta muy poco") : t("Aún no, pero ya sabes qué falta");
  const retry = `/lab?id=${encodeURIComponent(session.labId)}`;
  return (
    <div className="res">
      <section className="cm-dark res-hero" aria-labelledby="res-title">
        <div className="cm-ring" role="img" aria-label={t("{n} de {total} puntos", { n: r.score, total: r.max })}>
          <svg width="132" height="132" viewBox="0 0 132 132" aria-hidden="true">
            <circle cx="66" cy="66" r="56" fill="none" stroke="#2A2D35" strokeWidth="12" />
            <circle cx="66" cy="66" r="56" fill="none" stroke={r.passed ? "#3DBA7E" : "#F2B64C"} strokeWidth="12" strokeLinecap="round" strokeDasharray={`${Math.max(0.001, pct * 352)} 352`} transform="rotate(-90 66 66)" />
          </svg>
          <div aria-hidden="true">
            <strong>{r.score}</strong>
            <span>{t("de {n}", { n: r.max })}</span>
          </div>
        </div>
        <div style={{ display: "flex", flexDirection: "column", gap: 10, flex: 1, minWidth: 0 }}>
          <div className="row" style={{ gap: 8 }}>
            <span className="cm-dark-chip accent">+{out.xp} XP</span>
            {out.bonus ? <span className="cm-dark-chip">+{out.bonus} {t("de bonificación")}</span> : null}
            {out.tutor && <span className="cm-dark-chip" style={{ color: "#F2B64C" }}>{t("Hecho con el profesor")}</span>}
          </div>
          <div className="cm-kick">{session.title}</div>
          <h1 id="res-title" style={{ margin: 0, color: "#FFFFFF" }}>{verdict}</h1>
          <p style={{ margin: 0 }}>
            {r.passed
              ? out.tutor
                ? t("Lo has conseguido con ayuda. Repítelo sin el profesor para fijarlo y que cuente del todo.")
                : t("Lo has resuelto por tu cuenta. Sigue con el siguiente paso de tu ruta.")
              : t("Abajo tienes qué falta y por qué. Vuelve a intentarlo: cada intento cuenta para aprender.")}
          </p>
          <div className="row" style={{ gap: 12, marginTop: 6 }}>
            {!r.passed ? (
              <a className="cm-pillbtn primary" href={retry}>{t("Reintentar")}</a>
            ) : out.tutor ? (
              <a className="cm-pillbtn primary" href={retry}>{t("Repetir sin profesor")}</a>
            ) : (
              <a className="cm-pillbtn primary" href="/dashboard">{t("Siguiente paso")}</a>
            )}
            {!r.passed && session.type !== "boss" && !out.tutor && (
              <a className="cm-pillbtn dark" href={retry + "&tutor=1"}>
                <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="#F2B64C" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true"><path d="M2 9l10-5 10 5-10 5z" /><path d="M6 11v5c3 2 9 2 12 0v-5" /></svg>
                {t("Reintentar con profesor")}
              </a>
            )}
            <a className="cm-pillbtn dark" href="/dashboard">{t("Inicio")}</a>
          </div>
        </div>
      </section>

      <div className="res-grid">
        <section className="res-card" aria-labelledby="res-good">
          <h2 id="res-good"><span className="res-ico ok" aria-hidden="true">✓</span>{t("Lo que has hecho bien")}</h2>
          {good.length ? (
            <ul>
              {good.map((it: any) => (
                <li key={it.name}>
                  <strong>{it.name}</strong>
                  <span className="muted small">{Math.round(it.earned * 10) / 10}/{it.points} {t("puntos")}</span>
                </li>
              ))}
            </ul>
          ) : (
            <p className="muted">{t("Todavía nada completo. Empieza por lo que marca el primer objetivo.")}</p>
          )}
        </section>
        <section className="res-card" aria-labelledby="res-miss">
          <h2 id="res-miss"><span className="res-ico bad" aria-hidden="true">!</span>{t("Lo que falta y por qué")}</h2>
          {missing.length ? (
            <ul>
              {missing.map((c: any, i: number) => (
                <li key={i}>
                  <span className="row" style={{ gap: 8 }}>
                    <strong>{c.desc || c.type}</strong>
                    {c.critical && <span className="pill bad">{t("esencial")}</span>}
                  </span>
                  {c.detail && <span className="res-why">{c.detail}</span>}
                  <span className="muted small">{c.item}</span>
                </li>
              ))}
            </ul>
          ) : (
            <p className="muted">{t("¡Nada! Has cumplido todas las comprobaciones.")}</p>
          )}
        </section>
      </div>

      {(out.mentor?.length || out.skills?.length) ? (
        <section className="res-card res-review" aria-labelledby="res-rev">
          <h2 id="res-rev"><span className="res-ico amber" aria-hidden="true">?</span>{t("Qué repasar")}</h2>
          {out.mentor?.length ? (
            <ul>
              {out.mentor.map((m: string, i: number) => <li key={i}>{m}</li>)}
            </ul>
          ) : null}
          {out.skills?.length ? (
            <div className="row" style={{ gap: 8, marginTop: 8 }}>
              <span className="muted small">{t("Conceptos de este laboratorio:")}</span>
              {out.skills.map((sk: string) => (
                <a key={sk} className="res-skill" href={`/learn?skill=${encodeURIComponent(sk)}`}>{skillNames[sk] ?? sk}</a>
              ))}
            </div>
          ) : null}
        </section>
      ) : null}

      {r.process && (
        <details className="res-card">
          <summary><strong>{t("Cómo has trabajado")}</strong> <span className="muted small">{t("diagnóstico, seguridad, coste, comunicación…")}</span></summary>
          <div className="res-factors">
            {r.process.factors.map((f: any) => (
              <div key={f.name}>
                <div className="row" style={{ justifyContent: "space-between" }}>
                  <span>{label(factor, f.name, t)}</span>
                  <span className="muted small">{Math.round(f.score * 100)}%</span>
                </div>
                <Bar value={f.score * 100} label={label(factor, f.name, t)} />
                {f.signals?.length ? <div className="muted small">{f.signals.join("; ")}</div> : null}
              </div>
            ))}
          </div>
          {r.process.blindFixes?.length ? <p className="small warn">{t("Cambios antes de reunir evidencias:")} {r.process.blindFixes.join(", ")}</p> : null}
          {r.process.constraintViolations?.length ? <p className="small error">{t("Restricciones incumplidas:")} {r.process.constraintViolations.join(", ")}</p> : null}
          {r.feedback?.length ? <ul className="small">{r.feedback.map((f: string, i: number) => <li key={i}>{f}</li>)}</ul> : null}
        </details>
      )}
      {out.postmortemReview && (
        <section className="res-card">
          <h2>{t("Revisión del post-mortem")}</h2>
          <Markdown text={out.postmortemReview} />
        </section>
      )}
      {out.company && (
        <section className="res-card">
          <h2>Nebula Corporation</h2>
          <p className="small">
            {t("Día {n}.", { n: out.company.day })}{" "}
            {out.company.latentRisks ? t("Quedan {n} riesgo(s) latente(s) en el entorno…", { n: out.company.latentRisks }) : t("Ningún riesgo latente nuevo.")}
          </p>
          <a href="/company">{t("Volver a la empresa →")}</a>
        </section>
      )}
    </div>
  );
}


function Workspace({ session }: { session: SessionInfo }) {
  const { t: tr } = useI18n();
  const [tick, setTick] = useState(0);
  const tutorAuto = qs("tutor") === "1";
  const [panel, setPanel] = useState<"guide" | "desk" | "submit" | "tutor">(tutorAuto ? "tutor" : "guide");
  const [hints, setHints] = useState<any[]>([]);
  const [check, setCheck] = useState<any>(null);
  const [evidence, setEvidence] = useState<Record<string, string>>({});
  const [answers, setAnswers] = useState<Record<string, number[]>>({});
  const [just, setJust] = useState<Record<string, string>>({});
  const [out, setOut] = useState<any>(null);
  const [err, setErr] = useState("");
  const [busy, setBusy] = useState(false);
  const [desk, setDesk] = useState<any>(null);
  const [screenReader, setScreenReader] = useState(false);
  const [asideOpen, setAsideOpen] = useState(true);
  const term = useRef<TerminalHandle | null>(null);
  useEffect(() => {
    try {
      setScreenReader(localStorage.getItem("gcplab.screenReader") === "1");
      setAsideOpen(localStorage.getItem("gcplab.aside") !== "closed");
    } catch {
      /* storage unavailable */
    }
  }, []);
  const toggleScreenReader = () => {
    const next = !screenReader;
    setScreenReader(next);
    try {
      localStorage.setItem("gcplab.screenReader", next ? "1" : "0");
    } catch {
      /* storage unavailable */
    }
  };
  const onAside = (open: boolean) => {
    setAsideOpen(open);
    try {
      localStorage.setItem("gcplab.aside", open ? "open" : "closed");
    } catch {
      /* storage unavailable */
    }
  };

  useEffect(() => {
    api(`/api/sessions/${session.id}/views/desk`)
      .then((d) => {
        if (d?.ticket || d?.actors?.length) {
          setDesk(d);
        }
      })
      .catch(() => {});
  }, [session.id, tick]);
  const hasDesk = !!desk;
  useEffect(() => {
    if (hasDesk && !tutorAuto) setPanel((p) => (p === "guide" ? "desk" : p));
  }, [hasDesk]);

  const onCommand = useCallback(() => setTick((t) => t + 1), []);

  const hint = async () => {
    try {
      const h = await api(`/api/sessions/${session.id}/hint`, { body: {} });
      setHints((x) => [...x, h]);
    } catch (e: any) {
      setErr(e.message);
    }
  };
  const doCheck = async () => {
    setBusy(true);
    try {
      setCheck(await api(`/api/sessions/${session.id}/check`, { body: {} }));
    } catch (e: any) {
      setErr(e.message);
    } finally {
      setBusy(false);
    }
  };
  const submit = async () => {
    if (!confirm(tr("¿Enviar para evaluar? El entorno se cerrará."))) return;
    setBusy(true);
    setErr("");
    try {
      setOut(await api(`/api/sessions/${session.id}/submit`, { body: { evidence, answers, justifications: just } }));
    } catch (e: any) {
      setErr(e.message);
    } finally {
      setBusy(false);
    }
  };
  const stop = async () => {
    if (!confirm(tr("¿Abandonar este laboratorio?"))) return;
    await api(`/api/sessions/${session.id}/stop`, { body: {} }).catch(() => {});
    window.location.href = "/dashboard";
  };

  if (out) {
    return (
      <>
        <Nav />
        <main id="main" className="page" style={{ maxWidth: 1100 }}>
          <Result out={out} session={session} />
        </main>
      </>
    );
  }

  const tabs: ["guide" | "desk" | "submit" | "tutor", string][] = [
    ...(hasDesk ? ([["desk", tr("Ticket")]] as ["desk", string][]) : []),
    ...(session.type !== "boss" ? ([["tutor", tr("Profesor")]] as ["tutor", string][]) : []),
    ["guide", tr("Instrucciones")],
    ["submit", tr("Entrega")],
  ];

  const aside = (
    <div className="lp">
      <div className="lp-head">
        <div className="lp-meta">
          <span className="cc-pill">{session.fidelity}</span>
          {session.mode && <span className="cc-pill warn">{label(labMode, session.mode, tr)}</span>}
          <span className="lp-timer" title={tr("La sesión caduca a las {time}.", { time: new Date(session.expires).toLocaleTimeString() })}>
            <Icon name="timer" size={16} /> {new Date(session.expires).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" })}
          </span>
          <button type="button" className="cc-icon-btn" aria-label={tr("Ocultar las instrucciones")} onClick={() => onAside(false)}>
            <Icon name="close" size={18} />
          </button>
        </div>
        <h2 className="lp-title">{session.title}</h2>
        <div className="lp-tabs" role="tablist" aria-label={tr("Panel del laboratorio")}>
          {tabs.map(([id, name]) => (
            <button key={id} id={`lp-tab-${id}`} role="tab" aria-selected={panel === id} aria-controls="lp-body" className={panel === id ? "on" : ""} onClick={() => setPanel(id)}>
              {name}
            </button>
          ))}
        </div>
      </div>
      <div className="lp-body" id="lp-body" role="tabpanel" aria-labelledby={`lp-tab-${panel}`}>
        {panel === "desk" && desk && <Desk data={desk} sessionId={session.id} onChange={onCommand} />}
        {panel === "tutor" && (
          <Tutor
            sessionId={session.id}
            tick={tick}
            auto={tutorAuto}
            labType={session.type}
            run={(cmd, note) => term.current?.run(cmd, note)}
            paste={(cmd) => term.current?.paste(cmd)}
            onPanel={(id) => setPanel(id)}
            onCheck={() => {
              setPanel("guide");
              doCheck();
            }}
          />
        )}
        {panel === "guide" && (
          <>
            <StepGuide compact />
            <Markdown text={session.story} />
            {session.timeline?.length ? (
              <>
                <h3>{tr("Cronología")}</h3>
                {session.timeline.map((t, i) => (
                  <div key={i} className="small"><span className="muted">{t.at}</span> <strong>{t.from}:</strong> {t.text}</div>
                ))}
              </>
            ) : null}
            {session.objectives?.length ? (
              <>
                <h3>{tr("Objetivos")}</h3>
                <Checklist items={session.objectives} storeKey={`gcplab.obj.${session.id}`} />
              </>
            ) : null}
            {session.constraints?.length ? (
              <>
                <h3>{tr("Restricciones")}</h3>
                <ul className="lp-list">{session.constraints.map((o) => <li key={o}>{constraint(o, tr)}</li>)}</ul>
              </>
            ) : null}
            {session.instructions && (
              <details>
                <summary><strong>{tr("Instrucciones")}</strong></summary>
                <Markdown text={session.instructions} />
              </details>
            )}
            <div className="lp-note">
              <Icon name="info" size={18} />
              <span>
                {tr("Trabaja en la consola o en Cloud Shell: cada botón de la consola ejecuta su comando abajo. Prueba")} <code>why &lt;{tr("recurso")}&gt;</code>, <code>whatif …</code> {tr("y")} <code>help</code>.
              </span>
            </div>
            {hints.map((h) => (
              <div key={h.index} className="lp-hint">
                <Icon name="bulb" size={18} />
                <span><span className="cc-pill">{label(hintKind, h.kind, tr)} · −{h.cost}</span> {h.text}</span>
              </div>
            ))}
            <div className="lp-actions">
              <button className="cc-btn" onClick={hint} disabled={session.hintCount === 0 || (hints.length > 0 && hints[hints.length - 1].remaining <= 0)}>
                <Icon name="bulb" size={18} /> {tr("Pista (quedan {n})", { n: Math.max(0, session.hintCount - hints.length) })}
              </button>
              <button className="cc-btn" onClick={doCheck} disabled={busy}>
                <Icon name="check" size={18} /> {tr("Comprobar")}
              </button>
            </div>
            {check && (
              <div className="lp-check">
                {check.items?.map((it: any) => (
                  <div key={it.name} className="lp-check-item">
                    <span className={`cc-status ${it.earned >= it.points ? "ok" : "off"}`} aria-hidden="true">{it.earned >= it.points ? "✓" : ""}</span>
                    <span>
                      {it.name} <span className="muted">({Math.round(it.earned * 10) / 10}/{it.points})</span>
                      {it.failing?.map((f: string, i: number) => <div key={i} className="cc-help">– {f}</div>)}
                    </span>
                  </div>
                ))}
                {check.mentor?.length ? <ul className="lp-list">{check.mentor.map((m: string, i: number) => <li key={i}>{m}</li>)}</ul> : null}
              </div>
            )}
          </>
        )}
        {panel === "submit" && (
          <>
            {session.evidence && (
              <>
                <h3>{tr("Evidencias")}</h3>
                <p className="cc-help">{session.evidence.prompt}</p>
                {session.evidence.fields.map((f) => (
                  <div key={f} className="cc-field" style={{ marginBottom: 12 }}>
                    <label htmlFor={`ev-${f}`}>{label(evidenceField, f, tr)}</label>
                    <textarea id={`ev-${f}`} value={evidence[f] ?? ""} onChange={(e) => setEvidence({ ...evidence, [f]: e.target.value })} />
                  </div>
                ))}
              </>
            )}
            {session.quiz?.filter((q) => !q.after).length ? (
              <>
                <h3>{tr("Preguntas")}</h3>
                {session.quiz.filter((q) => !q.after).map((q) => (
                  <fieldset key={q.id} className="lp-quiz">
                    <legend>{q.question}</legend>
                    {q.options.map((o, i) => (
                      <label key={i} className="cc-check">
                        <input
                          type="checkbox"
                          checked={answers[q.id]?.includes(i) ?? false}
                          onChange={(e) => {
                            const cur = answers[q.id] ?? [];
                            setAnswers({ ...answers, [q.id]: e.target.checked ? [...cur, i] : cur.filter((x) => x !== i) });
                          }}
                        />
                        <span>{o}</span>
                      </label>
                    ))}
                    {q.justify && <textarea placeholder={tr("Justifica tu elección")} aria-label={tr("Justifica tu elección")} value={just[q.id] ?? ""} onChange={(e) => setJust({ ...just, [q.id]: e.target.value })} />}
                  </fieldset>
                ))}
              </>
            ) : null}
            {!session.evidence && !session.quiz?.filter((q) => !q.after).length && <p className="cc-help">{tr("Cuando termines, envía el laboratorio para que se evalúe.")}</p>}
            {err && <p className="error">{err}</p>}
            <div className="lp-actions">
              <button className="cc-btn primary" onClick={submit} disabled={busy}>{tr("Enviar")}</button>
              <button className="cc-btn danger" onClick={stop}>{tr("Abandonar")}</button>
            </div>
            <label className="cc-check" style={{ marginTop: 16 }}>
              <input type="checkbox" checked={screenReader} onChange={toggleScreenReader} />
              <span>{tr("Modo lector de pantalla para la terminal")}</span>
            </label>
          </>
        )}
        {err && panel !== "submit" && <p className="error">{err}</p>}
      </div>
    </div>
  );

  return (
    <>
      <a className="skip-link" href="#main">{tr("Saltar al contenido")}</a>
      <CloudConsole
        sessionId={session.id}
        project={session.project}
        region={session.region || "europe-west1"}
        zone={session.zone || "europe-west1-b"}
        tick={tick}
        run={(cmd, note) => term.current?.run(cmd, note) ?? Promise.resolve({ output: tr("La terminal no está lista."), exit: 1 })}
        paste={(cmd) => term.current?.paste(cmd)}
        focusShell={() => term.current?.focus()}
        aside={aside}
        asideOpen={asideOpen}
        onAside={onAside}
        terminal={
          <Terminal
            sessionId={session.id}
            onCommand={onCommand}
            screenReader={screenReader}
            controller={term}
            prompt={`student@cloudshell:~ (${session.project})$ `}
            banner={`\x1b[1mWelcome to Cloud Shell! Type "help" to get started.\x1b[0m\r\n${tr("Tu proyecto de Cloud Platform en esta sesión es")} \x1b[1;33m${session.project}\x1b[0m.\r\n${tr("Lo que hagas en la consola de arriba también se ejecuta aquí.")}`}
          />
        }
        editor={<ShellEditor sessionId={session.id} tick={tick} />}
      />
      <Tour />
    </>
  );
}

function LabPage() {
  useAuth();
  const { t } = useI18n();
  const [session, setSession] = useState<SessionInfo | null>(null);
  const [labId, setLabId] = useState("");
  const [err, setErr] = useState("");
  useEffect(() => {
    const sid = qs("session");
    setLabId(qs("id"));
    if (sid) {
      api<SessionInfo>(`/api/sessions/${sid}`)
        .then((s) => {
          if (s.status && s.status !== "running" && s.status !== "ready") setErr(t("Esta sesión está en estado «{status}».", { status: s.status }));
          else setSession(s);
        })
        .catch((e) => setErr(e.message));
    }
  }, []);
  const started = (s: SessionInfo, tutor?: boolean) => {
    window.history.replaceState(null, "", `/lab?session=${s.id}&id=${encodeURIComponent(s.labId)}${tutor ? "&tutor=1" : ""}`);
    setSession(s);
  };
  if (session) return <Workspace session={session} />;
  return (
    <>
      <Nav />
      {(
        <main id="main" className="page">
          {err && <p className="error">{err}</p>}
          {labId && !qs("session") ? <Briefing labId={labId} onStarted={started} /> : !err && <p className="muted">{t("Cargando…")}</p>}
          {err && labId && <a href={`/lab?id=${encodeURIComponent(labId)}`}>{t("Empezar un intento nuevo →")}</a>}
        </main>
      )}
    </>
  );
}

export default function Page() {
  return (
    <Suspense>
      <LabPage />
    </Suspense>
  );
}
