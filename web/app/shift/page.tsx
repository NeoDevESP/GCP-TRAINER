"use client";

// Support shift: a queue of mixed-priority bank support cases with SLA clocks.
// The learner chooses which ticket to open; the shift score combines the
// grade of each case, the SLAs met and whether urgent tickets went first.
// The shift lives in the learner's account (study runs) so it survives a
// reload or a change of device; case grades come from the lab attempts.

import { SHIFT_PASS, shiftPassed } from "@/lib/model";
import { useEffect, useMemo, useState } from "react";
import Nav from "@/components/Nav";
import { useAuth } from "@/components/useAuth";
import { labHref } from "@/components/learn";
import { api } from "@/lib/api";
import { useI18n } from "@/lib/i18n";
import { fmtTime, loadRun, saveRun, shuffle, syncStudy } from "@/lib/study";
import type { LabSummary } from "@/lib/types";

const KEY = "cm-shift";
// Compressed SLA per priority, in minutes from the start of the shift.
const SLA_MIN: Record<string, number> = { P1: 30, P2: 60, P3: 120, P4: 180 };
const rank = (p: string) => Number(p.replace("P", "")) || 5;
// Which bank the tickets come from.
const POOLS: Record<string, string[]> = { zentra: ["zentra"], atlas: ["support-bank"], all: ["zentra", "support-bank"] };

interface Shift {
  started: number;
  ids: string[];
  opened: string[];
}
interface ShiftResult {
  at: number;
  cases: number;
  solved: number;
  score: number;
  sla: number;
  order: number;
  /** passed the shift (score >= 70); older records lack it and are judged by score */
  passed?: boolean;
}
interface ShiftDoc {
  current?: Shift | null;
  history: ShiftResult[];
}
interface Attempt {
  labId: string;
  started: string;
  finished?: string;
  score: number;
  passed: boolean;
  status?: string;
  sessionId?: string;
  tutor?: boolean;
  solutionShown?: boolean;
}

const helpedAttempt = (a: Attempt) => !!(a.tutor || a.solutionShown);

const loadShifts = () => loadRun<ShiftDoc>(KEY) ?? { history: [] };

// pick draws n cases covering as many priorities as possible.
function pick(labs: LabSummary[], n: number): string[] {
  const pool = shuffle(labs.filter((l) => l.ticket?.priority));
  const out: LabSummary[] = [];
  for (const p of ["P1", "P2", "P3", "P4"]) {
    const l = pool.find((x) => x.ticket?.priority === p && !out.includes(x));
    if (l && out.length < n) out.push(l);
  }
  for (const l of pool) if (out.length < n && !out.includes(l)) out.push(l);
  return shuffle(out).map((l) => l.id);
}

// orderScore is the share of opened pairs where the more urgent ticket came first.
function orderScore(opened: string[], prio: (id: string) => number) {
  let ok = 0;
  let all = 0;
  for (let i = 0; i < opened.length; i++)
    for (let j = i + 1; j < opened.length; j++) {
      all++;
      if (prio(opened[i]) <= prio(opened[j])) ok++;
    }
  return all ? ok / all : 1;
}

