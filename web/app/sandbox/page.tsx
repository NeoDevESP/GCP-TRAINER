"use client";

// The free console: the simulated Google Cloud console and Cloud Shell with
// no mission, no score and no hints, to try things out. Pick a starting
// world (an empty project, a new project, Zentra, Banco Atlas…), go back to
// the one that is open, or continue the world saved last time. Nothing here
// counts towards progress (the server creates no attempt).

import { useEffect, useState } from "react";
import Nav from "@/components/Nav";
import { useAuth } from "@/components/useAuth";
import { api } from "@/lib/api";
import { useI18n } from "@/lib/i18n";
import type { SessionInfo } from "@/lib/types";

interface World {
  id: string;
  name: string;
  description: string;
}
interface SandboxState {
  worlds: World[];
  running?: { session: string; world: string; labId: string };
  saved?: { world: string; savedAt: string };
}

export default function SandboxPage() {
  useAuth();
  const { t, lang } = useI18n();
  const [st, setSt] = useState<SandboxState | null>(null);
  const [busy, setBusy] = useState("");
  const [err, setErr] = useState("");

  const load = () => api<SandboxState>("/api/sandbox").then(setSt).catch((e) => setErr(e.message));
  useEffect(() => {
    load();
  }, []);

  const open = (s: SessionInfo) => (window.location.href = `/lab?session=${s.id}&id=${encodeURIComponent(s.labId)}`);
  const start = async (body: { world?: string; resume?: boolean }, key: string) => {
    if (st?.running && !confirm(t("Ya tienes una consola libre abierta. Si empiezas otra, la abierta se cierra (lo que no hayas guardado se pierde). ¿Seguir?"))) return;
    setBusy(key);
    setErr("");
    try {
      open(await api<SessionInfo>("/api/sandbox/start", { body }));
    } catch (e: any) {
      setErr(e.message);
      setBusy("");
    }
  };
  const forget = async () => {
    if (!confirm(t("¿Borrar el mundo guardado? No se puede deshacer."))) return;
    await api("/api/sandbox/saved", { method: "DELETE" }).catch((e) => setErr(e.message));
    load();
  };
  const name = (id: string) => st?.worlds.find((w) => w.id === id)?.name ?? id;
  const when = (iso: string) => new Date(iso).toLocaleString(lang === "en" ? "en-GB" : "es-ES", { day: "numeric", month: "long", hour: "2-digit", minute: "2-digit" });

  return (
    <>
      <Nav active="/sandbox" />
      <main id="main" className="page">
        <header className="cm-head">
          <div>
            <h1>{t("Consola libre")}</h1>
            <p className="muted" style={{ margin: "6px 0 0", maxWidth: 760 }}>
              {t("La misma consola de Google Cloud y Cloud Shell de los laboratorios, sin misión, sin nota y sin reloj. Eres propietario del proyecto: crea, rompe y prueba lo que quieras. No cuenta para tu progreso.")}
            </p>
          </div>
        </header>
        {err && <p className="error" role="alert">{err}</p>}
        {!st ? (
          <p className="muted">{t("Cargando…")}</p>
        ) : (
          <>
            {(st.running || st.saved) && (
              <section className="sb-resume" aria-label={t("Continuar")}>
                {st.running && (
                  <div className="card">
                    <span className="db-kick">{t("Abierta ahora")}</span>
                    <h2>{name(st.running.world)}</h2>
                    <a className="btn" href={`/lab?session=${st.running.session}&id=${encodeURIComponent(st.running.labId)}`}>{t("Volver a la consola")}</a>
                  </div>
                )}
                {st.saved && (
                  <div className="card">
                    <span className="db-kick">{t("Mundo guardado")}</span>
                    <h2>{name(st.saved.world)}</h2>
                    <p className="muted small" style={{ marginTop: 0 }}>{t("Guardado el {d}", { d: when(st.saved.savedAt) })}</p>
                    <div className="row" style={{ gap: 8, flexWrap: "wrap" }}>
                      <button type="button" className="btn" disabled={!!busy} onClick={() => start({ resume: true }, "resume")}>
                        {busy === "resume" ? t("Preparando el entorno…") : t("Continuar donde lo dejé")}
                      </button>
                      <button type="button" className="btn secondary" onClick={forget}>{t("Borrar")}</button>
                    </div>
                  </div>
                )}
              </section>
            )}
            <h2 style={{ marginTop: 24 }}>{t("Empieza desde…")}</h2>
            <ul className="sb-worlds">
              {st.worlds.map((w) => (
                <li key={w.id} className="card">
                  <h3 style={{ marginTop: 0 }}>{w.name}</h3>
                  <p className="muted small">{w.description}</p>
                  <button type="button" className="btn secondary" disabled={!!busy} onClick={() => start({ world: w.id }, w.id)} aria-label={t("Abrir «{name}»", { name: w.name })}>
                    {busy === w.id ? t("Preparando el entorno…") : t("Abrir")}
                  </button>
                </li>
              ))}
            </ul>
          </>
        )}
      </main>
    </>
  );
}
