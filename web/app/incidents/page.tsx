"use client";

import { useEffect, useMemo, useState } from "react";
import Nav from "@/components/Nav";
import Mermaid from "@/components/Mermaid";
import { api } from "@/lib/api";
import { useAuth } from "@/components/useAuth";
import { useI18n } from "@/lib/i18n";
import { label, layer } from "@/lib/labels";

interface Lib {
  symptoms: Record<string, { id: string; name: string; description: string; layers: string[] }>;
  contexts: Record<string, { id: string; company: string; service: string; impact: string; priority: string }>;
  systems: { ID: string; Title: string; Topology: string; Failures: { ID: string; Title: string; Symptom: string; Layer: string; Kind: string; Skills: string[] }[] }[];
  graph: { symptom: string; layer: string; failure: string; title: string; system: string }[];
}

export default function Incidents() {
  useAuth();
  const { t, lang } = useI18n();
  const [lib, setLib] = useState<Lib | null>(null);
  const [system, setSystem] = useState("");
  const [symptom, setSymptom] = useState("");
  const [context, setContext] = useState("");
  const [difficulty, setDifficulty] = useState(2);
  const [mode, setMode] = useState("");
  const [err, setErr] = useState("");
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    api<Lib>("/api/failures")
      .then((l) => {
        setLib(l);
        setSystem(l.systems?.[0]?.ID ?? "");
      })
      .catch((e) => setErr(e.message));
  }, [lang]);

  const sys = lib?.systems.find((s) => s.ID === system);
  const symptomsForSystem = useMemo(() => [...new Set(sys?.Failures.map((f) => f.Symptom) ?? [])], [sys]);

  const graph = useMemo(() => {
    if (!lib) return "";
    const esc = (s: string) => s.replace(/["[\]]/g, "'");
    const lines = ["flowchart LR"];
    const edges = lib.graph.filter((e) => !system || e.system === system);
    const seen = new Set<string>();
    for (const e of edges) {
      const s = `S_${e.symptom.replace(/\W/g, "_")}`;
      const l = `L_${e.symptom.replace(/\W/g, "_")}_${e.layer.replace(/\W/g, "_")}`;
      if (!seen.has(s)) {
        lines.push(`  ${s}(["${esc(lib.symptoms[e.symptom]?.name ?? e.symptom)}"])`);
        seen.add(s);
      }
      if (!seen.has(l)) {
        lines.push(`  ${l}["${esc(label(layer, e.layer, t))}"]`);
        lines.push(`  ${s} --> ${l}`);
        seen.add(l);
      }
    }
    return lines.join("\n");
  }, [lib, system, t]);

  const generate = async () => {
    setBusy(true);
    setErr("");
    try {
      const s = await api<{ id: string; labId: string }>("/api/incidents", {
        body: { system, symptom: symptom || undefined, context: context || undefined, difficulty, mode: mode || undefined },
      });
      window.location.href = `/lab?session=${s.id}&id=${encodeURIComponent(s.labId)}`;
    } catch (e: any) {
      setErr(e.message);
      setBusy(false);
    }
  };

  return (
    <>
      <Nav active="/incidents" />
      <main id="main" className="page">
        <h1>{t("Generador de incidentes")}</h1>
        <p className="muted">
          {t(
            "Los incidentes se componen a partir de una biblioteca de fallos: un sistema, uno o varios modos de fallo, un contexto de negocio y una dificultad. Con más dificultad los tickets son vagos o engañosos, se acumulan varios fallos y el modo producción activa una sala de crisis P1. Nunca verás qué fallo se ha inyectado.",
          )}
        </p>
        {err && <p className="error">{err}</p>}
        {lib && (
          <div className="grid two">
            <div className="card col">
              <div>
                <label htmlFor="sys">{t("Sistema")}</label>
                <select id="sys" value={system} onChange={(e) => { setSystem(e.target.value); setSymptom(""); }}>
                  {lib.systems.map((s) => <option key={s.ID} value={s.ID}>{s.Title}</option>)}
                </select>
              </div>
              <div>
                <label htmlFor="sym">{t("Síntoma")}</label>
                <select id="sym" value={symptom} onChange={(e) => setSymptom(e.target.value)}>
                  <option value="">{t("Cualquiera (sorpréndeme)")}</option>
                  {symptomsForSystem.map((s) => <option key={s} value={s}>{lib.symptoms[s]?.name ?? s}</option>)}
                </select>
              </div>
              <div>
                <label htmlFor="ctx">{t("Contexto de negocio")}</label>
                <select id="ctx" value={context} onChange={(e) => setContext(e.target.value)}>
                  <option value="">{t("Aleatorio")}</option>
                  {Object.values(lib.contexts).map((c) => <option key={c.id} value={c.id}>{c.company} — {c.service} ({c.priority})</option>)}
                </select>
              </div>
              <div>
                <label htmlFor="dif">{t("Dificultad: {n}", { n: difficulty })}</label>
                <input id="dif" type="range" min={1} max={5} value={difficulty} onChange={(e) => setDifficulty(+e.target.value)} />
              </div>
              <div>
                <label htmlFor="mode">{t("Modo")}</label>
                <select id="mode" value={mode} onChange={(e) => setMode(e.target.value)}>
                  <option value="">{t("Ticket estándar")}</option>
                  <option value="production">{t("Producción (sala de crisis P1)")}</option>
                  <option value="unknown">{t("Problema desconocido (solo se conoce el impacto)")}</option>
                </select>
              </div>
              <button className="btn" onClick={generate} disabled={busy || !system}>
                {busy ? t("Preparando el entorno…") : t("Generar incidente")}
              </button>
            </div>
            <div className="card">
              <h3>
                {t("Grafo de síntomas")}
                {sys ? ` — ${sys.Title}` : ""}
              </h3>
              <p className="small muted">{t("Dónde buscar para cada síntoma, capa por capa.")}</p>
              {graph && <Mermaid chart={graph} />}
            </div>
          </div>
        )}
      </main>
    </>
  );
}
