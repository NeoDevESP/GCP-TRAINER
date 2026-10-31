"use client";

// The 10-day plan (about 4 hours a day) to start working in GCP platform
// support: each day mixes ACE labs, bank support cases, study topics, shifts
// and the practice exam. Labs, topics, shifts and exams tick themselves from
// the learner's progress; the few manual tasks are ticked by hand and saved
// in the account with the rest of the study progress.

import { useEffect, useMemo, useState } from "react";
import Nav from "@/components/Nav";
import { useAuth } from "@/components/useAuth";
import { labHref, useProgress } from "@/components/learn";
import { api } from "@/lib/api";
import { k, useI18n } from "@/lib/i18n";
import { loadRun, saveRun, syncStudy, type ExamRecord, type Progress, type Topic } from "@/lib/study";
import type { LabSummary } from "@/lib/types";

type Item = { lab: string } | { topic: string } | { exam: string } | { shift: number } | { task: string; text: string; href?: string };
interface Day {
  title: string;
  goal: string;
  items: Item[];
}

const L = (...ids: string[]) => ids.map((lab) => ({ lab }));

const Z = (...n: string[]) => n.map((x) => ({ lab: "zentra-" + x }));

const DAYS: Day[] = [
  { title: k("Día 1 · Bienvenida a Zentra: la consola y el proyecto"), goal: k("Entender cómo se organiza Google Cloud y el banco en el que trabajas."),
    items: [{ topic: "gcp-console" }, { task: "d1-handbook", text: k("Lee el manual de Zentra"), href: "/zentra" }, ...Z("z01-first-day", "z02-who-is-who"), ...L("ace-d01-context", "ace-d02-iam-storage-read", "ace-d03-secure-bucket")] },
  { title: k("Día 2 · Registros, VMs privadas e IAP"), goal: k("Saber quién cambió qué y entrar a una VM sin exponerla."),
    items: [{ topic: "support-method" }, ...Z("z03-log-detective", "z04-private-vm"), ...L("ace-d04-web-vm", "ace-d05-subnet-isolation", "ace-d12-private-egress", "bank-b03-no-egress")] },
  { title: k("Día 3 · Cloud Run, secretos y alertas"), goal: k("Microservicios privados, claves bien guardadas y avisos al NOC."),
    items: [{ topic: "security" }, ...Z("z05-private-run", "z06-secrets", "z07-noc-alert"), ...L("ace-d06-cloud-run", "ace-d07-cloud-sql", "ace-d08-logs-alert")] },
  { title: k("Día 4 · Accesos: altas, cambios y bajas"), goal: k("Dar y quitar permisos como en un banco: siempre por grupos."),
    items: [{ topic: "support-identity" }, { topic: "support" }, ...Z("z08-finops-basics", "z09-joiner", "z10-mover", "z11-urgent-leaver"), ...L("bank-b01-access-request", "bank-b12-leaver-joiner")] },
  { title: k("Día 5 · Externos, APIs y auditoría"), goal: k("Peticiones delicadas: consultores, APIs nuevas, auditores y grupos abiertos."),
    items: [{ topic: "support-google" }, ...Z("z12-external-consultant", "z13-enable-api", "z14-auditors", "z15-external-member"), ...L("ace-d13-lb-dns", "ace-d14-terraform", "bank-b15-sa-key-request"),
      { task: "d5-repeat", text: k("Repite z01, z02 y z03 sin tutor ni pistas") }] },
  { title: k("Día 6 · El flujo de pagos: Pub/Sub y BigQuery"), goal: k("Permisos entre servicios: publicar, entregar y escribir."),
    items: [{ topic: "support-data" }, ...Z("z16-publish-denied", "z17-push-backlog", "z18-bq-denied", "z31-bq-location"), ...L("ace-d18-api-db-firewall", "ace-d20-sql-saturated", "bank-b05-run-403", "bank-b25-wrong-identity")] },
  { title: k("Día 7 · GKE y balanceo"), goal: k("Pods que no arrancan, identidades de cargas y el balanceador."),
    items: [{ topic: "gke" }, { topic: "support-compute" }, { topic: "support-network" }, ...Z("z19-wi-namespace", "z20-armor-blocks-customers", "z33-redis-network", "z34-hpa-unknown"), ...L("ace-d16-gke-two-services", "ace-d17-gke-hpa", "bank-b19-gke-oomkilled", "bank-b24-gke-imagepull")] },
  { title: k("Día 8 · Seguridad e incidentes graves"), goal: k("Despliegues rotos, claves filtradas y respuesta a incidentes."),
    items: [{ topic: "support-governance" }, ...Z("z21-bad-release", "z22-leaked-key", "z32-sql-ha"), ...L("ace-d19-secret-env", "ace-d25-public-exposure", "ace-d27-deploy-rollback", "bank-b07-public-bucket", "bank-b13-audit-sink")] },
  { title: k("Día 9 · Guardia"), goal: k("Incidentes de producción con el reloj corriendo y varios tickets a la vez."),
    items: [...Z("z23-push-endpoint", "z24-healthcheck-path", "z25-fx-no-egress", "z26-secret-version", "z28-nat-ports", "z29-zone-stockout", "z30-cpu-quota"), ...L("bank-b08-region-outage", "bank-b18-not-gcp"), { shift: 1 }, { shift: 2 }] },
  { title: k("Día 10 · Día de la nómina y examen"), goal: k("El jefe final, el simulacro del ACE y un último turno."),
    items: [...Z("z27-payday-boss"), { exam: "ACE" }, { shift: 3 }, { task: "d10-review", text: k("Repasa los temas con las preguntas que fallaste en el simulacro") }] },
];

