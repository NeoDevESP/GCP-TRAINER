"use client";

// Operations pages of the console: Logs Explorer, Metrics Explorer,
// Billing reports, Network Topology and the Activity log. They read the
// session's telemetry views; commands still run in Cloud Shell.

import { useEffect, useMemo, useState } from "react";
import { api } from "@/lib/api";
import { useI18n } from "@/lib/i18n";
import Mermaid from "@/components/Mermaid";
import JsonTree from "@/components/JsonTree";
import type { Ctx } from "./pages";
import { Page, Pill, Table, Tool, q } from "./ui";
import { Icon } from "./icons";

function useView<T>(ctx: Ctx, kind: string, params = "") {
  const [data, setData] = useState<T | null>(null);
  const [err, setErr] = useState("");
  useEffect(() => {
    let live = true;
    api<T>(`/api/sessions/${ctx.sessionId}/views/${kind}${params}`)
      .then((d) => live && (setData(d), setErr("")))
      .catch((e) => live && setErr(e.message));
    return () => {
      live = false;
    };
  }, [ctx.sessionId, ctx.tick, kind, params]);
  return { data, err };
}

// ---------------------------------------------------------------- Logging

const SEV = ["DEFAULT", "DEBUG", "INFO", "NOTICE", "WARNING", "ERROR", "CRITICAL", "ALERT", "EMERGENCY"];

function sevIcon(s: string) {
  const i = SEV.indexOf(s);
  if (i >= 5) return <span className="cc-sev bad" title={s}>!!</span>;
  if (i === 4) return <span className="cc-sev warn" title={s}>!</span>;
  if (i >= 2) return <span className="cc-sev info" title={s}>i</span>;
  return <span className="cc-sev" title={s}>*</span>;
}

function summary(r: any, t: (s: string) => string) {
  if (r.textPayload) return r.textPayload;
  if (r.httpRequest) return `${r.httpRequest.requestMethod ?? ""} ${r.httpRequest.status ?? ""} ${r.httpRequest.requestUrl ?? ""}`;
  if (r.protoPayload) return `${r.protoPayload.methodName ?? ""} ${r.protoPayload.resourceName ?? ""} ${t("por")} ${r.protoPayload["authenticationInfo.principalEmail"] ?? "?"}`;
  if (r.jsonPayload) return JSON.stringify(r.jsonPayload);
  return JSON.stringify(r.labels ?? {});
}

