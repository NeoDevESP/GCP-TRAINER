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

type Item = { lab: string } | { topic: string } | { exam: string } | { shift: number } | { task: string; text: string };
interface Day {
  title: string;
  goal: string;
  items: Item[];
}

const L = (...ids: string[]) => ids.map((lab) => ({ lab }));

const DAYS: Day[] = [
  { title: k("Día 1 · Proyecto, IAM y almacenamiento"), goal: k("Orientarte en la consola y en el oficio: primer ticket de acceso."),
    items: [{ topic: "support" }, { topic: "support-method" }, ...L("ace-d01-context", "ace-d02-iam-storage-read", "ace-d03-secure-bucket", "bank-b01-access-request")] },
  { title: k("Día 2 · VMs, subredes y salida a internet"), goal: k("Redes privadas y las políticas de organización del banco."),
    items: [{ topic: "iam" }, ...L("ace-d04-web-vm", "ace-d05-subnet-isolation", "ace-d12-private-egress", "bank-b02-vm-policy", "bank-b03-no-egress", "bank-b12-leaver-joiner")] },
  { title: k("Día 3 · Cloud Run, Cloud SQL y secretos"), goal: k("Permisos entre servicios: el 403 y sus tres causas."),
    items: [{ topic: "security" }, ...L("ace-d06-cloud-run", "ace-d07-cloud-sql", "ace-d18-api-db-firewall", "bank-b05-run-403", "bank-b06-secret-access", "bank-b14-secret-rotation", "bank-b15-sa-key-request", "bank-b25-wrong-identity")] },
  { title: k("Día 4 · Observabilidad, balanceo y conectividad"), goal: k("Del síntoma (502, timeout) a la causa."),
    items: [{ topic: "support-network" }, ...L("ace-d08-logs-alert", "ace-d09-health-check", "ace-d11-app-403", "bank-b04-app-db", "bank-b21-lb-502-healthcheck", "bank-b27-run-sql-timeout")] },
  { title: k("Día 5 · Automatización y repaso"), goal: k("Terraform y CI/CD, y repetir sin tutor los primeros casos."),
    items: [{ topic: "support-google" }, ...L("ace-d13-lb-dns", "ace-d14-terraform", "ace-d15-cloud-build", "bank-b16-region-policy", "bank-b20-oslogin-ssh"),
      { task: "d5-repeat", text: k("Repite b01, b02 y b03 sin tutor ni pistas") }] },
  { title: k("Día 6 · GKE"), goal: k("Pods que no arrancan: CrashLoopBackOff, OOMKilled e ImagePullBackOff."),
    items: [{ topic: "gke" }, { topic: "support-compute" }, ...L("ace-d16-gke-two-services", "ace-d17-gke-hpa", "bank-b09-gke-crashloop", "bank-b19-gke-oomkilled", "bank-b24-gke-imagepull"),
      { task: "d6-repeat", text: k("Repite b04, b05 y b06 sin tutor") }] },
  { title: k("Día 7 · Seguridad y auditoría"), goal: k("Incidentes de seguridad, registros de auditoría y exposición pública."),
    items: [{ topic: "support-governance" }, ...L("ace-d19-secret-env", "ace-d24-cmek", "ace-d25-public-exposure", "bank-b07-public-bucket", "bank-b13-audit-sink", "bank-b22-sink-no-writer")] },
  { title: k("Día 8 · Datos, costes y caídas del proveedor"), goal: k("Bases de datos saturadas, facturas, discos llenos y caídas de Google."),
    items: [{ topic: "support-data" }, ...L("ace-d20-sql-saturated", "ace-d26-hybrid-design", "ace-d27-deploy-rollback", "bank-b08-region-outage", "bank-b10-sql-connections", "bank-b11-bill-spike", "bank-b17-disk-full", "bank-b26-provider-outage")] },
  { title: k("Día 9 · Turnos de soporte"), goal: k("Trabajar varios tickets a la vez, priorizando por impacto."),
    items: [...L("bank-b18-not-gcp", "bank-b23-priority-pressure"), { shift: 1 }, { shift: 2 },
      { task: "d9-solo", text: k("Haz los casos de banca que tengas por debajo de 80 sin tutor") }] },
  { title: k("Día 10 · Examen y repaso"), goal: k("Simulacro completo del ACE, repaso de fallos y un último turno."),
    items: [{ exam: "ACE" }, { shift: 3 }, { task: "d10-review", text: k("Repasa los temas con las preguntas que fallaste en el simulacro") }] },
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
    if ("lab" in it) return { text: labs[it.lab]?.title ?? it.lab, kind: it.lab.startsWith("bank-") ? t("Caso de soporte") : t("Laboratorio ACE"), href: labHref(it.lab, running[it.lab]) };
    if ("topic" in it) return { text: topics[it.topic]?.name ?? it.topic, kind: t("Tema · test ≥ 70"), href: `/learn?topic=${it.topic}` };
    if ("exam" in it) return { text: t("Simulacro completo del {cert}", { cert: it.exam }), kind: t("Examen"), href: `/learn?cert=${it.exam}&exam=full` };
    if ("shift" in it) return { text: t("Turno de soporte n.º {n}", { n: it.shift }), kind: t("Turno"), href: "/shift" };
    return { text: t(it.text), kind: t("Tarea"), href: "" };
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
                        <label className="pl-check">
                          <input type="checkbox" checked={ok} onChange={() => toggle(it.task)} />
                          <span>{lb.text}</span>
                        </label>
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
