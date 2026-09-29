"use client";

import { useEffect, useState } from "react";
import { api, getToken, setToken } from "@/lib/api";

export default function Home() {
  const [mode, setMode] = useState<"login" | "register">("login");
  const [email, setEmail] = useState("");
  const [name, setName] = useState("");
  const [password, setPassword] = useState("");
  const [err, setErr] = useState("");
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    // OIDC callback lands here with #token=...
    const m = window.location.hash.match(/token=([^&]+)/);
    if (m) {
      setToken(decodeURIComponent(m[1]));
      window.location.replace("/dashboard");
      return;
    }
    if (getToken()) window.location.replace("/dashboard");
  }, []);

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    setErr("");
    setBusy(true);
    try {
      const body = mode === "login" ? { email, password } : { email, name, password };
      const res = await api<{ token: string }>(`/api/auth/${mode}`, { body });
      setToken(res.token);
      window.location.href = "/dashboard";
    } catch (e: any) {
      setErr(e.message);
    } finally {
      setBusy(false);
    }
  };

  return (
    <main className="page" style={{ maxWidth: 980 }}>
      <div className="grid two" style={{ alignItems: "start", marginTop: 40 }}>
        <section>
          <h1 style={{ fontSize: 34 }}>
            Cloud <span style={{ color: "var(--accent)" }}>Mastery</span>
          </h1>
          <p className="muted" style={{ fontSize: 17 }}>
            Learn Google Cloud by doing the job: build, break, diagnose and repair systems in a deterministic simulator,
            run a persistent company, answer tickets from people who don't know the root cause, and graduate to real sandboxes.
          </p>
          <ul className="muted">
            <li>Real <code>gcloud</code>, <code>gsutil</code>, <code>bq</code>, <code>kubectl</code> and <code>terraform</code> workflows</li>
            <li>Incidents generated from a failure library, P1 war rooms and chaos experiments</li>
            <li>Process-aware assessment: diagnosis, security, cost, risk and communication</li>
            <li>Nine competence dimensions, spaced retention and an autonomy ladder</li>
            <li>Career stages from intern to architect with verifiable transcripts</li>
          </ul>
        </section>
        <form className="card col" onSubmit={submit}>
          <div className="tabs">
            <button type="button" className={mode === "login" ? "active" : ""} onClick={() => setMode("login")}>
              Sign in
            </button>
            <button type="button" className={mode === "register" ? "active" : ""} onClick={() => setMode("register")}>
              Create account
            </button>
          </div>
          <div>
            <label htmlFor="email">Email</label>
            <input id="email" type="email" autoComplete="email" value={email} onChange={(e) => setEmail(e.target.value)} required />
          </div>
          {mode === "register" && (
            <div>
              <label htmlFor="name">Display name</label>
              <input id="name" value={name} onChange={(e) => setName(e.target.value)} required />
            </div>
          )}
          <div>
            <label htmlFor="pw">Password</label>
            <input
              id="pw"
              type="password"
              autoComplete={mode === "login" ? "current-password" : "new-password"}
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              required
            />
          </div>
          {err && <p className="error small">{err}</p>}
          <button className="btn" disabled={busy}>
            {mode === "login" ? "Sign in" : "Create account"}
          </button>
          <a className="small" href="/api/auth/oidc/login">
            Sign in with Google Workspace (if configured)
          </a>
          <a className="small" href="/verify">
            Verify a learner transcript
          </a>
        </form>
      </div>
    </main>
  );
}
