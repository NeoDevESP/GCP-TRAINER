"use client";

import React, { Suspense, useCallback, useEffect, useRef, useState } from "react";
import Nav from "@/components/Nav";
import Terminal, { type TerminalHandle } from "@/components/Terminal";
import CloudConsole from "@/components/console/CloudConsole";
import CodeEditor from "@/components/CodeEditor";
import Markdown from "@/components/Markdown";
import Bar from "@/components/Bar";
import { api, qs } from "@/lib/api";
import { useAuth } from "@/components/useAuth";
import { useI18n } from "@/lib/i18n";
import { constraint, evidenceField, factor, hintKind, label, labLevel, labMode, labType, role, ticketStatus, validator } from "@/lib/labels";
import { Icon } from "@/components/console/icons";
import type { SessionInfo } from "@/lib/types";


function Briefing({ labId, onStarted }: { labId: string; onStarted: (s: SessionInfo) => void }) {
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
  const start = async () => {
    setBusy(true);
    setErr("");
    try {
      const s = await api<SessionInfo>(`/api/labs/${encodeURIComponent(labId)}/start`, { body: { fidelity } });
      onStarted(s);
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
        <span className="pill">{label(labType, lab.type, t)}</span>
        <span className="pill">{label(labLevel, lab.level, t)}</span>
        <span>{lab.minutes} min</span>
      </div>
      <h1>{lab.title}</h1>
      <Markdown text={lab.story || lab.summary} />
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
        <>
          <h3>{t("Cómo se te evaluará")}</h3>
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
        </>
      ) : null}
      <div className="row" style={{ marginTop: 16 }}>
        <label htmlFor="fid" style={{ margin: 0 }}>{t("Fidelidad")}</label>
        <select id="fid" value={fidelity} onChange={(e) => setFidelity(e.target.value)} style={{ width: 200 }}>
          {(lab.fidelity ?? ["F0"]).map((f: string) => (
            <option key={f} value={f}>
              {f === "F0" ? t("F0 — simulador") : f === "F1" ? t("F1 — emuladores") : t("F2 — proyecto real de GCP")}
            </option>
          ))}
        </select>
        <button className="btn" onClick={start} disabled={busy}>
          {busy ? t("Preparando el entorno…") : t("Empezar laboratorio")}
        </button>
      </div>
      {err && <p className="error">{err}</p>}
    </div>
  );
}