export function LogsExplorer({ ctx }: { ctx: Ctx }) {
  const { t } = useI18n();
  const [draft, setDraft] = useState("");
  const [query, setQuery] = useState("");
  const [sev, setSev] = useState("");
  const [open, setOpen] = useState<number | null>(null);
  const full = [query.trim(), sev ? `severity>=${sev}` : ""].filter(Boolean).join("\n");
  const { data, err } = useView<any[]>(ctx, "logs", `?limit=300&filter=${encodeURIComponent(full.replace(/\n/g, " "))}`);
  const rows = data ?? [];
  const buckets = useMemo(() => {
    const m = new Map<string, number>();
    for (const r of rows) {
      const k = String(r.timestamp ?? "").slice(0, 16);
      m.set(k, (m.get(k) ?? 0) + 1);
    }
    return [...m.entries()].sort();
  }, [rows]);
  const max = Math.max(1, ...buckets.map(([, n]) => n));
  const run = () => setQuery(draft);
  return (
    <Page
      title={t("Explorador de registros")}
      actions={
        <>
          <Tool icon="refresh" label={t("Actualizar")} onClick={ctx.refresh} />
          <Tool icon="shell" label={t("Leer en Cloud Shell")} onClick={() => ctx.run(`gcloud logging read ${q(full.replace(/\n/g, " ") || "severity>=DEFAULT")} --limit=20 --freshness=1d`, t("Leer registros"))} />
        </>
      }
    >
      <div className="cc-logq">
        <div className="cc-logq-head">
          <strong>{t("Consulta")}</strong>
          <label className="cc-inline">
            {t("Gravedad")}
            <select value={sev} onChange={(e) => setSev(e.target.value)}>
              <option value="">{t("Todas las gravedades")}</option>
              {SEV.slice(1).map((s) => <option key={s} value={s}>{s}</option>)}
            </select>
          </label>
          <button type="button" className="cc-btn primary" onClick={run}>
            <Icon name="play" size={18} /> {t("Ejecutar consulta")}
          </button>
        </div>
        <textarea
          className="cc-code-input"
          value={draft}
          onChange={(e) => setDraft(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "Enter" && (e.ctrlKey || e.metaKey)) run();
          }}
          aria-label={t("Consulta de registros")}
          placeholder={'resource.type="cloud_run_revision"\nseverity>=ERROR\ntextPayload:"timeout"'}
          spellCheck={false}
        />
        <div className="cc-help">{t("Una condición por línea (se combinan con AND). Ctrl+Intro ejecuta la consulta.")}</div>
      </div>
      {err && <p className="error">{err}</p>}
      <div className="cc-hist" aria-hidden="true">
        {buckets.map(([k, n]) => (
          <span key={k} title={`${k} · ${n}`} style={{ height: `${(n / max) * 100}%` }} />
        ))}
      </div>
      <div className="cc-table-wrap">
        <div className="cc-filter">
          <strong>{t("Resultados de la consulta")}</strong>
          <span className="cc-help" style={{ margin: 0 }}>{t("{n} entradas", { n: rows.length })}</span>
        </div>
        <div className="cc-table-scroll">
          <table className="cc-table cc-logs">
            <caption className="sr-only">{t("Entradas de registro")}</caption>
            <thead>
              <tr><th style={{ width: 36 }}><span className="sr-only">{t("Gravedad")}</span></th><th>{t("Marca de tiempo")}</th><th>{t("Recurso")}</th><th>{t("Resumen")}</th></tr>
            </thead>
            <tbody>
              {rows.map((r, i) => (
                <FragmentRow key={i} open={open === i} onToggle={() => setOpen(open === i ? null : i)} r={r} t={t} />
              ))}
            </tbody>
          </table>
          {!rows.length && <p className="cc-empty">{t("Ninguna entrada coincide.")}</p>}
        </div>
      </div>
    </Page>
  );
}

function FragmentRow({ r, open, onToggle, t }: { r: any; open: boolean; onToggle: () => void; t: (s: string) => string }) {
  return (
    <>
      <tr className={open ? "selected" : ""} onClick={onToggle} style={{ cursor: "pointer" }}>
        <td>{sevIcon(r.severity)}</td>
        <td style={{ whiteSpace: "nowrap" }}>
          <button type="button" className="cc-link" aria-expanded={open} onClick={(e) => { e.stopPropagation(); onToggle(); }}>
            {String(r.timestamp ?? "").replace("T", " ").replace(/\..*|Z$/, "")}
          </button>
        </td>
        <td className="cc-mono-sm">{r.resource?.type}<div className="cc-help" style={{ margin: 0 }}>{Object.values(r.resource?.labels ?? {}).join(" ")}</div></td>
        <td className="cc-mono-sm" style={{ wordBreak: "break-word" }}>{summary(r, t)}</td>
      </tr>
      {open && (
        <tr>
          <td colSpan={4} className="cc-log-json"><JsonTree data={r} hideEmpty={false} /></td>
        </tr>
      )}
    </>
  );
}

// ---------------------------------------------------------------- Monitoring

function LineChart({ points, height = 220 }: { points: { t: string; v: number }[]; height?: number }) {
  const { t } = useI18n();
  const width = 720;
  if (!points?.length) return <p className="cc-empty">{t("sin datos")}</p>;
  const vs = points.map((p) => p.v);
  const min = Math.min(0, ...vs);
  const max = Math.max(...vs) || 1;
  const pad = 36;
  const x = (i: number) => pad + (points.length === 1 ? (width - pad) / 2 : (i / (points.length - 1)) * (width - pad - 8));
  const y = (v: number) => 8 + (1 - (v - min) / (max - min || 1)) * (height - 32);
  const d = points.map((p, i) => `${i ? "L" : "M"}${x(i).toFixed(1)},${y(p.v).toFixed(1)}`).join(" ");
  const ticks = [min, (min + max) / 2, max];
  return (
    <svg viewBox={`0 0 ${width} ${height}`} className="cc-chart" role="img" aria-label={t("Mínimo {a}, máximo {b}", { a: Math.round(min * 100) / 100, b: Math.round(max * 100) / 100 })}>
      {ticks.map((v, i) => (
        <g key={i}>
          <line x1={pad} x2={width} y1={y(v)} y2={y(v)} className="grid" />
          <text x={pad - 6} y={y(v) + 4} textAnchor="end">{Math.round(v * 100) / 100}</text>
        </g>
      ))}
      <path d={`${d} L${x(points.length - 1)},${y(min)} L${x(0)},${y(min)} Z`} className="area" />
      <path d={d} className="line" />
      <text x={pad} y={height - 4}>{String(points[0].t).slice(11, 16)}</text>
      <text x={width} y={height - 4} textAnchor="end">{String(points[points.length - 1].t).slice(11, 16)}</text>
    </svg>
  );
}