export default function ShiftPage() {
  useAuth();
  const { t } = useI18n();
  const [labs, setLabs] = useState<LabSummary[] | null>(null);
  const [atts, setAtts] = useState<Attempt[]>([]);
  const [doc, setDoc] = useState<ShiftDoc>({ history: [] });
  const [size, setSize] = useState(5);
  const [pool, setPool] = useState("zentra");
  const [now, setNow] = useState(Date.now());
  const [err, setErr] = useState("");

  useEffect(() => {
    api<{ labs: LabSummary[] }>("/api/catalog")
      .then((r) => setLabs(r.labs.filter((l) => POOLS.all.includes(l.track) && l.ticket && l.type !== "guided")))
      .catch((e) => setErr(e.message));
    api<Attempt[]>("/api/me/attempts").then((a) => setAtts(a ?? [])).catch(() => {});
    syncStudy()
      .catch(() => {})
      .finally(() => setDoc(loadShifts()));
    const iv = setInterval(() => setNow(Date.now()), 1000);
    return () => clearInterval(iv);
  }, []);

  const byId = useMemo(() => Object.fromEntries((labs ?? []).map((l) => [l.id, l])), [labs]);
  const cur = doc.current ?? null;
  const prio = (id: string) => rank(byId[id]?.ticket?.priority ?? "P4");
  const save = (d: ShiftDoc, now = false) => {
    setDoc(d);
    return saveRun(KEY, d, now);
  };

  // Result of each case of the current shift since it started. A ticket is
  // solved only by a passed hand-in; one passed with the solution or the
  // tutor counts half. A failed hand-in is "tried", never solved.
  const results = useMemo(() => {
    const r: Record<string, { score: number; finished: number; passed: boolean; helped: boolean; tried: boolean; running?: string }> = {};
    if (!cur) return r;
    const rank = (x?: { passed: boolean; helped: boolean; score: number }) => (x ? (x.passed ? (x.helped ? 2 : 3) : 1) * 1000 + x.score : -1);
    for (const a of atts) {
      if (!cur.ids.includes(a.labId) || Date.parse(a.started) < cur.started) continue;
      const prev = r[a.labId];
      if (a.status === "running" && a.sessionId) r[a.labId] = { ...(prev ?? { score: 0, finished: 0, passed: false, helped: false, tried: false }), running: a.sessionId };
      if (!a.finished) continue;
      const cand = { score: a.score, finished: Date.parse(a.finished), passed: !!a.passed, helped: helpedAttempt(a), tried: true };
      if (rank(cand) > rank(r[a.labId]?.tried ? r[a.labId] : undefined)) r[a.labId] = { ...r[a.labId], ...cand };
    }
    return r;
  }, [atts, cur]);

  const deadline = (id: string) => (cur ? cur.started + (SLA_MIN[byId[id]?.ticket?.priority ?? "P4"] ?? 180) * 60000 : 0);
  const solved = (id: string) => !!results[id]?.passed;

  const summary = (s: Shift): ShiftResult => {
    const scores = s.ids.map((id) => (solved(id) ? results[id].score * (results[id].helped ? 0.5 : 1) : 0));
    const done = s.ids.filter(solved);
    const slaOk = done.filter((id) => results[id].finished <= deadline(id)).length;
    const order = orderScore(s.opened, prio);
    const avg = scores.reduce((a, b) => a + b, 0) / Math.max(1, scores.length);
    const sla = slaOk / Math.max(1, s.ids.length);
    const score = Math.round(0.6 * avg + 25 * sla + 15 * order);
    return { at: Date.now(), cases: s.ids.length, solved: done.length, sla: Math.round(sla * 100), order: Math.round(order * 100), score, passed: score >= SHIFT_PASS && done.length > 0 };
  };

  const start = () => labs && save({ ...doc, current: { started: Date.now(), ids: pick(labs.filter((l) => POOLS[pool].includes(l.track)), size), opened: [] } });
  const open = async (id: string) => {
    if (cur && !cur.opened.includes(id)) await save({ ...doc, current: { ...cur, opened: [...cur.opened, id] } }, true);
    // shift tickets run in production mode (hints yes; solution and tutor no)
    window.location.href = labHref(id, results[id]?.running) + (results[id]?.running ? "" : "&mode=production");
  };
  const finish = () => {
    if (!cur) return;
    if (cur.ids.some((id) => !solved(id)) && !confirm(t("Quedan tickets sin resolver; contarán como 0. ¿Terminar el turno?"))) return;
    save({ current: null, history: [summary(cur), ...doc.history].slice(0, 20) });
  };

  const live = cur ? summary(cur) : null;
  const queue = cur ? cur.ids.filter((id) => byId[id]) : [];

  return (
    <>
      <Nav active="/shift" />
      <main id="main" className="page">
        <header className="cm-head">
          <div>
            <h1>{t("Turno de soporte")}</h1>
            <p className="muted" style={{ margin: "6px 0 0", maxWidth: 700 }}>
              {t("Te llega una cola de tickets del banco con prioridades mezcladas y su reloj de SLA. Tú decides el orden. La nota del turno suma la nota de cada caso (60 %), los SLA cumplidos (25 %) y haber atendido antes lo más urgente (15 %).")}
            </p>
          </div>
        </header>
        {err && <p className="error">{err}</p>}
        {!labs ? (
          <p className="muted">{t("Cargando…")}</p>
        ) : !cur ? (
          <section className="card sh-start">
            <h2>{t("Empezar un turno")}</h2>
            <p className="muted">{t("SLA del turno, contados desde que empieza: P1 30 min, P2 1 h, P3 2 h, P4 3 h.")}</p>
            <div className="row" role="group" aria-label={t("Banco")}>
              {[["zentra", t("Zentra")], ["atlas", t("Banco Atlas")], ["all", t("Los dos")]].map(([id, name]) => (
                <button key={id} type="button" className={`btn ${pool === id ? "" : "secondary"}`} aria-pressed={pool === id} onClick={() => setPool(id)}>
                  {name}
                </button>
              ))}
            </div>
            <div className="row" role="group" aria-label={t("Número de tickets")}>
              {[4, 5, 6].map((n) => (
                <button key={n} type="button" className={`btn ${size === n ? "" : "secondary"}`} aria-pressed={size === n} onClick={() => setSize(n)}>
                  {t("{n} tickets", { n })}
                </button>
              ))}
              <button type="button" className="cm-pillbtn ink sm" onClick={start} disabled={!labs.length}>{t("Empezar turno")}</button>
            </div>
          </section>
        ) : (
          <>
            <section className="card sh-bar">
              <div>
                <span className="muted">{t("Tiempo de turno")}</span>
                <strong className="sh-clock">{fmtTime(Math.floor((now - cur.started) / 1000))}</strong>
              </div>
              <div>
                <span className="muted">{t("Resueltos")}</span>
                <strong>{live?.solved} / {cur.ids.length}</strong>
              </div>
              <div>
                <span className="muted">{t("Nota provisional")}</span>
                <strong>{live?.score}</strong>
              </div>
              <button type="button" className="btn secondary" onClick={finish}>{t("Terminar turno")}</button>
            </section>
            <ol className="sh-queue" aria-label={t("Cola de tickets")}>
              {queue.map((id) => {
                const l = byId[id];
                const tk = l.ticket!;
                const left = Math.floor((deadline(id) - now) / 1000);
                const res = results[id];
                const done = solved(id);
                const met = done && res.finished <= deadline(id);
                return (
                  <li key={id} className={`card sh-ticket${done ? " done" : ""}`}>
                    <span className={`sh-prio p${rank(tk.priority)}`}>{tk.priority}</span>
                    <div className="sh-body">
                      <div className="muted sh-id">{tk.id} · {tk.kind}</div>
                      <div className="sh-title">{l.title}</div>
                    </div>
                    <div className="sh-sla">
                      {done ? (
                        <span className={met ? "sh-ok" : "sh-late"}>{met ? t("SLA cumplido") : t("SLA incumplido")} · {res.score}{res.helped ? " · " + t("con ayuda") : ""}</span>
                      ) : res?.tried ? (
                        <span className="sh-late">{t("Entregado sin resolver ({n}): vuelve a intentarlo", { n: res.score })}</span>
                      ) : left > 0 ? (
                        <span className={left < 600 ? "sh-late" : ""}>{t("SLA en {time}", { time: fmtTime(left) })}</span>
                      ) : (
                        <span className="sh-late">{t("SLA vencido")}</span>
                      )}
                    </div>
                    <button type="button" className={done ? "btn secondary" : "btn"} onClick={() => open(id)}>
                      {done ? t("Repetir") : cur.opened.includes(id) ? t("Continuar") : t("Abrir")}
                    </button>
                  </li>
                );
              })}
            </ol>
            <p className="muted" style={{ fontSize: 14 }}>{t("Consejo: lee primero todos los tickets y empieza por el de mayor impacto. Un P4 resuelto rápido no compensa un P1 fuera de plazo.")} <a href="/guide">{t("Guía de diagnóstico")}</a></p>
          </>
        )}
        {doc.history.length > 0 && (
          <section className="card" style={{ marginTop: 20 }}>
            <h2>{t("Turnos anteriores")}</h2>
            <table className="sh-hist">
              <thead>
                <tr>
                  <th>{t("Fecha")}</th>
                  <th>{t("Nota")}</th>
                  <th>{t("Resueltos")}</th>
                  <th>{t("SLA")}</th>
                  <th>{t("Orden")}</th>
                </tr>
              </thead>
              <tbody>
                {doc.history.map((h) => (
                  <tr key={h.at}>
                    <td>{new Date(h.at).toLocaleString()}</td>
                    <td><strong>{h.score}</strong> {shiftPassed(h) ? <span className="pill ok">{t("superado")}</span> : <span className="pill bad">{t("no superado")}</span>}</td>
                    <td>{h.solved}/{h.cases}</td>
                    <td>{h.sla} %</td>
                    <td>{h.order} %</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </section>
        )}
      </main>
    </>
  );
}
