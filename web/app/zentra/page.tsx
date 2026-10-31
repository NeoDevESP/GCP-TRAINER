"use client";

// Zentra: the handbook of the fictional digital bank where the "zentra"
// route happens. Who they are, teams and people, identity, the Google Cloud
// organization, the architecture, the network, the house rules and how
// support works, with links to start the route or a support shift.

import { useEffect, useMemo, useState } from "react";
import Nav from "@/components/Nav";
import Mermaid from "@/components/Mermaid";
import Markdown from "@/components/Markdown";
import { useAuth } from "@/components/useAuth";
import { labHref, nextLab, useProgress } from "@/components/learn";
import { api } from "@/lib/api";
import { useI18n } from "@/lib/i18n";
import type { LabSummary } from "@/lib/types";

interface Card {
  title: string;
  text: string;
  meta: string;
}
interface Section {
  id: string;
  title: string;
  body: string;
  diagram: string;
  cards: Card[];
}
interface World {
  id: string;
  name: string;
  tagline: string;
  intro: string;
  track: string;
  facts: { value: string; label: string }[];
  sections: Section[];
  people: { name: string; email: string; role: string; team: string; about: string }[];
  glossary: { term: string; def: string }[];
}

export default function ZentraPage() {
  useAuth();
  const { t, lang } = useI18n();
  const [w, setW] = useState<World | null>(null);
  const [labs, setLabs] = useState<LabSummary[]>([]);
  const [err, setErr] = useState("");
  const { state, running } = useProgress();

  useEffect(() => {
    api<World>("/api/worlds/zentra").then(setW).catch((e) => setErr(e.message));
    api<{ labs: LabSummary[] }>("/api/catalog").then((r) => setLabs(r.labs.filter((l) => l.track === "zentra"))).catch(() => {});
  }, [lang]);

  const ids = useMemo(() => [...labs].sort((a, b) => a.day - b.day).map((l) => l.id), [labs]);
  const next = nextLab(ids, state);
  const done = ids.filter((id) => state[id] === "done").length;

  return (
    <>
      <Nav active="/zentra" />
      <main id="main" className="page">
        {err && <p className="error">{err}</p>}
        {!w ? (
          <p className="muted">{t("Cargando…")}</p>
        ) : (
          <>
            <header className="zt-hero">
              <div className="zt-logo" aria-hidden="true">Z</div>
              <div style={{ flex: 1, minWidth: 0 }}>
                <p className="zt-kicker">{t("Manual de empresa")}</p>
                <h1>{w.name}</h1>
                <p className="zt-tagline">{w.tagline}</p>
              </div>
              <div className="zt-cta">
                {next && (
                  <a className="cm-pillbtn ink sm" href={labHref(next, running[next])}>
                    {done ? t("Seguir con el caso siguiente") : t("Empezar el primer día")}
                  </a>
                )}
                <a className="btn secondary" href={`/catalog?track=${w.track}`}>{t("Ver los {n} casos", { n: ids.length })}</a>
                <a className="btn secondary" href="/shift">{t("Turno de soporte")}</a>
                <a className="btn secondary" href="/guide">{t("Guía de diagnóstico")}</a>
                {ids.length > 0 && <span className="muted zt-progress">{t("{done} de {total} casos superados", { done, total: ids.length })}</span>}
              </div>
            </header>
            <div className="zt-intro card">
              <Markdown text={w.intro} />
            </div>
            <ul className="zt-facts" aria-label={t("Zentra en cifras")}>
              {w.facts.map((f) => (
                <li key={f.label}>
                  <strong>{f.value}</strong>
                  <span>{f.label}</span>
                </li>
              ))}
            </ul>
            <div className="zt-layout">
              <nav className="zt-toc" aria-label={t("Capítulos del manual")}>
                <ol>
                  {w.sections.map((s) => (
                    <li key={s.id}><a href={`#${s.id}`}>{s.title}</a></li>
                  ))}
                  <li><a href="#personas">{t("Personas")}</a></li>
                  <li><a href="#glosario">{t("Glosario")}</a></li>
                </ol>
              </nav>
              <div className="zt-body">
                {w.sections.map((s) => (
                  <section key={s.id} id={s.id} className="zt-section">
                    <h2>{s.title}</h2>
                    <Markdown text={s.body} />
                    {s.diagram && (
                      <figure className="zt-diagram card">
                        <Mermaid chart={s.diagram} />
                      </figure>
                    )}
                    {s.cards.length > 0 && (
                      <ul className="zt-cards">
                        {s.cards.map((c) => (
                          <li key={c.title} className="card">
                            <h3>{c.title}</h3>
                            <p>{c.text}</p>
                            {c.meta && <span className="zt-meta">{c.meta}</span>}
                          </li>
                        ))}
                      </ul>
                    )}
                  </section>
                ))}
                <section id="personas" className="zt-section">
                  <h2>{t("Personas")}</h2>
                  <p className="muted">{t("Te las encontrarás en los tickets. Pregúntales con ask ROL \"…\" desde el terminal del caso.")}</p>
                  <ul className="zt-people">
                    {w.people.map((p) => (
                      <li key={p.email} className="card">
                        <span className="zt-avatar" aria-hidden="true">{p.name.split(" ").map((x) => x[0]).slice(0, 2).join("")}</span>
                        <div>
                          <strong>{p.name}</strong>
                          <div className="muted zt-role">{p.role} · {p.team}</div>
                          <p>{p.about}</p>
                          <code className="zt-email">{p.email}</code>
                        </div>
                      </li>
                    ))}
                  </ul>
                </section>
                <section id="glosario" className="zt-section">
                  <h2>{t("Glosario")}</h2>
                  <dl className="zt-glossary">
                    {w.glossary.map((g) => (
                      <div key={g.term}>
                        <dt>{g.term}</dt>
                        <dd>{g.def}</dd>
                      </div>
                    ))}
                  </dl>
                </section>
              </div>
            </div>
          </>
        )}
      </main>
    </>
  );
}
