"use client";

import { Suspense, useCallback, useEffect, useRef, useState } from "react";
import Nav from "@/components/Nav";
import Terminal from "@/components/Terminal";
import Mermaid from "@/components/Mermaid";
import CodeEditor from "@/components/CodeEditor";
import Markdown from "@/components/Markdown";
import JsonTree from "@/components/JsonTree";
import Sparkline from "@/components/Sparkline";
import Bar from "@/components/Bar";
import { api, qs } from "@/lib/api";
import { useAuth } from "@/components/useAuth";
import { useI18n } from "@/lib/i18n";
import { constraint, evidenceField, factor, hintKind, label, labLevel, labMode, labType, role, tab as tabLabel, ticketStatus, validator } from "@/lib/labels";
import type { SessionInfo } from "@/lib/types";

const TABS = ["desk", "console", "topology", "logs", "metrics", "iam", "cost", "files", "history"] as const;
type Tab = (typeof TABS)[number];

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

function Logs({ sessionId, tick }: { sessionId: string; tick: number }) {
  const { t } = useI18n();
  const [filter, setFilter] = useState("");
  const [applied, setApplied] = useState("");
  const [rows, setRows] = useState<any[]>([]);
  const [err, setErr] = useState("");
  useEffect(() => {
    api<any[]>(`/api/sessions/${sessionId}/views/logs?limit=200&filter=${encodeURIComponent(applied)}`)
      .then((r) => {
        setRows(r ?? []);
        setErr("");
      })
      .catch((e) => setErr(e.message));
  }, [sessionId, applied, tick]);
  return (
    <div className="col">
      <form className="row" onSubmit={(e) => { e.preventDefault(); setApplied(filter); }}>
        <input value={filter} onChange={(e) => setFilter(e.target.value)} placeholder={t('Consulta de registros, p. ej. severity>=ERROR resource.type="cloud_run_revision"')} aria-label={t("Consulta de registros")} style={{ flex: 1, width: "auto" }} />
        <button className="btn secondary">{t("Ejecutar consulta")}</button>
      </form>
      {err && <p className="error small">{err}</p>}
      <table>
        <thead><tr><th>{t("Hora")}</th><th>{t("Gravedad")}</th><th>{t("Recurso")}</th><th>{t("Mensaje")}</th></tr></thead>
        <tbody>
          {rows.map((r, i) => (
            <tr key={i}>
              <td className="small muted" style={{ whiteSpace: "nowrap" }}>{r.timestamp?.replace("T", " ").replace(/\..*|Z$/, "")}</td>
              <td><span className={`pill ${/ERROR|CRITICAL|ALERT|EMERGENCY/.test(r.severity) ? "bad" : r.severity === "WARNING" ? "warn" : ""}`}>{r.severity}</span></td>
              <td className="small">{r.resource?.type}<div className="muted">{Object.values(r.resource?.labels ?? {}).join(" ")}</div></td>
              <td className="small" style={{ fontFamily: "var(--mono)", wordBreak: "break-word" }}>
                {r.textPayload ?? (r.httpRequest ? `${r.httpRequest.requestMethod ?? ""} ${r.httpRequest.requestUrl ?? ""} → ${r.httpRequest.status ?? ""}` : r.protoPayload ? `${r.protoPayload.methodName ?? ""} ${r.protoPayload.resourceName ?? ""} ${t("por")} ${r.protoPayload["authenticationInfo.principalEmail"] ?? "?"}` : JSON.stringify(r.labels ?? {}))}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
      {!rows.length && <p className="muted">{t("Ninguna entrada coincide.")}</p>}
    </div>
  );
}

function Files({ sessionId, tick }: { sessionId: string; tick: number }) {
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
    <div style={{ display: "grid", gridTemplateColumns: "200px 1fr", gap: 10, height: "100%", minHeight: 300 }}>
      <div className="col small" style={{ overflow: "auto" }}>
        {Object.keys(files).sort().map((p) => (
          <a key={p} href="#" onClick={(e) => { e.preventDefault(); open(p); }} style={{ fontWeight: p === sel ? 600 : 400, wordBreak: "break-all" }}>
            {p}
          </a>
        ))}
        <form onSubmit={(e) => { e.preventDefault(); if (newName) { setFiles({ ...files, [newName]: "" }); open(newName); setNewName(""); } }}>
          <input placeholder={t("archivo nuevo (p. ej. main.tf)")} aria-label={t("Nombre del archivo nuevo")} value={newName} onChange={(e) => setNewName(e.target.value)} />
        </form>
      </div>
      <div style={{ display: "grid", gridTemplateRows: "auto 1fr", minHeight: 300 }}>
        {sel ? (
          <>
            <div className="row small" style={{ marginBottom: 6 }}>
              <strong>{sel}</strong>
              <button className="btn secondary" onClick={save} disabled={!dirty}>{t("Guardar")}</button>
              <span className="muted">{msg}</span>
            </div>
            <div style={{ border: "1px solid var(--border)", borderRadius: 8, overflow: "hidden", minHeight: 280 }}>
              <CodeEditor path={sel} value={draft} onChange={(v) => { setDraft(v); setDirty(true); }} />
            </div>
          </>
        ) : (
          <p className="muted">
            {t("Elige un archivo o crea uno. Los archivos se comparten con la terminal")} (<code>cat</code>, <code>terraform apply</code>, <code>kubectl apply -f</code>, <code>docker build</code>…).
          </p>
        )}
      </div>
    </div>
  );
}

function View({ sessionId, kind, tick }: { sessionId: string; kind: Tab; tick: number }) {
  const { t } = useI18n();
  // Data is tagged with the view it belongs to so a tab switch never renders
  // one view's payload with another view's renderer.
  const [loaded, setLoaded] = useState<{ kind: Tab; data: any } | null>(null);
  const [err, setErr] = useState("");
  useEffect(() => {
    if (kind === "logs" || kind === "files") return;
    let live = true;
    api(`/api/sessions/${sessionId}/views/${kind}`)
      .then((d) => {
        if (!live) return;
        setLoaded({ kind, data: d });
        setErr("");
      })
      .catch((e) => live && setErr(e.message));
    return () => {
      live = false;
    };
  }, [sessionId, kind, tick]);
  if (kind === "logs") return <Logs sessionId={sessionId} tick={tick} />;
  if (kind === "files") return <Files sessionId={sessionId} tick={tick} />;
  if (err) return <p className="error small">{err}</p>;
  if (!loaded || loaded.kind !== kind) return <p className="muted">{t("Cargando…")}</p>;
  const data = loaded.data;
  if (data == null) return <p className="muted">{t("Aún no hay nada aquí.")}</p>;
  switch (kind) {
    case "desk":
      return <Desk data={data} />;
    case "topology":
      return data.mermaid ? <Mermaid chart={data.mermaid} /> : <p className="muted">{t("Todavía no hay recursos.")}</p>;
    case "metrics": {
      const keys = Object.keys(data);
      if (!keys.length) return <p className="muted">{t("Aún no hay métricas. Aparecen a medida que avanza el tiempo simulado.")}</p>;
      return (
        <table>
          <tbody>
            {keys.map((k) => {
              const pts = data[k] ?? [];
              return (
                <tr key={k}>
                  <td className="small" style={{ fontFamily: "var(--mono)", wordBreak: "break-all" }}>{k}</td>
                  <td><Sparkline points={pts.slice(-60)} /></td>
                  <td className="small">{pts.length ? Math.round(pts[pts.length - 1].v * 100) / 100 : ""}</td>
                </tr>
              );
            })}
          </tbody>
        </table>
      );
    }
    case "cost":
      return (
        <div>
          <div className="kpi">€{(data.monthlyEur ?? 0).toFixed(2)} <span className="muted small">{t("/ mes (estimación)")}</span></div>
          <table>
            <thead><tr><th>{t("Recurso")}</th><th>SKU</th><th>{t("€/mes")}</th></tr></thead>
            <tbody>
              {(data.lines ?? []).map((l: any, i: number) => (
                <tr key={i}><td>{l.resource}</td><td className="small muted">{l.sku}</td><td>{l.monthlyEur.toFixed(2)}</td></tr>
              ))}
            </tbody>
          </table>
        </div>
      );
    case "history":
      return (
        <table>
          <caption className="sr-only">{t("Comandos que has ejecutado, con su salida")}</caption>
          <thead><tr><th>{t("Hora")}</th><th>{t("Comando")}</th><th>{t("Código")}</th></tr></thead>
          <tbody>
            {(Array.isArray(data) ? data : []).map((r: any, i: number) => (
              <tr key={i}>
                <td className="small muted" style={{ whiteSpace: "nowrap" }}>{String(r.at ?? "").replace("T", " ").replace(/Z$/, "")}</td>
                <td className="small" style={{ fontFamily: "var(--mono)", whiteSpace: "pre-wrap" }}>
                  {r.line}
                  {r.output ? (
                    <details>
                      <summary className="muted">{t("salida")}</summary>
                      <pre style={{ margin: 0 }}>{r.output}</pre>
                    </details>
                  ) : null}
                </td>
                <td><span className={`pill ${r.exit ? "bad" : "ok"}`}>{r.exit}</span></td>
              </tr>
            ))}
          </tbody>
        </table>
      );
    default:
      return <JsonTree data={data} />;
  }
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
  const [tab, setTab] = useState<Tab>("console");
  const [tick, setTick] = useState(0);
  const [hints, setHints] = useState<any[]>([]);
  const [check, setCheck] = useState<any>(null);
  const [evidence, setEvidence] = useState<Record<string, string>>({});
  const [answers, setAnswers] = useState<Record<string, number[]>>({});
  const [just, setJust] = useState<Record<string, string>>({});
  const [out, setOut] = useState<any>(null);
  const [err, setErr] = useState("");
  const [busy, setBusy] = useState(false);
  const [hasDesk, setHasDesk] = useState(false);
  const [screenReader, setScreenReader] = useState(false);
  useEffect(() => {
    try {
      setScreenReader(localStorage.getItem("gcplab.screenReader") === "1");
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

  useEffect(() => {
    api(`/api/sessions/${session.id}/views/desk`)
      .then((d) => {
        if (d?.ticket || d?.actors?.length) {
          setHasDesk(true);
          setTab("desk");
        }
      })
      .catch(() => {});
  }, [session.id]);

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
      <main id="main" className="page" style={{ maxWidth: 900 }}>
        <Result out={out} />
      </main>
    );
  }

  const tabs = TABS.filter((t) => t !== "desk" || hasDesk);
  return (
    <main id="main" className="workspace">
      <aside className="side card" aria-label={tr("Enunciado y entrega")}>
        <div className="row small muted">
          <span className="pill">{session.fidelity}</span>
          {session.mode && <span className="pill warn">{label(labMode, session.mode, tr)}</span>}
          <span>{tr("proyecto")} <code>{session.project}</code></span>
        </div>
        <h2>{session.title}</h2>
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
            <ul className="small">{session.objectives.map((o) => <li key={o}>{o}</li>)}</ul>
          </>
        ) : null}
        {session.constraints?.length ? (
          <>
            <h3>{tr("Restricciones")}</h3>
            <ul className="small">{session.constraints.map((o) => <li key={o}>{constraint(o, tr)}</li>)}</ul>
          </>
        ) : null}
        {session.instructions && (
          <details>
            <summary><strong>{tr("Instrucciones")}</strong></summary>
            <Markdown text={session.instructions} />
          </details>
        )}
        <h3>{tr("Ayuda")}</h3>
        <p className="small muted">
          {tr("Prueba")} <code>why &lt;{tr("recurso")}&gt;</code> {tr("para ver la cadena causal,")} <code>whatif …</code> {tr("para explorar consecuencias y")} <code>help</code> {tr("para ver los comandos.")}
        </p>
        {hints.map((h) => (
          <div key={h.index} className="small card" style={{ padding: 8, marginBottom: 6 }}>
            <span className="pill">{label(hintKind, h.kind, tr)} · −{h.cost}</span> {h.text}
          </div>
        ))}
        <div className="row">
          <button className="btn secondary" onClick={hint} disabled={session.hintCount === 0 || (hints.length > 0 && hints[hints.length - 1].remaining <= 0)}>
            {tr("Pista (quedan {n})", { n: Math.max(0, session.hintCount - hints.length) })}
          </button>
          <button className="btn secondary" onClick={doCheck} disabled={busy}>{tr("Comprobar")}</button>
        </div>
        {check && (
          <div className="small" style={{ marginTop: 8 }}>
            {check.items?.map((it: any) => (
              <div key={it.name}>
                {it.earned >= it.points ? "✓" : "○"} {it.name} <span className="muted">({Math.round(it.earned * 10) / 10}/{it.points})</span>
                {it.failing?.map((f: string, i: number) => <div key={i} className="muted" style={{ marginLeft: 16 }}>– {f}</div>)}
              </div>
            ))}
            {check.mentor?.length ? <ul>{check.mentor.map((m: string, i: number) => <li key={i}>{m}</li>)}</ul> : null}
          </div>
        )}
        {session.evidence && (
          <>
            <h3>{tr("Evidencias")}</h3>
            <p className="small muted">{session.evidence.prompt}</p>
            {session.evidence.fields.map((f) => (
              <div key={f}>
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
              <div key={q.id} style={{ marginBottom: 10 }}>
                <div className="small"><strong>{q.question}</strong></div>
                {q.options.map((o, i) => (
                  <label key={i} className="small" style={{ display: "flex", gap: 6, color: "var(--text)" }}>
                    <input
                      type="checkbox"
                      style={{ width: "auto" }}
                      checked={answers[q.id]?.includes(i) ?? false}
                      onChange={(e) => {
                        const cur = answers[q.id] ?? [];
                        setAnswers({ ...answers, [q.id]: e.target.checked ? [...cur, i] : cur.filter((x) => x !== i) });
                      }}
                    />
                    {o}
                  </label>
                ))}
                {q.justify && <textarea placeholder={tr("Justifica tu elección")} aria-label={tr("Justifica tu elección")} value={just[q.id] ?? ""} onChange={(e) => setJust({ ...just, [q.id]: e.target.value })} />}
              </div>
            ))}
          </>
        ) : null}
        {err && <p className="error small">{err}</p>}
        <div className="row" style={{ marginTop: 12 }}>
          <button className="btn" onClick={submit} disabled={busy}>{tr("Enviar")}</button>
          <button className="btn danger" onClick={stop}>{tr("Abandonar")}</button>
        </div>
        <p className="small muted">{tr("La sesión caduca a las {time}.", { time: new Date(session.expires).toLocaleTimeString() })}</p>
        <label className="small" style={{ display: "flex", gap: 6, alignItems: "center", color: "var(--text)" }}>
          <input type="checkbox" style={{ width: "auto" }} checked={screenReader} onChange={toggleScreenReader} />
          {tr("Modo lector de pantalla para la terminal")}
        </label>
      </aside>
      <section className="main">
        <div className="term">
          <Terminal sessionId={session.id} onCommand={onCommand} screenReader={screenReader} banner={`\x1b[36mCloud Shell — ${tr("proyecto")} ${session.project} (${session.fidelity})\x1b[0m\r\n${tr("Escribe 'help' para ver los comandos.")}`} />
        </div>
        <div className="card tabpanel" style={{ padding: "8px 12px" }}>
          <div className="row" style={{ gap: 0, flexWrap: "nowrap", alignItems: "stretch" }}>
          <div className="tabs" role="tablist" aria-label={tr("Vistas del entorno")} style={{ flex: 1 }}>
            {tabs.map((t) => (
              <button
                key={t}
                id={`tab-${t}`}
                role="tab"
                aria-selected={tab === t}
                aria-controls="view-panel"
                tabIndex={tab === t ? 0 : -1}
                className={tab === t ? "active" : ""}
                onClick={() => setTab(t)}
                onKeyDown={(e) => {
                  const i = tabs.indexOf(t);
                  const next = e.key === "ArrowRight" ? tabs[(i + 1) % tabs.length] : e.key === "ArrowLeft" ? tabs[(i - 1 + tabs.length) % tabs.length] : null;
                  if (next) {
                    e.preventDefault();
                    setTab(next);
                    document.getElementById(`tab-${next}`)?.focus();
                  }
                }}
                style={{ textTransform: "capitalize" }}
              >
                {label(tabLabel, t, tr)}
              </button>
            ))}
          </div>
          <button className="btn secondary" onClick={() => setTick((x) => x + 1)} title={tr("Actualizar vistas")} aria-label={tr("Actualizar vistas")} style={{ border: "none", borderBottom: "1px solid var(--border)", borderRadius: 0 }}>
            ↻
          </button>
          </div>
          <div className="panel-body" id="view-panel" role="tabpanel" aria-labelledby={`tab-${tab}`} aria-live="polite">
            <View sessionId={session.id} kind={tab} tick={tick} />
          </div>
        </div>
      </section>
    </main>
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
  return (
    <>
      <Nav />
      {session ? (
        <Workspace session={session} />
      ) : (
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
