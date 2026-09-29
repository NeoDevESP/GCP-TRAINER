"use client";

import { useEffect, useState } from "react";
import { api, getToken, setToken } from "@/lib/api";
import { useI18n } from "@/lib/i18n";
import { LangSwitch } from "@/components/Nav";

export default function Home() {
  const { t } = useI18n();
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
    <main id="main" className="page" style={{ maxWidth: 980 }}>
      <div className="row" style={{ justifyContent: "flex-end" }}>
        <LangSwitch />
      </div>
      <div className="grid two" style={{ alignItems: "start", marginTop: 24 }}>
        <section>
          <h1 style={{ fontSize: 34 }}>
            Cloud <span style={{ color: "var(--accent)" }}>Mastery</span>
          </h1>
          <p className="muted" style={{ fontSize: 17 }}>
            {t(
              "Aprende Google Cloud haciendo el trabajo: construye, rompe, diagnostica y repara sistemas en un simulador determinista, gestiona una empresa persistente, atiende tickets de personas que no conocen la causa raíz y da el salto a entornos reales.",
            )}
          </p>
          <ul className="muted">
            <li>{t("Flujos reales con gcloud, gsutil, bq, kubectl y terraform")}</li>
            <li>{t("Incidentes generados a partir de una biblioteca de fallos, salas de crisis P1 y experimentos de caos")}</li>
            <li>{t("Evaluación del proceso: diagnóstico, seguridad, coste, riesgo y comunicación")}</li>
            <li>{t("Nueve dimensiones de competencia, retención espaciada y una escalera de autonomía")}</li>
            <li>{t("Etapas de carrera de becario a arquitecto con expedientes verificables")}</li>
          </ul>
        </section>
        <form className="card col" onSubmit={submit}>
          <div className="tabs">
            <button type="button" className={mode === "login" ? "active" : ""} onClick={() => setMode("login")}>
              {t("Iniciar sesión")}
            </button>
            <button type="button" className={mode === "register" ? "active" : ""} onClick={() => setMode("register")}>
              {t("Crear cuenta")}
            </button>
          </div>
          <div>
            <label htmlFor="email">{t("Correo electrónico")}</label>
            <input id="email" type="email" autoComplete="email" value={email} onChange={(e) => setEmail(e.target.value)} required />
          </div>
          {mode === "register" && (
            <div>
              <label htmlFor="name">{t("Nombre visible")}</label>
              <input id="name" value={name} onChange={(e) => setName(e.target.value)} required />
            </div>
          )}
          <div>
            <label htmlFor="pw">{t("Contraseña")}</label>
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
            {mode === "login" ? t("Iniciar sesión") : t("Crear cuenta")}
          </button>
          <a className="small" href="/api/auth/oidc/login">
            {t("Entrar con Google Workspace (si está configurado)")}
          </a>
          <a className="small" href="/verify">
            {t("Verificar el expediente de un alumno")}
          </a>
        </form>
      </div>
    </main>
  );
}
