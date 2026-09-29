"use client";

import { useEffect, useMemo, useState } from "react";
import Nav from "@/components/Nav";
import Mermaid from "@/components/Mermaid";
import Bar from "@/components/Bar";
import { api } from "@/lib/api";
import { useAuth } from "@/components/useAuth";
import type { Profile } from "@/lib/types";

interface Node {
  id: string;
  name: string;
  branch: string;
  requires?: string[];
  unlocks?: string[];
  depth: number;
}

export default function Skills() {
  useAuth();
  const [nodes, setNodes] = useState<Node[]>([]);
  const [p, setP] = useState<Profile | null>(null);
  const [branch, setBranch] = useState("");
  const [career, setCareer] = useState<any>(null);
  const [err, setErr] = useState("");
  useEffect(() => {
    api<Node[]>("/api/catalog/skills/graph").then(setNodes).catch((e) => setErr(e.message));
    api<Profile>("/api/me").then(setP).catch(() => {});
    api("/api/catalog/career").then(setCareer).catch(() => {});
  }, []);

  const mastery = useMemo(() => Object.fromEntries((p?.skills ?? []).map((s) => [s.skill, s.mastery])), [p]);
  const branches = useMemo(() => [...new Set(nodes.map((n) => n.branch))].sort(), [nodes]);
  // The full graph is too dense to read; open on one branch.
  useEffect(() => {
    if (branches.length && !branch) setBranch(branches.includes("iam") ? "iam" : branches[0]);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [branches]);

  const chart = useMemo(() => {
    const sel = nodes.filter((n) => !branch || n.branch === branch);
    const ids = new Set(sel.map((n) => n.id));
    // include direct prerequisites from other branches for context
    sel.forEach((n) => n.requires?.forEach((r) => ids.add(r)));
    const id = (s: string) => s.replace(/\W/g, "_");
    const lines = ["flowchart LR", "  classDef done fill:#e6f4ea,stroke:#188038", "  classDef mid fill:#fef7e0,stroke:#b06000", "  classDef ext stroke-dasharray:4"];
    for (const n of nodes.filter((n) => ids.has(n.id))) {
      const m = mastery[n.id] ?? 0;
      lines.push(`  ${id(n.id)}["${n.name.replace(/"/g, "'")}${m ? ` · ${Math.round(m)}` : ""}"]`);
      if (m >= 80) lines.push(`  class ${id(n.id)} done`);
      else if (m > 0) lines.push(`  class ${id(n.id)} mid`);
      if (branch && n.branch !== branch) lines.push(`  class ${id(n.id)} ext`);
    }
    for (const n of nodes.filter((n) => ids.has(n.id))) for (const r of n.requires ?? []) if (ids.has(r)) lines.push(`  ${id(r)} --> ${id(n.id)}`);
    return lines.join("\n");
  }, [nodes, branch, mastery]);

  return (
    <>
      <Nav active="/skills" />
      <main className="page">
        <h1>Skill graph</h1>
        <p className="muted">Skills unlock in dependency order. Green is mastered (≥80), amber is in progress.</p>
        {err && <p className="error">{err}</p>}
        <div className="row" style={{ marginBottom: 12 }}>
          <select value={branch} onChange={(e) => setBranch(e.target.value)} style={{ width: 240 }} aria-label="Branch">
            <option value="">All branches ({nodes.length} skills)</option>
            {branches.map((b) => <option key={b}>{b}</option>)}
          </select>
        </div>
        <div className="card" style={{ overflow: "auto" }}>{nodes.length > 0 && <Mermaid chart={chart} />}</div>
        {p && (
          <div className="card" style={{ marginTop: 16 }}>
            <h2>Branch mastery</h2>
            <table>
              <tbody>
                {Object.entries(p.branches ?? {}).sort().map(([b, v]) => (
                  <tr key={b}><td style={{ width: 180 }}>{b}</td><td><Bar value={v} /></td><td className="small muted" style={{ width: 50 }}>{Math.round(v)}</td></tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
        {career && (
          <div className="card" style={{ marginTop: 16 }}>
            <h2>Career path</h2>
            <div className="grid three">
              {(career.stages ?? []).map((s: any, i: number) => (
                <div key={s.id} className="card" style={{ borderColor: p && p.career.stageIndex >= i ? "var(--ok)" : undefined }}>
                  <div className="muted small">Stage {i + 1}</div>
                  <strong>{s.name}</strong>
                  <p className="small muted">{s.scenario}</p>
                  {s.capstones?.length ? <div className="small">Capstones: {s.capstones.map((c: string) => <a key={c} href={`/lab?id=${encodeURIComponent(c)}`} style={{ marginRight: 6 }}>{c.split("/").pop()}</a>)}</div> : null}
                </div>
              ))}
            </div>
          </div>
        )}
      </main>
    </>
  );
}
