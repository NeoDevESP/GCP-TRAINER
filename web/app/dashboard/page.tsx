"use client";

import { useEffect, useState } from "react";
import Nav from "@/components/Nav";
import Bar from "@/components/Bar";
import { api } from "@/lib/api";
import { useAuth } from "@/components/useAuth";
import type { Profile, Recommendation } from "@/lib/types";

async function startRec(r: Recommendation) {
  if (r.gen) {
    const s = await api<{ id: string }>("/api/incidents", { body: r.gen });
    window.location.href = `/lab?session=${s.id}`;
  } else if (r.labId) {
    window.location.href = `/lab?id=${encodeURIComponent(r.labId)}`;
  }
}

function RecList({ items }: { items: Recommendation[] | null }) {
  const [err, setErr] = useState("");
  if (!items?.length) return <p className="muted">Nothing queued — pick any lab from the catalog.</p>;
  return (
    <div className="col">
      {items.map((r, i) => (
        <div key={i} className="row" style={{ justifyContent: "space-between", borderBottom: "1px solid var(--border)", paddingBottom: 8 }}>
          <div style={{ minWidth: 0, flex: 1 }}>
            <div>
              <strong>{r.title}</strong> <span className="pill">{r.kind}</span>
            </div>
            <div className="muted small">{r.reason}</div>
          </div>
          <button className="btn secondary" onClick={() => startRec(r).catch((e) => setErr(e.message))}>
            Start
          </button>
        </div>
      ))}
      {err && <p className="error small">{err}</p>}
    </div>
  );
}