export function MetricsExplorer({ ctx }: { ctx: Ctx }) {
  const { t } = useI18n();
  const { data } = useView<Record<string, { t: string; v: number }[]>>(ctx, "metrics");
  const keys = Object.keys(data ?? {});
  const [sel, setSel] = useState("");
  const cur = sel && keys.includes(sel) ? sel : keys[0] ?? "";
  const alerts = Object.values(ctx.data.alertPolicies ?? {}) as any[];
  return (
    <Page title={t("Explorador de métricas")} actions={<Tool icon="refresh" label={t("Actualizar")} onClick={ctx.refresh} />}>
      {!keys.length ? (
        <p className="cc-empty">{t("Aún no hay métricas. Aparecen a medida que avanza el tiempo simulado.")}</p>
      ) : (
        <div className="cc-metrics">
          <div className="card">
            <div className="cc-field" style={{ maxWidth: 560 }}>
              <label htmlFor="cc-metric">{t("Métrica")}</label>
              <select id="cc-metric" value={cur} onChange={(e) => setSel(e.target.value)}>
                {keys.map((k) => <option key={k} value={k}>{k}</option>)}
              </select>
            </div>
            <LineChart points={(data?.[cur] ?? []).slice(-120)} />
          </div>
          <h2 className="cc-h2">{t("Todas las métricas")}</h2>
          <div className="cc-grid">
            {keys.map((k) => {
              const pts = data?.[k] ?? [];
              return (
                <button key={k} type="button" className={`card cc-res ${k === cur ? "on" : ""}`} onClick={() => setSel(k)}>
                  <span className="cc-mono-sm" style={{ wordBreak: "break-all" }}>{k}</span>
                  <span className="kpi">{pts.length ? Math.round(pts[pts.length - 1].v * 100) / 100 : "—"}</span>
                </button>
              );
            })}
          </div>
        </div>
      )}
      <h2 className="cc-h2">{t("Políticas de alertas")}</h2>
      <Table
        caption={t("Políticas de alertas")}
        filter={false}
        rows={alerts}
        rowKey={(a: any) => a.name ?? a.displayName}
        empty={t("No hay políticas de alertas. Créalas con gcloud alpha monitoring policies create.")}
        cols={[
          { key: "name", label: t("Nombre"), render: (a: any) => a.displayName ?? a.name },
          { key: "cond", label: t("Condición"), render: (a: any) => <code>{a.filter ?? a.condition ?? JSON.stringify(a.conditions ?? "")}</code> },
          { key: "on", label: t("Estado"), render: (a: any) => (a.enabled === false ? <Pill>{t("Inhabilitada")}</Pill> : <Pill tone="ok">{t("Habilitada")}</Pill>) },
        ]}
      />
    </Page>
  );
}

// ---------------------------------------------------------------- Billing

