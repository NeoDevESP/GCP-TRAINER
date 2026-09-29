"use client";

import { useEffect, useState } from "react";
import Nav from "@/components/Nav";
import Bar from "@/components/Bar";
import { api } from "@/lib/api";
import { useAuth } from "@/components/useAuth";
import { k, useI18n } from "@/lib/i18n";

const pct = (v: number) => `${Math.round((v ?? 0) * 100)}%`;

export default function Admin() {
  useAuth();
  const { t, lang } = useI18n();
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
  }, [lang]);
  useEffect(() => {
    api(`/api/admin/kpis${sel ? `?class=${sel}` : ""}`).then(setKpis).catch((e) => setErr(e.message));
    if (sel) api(`/api/classes/${sel}/analytics`).then(setAnalytics).catch((e) => setErr(e.message));
    else setAnalytics(null);
  }, [sel, lang]);

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
        [k("Éxito al primer intento"), "firstAttemptSuccess", pct(kpis.firstAttemptSuccess)],
        [k("Dependencia de pistas"), "hintDependency", pct(kpis.hintDependency)],
        [k("Intentos hasta el dominio"), "retryToMastery", (kpis.retryToMastery ?? 0).toFixed(1)],
        [k("Retención a 7 días"), "retention7d", pct(kpis.retention7d)],
        [k("Retención a 30 días"), "retention30d", pct(kpis.retention30d)],
        [k("Tasa de causa raíz en incidentes"), "incidentRootCauseRate", pct(kpis.incidentRootCauseRate)],
        [k("Tasa de aprobado en proyectos finales"), "capstonePassRate", pct(kpis.capstonePassRate)],
        [k("Cobertura de habilidades"), "skillCoverage", pct(kpis.skillCoverage)],
        [k("Aprovisionamiento p95"), "provisionP95Ms", `${kpis.provisionP95Ms} ms`],
        [k("Tasa de error del evaluador"), "graderErrorRate", pct(kpis.graderErrorRate)],
        [k("Laboratorios con corrección automática"), "autoGradedLabs", pct(kpis.autoGradedLabs)],
        [k("Tasa de fallos de infraestructura"), "infraFailureRate", pct(kpis.infraFailureRate)],
      ]
    : [];

  return (
    <>
      <Nav active="/admin" />
      <main id="main" className="page">
        <h1>{t("Consola del instructor")}</h1>
        {err && <p className="error">{err}</p>}
        <div className="row" style={{ marginBottom: 12 }}>
          <select value={sel} onChange={(e) => setSel(e.target.value)} style={{ width: 260 }} aria-label={t("Clase")}>
            <option value="">{t("Todo el alumnado")}</option>
            {classes.map((c) => <option key={c.id} value={c.id}>{c.name} ({c.members?.length ?? 0})</option>)}
          </select>
          <input placeholder={t("Nombre de la nueva clase")} value={newClass} onChange={(e) => setNewClass(e.target.value)} style={{ width: 200 }} />
          <button className="btn secondary" onClick={create}>{t("Crear clase")}</button>
        </div>
        <div className="grid two">
          <div className="card">
            <h2>{t("KPI de aprendizaje")}</h2>
            {kpis && (
              <>
                <p className="small muted">{t("{n} alumnos activos · {a} intentos", { n: kpis.activeUsers, a: kpis.attempts })}</p>
                <table>
                  <thead><tr><th>KPI</th><th>{t("Valor")}</th><th>{t("Objetivo")}</th></tr></thead>
                  <tbody>
                    {rows.map(([name, key, v]) => (
                      <tr key={key}><td>{t(name)}</td><td>{v}</td><td className="small muted">{kpis.targets?.[key]}</td></tr>
                    ))}
                  </tbody>
                </table>
              </>
            )}
          </div>
          <div className="card">
            <h2>{analytics ? analytics.class.name : t("Analítica de la clase")}</h2>
            {analytics ? (
              <>
                <div className="row">
                  <input placeholder={t("correo del alumno")} value={member} onChange={(e) => setMember(e.target.value)} style={{ width: 240 }} />
                  <button className="btn secondary" onClick={add}>{t("Añadir alumno")}</button>
                </div>
                <h3>{t("Dominio del equipo por rama")}</h3>
                <table>
                  <tbody>
                    {Object.entries(analytics.teamBranches ?? {}).sort().map(([b, v]: any) => (
                      <tr key={b}><td style={{ width: 160 }}>{b}</td><td><Bar value={v} /></td></tr>
                    ))}
                  </tbody>
                </table>
                <h3>{t("Alumnado")}</h3>
                <table>
                  <thead><tr><th>{t("Nombre")}</th><th>{t("Nivel")}</th><th>XP</th><th>{t("Mejor preparación")}</th></tr></thead>
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
              <p className="muted">{t("Selecciona una clase para ver su alumnado y el mapa de calor de habilidades del equipo.")}</p>
            )}
          </div>
        </div>
        {kpis?.perLab?.length ? (
          <div className="card" style={{ marginTop: 16 }}>
            <h2>{t("Calibración de dificultad por laboratorio")}</h2>
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
            <h2>{t("Pool de proyectos reales de GCP (F2)")}</h2>
            {pool?.enabled ? (
              <>
                <p className="small">{t("Controlador:")} <code>{pool.driver}</code></p>
                <pre>{JSON.stringify(pool.stats, null, 2)}</pre>
                <button className="btn secondary" onClick={janitor}>{t("Ejecutar la limpieza ahora")}</button>
              </>
            ) : (
              <p className="muted">{t("No hay pool de sandboxes configurado; los laboratorios se ejecutan en el simulador y los emuladores.")}</p>
            )}
          </div>
          <div className="card">
            <h2>{t("Salud del contenido")}</h2>
            {content && (
              <>
                <p>{t("{n} laboratorios cargados · {p} problemas", { n: content.labs, p: content.problems?.length ?? 0 })}</p>
                {content.problems?.length ? <ul className="small error">{content.problems.map((p: string, i: number) => <li key={i}>{p}</li>)}</ul> : <p className="small ok">{t("Todo el contenido es válido.")}</p>}
                <p className="small muted">{t("Tipos de fallo: {list}", { list: content.faultTypes?.join(", ") ?? "" })}</p>
              </>
            )}
          </div>
        </div>
      </main>
    </>
  );
}
