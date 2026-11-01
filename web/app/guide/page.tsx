"use client";

// Quick diagnosis guide: one card per symptom a GCP support engineer meets
// (the error text, what it means, the first commands, usual causes, the
// right fix and what not to do), searchable by error text, with links to
// the practice cases where it happens.

import { useEffect, useMemo, useState } from "react";
import Nav from "@/components/Nav";
import Markdown from "@/components/Markdown";
import { useAuth } from "@/components/useAuth";
import { StatusPill, labHref, useProgress } from "@/components/learn";
import { api, qs } from "@/lib/api";
import { k, useI18n } from "@/lib/i18n";

interface Runbook {
  id: string;
  area: string;
  symptom: string;
  errors: string[];
  means: string;
  steps: { text: string; cmd: string }[];
  causes: string[];
  fix: string;
  avoid: string;
  labs: { id: string; title: string; track: string }[];
}

const AREAS: [string, string][] = [
  ["", k("Todo")],
  ["access", k("Accesos")],
  ["network", k("Red")],
  ["compute", k("Cómputo y costes")],
  ["serverless", k("Cloud Run y secretos")],
  ["data", k("Datos")],
  ["gke", k("GKE")],
  ["security", k("Seguridad")],
];

// fold lowercases and strips accents so "conexion" finds "conexión".
const fold = (s: string) => s.toLowerCase().normalize("NFD").replace(/[̀-ͯ]/g, "");

function Copy({ text }: { text: string }) {
  const { t } = useI18n();
  const [ok, setOk] = useState(false);
  return (
    <button
      type="button"
      className="gd-copy"
      onClick={() => {
        navigator.clipboard?.writeText(text).then(() => {
          setOk(true);
          setTimeout(() => setOk(false), 1500);
        }).catch(() => {});
      }}
    >
      {ok ? t("Copiado") : t("Copiar")}
    </button>
  );
}

export default function GuidePage() {
  useAuth();
  const { t, lang } = useI18n();
  const { state, status, running } = useProgress();
  const [list, setList] = useState<Runbook[] | null>(null);
  const [q, setQ] = useState("");
  const [area, setArea] = useState("");
  const [open, setOpen] = useState<string>("");
  const [err, setErr] = useState("");

  useEffect(() => {
    if (qs("q")) setQ(qs("q"));
    if (qs("id")) setOpen(qs("id"));
  }, []);
  useEffect(() => {
    api<{ runbooks: Runbook[] }>("/api/runbooks").then((r) => setList(r.runbooks)).catch((e) => setErr(e.message));
  }, [lang]);
  useEffect(() => {
    if (open && list) document.getElementById("rb-" + open)?.scrollIntoView({ block: "start" });
  }, [open, list]);

  const shown = useMemo(() => {
    const words = fold(q).split(/\s+/).filter(Boolean);
    return (list ?? []).filter((r) => {
      if (area && r.area !== area) return false;
      if (!words.length) return true;
      const hay = fold([r.symptom, r.means, r.fix, ...r.errors, ...r.causes, ...r.steps.map((s) => s.text + " " + s.cmd)].join(" "));
      return words.every((w) => hay.includes(w));
    });
  }, [list, q, area]);

  return (
    <>
      <Nav active="/guide" />
      <main id="main" className="page">
        <header className="cm-head">
          <div>
            <h1>{t("Guía rápida de diagnóstico")}</h1>
            <p className="muted" style={{ margin: "6px 0 0", maxWidth: 720 }}>
              {t("Lo que tendrías abierto en tu primera guardia: cada síntoma con el error que ves, qué significa, los primeros comandos, las causas típicas, la solución buena y lo que no debes hacer. Pega el error en el buscador.")}
            </p>
          </div>
        </header>
        {err && <p className="error">{err}</p>}
        <div className="gd-tools">
          <label className="cm-search gd-search">
            <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" aria-hidden="true"><circle cx="11" cy="11" r="7" /><path d="M20 20l-3.5-3.5" /></svg>
            <span className="sr-only">{t("Buscar un síntoma o un error")}</span>
            <input type="search" value={q} onChange={(e) => setQ(e.target.value)} placeholder={t("Pega el error o describe el síntoma")} />
          </label>
          <div className="gd-areas" role="group" aria-label={t("Área")}>
            {AREAS.map(([id, name]) => (
              <button key={id} type="button" className={`res-skill${area === id ? " on" : ""}`} aria-pressed={area === id} onClick={() => setArea(id)}>
                {t(name)}
              </button>
            ))}
          </div>
        </div>
        {!list ? (
          <p className="muted">{t("Cargando…")}</p>
        ) : (
          <>
            <p className="muted small" aria-live="polite">{t("{n} síntomas", { n: shown.length })}</p>
            {shown.length === 0 && <p className="muted">{t("Nada coincide. Prueba con menos palabras o con el código del error (403, 502, QUOTA_EXCEEDED…).")}</p>}
            <ul className="gd-list">
              {shown.map((r) => (
                <li key={r.id} id={"rb-" + r.id}>
                  <details className="card gd-card" open={open === r.id || (!!q && shown.length <= 3)}>
                    <summary>
                      <span className="gd-area">{t(AREAS.find(([id]) => id === r.area)?.[1] ?? r.area)}</span>
                      <span className="gd-symptom">{r.symptom}</span>
                    </summary>
                    <div className="gd-body">
                      {r.errors.length > 0 && (
                        <div className="gd-errors">
                          {r.errors.map((e) => <code key={e}>{e}</code>)}
                        </div>
                      )}
                      <h3>{t("Qué significa")}</h3>
                      <Markdown text={r.means} />
                      <h3>{t("Primeros pasos")}</h3>
                      <ol className="gd-steps">
                        {r.steps.map((s, i) => (
                          <li key={i}>
                            <span>{s.text}</span>
                            {s.cmd && (
                              <span className="gd-cmd">
                                <code>{s.cmd}</code>
                                <Copy text={s.cmd} />
                              </span>
                            )}
                          </li>
                        ))}
                      </ol>
                      <h3>{t("Causas típicas")}</h3>
                      <ul className="gd-causes">
                        {r.causes.map((c) => <li key={c}>{c}</li>)}
                      </ul>
                      <div className="gd-fix">
                        <strong>{t("Solución")}</strong>
                        <Markdown text={r.fix} />
                      </div>
                      {r.avoid && (
                        <div className="gd-avoid">
                          <strong>{t("No lo hagas")}</strong>
                          <Markdown text={r.avoid} />
                        </div>
                      )}
                      {r.labs.length > 0 && (
                        <>
                          <h3>{t("Practícalo")}</h3>
                          <ul className="gd-labs">
                            {r.labs.map((l) => (
                              <li key={l.id}>
                                <a href={labHref(l.id, running[l.id])}>{l.title}</a>
                                <StatusPill status={status[l.id]?.status} />
                              </li>
                            ))}
                          </ul>
                        </>
                      )}
                    </div>
                  </details>
                </li>
              ))}
            </ul>
          </>
        )}
      </main>
    </>
  );
}