export function Billing({ ctx }: { ctx: Ctx }) {
  const { t } = useI18n();
  const { data } = useView<{ monthlyEur: number; lines: { resource: string; sku: string; monthlyEur: number }[] }>(ctx, "cost");
  const lines = data?.lines ?? [];
  const byService = useMemo(() => {
    const m = new Map<string, number>();
    for (const l of lines) {
      const s = l.resource.split("/")[0];
      m.set(s, (m.get(s) ?? 0) + l.monthlyEur);
    }
    return [...m.entries()].sort((a, b) => b[1] - a[1]);
  }, [lines]);
  const max = Math.max(1, ...byService.map(([, v]) => v));
  return (
    <Page title={t("Informes de facturación")} actions={<Tool icon="refresh" label={t("Actualizar")} onClick={ctx.refresh} />}>
      <div className="cc-grid">
        <div className="card">
          <div className="cc-help" style={{ margin: 0 }}>{t("Previsión de coste mensual")}</div>
          <div className="kpi">€{(data?.monthlyEur ?? 0).toFixed(2)}</div>
          <div className="cc-help">{t("Estimación con los recursos actuales del proyecto {p}.", { p: ctx.project })}</div>
        </div>
        <div className="card">
          <div className="cc-help" style={{ margin: 0 }}>{t("Servicios con coste")}</div>
          <div className="kpi">{byService.length}</div>
        </div>
      </div>
      <h2 className="cc-h2">{t("Coste por servicio")}</h2>
      <div className="card cc-bars">
        {byService.length ? (
          byService.map(([s, v]) => (
            <div key={s} className="cc-barrow">
              <span>{s}</span>
              <span className="cc-barwrap"><span style={{ width: `${(v / max) * 100}%` }} /></span>
              <span>€{v.toFixed(2)}</span>
            </div>
          ))
        ) : (
          <p className="cc-empty">{t("Todavía no hay costes.")}</p>
        )}
      </div>
      <h2 className="cc-h2">{t("Detalle por SKU")}</h2>
      <Table
        caption={t("Coste por recurso y SKU")}
        rows={lines}
        rowKey={(l) => `${l.resource}|${l.sku}`}
        empty={t("Todavía no hay costes.")}
        cols={[
          { key: "r", label: t("Recurso"), render: (l) => <code>{l.resource}</code> },
          { key: "sku", label: "SKU", render: (l) => l.sku },
          { key: "eur", label: t("€/mes"), render: (l) => l.monthlyEur.toFixed(2) },
        ]}
      />
    </Page>
  );
}

// ---------------------------------------------------------------- Topology

export function Topology({ ctx }: { ctx: Ctx }) {
  const { t } = useI18n();
  const { data } = useView<{ mermaid: string }>(ctx, "topology");
  return (
    <Page title={t("Topología de red")} intro={t("Recursos del proyecto y cómo se conectan; las conexiones rotas aparecen marcadas.")} actions={<Tool icon="refresh" label={t("Actualizar")} onClick={ctx.refresh} />}>
      <div className="card">{data?.mermaid ? <Mermaid chart={data.mermaid} /> : <p className="cc-empty">{t("Todavía no hay recursos.")}</p>}</div>
    </Page>
  );
}

// ---------------------------------------------------------------- Activity

export function Activity({ ctx }: { ctx: Ctx }) {
  const { t } = useI18n();
  const { data } = useView<any[]>(ctx, "history");
  const rows = (Array.isArray(data) ? data : []).map((r, i) => ({ ...r, i })).reverse();
  return (
    <Page title={t("Registro de actividad")} intro={t("Todo lo que se ha ejecutado en este proyecto, desde la consola o desde Cloud Shell.")}>
      <Table
        caption={t("Comandos que has ejecutado, con su salida")}
        rows={rows}
        rowKey={(r: any) => String(r.i)}
        empty={t("Todavía no hay actividad.")}
        cols={[
          { key: "st", label: t("Estado"), render: (r: any) => (r.exit ? <Pill tone="bad">{t("Error")}</Pill> : <Pill tone="ok">OK</Pill>) },
          { key: "at", label: t("Hora"), render: (r: any) => <span style={{ whiteSpace: "nowrap" }}>{String(r.at ?? "").replace("T", " ").replace(/\..*|Z$/, "")}</span> },
          {
            key: "cmd",
            label: t("Comando"),
            text: (r: any) => r.line,
            render: (r: any) => (
              <div>
                <code style={{ whiteSpace: "pre-wrap", wordBreak: "break-all" }}>{r.line}</code>
                {r.output ? (
                  <details>
                    <summary className="cc-help">{t("salida")}</summary>
                    <pre className="cc-pre">{r.output}</pre>
                  </details>
                ) : null}
              </div>
            ),
          },
        ]}
      />
    </Page>
  );
}