const key = (it: Item) => ("lab" in it ? "lab:" + it.lab : "topic" in it ? "topic:" + it.topic : "exam" in it ? "exam:" + it.exam : "shift" in it ? "shift:" + it.shift : "task:" + it.task);

export default function PlanPage() {
  useAuth();
  const { t } = useI18n();
  const { state, running } = useProgress();
  const [labs, setLabs] = useState<Record<string, LabSummary>>({});
  const [topics, setTopics] = useState<Record<string, Topic>>({});
  const [prog, setProg] = useState<Progress>({});
  const [hist, setHist] = useState<ExamRecord[]>([]);
  const [shifts, setShifts] = useState(0);
  const [checked, setChecked] = useState<Record<string, boolean>>({});
  const [open, setOpen] = useState<number | null>(null);
  const [ready, setReady] = useState(false);

  useEffect(() => {
    Promise.all([
      api<{ labs: LabSummary[] }>("/api/catalog").then((r) => setLabs(Object.fromEntries(r.labs.map((l) => [l.id, l])))),
      api<{ topics: Topic[] }>("/api/learn").then((r) => setTopics(Object.fromEntries(r.topics.map((x) => [x.id, x])))),
      syncStudy().then((s) => {
        setProg(s.progress);
        setHist(s.history);
      }),
    ])
      .catch(() => {})
      .finally(() => {
        setChecked(loadRun<{ checked: Record<string, boolean> }>("cm-plan")?.checked ?? {});
        setShifts(loadRun<{ history: unknown[] }>("cm-shift")?.history?.length ?? 0);
        setReady(true);
      });
  }, []);

  const done = (it: Item): boolean => {
    if ("lab" in it) return state[it.lab] === "done";
    if ("topic" in it) return (prog[it.topic]?.best ?? 0) >= 70;
    if ("exam" in it) return hist.some((h) => h.cert === it.exam && h.full);
    if ("shift" in it) return shifts >= it.shift;
    return !!checked[it.task];
  };
  const toggle = (task: string) => {
    const c = { ...checked, [task]: !checked[task] };
    setChecked(c);
    saveRun("cm-plan", { checked: c });
  };

  const days = useMemo(() => DAYS.map((d) => ({ ...d, items: d.items.filter((it) => !("lab" in it) || labs[it.lab]) })), [labs]);
  const counts = days.map((d) => d.items.filter(done).length);
  const total = days.reduce((a, d) => a + d.items.length, 0);
  const allDone = counts.reduce((a, b) => a + b, 0);
  const today = Math.max(0, days.findIndex((d, i) => counts[i] < d.items.length));
  const shown = open ?? today;

  const label = (it: Item) => {
    if ("lab" in it) {
      const kind = it.lab.startsWith("zentra-") ? (labs[it.lab]?.type === "guided" ? t("Tutorial Zentra") : t("Caso Zentra")) : it.lab.startsWith("bank-") ? t("Caso Banco Atlas") : t("Laboratorio ACE");
      return { text: labs[it.lab]?.title ?? it.lab, kind, href: labHref(it.lab, running[it.lab]) };
    }
    if ("topic" in it) return { text: topics[it.topic]?.name ?? it.topic, kind: t("Tema · test ≥ 70"), href: `/learn?topic=${it.topic}` };
    if ("exam" in it) return { text: t("Simulacro completo del {cert}", { cert: it.exam }), kind: t("Examen"), href: `/learn?cert=${it.exam}&exam=full` };
    if ("shift" in it) return { text: t("Turno de soporte n.º {n}", { n: it.shift }), kind: t("Turno"), href: "/shift" };
    return { text: t(it.text), kind: t("Tarea"), href: it.href ?? "" };
  };

  return (
    <>
      <Nav active="/plan" />
      <main id="main" className="page">
        <header className="cm-head">
          <div>
            <h1>{t("Plan de 10 días")}</h1>
            <p className="muted" style={{ margin: "6px 0 0", maxWidth: 700 }}>
              {t("Unas 4 horas al día para llegar con soltura a un puesto de soporte GCP: cada día mezcla laboratorios del ACE, casos reales de soporte en banca y un tema de estudio. Las casillas se marcan solas al superar cada cosa.")}
            </p>
          </div>
          <div className="pl-total" aria-label={t("Progreso del plan")}>
            <strong>{total ? Math.round((100 * allDone) / total) : 0} %</strong>
            <span className="muted">{t("{done} de {total} hechos", { done: allDone, total })}</span>
          </div>
        </header>
        {!ready ? (
          <p className="muted">{t("Cargando…")}</p>
        ) : (
          <div className="pl-grid">
            <ol className="pl-days" aria-label={t("Días")}>
              {days.map((d, i) => (
                <li key={i}>
                  <button type="button" className={`pl-day${i === shown ? " active" : ""}${counts[i] === d.items.length ? " done" : ""}`} aria-current={i === shown ? "step" : undefined} onClick={() => setOpen(i)}>
                    <span className="pl-num">{counts[i] === d.items.length ? "✓" : i + 1}</span>
                    <span className="pl-dtitle">{t(d.title)}</span>
                    <span className="pl-meter" aria-hidden="true"><span style={{ width: `${(100 * counts[i]) / Math.max(1, d.items.length)}%` }} /></span>
                  </button>
                </li>
              ))}
            </ol>
            <section className="card pl-detail">
              <h2>{t(days[shown].title)}</h2>
              <p className="muted">{t(days[shown].goal)} · {t("≈ 4 h")}{shown === today ? " · " + t("toca hoy") : ""}</p>
              <ul className="pl-items">
                {days[shown].items.map((it) => {
                  const lb = label(it);
                  const ok = done(it);
                  return (
                    <li key={key(it)} className={ok ? "done" : ""}>
                      {"task" in it ? (
                        <span className="pl-check">
                          <input type="checkbox" id={"pl-" + it.task} checked={ok} onChange={() => toggle(it.task)} />
                          <label htmlFor={"pl-" + it.task}>{lb.text}</label>
                          {lb.href && <a href={lb.href} className="pl-open">{t("Abrir")}</a>}
                        </span>
                      ) : (
                        <a href={lb.href} className="pl-check">
                          <span className="pl-box" aria-hidden="true">{ok ? "✓" : ""}</span>
                          <span>{lb.text}</span>
                          <span className="sr-only">{ok ? t("hecho") : t("pendiente")}</span>
                        </a>
                      )}
                      <span className="pl-kind">{lb.kind}</span>
                    </li>
                  );
                })}
              </ul>
            </section>
          </div>
        )}
      </main>
    </>
  );
}
