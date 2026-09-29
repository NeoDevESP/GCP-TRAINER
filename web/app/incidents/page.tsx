"use client";

import { useEffect, useMemo, useState } from "react";
import Nav from "@/components/Nav";
import Mermaid from "@/components/Mermaid";
import { api } from "@/lib/api";
import { useAuth } from "@/components/useAuth";

interface Lib {
  symptoms: Record<string, { id: string; name: string; description: string; layers: string[] }>;
  contexts: Record<string, { id: string; company: string; service: string; impact: string; priority: string }>;
  systems: { ID: string; Title: string; Topology: string; Failures: { ID: string; Title: string; Symptom: string; Layer: string; Kind: string; Skills: string[] }[] }[];
  graph: { symptom: string; layer: string; failure: string; title: string; system: string }[];
}

export default function Incidents() {
  useAuth();
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
  }, []);

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
        lines.push(`  ${l}["${esc(e.layer)}"]`);
        lines.push(`  ${s} --> ${l}`);
        seen.add(l);
      }
    }
    return lines.join("\n");
  }, [lib, system]);

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
      <main className="page">
        <h1>Incident generator</h1>
        <p className="muted">
          Incidents are composed from a failure library: a system, one or more failure modes, a business context and a
          difficulty. At higher difficulty tickets get vague or misleading, several failures stack, and production mode runs
          a P1 war-room timeline. You never see which failure was injected.
        </p>
        {err && <p className="error">{err}</p>}
        {lib && (
          <div className="grid two">
            <div className="card col">
              <div>
                <label htmlFor="sys">System</label>
                <select id="sys" value={system} onChange={(e) => { setSystem(e.target.value); setSymptom(""); }}>
                  {lib.systems.map((s) => <option key={s.ID} value={s.ID}>{s.Title}</option>)}
                </select>
              </div>
              <div>
                <label htmlFor="sym">Symptom</label>
                <select id="sym" value={symptom} onChange={(e) => setSymptom(e.target.value)}>
                  <option value="">Any (surprise me)</option>
                  {symptomsForSystem.map((s) => <option key={s} value={s}>{lib.symptoms[s]?.name ?? s}</option>)}
                </select>
              </div>
              <div>
                <label htmlFor="ctx">Business context</label>
                <select id="ctx" value={context} onChange={(e) => setContext(e.target.value)}>
                  <option value="">Random</option>
                  {Object.values(lib.contexts).map((c) => <option key={c.id} value={c.id}>{c.company} — {c.service} ({c.priority})</option>)}
                </select>
              </div>
              <div>
                <label htmlFor="dif">Difficulty: {difficulty}</label>
                <input id="dif" type="range" min={1} max={5} value={difficulty} onChange={(e) => setDifficulty(+e.target.value)} />
              </div>
              <div>
                <label htmlFor="mode">Mode</label>
                <select id="mode" value={mode} onChange={(e) => setMode(e.target.value)}>
                  <option value="">Standard ticket</option>
                  <option value="production">Production (P1 war room)</option>
                  <option value="unknown">Unknown problem (only the impact is known)</option>
                </select>
              </div>
              <button className="btn" onClick={generate} disabled={busy || !system}>
                {busy ? "Building the environment…" : "Generate incident"}
              </button>
            </div>
            <div className="card">
              <h3>Symptom graph{sys ? ` — ${sys.Title}` : ""}</h3>
              <p className="small muted">Where to look for each symptom, layer by layer.</p>
              {graph && <Mermaid chart={graph} />}
            </div>
          </div>
        )}
      </main>
    </>
  );
}
