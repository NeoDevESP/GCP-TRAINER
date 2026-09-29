// Human labels for identifiers the API returns (Spanish; t() translates them).
import { k } from "./i18n";

export const recKind: Record<string, string> = {
  next: k("siguiente"),
  remediation: k("refuerzo"),
  retest: k("repetición"),
  mock: k("simulacro"),
  review: k("repaso"),
  stealth: k("incidente sorpresa"),
  dimension: k("dimensión"),
  frontier: k("nuevo"),
};

export const dimension: Record<string, string> = {
  knowledge: k("Conocimiento"),
  execution: k("Ejecución"),
  troubleshooting: k("Resolución de problemas"),
  architecture: k("Arquitectura"),
  security: k("Seguridad"),
  cost: k("Coste"),
  operations: k("Operaciones"),
  autonomy: k("Autonomía"),
  retention: k("Retención"),
};

export const labLevel: Record<string, string> = {
  basic: k("básico"),
  intermediate: k("intermedio"),
  advanced: k("avanzado"),
  professional: k("profesional"),
};

export const labType: Record<string, string> = {
  guided: k("guiado"),
  challenge: k("reto"),
  incident: k("incidente"),
  quiz: k("test"),
  capstone: k("proyecto final"),
  boss: k("jefe final"),
  "case-study": k("caso práctico"),
  interview: k("entrevista"),
};

export const labMode: Record<string, string> = {
  production: k("producción"),
  unknown: k("problema desconocido"),
  career: k("carrera"),
  architecture: k("arquitectura"),
  interview: k("entrevista"),
  learn: k("aprendizaje"),
  lab: k("laboratorio"),
};

export const tab: Record<string, string> = {
  desk: k("ticket"),
  console: k("consola"),
  topology: k("topología"),
  logs: k("registros"),
  metrics: k("métricas"),
  iam: k("IAM"),
  cost: k("coste"),
  files: k("archivos"),
  history: k("historial"),
};

export const factor: Record<string, string> = {
  resolution: k("resolución"),
  diagnosis: k("diagnóstico"),
  security: k("seguridad"),
  cost: k("coste"),
  efficiency: k("eficiencia"),
  risk: k("riesgo"),
  communication: k("comunicación"),
  documentation: k("documentación"),
  autonomy: k("autonomía"),
};

export const ticketStatus: Record<string, string> = {
  NEW: k("NUEVO"),
  IN_PROGRESS: k("EN CURSO"),
  RESOLVED: k("RESUELTO"),
  ESCALATED: k("ESCALADO"),
};

export const role: Record<string, string> = {
  developer: k("desarrollo"),
  security: k("seguridad"),
  finance: k("finanzas"),
  manager: k("dirección"),
  customer: k("cliente"),
  sre: k("SRE"),
  ops: k("operaciones"),
};

/** label returns the translated label of an id, or the id itself. */
export function label(map: Record<string, string>, id: string | undefined, t: (s: string) => string): string {
  if (!id) return "";
  return map[id] ? t(map[id]) : id;
}

export const evidenceField: Record<string, string> = {
  rootCause: k("Causa raíz"),
  fix: k("Corrección"),
  verification: k("Verificación"),
  prevention: k("Prevención"),
  postmortem: k("Post-mortem"),
  explanation: k("Explicación"),
  design: k("Decisiones de diseño"),
  rolloutPlan: k("Plan de despliegue"),
};

export const validator: Record<string, string> = {
  functional: k("funcional"),
  state: k("estado"),
  security: k("seguridad"),
  diagnosis: k("diagnóstico"),
  cost: k("coste"),
  reliability: k("fiabilidad"),
  evidence: k("evidencia"),
  quiz: k("preguntas"),
};

export const hintKind: Record<string, string> = {
  hint: k("pista"),
  partial: k("pista parcial"),
  solution: k("solución"),
};

/** constraint turns a lab constraint code into a sentence. */
export function constraint(c: string, t: (s: string, v?: Record<string, string | number>) => string): string {
  if (c.startsWith("protect:")) return t("No modifiques {name} (pertenece a otro equipo)", { name: c.slice(8) });
  const m: Record<string, string> = {
    "no-downtime": k("Sin interrupción del servicio"),
    "no-public": k("Nada público (sin allUsers ni allAuthenticatedUsers)"),
    "no-basic-roles": k("Sin roles básicos (Owner, Editor, Viewer)"),
    "no-delete": k("No borres recursos"),
  };
  return m[c] ? t(m[c]) : c;
}

/** causal layers of the symptom graph */
export const layer: Record<string, string> = {
  dns: k("DNS"),
  "load-balancer": k("balanceador de carga"),
  "backend-health": k("salud de los backends"),
  firewall: k("cortafuegos"),
  timeout: k("tiempos de espera"),
  application: k("aplicación"),
  "k8s-readiness": k("preparación de pods (readiness)"),
  "k8s-workload": k("carga de trabajo de Kubernetes"),
  "k8s-service": k("Service de Kubernetes"),
  release: k("versión desplegada"),
  configuration: k("configuración"),
  identity: k("identidad y permisos"),
  "network-path": k("camino de red"),
  database: k("base de datos"),
  quota: k("cuotas"),
  capacity: k("capacidad"),
  "connection-pool": k("pool de conexiones"),
  autoscaling: k("autoescalado"),
  "storage-iam": k("permisos de almacenamiento"),
  "service-iam": k("permisos del servicio"),
  "org-policy": k("políticas de organización"),
  compute: k("cómputo"),
  storage: k("almacenamiento"),
  "network-egress": k("tráfico de salida"),
  "data-processing": k("procesamiento de datos"),
  "idle-resources": k("recursos ociosos"),
  tags: k("etiquetas de red"),
  "health-check": k("comprobación de estado"),
};
