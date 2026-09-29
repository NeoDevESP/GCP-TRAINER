"use client";

import { useState } from "react";
import { api } from "@/lib/api";

export default function Verify() {
  const [text, setText] = useState("");
  const [res, setRes] = useState<any>(null);
  const [err, setErr] = useState("");
  const [t, setT] = useState<any>(null);
  const check = async () => {
    setErr("");
    setRes(null);
    try {
      const parsed = JSON.parse(text);
      setT(parsed);
      setRes(await api("/api/auth/verify-transcript", { body: parsed }));
    } catch (e: any) {
      setErr(e.message);
    }
  };
  return (
    <main className="page" style={{ maxWidth: 820 }}>
      <h1>Verify a transcript</h1>
      <p className="muted">
        Learners can download a signed transcript of their competences, career stage and completed capstones. Paste it here
        to check that it was issued by this platform and has not been modified.
      </p>
      <textarea style={{ minHeight: 220, fontFamily: "var(--mono)" }} value={text} onChange={(e) => setText(e.target.value)} placeholder="{ … transcript JSON … }" />
      <div className="row" style={{ marginTop: 8 }}>
        <button className="btn" onClick={check} disabled={!text.trim()}>Verify</button>
        <a href="/">Back</a>
      </div>
      {err && <p className="error">{err}</p>}
      {res && (
        <div className="card" style={{ marginTop: 16, borderColor: res.valid ? "var(--ok)" : "var(--bad)" }}>
          <h2>{res.valid ? "✓ Valid transcript" : "✗ Invalid signature"}</h2>
          {res.valid && (
            <>
              <p>
                <strong>{res.user}</strong> — career stage <strong>{res.careerStage}</strong>, issued {new Date(res.issued).toLocaleString()}.
              </p>
              <p className="small muted">Autonomy: {t?.autonomy} · {t?.labsPassed?.length ?? 0} labs passed · specializations: {t?.specializations?.join(", ") || "none"}</p>
              {t?.dimensions && (
                <table>
                  <tbody>
                    {Object.entries(t.dimensions).map(([k, v]: any) => (
                      <tr key={k}><td style={{ textTransform: "capitalize" }}>{k}</td><td>{Math.round(v * 100)}%</td></tr>
                    ))}
                  </tbody>
                </table>
              )}
            </>
          )}
        </div>
      )}
    </main>
  );
}
