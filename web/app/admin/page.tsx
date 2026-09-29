"use client";

import { useEffect, useState } from "react";
import Nav from "@/components/Nav";
import Bar from "@/components/Bar";
import { api } from "@/lib/api";
import { useAuth } from "@/components/useAuth";

const pct = (v: number) => `${Math.round((v ?? 0) * 100)}%`;

export default function Admin() {
  useAuth();
  const [kpis, setKpis] = useState<any>(null);
  const [classes, setClasses] = useState<any[]>([]);
  const [sel, setSel] = useState("");
  const [analytics, setAnalytics] = useState<any>(null);
  const [pool, setPool] = useState<any>(null);
  const [content, setContent] = useState<any>(null);
  const [newClass, setNewClass] = useState("");
  const [member, setMember] = useState("");
  const [err, setErr] = useState("");

  const loadClasses = () => api<any[]>("/api/classes").then((c) => setClasses(c ?? [])).catch((e) => setErr(e.message));
  useEffect(() => {
    loadClasses();
    api("/api/admin/pool").then(setPool).catch(() => {});
    api("/api/admin/content").then(setContent).catch(() => {});
  }, []);
  useEffect(() => {
    api(`/api/admin/kpis${sel ? `?class=${sel}` : ""}`).then(setKpis).catch((e) => setErr(e.message));
    if (sel) api(`/api/classes/${sel}/analytics`).then(setAnalytics).catch((e) => setErr(e.message));
    else setAnalytics(null);
  }, [sel]);

  const create = async () => {
    if (!newClass) return;
    await api("/api/classes", { body: { name: newClass } }).catch((e) => setErr(e.message));
    setNewClass("");
    loadClasses();
  };
  const add = async () => {
    if (!member || !sel) return;
    try {
      await api(`/api/classes/${sel}/members`, { body: { email: member } });
      setMember("");
      setAnalytics(await api(`/api/classes/${sel}/analytics`));
    } catch (e: any) {
      setErr(e.message);
    }
  };
  const janitor = async () => {
    await api("/api/admin/pool/janitor", { body: {} }).catch((e) => setErr(e.message));
    api("/api/admin/pool").then(setPool);
  };

  const rows: [string, string, any][] = kpis
    ? [
        ["First-attempt success", "firstAttemptSuccess", pct(kpis.firstAttemptSuccess)],
        ["Hint dependency", "hintDependency", pct(kpis.hintDependency)],
        ["Retries to mastery", "retryToMastery", (kpis.retryToMastery ?? 0).toFixed(1)],
        ["Retention 7d", "retention7d", pct(kpis.retention7d)],
        ["Retention 30d", "retention30d", pct(kpis.retention30d)],
        ["Incident root-cause rate", "incidentRootCauseRate", pct(kpis.incidentRootCauseRate)],
        ["Capstone pass rate", "capstonePassRate", pct(kpis.capstonePassRate)],
        ["Skill coverage", "skillCoverage", pct(kpis.skillCoverage)],
        ["Provisioning p95", "provisionP95Ms", `${kpis.provisionP95Ms} ms`],
        ["Grader error rate", "graderErrorRate", pct(kpis.graderErrorRate)],
        ["Auto-graded labs", "autoGradedLabs", pct(kpis.autoGradedLabs)],
        ["Infra failure rate", "infraFailureRate", pct(kpis.infraFailureRate)],
      ]
    : [];

  return (
    <>
      <Nav active="/admin" />
      <main className="page">
        <h1>Instructor console</h1>
        {err && <p className="error">{err}</p>}
        <div className="row" style={{ marginBottom: 12 }}>
          <select value={sel} onChange={(e) => setSel(e.target.value)} style={{ width: 260 }} aria-label="Class">
            <option value="">All learners</option>
            {classes.map((c) => <option key={c.id} value={c.id}>{c.name} ({c.members?.length ?? 0})</option>)}
          </select>
          <input placeholder="New class name" value={newClass} onChange={(e) => setNewClass(e.target.value)} style={{ width: 200 }} />
          <button className="btn secondary" onClick={create}>Create class</button>
        </div>
        <div className="grid two">
          <div className="card">
            <h2>Learning KPIs</h2>
            {kpis && (
              <>
                <p className="small muted">{kpis.activeUsers} active learners · {kpis.attempts} attempts</p>
                <table>
                  <thead><tr><th>KPI</th><th>Value</th><th>Target</th></tr></thead>
                  <tbody>
                    {rows.map(([label, key, v]) => (
                      <tr key={key}><td>{label}</td><td>{v}</td><td className="small muted">{kpis.targets?.[key]}</td></tr>
                    ))}
                  </tbody>
                </table>
              </>
            )}
          </div>
          <div className="card">
            <h2>{analytics ? analytics.class.name : "Class analytics"}</h2>
            {analytics ? (
              <>
                <div className="row">
                  <input placeholder="learner email" value={member} onChange={(e) => setMember(e.target.value)} style={{ width: 240 }} />
                  <button className="btn secondary" onClick={add}>Add learner</button>
                </div>
                <h3>Team branch mastery</h3>
                <table>
                  <tbody>
                    {Object.entries(analytics.teamBranches ?? {}).sort().map(([b, v]: any) => (
                      <tr key={b}><td style={{ width: 160 }}>{b}</td><td><Bar value={v} /></td></tr>
                    ))}
                  </tbody>
                </table>
                <h3>Learners</h3>
                <table>
                  <thead><tr><th>Name</th><th>Level</th><th>XP</th><th>Best readiness</th></tr></thead>
                  <tbody>
                    {(analytics.members ?? []).map((m: any) => {
                      const best = [...(m.readiness ?? [])].sort((a: any, b: any) => b.percent - a.percent)[0];
                      return (
                        <tr key={m.email}><td>{m.user}<div className="small muted">{m.email}</div></td><td>{m.level}</td><td>{m.xp}</td><td className="small">{best ? `${best.cert} ${Math.round(best.percent)}%` : "—"}</td></tr>
                      );
                    })}
                  </tbody>
                </table>
              </>
            ) : (
              <p className="muted">Select a class to see its learners and team skill heatmap.</p>
            )}
          </div>
        </div>
        {kpis?.perLab?.length ? (
          <div className="card" style={{ marginTop: 16 }}>
            <h2>Per-lab difficulty calibration</h2>
            <table>
              <thead><tr>{Object.keys(kpis.perLab[0]).map((k) => <th key={k}>{k}</th>)}</tr></thead>
              <tbody>
                {kpis.perLab.map((l: any, i: number) => (
                  <tr key={i}>{Object.values(l).map((v: any, j) => <td key={j} className="small">{typeof v === "number" && v < 1 && v > 0 ? pct(v) : String(v)}</td>)}</tr>
                ))}
              </tbody>
            </table>
          </div>
        ) : null}
        <div className="grid two" style={{ marginTop: 16 }}>
          <div className="card">
            <h2>Real-GCP project pool (F2)</h2>
            {pool?.enabled ? (
              <>
                <p className="small">Driver: <code>{pool.driver}</code></p>
                <pre>{JSON.stringify(pool.stats, null, 2)}</pre>
                <button className="btn secondary" onClick={janitor}>Run janitor now</button>
              </>
            ) : (
              <p className="muted">No sandbox pool configured; labs run on the simulator and emulators.</p>
            )}
          </div>
          <div className="card">
            <h2>Content health</h2>
            {content && (
              <>
                <p>{content.labs} labs loaded · {content.problems?.length ?? 0} problems</p>
                {content.problems?.length ? <ul className="small error">{content.problems.map((p: string, i: number) => <li key={i}>{p}</li>)}</ul> : <p className="small ok">All content validates.</p>}
                <p className="small muted">Fault types: {content.faultTypes?.join(", ")}</p>
              </>
            )}
          </div>
        </div>
      </main>
    </>
  );
}
