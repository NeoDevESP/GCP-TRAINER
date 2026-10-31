// The 10-day plan (about 4 hours a day), shared by the plan page and the
// dashboard's "today" card.

import { k } from "@/lib/i18n";
import { loadHistory, loadProgress, loadRun, type ExamRecord, type Progress } from "@/lib/study";
import type { LabSummary } from "@/lib/types";

export type Item = { lab: string } | { topic: string } | { exam: string } | { shift: number } | { task: string; text: string; href?: string };
export interface Day {
  title: string;
  goal: string;
  items: Item[];
}

const L = (...ids: string[]) => ids.map((lab) => ({ lab }));

const Z = (...n: string[]) => n.map((x) => ({ lab: "zentra-" + x }));
const T = Z; // week 0 and week 1 workshops share the zentra- prefix

export const DAYS: Day[] = [
  { title: k("Día 1 · Desde cero"), goal: k("Si nunca has tocado la nube: qué es Google Cloud, la terminal, redes, permisos, almacenamiento, registros y Linux, con un taller guiado para cada cosa."),
    items: [{ topic: "gcp-console" }, ...T("t01-what-is-cloud", "t02-terminal", "t03-gcloud-read", "t04-networks", "t05-firewall", "t06-permissions", "t07-storage", "t08-logs", "t09-linux", "t10-week0-test")] },
  { title: k("Día 2 · El oficio de soporte"), goal: k("Cómo se trabaja un ticket en un banco: prioridades, primer contacto, escribir al cliente, escalar a otro equipo y a Google, un P1, cambios, decir que no y el postmortem."),
    items: [{ topic: "support" }, { topic: "support-craft" }, ...T("s01-triage", "s02-first-contact", "s03-writing", "s04-escalate-team", "s05-google-case", "s06-p1-incident", "s07-change", "s08-say-no", "s09-postmortem", "s10-week1-test")] },
  { title: k("Día 3 · Bienvenida a Zentra: el proyecto, los registros y las VMs privadas"), goal: k("Conocer el banco en el que trabajas, saber quién cambió qué y entrar a una VM sin exponerla."),
    items: [{ topic: "support-method" }, { task: "d1-handbook", text: k("Lee el manual de Zentra"), href: "/zentra" }, { topic: "security" }, ...Z("z01-first-day", "z02-who-is-who", "z53-first-ticket", "z03-log-detective", "z04-private-vm", "z05-private-run", "z06-secrets", "z07-noc-alert", "z54-week1-test")] },
  { title: k("Día 4 · Accesos: altas, cambios y bajas"), goal: k("Dar y quitar permisos como en un banco: siempre por grupos."),
    items: [{ topic: "support-identity" }, ...Z("z08-finops-basics", "z09-joiner", "z10-mover", "z11-urgent-leaver", "z55-service-account"), ...L("bank-b01-access-request", "bank-b12-leaver-joiner")] },
  { title: k("Día 5 · Externos, APIs y auditoría"), goal: k("Peticiones delicadas: consultores, APIs nuevas, auditores y grupos abiertos."),
    items: [{ topic: "support-google" }, ...Z("z12-external-consultant", "z13-enable-api", "z14-auditors", "z15-external-member", "z40-api-disabled", "z49-temporary-access", "z56-access-review"), ...L("ace-d13-lb-dns", "ace-d14-terraform", "bank-b15-sa-key-request"),
      { task: "d5-repeat", text: k("Repite z01, z02 y z03 sin tutor ni pistas") }] },
  { title: k("Día 6 · El flujo de pagos: Pub/Sub y BigQuery"), goal: k("Permisos entre servicios: publicar, entregar y escribir."),
    items: [{ topic: "support-data" }, ...Z("z16-publish-denied", "z17-push-backlog", "z18-bq-denied", "z31-bq-location", "z35-sql-connections", "z37-private-google-access", "z51-slow-query"), ...L("ace-d18-api-db-firewall", "ace-d20-sql-saturated", "bank-b05-run-403", "bank-b25-wrong-identity")] },
  { title: k("Día 7 · GKE y balanceo"), goal: k("Pods que no arrancan, identidades de cargas y el balanceador."),
    items: [{ topic: "gke" }, { topic: "support-compute" }, { topic: "support-network" }, ...Z("z19-wi-namespace", "z20-armor-blocks-customers", "z33-redis-network", "z34-hpa-unknown", "z41-readiness-probe"), ...L("ace-d16-gke-two-services", "ace-d17-gke-hpa", "bank-b19-gke-oomkilled", "bank-b24-gke-imagepull")] },
  { title: k("Día 8 · Seguridad e incidentes graves"), goal: k("Despliegues rotos, claves filtradas, perímetros de VPC Service Controls, claves de cifrado propias y políticas de denegación."),
    items: [{ topic: "support-governance" }, { topic: "support-banking" }, ...Z("z21-bad-release", "z22-leaked-key", "z47-public-kyc-bucket", "z50-retention-lock", "z59-week3-test", "z71-vpcsc-ingress", "z72-vpcsc-dryrun", "z73-cmek-agent", "z74-cmek-version", "z77-deny-policy"), ...L("ace-d25-public-exposure", "bank-b13-audit-sink")] },
  { title: k("Día 9 · Guardia"), goal: k("Incidentes de producción con el reloj corriendo, la conexión con el CPD y varios tickets a la vez."),
    items: [...Z("z23-push-endpoint", "z24-healthcheck-path", "z36-disk-grow", "z44-listen-localhost", "z48-key-permissions", "z25-fx-no-egress", "z26-secret-version", "z28-nat-ports", "z29-zone-stockout", "z30-cpu-quota", "z38-firewall-priority", "z39-private-dns", "z43-default-route", "z45-lost-tag", "z46-named-port", "z67-week5-test", "z75-bgp-asn", "z76-dns-forwarding", "z78-vendor-static-ip"), { shift: 1 }, { shift: 2 }] },
  { title: k("Día 10 · Día de la nómina y examen"), goal: k("El jefe final, el simulacro del ACE y un último turno."),
    items: [...Z("z27-payday-boss", "z42-month-end-costs", "z69-week6-test", "z70-canary", "z52-zone-outage", "z79-terraform-drift", "z80-week9-test", "z32-sql-ha"), { exam: "ACE" }, { shift: 3 }, { task: "d10-review", text: k("Repasa los temas con las preguntas que fallaste en el simulacro") }, { task: "d10-final", text: k("Examen final de Zentra: la certificación interna de Soporte N2"), href: "/final" }] },
];

