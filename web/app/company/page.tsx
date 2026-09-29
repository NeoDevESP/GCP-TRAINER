"use client";

import { useEffect, useState } from "react";
import Nav from "@/components/Nav";
import { api } from "@/lib/api";
import { useAuth } from "@/components/useAuth";
import { useI18n } from "@/lib/i18n";
import { label, labType } from "@/lib/labels";

export default function CompanyPage() {
  useAuth();
  const { t, lang } = useI18n();
  const [c, setC] = useState<any>(null);
  const [err, setErr] = useState("");
  const [busy, setBusy] = useState("");

  const load = () => api("/api/company").then(setC).catch((e) => setErr(e.message));
  useEffect(() => {
    load();
  }, [lang]);

  const create = async (reset = false) => {
    if (reset && !confirm(t("¿Empezar de nuevo? Se perderá la historia de tu empresa."))) return;
    setBusy("create");
    try {
      setC(await api(`/api/company${reset ? "?reset=1" : ""}`, { body: {} }));
    } catch (e: any) {
      setErr(e.message);
    } finally {
      setBusy("");
    }
  };
  const start = async (id: string) => {
    setBusy(id);
    setErr("");
    try {
      const s = await api<{ id: string }>(`/api/company/missions/${encodeURIComponent(id)}/start`, { body: {} });
      window.location.href = `/lab?session=${s.id}&id=${encodeURIComponent(id)}`;
    } catch (e: any) {
      setErr(e.message);
      setBusy("");
    }
  };

  if (!c)
    return (
      <>
        <Nav active="/company" />
        <main id="main" className="page">
          {err ? <p className="error">{err}</p> : <p className="muted">{t("Cargando…")}</p>}
        </main>
      </>
    );

  if (c.exists === false) {
    return (
      <>
        <Nav active="/company" />
        <main id="main" className="page" style={{ maxWidth: 800 }}>
          <h1>{t("Únete a {name}", { name: c.name })}</h1>
          <p>
            {t(
              "Eres la nueva persona de ingeniería cloud en {name}. Su entorno de Google Cloud persiste entre misiones: lo que construyes se queda, lo que dejas inseguro o sin copia de seguridad vuelve como incidentes, y la disponibilidad, la postura de seguridad, el coste y la satisfacción de las partes interesadas son responsabilidad tuya.",
              { name: c.name },
            )}
          </p>
          <h3>{t("Proyectos")}</h3>
          <ul>
            {(c.projects ?? []).map((p: any) => (
              <li key={p.key}>
                <code>{p.key}</code>{" "}
                <span className="muted">
                  {p.folder} — {p.purpose}
                </span>
              </li>
            ))}
          </ul>
          {err && <p className="error">{err}</p>}
          <button className="btn" onClick={() => create()} disabled={!!busy}>
            {t("Empezar el día 1")}
          </button>
        </main>
      </>
    );
  }

  const m = c.metrics ?? {};
  return (
    <>
      <Nav active="/company" />
      <main id="main" className="page">
        <div className="row" style={{ justifyContent: "space-between" }}>
          <h1>{t("{name} — día {day}", { name: c.name, day: c.day })}</h1>
          <button className="btn danger" onClick={() => create(true)}>
            {t("Empezar de nuevo")}
          </button>
        </div>
        {err && <p className="error">{err}</p>}
        <div className="grid three">
          <div className="card"><div className="muted small">{t("Disponibilidad")}</div><div className="kpi">{Math.round(m.availability ?? 0)}%</div></div>
          <div className="card"><div className="muted small">{t("Postura de seguridad")}</div><div className="kpi">{Math.round(m.security ?? 0)}</div></div>
          <div className="card"><div className="muted small">{t("Coste mensual")}</div><div className="kpi">€{(m.monthlyCostEur ?? 0).toFixed(0)}</div></div>
          <div className="card"><div className="muted small">{t("Satisfacción de las partes interesadas")}</div><div className="kpi">{Math.round(m.satisfaction ?? 0)}%</div></div>
        </div>

        {c.incidents?.length ? (
          <div className="card" style={{ marginTop: 16, borderColor: "var(--bad)" }}>
            <h2>{t("Consecuencias abiertas")}</h2>
            {c.incidents.map((p: any, i: number) => (
              <div key={i} className="row" style={{ justifyContent: "space-between" }}>
                <div>
                  <strong>{p.consequence}</strong> <span className="muted small">{p.note}</span>
                </div>
                <button className="btn" onClick={() => start(p.mission)} disabled={!!busy}>
                  {t("Responder")}
                </button>
              </div>
            ))}
          </div>
        ) : null}

        <div className="grid two" style={{ marginTop: 16 }}>
          <div className="card">
            <h2>{t("Misiones")}</h2>
            <table>
              <tbody>
                {(c.missions ?? []).map((ms: any) => (
                  <tr key={ms.id}>
                    <td>
                      <strong>{ms.title}</strong> {ms.incident && <span className="pill bad">{t("incidente")}</span>}{" "}
                      <span className="pill">{label(labType, ms.kind, t)}</span>
                      <div className="small muted">{ms.summary}</div>
                      {!ms.available && !ms.done && ms.why && <div className="small warn">{ms.why}</div>}
                    </td>
                    <td style={{ width: 120, textAlign: "right" }}>
                      {ms.done ? (
                        <span className="pill ok">
                          {t("hecha")} {ms.score ? `· ${ms.score}` : ""}
                        </span>
                      ) : ms.available ? (
                        <button className="btn secondary" onClick={() => start(ms.id)} disabled={!!busy}>
                          {busy === ms.id ? "…" : c.active === ms.id ? t("Reanudar") : t("Empezar")}
                        </button>
                      ) : (
                        <span className="pill">{t("bloqueada")}</span>
                      )}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
          <div className="col">
            <div className="card">
              <h2>{t("Diario")}</h2>
              {c.journal?.length ? (
                [...c.journal].reverse().slice(0, 30).map((j: any, i: number) => (
                  <div key={i} className="small" style={{ borderBottom: "1px solid var(--border)", padding: "4px 0" }}>
                    <span className="muted">{t("Día {n}", { n: j.day })}</span> {j.text ?? j.entry ?? JSON.stringify(j)}
                  </div>
                ))
              ) : (
                <p className="muted">{t("Nada todavía.")}</p>
              )}
            </div>
            <div className="card">
              <h2>{t("Personas")}</h2>
              <table>
                <tbody>
                  {(c.people ?? []).map((p: any) => (
                    <tr key={p.name}>
                      <td><strong>{p.name}</strong></td>
                      <td className="small muted">{p.role}</td>
                      <td className="small">{p.persona}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </div>
        </div>
      </main>
    </>
  );
}
