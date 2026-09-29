"use client";

import { useEffect, useMemo, useState } from "react";
import Nav from "@/components/Nav";
import { api } from "@/lib/api";
import { useAuth } from "@/components/useAuth";
import type { LabSummary } from "@/lib/types";

interface Catalog {
  labs: LabSummary[];
  tracks: { id: string; name: string; description?: string }[] | Record<string, any>;
  branches: any;
}

export default function CatalogPage() {
  useAuth();
  const [cat, setCat] = useState<Catalog | null>(null);
  const [track, setTrack] = useState("");
  const [q, setQ] = useState("");
  const [err, setErr] = useState("");
  useEffect(() => {
    api<Catalog>("/api/catalog").then(setCat).catch((e) => setErr(e.message));
  }, []);

  const tracks = useMemo(() => {
    const s = new Set<string>();
    cat?.labs.forEach((l) => s.add(l.track));
    return [...s].sort();
  }, [cat]);

  const labs = useMemo(
    () =>
      (cat?.labs ?? []).filter(
        (l) =>
          (!track || l.track === track) &&
          (!q || (l.title + " " + l.summary + " " + l.skills.join(" ")).toLowerCase().includes(q.toLowerCase())),
      ),
    [cat, track, q],
  );

  return (
    <>
      <Nav active="/catalog" />
      <main id="main" className="page">
        <h1>Lab catalog</h1>
        {err && <p className="error">{err}</p>}
        <div className="row" style={{ marginBottom: 16 }}>
          <select value={track} onChange={(e) => setTrack(e.target.value)} style={{ width: 200 }} aria-label="Track">
            <option value="">All tracks</option>
            {tracks.map((t) => (
              <option key={t}>{t}</option>
            ))}
          </select>
          <input placeholder="Search labs, skills…" value={q} onChange={(e) => setQ(e.target.value)} style={{ maxWidth: 360 }} />
          <span className="muted small">{labs.length} labs</span>
        </div>
        <div className="grid three">
          {labs.map((l) => (
            <a key={l.id} href={`/lab?id=${encodeURIComponent(l.id)}`} className="card" style={{ color: "inherit", textDecoration: "none" }}>
              <div className="row small muted">
                <span>{l.track}{l.day ? ` · day ${l.day}` : ""}</span>
                <span className="pill">{l.type}</span>
                <span className="pill">{l.level}</span>
              </div>
              <h3 style={{ margin: "6px 0" }}>{l.title}</h3>
              <p className="small muted" style={{ margin: 0 }}>{l.summary}</p>
              <div className="row small muted" style={{ marginTop: 8 }}>
                <span>{"★".repeat(Math.max(1, Math.min(5, l.difficulty)))}</span>
                <span>{l.minutes} min</span>
                <span>{l.fidelity?.join("/")}</span>
              </div>
            </a>
          ))}
        </div>
      </main>
    </>
  );
}