export const key = (it: Item) => ("lab" in it ? "lab:" + it.lab : "topic" in it ? "topic:" + it.topic : "exam" in it ? "exam:" + it.exam : "shift" in it ? "shift:" + it.shift : "task:" + it.task);


export interface PlanState {
  labs: Record<string, LabSummary>;
  state: Record<string, string>;
  prog: Progress;
  hist: ExamRecord[];
  shifts: number;
  checked: Record<string, boolean>;
}

// planState reads what the plan needs from the study store (already synced).
export const planLocal = () => ({
  prog: loadProgress(),
  hist: loadHistory(),
  checked: loadRun<{ checked: Record<string, boolean> }>("cm-plan")?.checked ?? {},
  shifts: loadRun<{ history: unknown[] }>("cm-shift")?.history?.length ?? 0,
});

export function itemDone(it: Item, s: PlanState): boolean {
  if ("lab" in it) return s.state[it.lab] === "done";
  if ("topic" in it) return (s.prog[it.topic]?.best ?? 0) >= 70;
  if ("exam" in it) return s.hist.some((h) => h.cert === it.exam && h.full);
  if ("shift" in it) return s.shifts >= it.shift;
  return !!s.checked[it.task];
}

// planStatus lists the days with the labs that exist, how many items of each
// are done and which day is today (the first one not finished).
export function planStatus(s: PlanState) {
  const days = DAYS.map((d) => ({ ...d, items: d.items.filter((it) => !("lab" in it) || s.labs[it.lab]) }));
  const counts = days.map((d) => d.items.filter((it) => itemDone(it, s)).length);
  const total = days.reduce((a, d) => a + d.items.length, 0);
  const done = counts.reduce((a, b) => a + b, 0);
  const today = Math.max(0, days.findIndex((d, i) => counts[i] < d.items.length));
  return { days, counts, total, done, today };
}