export default function Dashboard() {
  useAuth();
  const [p, setP] = useState<Profile | null>(null);
  const [attempts, setAttempts] = useState<any[]>([]);
  const [err, setErr] = useState("");
  const [track, setTrack] = useState("ace-30");
  const [tracks, setTracks] = useState<{ id: string; title: string }[]>([]);
  useEffect(() => {
    api<{ tracks: { id: string; title: string }[] }>("/api/catalog")
      .then((c) => setTracks(c.tracks ?? []))
      .catch(() => {});
  }, []);

  useEffect(() => {
    api<Profile>(`/api/me?track=${track}`).then(setP).catch((e) => setErr(e.message));
  }, [track]);
  useEffect(() => {
    api<any[]>("/api/me/attempts").then((a) => setAttempts(a ?? [])).catch(() => {});
  }, []);

  const downloadTranscript = async () => {
    const t = await api("/api/me/transcript");
    const blob = new Blob([JSON.stringify(t, null, 2)], { type: "application/json" });
    const a = document.createElement("a");
    a.href = URL.createObjectURL(blob);
    a.download = "cloud-mastery-transcript.json";
    a.click();
  };

  if (err) return (<><Nav active="/dashboard" /><main id="main" className="page"><p className="error">{err}</p></main></>);
  if (!p) return (<><Nav active="/dashboard" /><main id="main" className="page"><p className="muted">Loading…</p></main></>);

  const running = attempts.filter((a) => a.status === "running");
  return (
    <>
      <Nav active="/dashboard" />
      <main id="main" className="page">
        <div className="row" style={{ justifyContent: "space-between" }}>
          <h1>Welcome back, {p.user.name}</h1>
          <button className="btn secondary" onClick={downloadTranscript}>
            Download signed transcript
          </button>
        </div>
        <div className="grid three">
          <div className="card">
            <div className="muted small">Level</div>
            <div className="kpi">{p.level}</div>
            <Bar value={p.levelProgress * 100} />
            <div className="muted small">{p.xp} XP · {p.bonusXp} bonus · streak {p.streak}d</div>
          </div>
          <div className="card">
            <div className="muted small">Career stage</div>
            <div className="kpi" style={{ textTransform: "capitalize" }}>{p.career.stage}</div>
            {p.career.next && (
              <div className="small muted">
                Next: <strong>{p.career.next}</strong>
                {p.career.nextNeeds?.length ? <ul>{p.career.nextNeeds.map((n) => <li key={n}>{n}</li>)}</ul> : null}
              </div>
            )}
          </div>
          <div className="card">
            <div className="muted small">Autonomy</div>
            <div className="kpi">{p.student.autonomy.stage}</div>
            <Bar value={p.student.autonomy.progress * 100} />
            <div className="small muted">{p.student.autonomy.why}</div>
          </div>
        </div>

        {running.length > 0 && (
          <div className="card" style={{ marginTop: 16 }}>
            <h3>Running labs</h3>
            {running.map((a) => (
              <div key={a.id} className="row">
                <a href={`/lab?session=${a.sessionId ?? ""}&id=${encodeURIComponent(a.labId)}`}>{a.labId}</a>
                <span className="muted small">started {new Date(a.started).toLocaleString()}</span>
              </div>
            ))}
          </div>
        )}

        <div className="grid two" style={{ marginTop: 16 }}>
          <div className="card">
            <h2>Adaptive plan</h2>
            <RecList items={p.adaptive} />
            <h3>Track recommendations</h3>
            <div className="row small">
              <label htmlFor="track" style={{ margin: 0 }}>Track</label>
              <select id="track" value={track} onChange={(e) => setTrack(e.target.value)} style={{ width: "auto" }}>
                {(tracks.length ? tracks : [{ id: "ace-30", title: "Associate Cloud Engineer — 30 days" }]).map((t) => (
                  <option key={t.id} value={t.id}>
                    {t.title}
                  </option>
                ))}
              </select>
            </div>
            <RecList items={p.recommendations} />
          </div>
          <div className="card">
            <h2>Competence dimensions</h2>
            <table>
              <tbody>
                {p.student.dimensions.map((d) => (
                  <tr key={d.dimension}>
                    <td style={{ textTransform: "capitalize", width: 130 }}>{d.dimension}</td>
                    <td><Bar value={d.score * 100} /></td>
                    <td className="muted small" style={{ width: 90 }}>{Math.round(d.score * 100)}% · {d.evidence} ev.</td>
                  </tr>
                ))}
              </tbody>
            </table>
            {p.student.weakestDimensions?.length ? (
              <p className="small muted">Focus next on: {p.student.weakestDimensions.join(", ")}</p>
            ) : null}
          </div>
        </div>

        <div className="grid two" style={{ marginTop: 16 }}>
          <div className="card">
            <h2>Certification readiness</h2>
            {p.readiness.map((r) => (
              <div key={r.cert} style={{ marginBottom: 10 }}>
                <div className="row" style={{ justifyContent: "space-between" }}>
                  <strong>{r.name}</strong>
                  <span>{Math.round(r.percent)}%</span>
                </div>
                <Bar value={r.percent} />
                <div className="small muted">{r.explanation}</div>
              </div>
            ))}
          </div>
          <div className="card">
            <h2>Retention</h2>
            {p.student.retention?.length ? (
              <table>
                <thead><tr><th>Skill</th><th>Recall</th><th>Review</th></tr></thead>
                <tbody>
                  {p.student.retention.slice(0, 12).map((r) => (
                    <tr key={r.skill}>
                      <td>{r.skill}</td>
                      <td><Bar value={r.retention * 100} /></td>
                      <td>{r.due ? <span className="pill warn">due</span> : <span className="muted small">{new Date(r.dueAt).toLocaleDateString()}</span>}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            ) : (
              <p className="muted">Pass labs without hints to start building durable memory.</p>
            )}
            {p.student.recurringErrors?.length ? (
              <>
                <h3>Recurring errors</h3>
                <ul className="small">
                  {p.student.recurringErrors.map((e) => (
                    <li key={e.pattern}>{e.pattern} ×{e.count} <span className="muted">({e.labs.join(", ")})</span></li>
                  ))}
                </ul>
              </>
            ) : null}
          </div>
        </div>

        <div className="grid two" style={{ marginTop: 16 }}>
          <div className="card">
            <h2>Skills</h2>
            {p.skills?.length ? (
              <table>
                <thead><tr><th>Skill</th><th>Mastery</th><th></th></tr></thead>
                <tbody>
                  {[...p.skills].sort((a, b) => b.mastery - a.mastery).map((s) => (
                    <tr key={s.skill}>
                      <td>{s.skill} {s.masteryBadge && <span className="pill ok">mastered</span>}</td>
                      <td style={{ width: "40%" }}><Bar value={s.mastery} /></td>
                      <td className="small muted">{Math.round(s.mastery)}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            ) : (
              <p className="muted">No skills measured yet.</p>
            )}
          </div>
          <div className="card">
            <h2>Specializations</h2>
            {p.career.specializations?.map((s) => (
              <div key={s.id} style={{ marginBottom: 10 }}>
                <div className="row" style={{ justifyContent: "space-between" }}>
                  <strong>{s.name}</strong>
                  {s.done ? <span className="pill ok">complete</span> : !s.unlocked ? <span className="pill">locked</span> : <span>{Math.round(s.percent)}%</span>}
                </div>
                <Bar value={s.percent} />
                {s.missing?.length ? <div className="small muted">Missing: {s.missing.join(", ")}</div> : null}
              </div>
            ))}
            <h3>Badges</h3>
            <div className="row">
              {p.badges?.length ? p.badges.map((b) => <span key={b.id} className="pill ok" title={b.reason}>{b.name}</span>) : <span className="muted">None yet.</span>}
            </div>
          </div>
        </div>
      </main>
    </>
  );
}