function Desk({ data }: { data: any }) {
  const { t: tr } = useI18n();
  if (!data?.ticket && !data?.actors?.length) return <p className="muted">{tr("Este laboratorio no tiene ticket. Usa la terminal y los objetivos de la izquierda.")}</p>;
  const t = data.ticket;
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
            <div key={i} className="small" style={{ borderTop: "1px solid var(--border)", padding: "6px 0" }}>
              <strong>{c.from}</strong> <span className="muted">{c.at}{c.public ? ` · ${tr("público")}` : ""}</span>
              <div style={{ whiteSpace: "pre-wrap" }}>{c.text}</div>
            </div>
          ))}
          {t.attachments?.map((a: any) => (
            <details key={a.name}>
              <summary className="small">📎 {a.name}</summary>
              <pre>{a.content}</pre>
            </details>
          ))}
          <p className="small muted">
            {tr("Trabaja el ticket desde la terminal:")} <code>ticket show</code>, <code>ticket comment &quot;…&quot;</code>, <code>ticket resolve &quot;…&quot;</code>, <code>team</code>, <code>ask &lt;{tr("nombre")}&gt; &quot;{tr("pregunta")}&quot;</code>.
          </p>
        </div>
      )}
      {data.actors?.length ? (
        <div className="card">
          <h3>{tr("Personas")}</h3>
          <table>
            <tbody>
              {data.actors.map((a: any) => (
                <tr key={a.name}>
                  <td><strong>{a.name}</strong></td>
                  <td className="muted small">{label(role, a.role, tr)}</td>
                  <td className="small">{a.persona}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      ) : null}
      {data.questions?.length ? (
        <div className="card">
          <h3>{tr("Conversaciones")}</h3>
          {data.questions.map((q: any, i: number) => (
            <div key={i} className="small" style={{ marginBottom: 6 }}>
              <strong>{tr("Tú → {who}:", { who: q.actor })}</strong> {q.question}
              <div className="muted">{q.answer}</div>
            </div>
          ))}
        </div>
      ) : null}
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

function Result({ out }: { out: any }) {
  const { t } = useI18n();
  const r = out.result;
  return (
    <div className="card" style={{ borderColor: r.passed ? "var(--ok)" : "var(--bad)" }}>
      <h2>{r.passed ? t("Superado") : t("Aún no superado")} — {r.score}/{r.max}</h2>
      {r.criticalFailed && <p className="error">{t("Ha fallado un criterio crítico.")}</p>}
      <p className="small">+{out.xp} XP{out.bonus ? ` (+${out.bonus} ${t("de bonificación")}: ${out.bonusReasons?.join(", ")})` : ""}</p>
      <table>
        <tbody>
          {r.items.map((it: any) => (
            <tr key={it.name}>
              <td>
                {it.name} {it.critical && <span className="pill">{t("crítico")}</span>}
                {it.checks?.filter((c: any) => !c.pass && c.desc).map((c: any, i: number) => (
                  <div key={i} className="small error">✗ {c.desc}</div>
                ))}
              </td>
              <td style={{ width: 90 }}>{Math.round(it.earned * 10) / 10}/{it.points}</td>
            </tr>
          ))}
        </tbody>
      </table>
      {r.process && (
        <>
          <h3>{t("Cómo has trabajado")}</h3>
          <table>
            <tbody>
              {r.process.factors.map((f: any) => (
                <tr key={f.name}>
                  <td style={{ width: 140 }}>{label(factor, f.name, t)}</td>
                  <td><Bar value={f.score * 100} label={label(factor, f.name, t)} /></td>
                  <td className="small muted">{f.signals?.join("; ")}</td>
                </tr>
              ))}
            </tbody>
          </table>
          {r.process.blindFixes?.length ? <p className="small warn">{t("Cambios antes de reunir evidencias:")} {r.process.blindFixes.join(", ")}</p> : null}
          {r.process.constraintViolations?.length ? <p className="small error">{t("Restricciones incumplidas:")} {r.process.constraintViolations.join(", ")}</p> : null}
        </>
      )}
      {r.feedback?.length ? <ul className="small">{r.feedback.map((f: string, i: number) => <li key={i}>{f}</li>)}</ul> : null}
      {out.mentor?.length ? (
        <>
          <h3>{t("Mentor")}</h3>
          <ul className="small">{out.mentor.map((m: string, i: number) => <li key={i}>{m}</li>)}</ul>
        </>
      ) : null}
      {out.postmortemReview && (
        <>
          <h3>{t("Revisión del post-mortem")}</h3>
          <Markdown text={out.postmortemReview} />
        </>
      )}
      {out.company && (
        <>
          <h3>Nebula Corporation</h3>
          <p className="small">
            {t("Día {n}.", { n: out.company.day })}{" "}
            {out.company.latentRisks ? t("Quedan {n} riesgo(s) latente(s) en el entorno…", { n: out.company.latentRisks }) : t("Ningún riesgo latente nuevo.")}
          </p>
          <a href="/company">{t("Volver a la empresa →")}</a>
        </>
      )}
      <div className="row" style={{ marginTop: 10 }}>
        <a className="btn secondary" href="/dashboard">{t("Panel")}</a>
        <a className="btn secondary" href="/catalog">{t("Más laboratorios")}</a>
      </div>
    </div>
  );
}


function Workspace({ session }: { session: SessionInfo }) {
  const { t: tr } = useI18n();
  const [tick, setTick] = useState(0);
  const [panel, setPanel] = useState<"guide" | "desk" | "submit">("guide");
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
    if (hasDesk) setPanel((p) => (p === "guide" ? "desk" : p));
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
        <main id="main" className="page" style={{ maxWidth: 900 }}>
          <Result out={out} />
        </main>
      </>
    );
  }

  const tabs: ["guide" | "desk" | "submit", string][] = [
    ...(hasDesk ? ([["desk", tr("Ticket")]] as ["desk", string][]) : []),
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
        {panel === "desk" && desk && <Desk data={desk} />}
        {panel === "guide" && (
          <>
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
                <ul className="lp-list">{session.objectives.map((o) => <li key={o}>{o}</li>)}</ul>
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
            banner={`\x1b[1mWelcome to Cloud Shell! Type "help" to get started.\x1b[0m\r\n${tr("Tu proyecto de Cloud Platform en esta sesión es")} \x1b[1;33m${session.project}\x1b[0m.\r\n${tr("Lo que hagas en la consola de arriba también se ejecuta aquí.")}`}
          />
        }
        editor={<ShellEditor sessionId={session.id} tick={tick} />}
      />
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
  const started = (s: SessionInfo) => {
    window.history.replaceState(null, "", `/lab?session=${s.id}&id=${encodeURIComponent(s.labId)}`);
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
