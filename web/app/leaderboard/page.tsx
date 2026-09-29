"use client";

import { useEffect, useState } from "react";
import Nav from "@/components/Nav";
import { api } from "@/lib/api";
import { useAuth } from "@/components/useAuth";

export default function Leaderboard() {
  useAuth();
  const [d, setD] = useState<any>(null);
  const [err, setErr] = useState("");
  const load = () => api("/api/leaderboard").then(setD).catch((e) => setErr(e.message));
  useEffect(() => {
    load();
  }, []);
  const toggle = async () => {
    await api("/api/me/leaderboard", { body: { optIn: !d.optIn } });
    load();
  };
  return (
    <>
      <Nav active="/leaderboard" />
      <main id="main" className="page" style={{ maxWidth: 800 }}>
        <h1>Weekly league</h1>
        <p className="muted">
          Leagues are opt-in and rank this week&apos;s XP normalised by difficulty, so hard labs count more than grinding easy ones.
        </p>
        {err && <p className="error">{err}</p>}
        {d && (
          <div className="card">
            <div className="row" style={{ justifyContent: "space-between" }}>
              <h2 style={{ margin: 0, textTransform: "capitalize" }}>{d.league} league</h2>
              <button className="btn secondary" onClick={toggle}>{d.optIn ? "Leave leaderboard" : "Join leaderboard"}</button>
            </div>
            <table style={{ marginTop: 10 }}>
              <thead><tr><th>#</th><th>Name</th><th>Level</th><th>Points</th></tr></thead>
              <tbody>
                {(d.entries ?? []).map((e: any, i: number) => (
                  <tr key={e.userId}><td>{i + 1}</td><td>{e.name}</td><td>{e.level}</td><td>{Math.round(e.points)}</td></tr>
                ))}
              </tbody>
            </table>
            {!d.entries?.length && <p className="muted">No one has scored in this league this week.</p>}
          </div>
        )}
      </main>
    </>
  );
}
