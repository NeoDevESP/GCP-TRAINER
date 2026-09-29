"use client";

// CloudConsole reproduces the Google Cloud console for a lab session: top
// bar with project picker and search, product navigation menu, per-product
// sidebar, pages, a right-hand panel (the lab instructions, like an in-console
// tutorial) and Cloud Shell docked at the bottom, with its terminal and
// editor. Every button runs the equivalent command in Cloud Shell, where the
// learner sees it typed out with its output.

import { useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import { api } from "@/lib/api";
import { k, useI18n } from "@/lib/i18n";
import { Icon } from "./icons";
import * as P from "./pages";
import * as O from "./ops";
import * as M from "./more";

type Sub = { id: string; label: string; count?: (d: any) => number };
type Product = { id: string; label: string; icon: string; group: string; pages: Sub[] };

const n = (m: any) => (m ? (Array.isArray(m) ? m.length : Object.keys(m).length) : 0);

export const PRODUCTS: Product[] = [
  { id: "iam", label: k("IAM y administración"), icon: "person", group: k("Gestión"), pages: [{ id: "iam", label: k("IAM") }, { id: "sa", label: k("Cuentas de servicio"), count: (d) => n(d.serviceAccounts) }, { id: "roles", label: k("Roles"), count: (d) => n(d.customRoles) }, { id: "orgpolicies", label: k("Políticas de organización") }] },
  { id: "apis", label: k("APIs y servicios"), icon: "api", group: k("Gestión"), pages: [{ id: "apis", label: k("APIs y servicios habilitados") }] },
  { id: "billing", label: k("Facturación"), icon: "billing", group: k("Gestión"), pages: [{ id: "billing", label: k("Informes") }] },
  { id: "compute", label: k("Compute Engine"), icon: "compute", group: k("Computación"), pages: [{ id: "vm", label: k("Instancias de VM"), count: (d) => n(d.instances) }, { id: "templates", label: k("Plantillas de instancia"), count: (d) => n(d.instanceTemplates) }, { id: "groups", label: k("Grupos de instancias"), count: (d) => n(d.instanceGroups) }, { id: "healthchecks", label: k("Comprobaciones de estado"), count: (d) => n(d.healthChecks) }, { id: "disks", label: k("Discos"), count: (d) => n(d.disks) }, { id: "snapshots", label: k("Instantáneas"), count: (d) => n(d.snapshots) }] },
  { id: "gke", label: k("Kubernetes Engine"), icon: "gke", group: k("Computación"), pages: [{ id: "gke", label: k("Clústeres"), count: (d) => n(d.clusters) }, { id: "workloads", label: k("Cargas de trabajo") }, { id: "k8sservices", label: k("Servicios e Ingress") }, { id: "k8sconfig", label: k("Secretos y ConfigMaps") }] },
  { id: "run", label: k("Cloud Run"), icon: "run", group: k("Sin servidor"), pages: [{ id: "run", label: k("Servicios"), count: (d) => n(d.runServices) }] },
  { id: "storage", label: k("Cloud Storage"), icon: "storage", group: k("Almacenamiento"), pages: [{ id: "buckets", label: k("Buckets"), count: (d) => n(d.buckets) }] },
  { id: "sql", label: k("SQL"), icon: "sql", group: k("Bases de datos"), pages: [{ id: "sql", label: k("Instancias"), count: (d) => n(d.sqlInstances) }] },
  { id: "bigquery", label: k("BigQuery"), icon: "bigquery", group: k("Analíticas"), pages: [{ id: "bigquery", label: k("Estudio de BigQuery"), count: (d) => n(d.datasets) }] },
  { id: "pubsub", label: k("Pub/Sub"), icon: "pubsub", group: k("Analíticas"), pages: [{ id: "pubsub", label: k("Temas y suscripciones"), count: (d) => n(d.topics) }] },
  { id: "vpc", label: k("Red de VPC"), icon: "network", group: k("Redes"), pages: [{ id: "vpc", label: k("Redes de VPC"), count: (d) => n(d.networks) }, { id: "addresses", label: k("Direcciones IP"), count: (d) => n(d.addresses) }, { id: "firewall", label: k("Cortafuegos"), count: (d) => n(d.firewalls) }, { id: "routes", label: k("Rutas") }] },
  { id: "netservices", label: k("Servicios de red"), icon: "network", group: k("Redes"), pages: [{ id: "lb", label: k("Balanceo de carga"), count: (d) => n(d.forwardingRules) }, { id: "dns", label: k("Cloud DNS"), count: (d) => n(d.dnsZones) }, { id: "nat", label: k("Cloud NAT") }] },
  { id: "nic", label: k("Network Intelligence Center"), icon: "network", group: k("Redes"), pages: [{ id: "topology", label: k("Topología de red") }] },
  { id: "secrets", label: k("Secret Manager"), icon: "lock", group: k("Seguridad"), pages: [{ id: "secrets", label: k("Secretos"), count: (d) => n(d.secrets) }] },
  { id: "kms", label: k("Gestión de claves"), icon: "key", group: k("Seguridad"), pages: [{ id: "kms", label: k("Llaveros"), count: (d) => n(d.keyRings) }] },
  { id: "artifacts", label: k("Artifact Registry"), icon: "storage", group: k("CI/CD"), pages: [{ id: "artifacts", label: k("Repositorios"), count: (d) => n(d.artifactRepos) }] },
  { id: "build", label: k("Cloud Build"), icon: "refresh", group: k("CI/CD"), pages: [{ id: "build", label: k("Historial y activadores"), count: (d) => n(d.builds) }] },
  { id: "logging", label: k("Logging"), icon: "logs", group: k("Operaciones"), pages: [{ id: "logs", label: k("Explorador de registros") }, { id: "logmetrics", label: k("Métricas basadas en registros"), count: (d) => n(d.logMetrics) }] },
  { id: "monitoring", label: k("Monitoring"), icon: "monitoring", group: k("Operaciones"), pages: [{ id: "metrics", label: k("Explorador de métricas") }, { id: "alerting", label: k("Alertas"), count: (d) => n(d.alertPolicies) }] },
  { id: "activity", label: k("Registro de actividad"), icon: "logs", group: k("Operaciones"), pages: [{ id: "activity", label: k("Actividad") }] },
];

export const productOf = (page: string) => PRODUCTS.find((p) => p.pages.some((s) => s.id === page));

type Toast = { ok: boolean; text: string; detail?: string; cmd: string; at: Date };

export default function CloudConsole({
  sessionId,
  project,
  region,
  zone,
  tick,
  run,
  paste,
  focusShell,
  aside,
  asideOpen,
  onAside,
  terminal,
  editor,
}: {
  sessionId: string;
  project: string;
  region: string;
  zone: string;
  tick: number;
  run: (cmd: string, note: string) => Promise<{ output: string; exit: number }>;
  paste: (cmd: string) => void;
  focusShell: () => void;
  /** right-hand panel (lab instructions) */
  aside: ReactNode;
  asideOpen: boolean;
  onAside: (open: boolean) => void;
  /** Cloud Shell terminal and editor, kept mounted */
  terminal: ReactNode;
  editor: ReactNode;
}) {
  const { t } = useI18n();
  const key = `gcplab.console.${sessionId}`;
  const [page, setPage] = useState("home");
  const [data, setData] = useState<any>(null);
  const [history, setHistory] = useState<any[]>([]);
  const [err, setErr] = useState("");
  const [toast, setToast] = useState<Toast | null>(null);
  const [notes, setNotes] = useState<Toast[]>([]);
  const [search, setSearch] = useState("");
  const [menu, setMenu] = useState(false);
  const [popup, setPopup] = useState<"" | "bell" | "help" | "project">("");
  const [creating, setCreating] = useState("");
  const [reload, setReload] = useState(0);
  const [shell, setShell] = useState<"open" | "min" | "max">("open");
  const [shellMode, setShellMode] = useState<"terminal" | "editor">("terminal");
  const [shellH, setShellH] = useState(300);
  const menuRef = useRef<HTMLDivElement>(null);
  const root = useRef<HTMLDivElement>(null);

  useEffect(() => {
    try {
      setPage(sessionStorage.getItem(key) || "home");
      const h = Number(localStorage.getItem("gcplab.shellHeight"));
      if (h > 120) setShellH(h);
    } catch {
      /* storage unavailable */
    }
  }, [key]);

  useEffect(() => {
    let live = true;
    api(`/api/sessions/${sessionId}/views/console`)
      .then((d) => {
        if (!live) return;
        setData(d ?? {});
        setErr("");
      })
      .catch((e) => live && setErr(e.message));
    if (page === "home") {
      api<any[]>(`/api/sessions/${sessionId}/views/history`)
        .then((h) => live && setHistory(Array.isArray(h) ? h : []))
        .catch(() => {});
    }
    return () => {
      live = false;
    };
  }, [sessionId, tick, page, reload]);

  useEffect(() => {
    if (!toast) return;
    const id = setTimeout(() => setToast(null), toast.ok ? 4000 : 9000);
    return () => clearTimeout(id);
  }, [toast]);

  useEffect(() => {
    if (!menu && !popup) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") {
        setMenu(false);
        setPopup("");
      }
    };
    window.addEventListener("keydown", onKey);
    if (menu) menuRef.current?.querySelector<HTMLElement>("button")?.focus();
    return () => window.removeEventListener("keydown", onKey);
  }, [menu, popup]);

  const go = (id: string) => {
    const prod = PRODUCTS.find((p) => p.id === id);
    const target = prod ? prod.pages[0].id : id;
    setMenu(false);
    setPopup("");
    setCreating("");
    setPage(target);
    try {
      sessionStorage.setItem(key, target);
    } catch {
      /* storage unavailable */
    }
  };

  const openShell = () => {
    if (shell === "min") setShell("open");
    setShellMode("terminal");
    setTimeout(focusShell, 50);
  };

  const runOut = async (cmd: string, note: string) => {
    if (shell === "min") setShell("open");
    setShellMode("terminal");
    const r = await run(cmd, note);
    const errLine = r.output.split("\n").find((l) => /^ERROR|error:/i.test(l.trim()));
    const tt: Toast = r.exit === 0 ? { ok: true, text: note, cmd, at: new Date() } : { ok: false, text: note, cmd, at: new Date(), detail: errLine ?? r.output.trim().split("\n").pop() };
    setToast(tt);
    setNotes((x) => [tt, ...x].slice(0, 30));
    return r;
  };
  const exec = async (cmd: string, note: string) => (await runOut(cmd, note)).exit === 0;
  const runAll = async (cmds: string[], note: string) => {
    for (const c of cmds) {
      if (!(await exec(c, note))) break;
    }
  };
  const pasteCmd = (cmd: string) => {
    if (shell === "min") setShell("open");
    setShellMode("terminal");
    setTimeout(() => paste(cmd), 50);
  };

  // Keyboard-free resize of Cloud Shell by dragging its top edge.
  const drag = (e: React.PointerEvent) => {
    const startY = e.clientY;
    const startH = shellH;
    const total = root.current?.clientHeight ?? 800;
    const move = (ev: PointerEvent) => setShellH(Math.min(total - 160, Math.max(140, startH + startY - ev.clientY)));
    const up = () => {
      window.removeEventListener("pointermove", move);
      window.removeEventListener("pointerup", up);
      setShellH((h) => {
        try {
          localStorage.setItem("gcplab.shellHeight", String(h));
        } catch {
          /* storage unavailable */
        }
        return h;
      });
    };
    window.addEventListener("pointermove", move);
    window.addEventListener("pointerup", up);
  };

  const ctx: P.Ctx | null = data
    ? { sessionId, tick: tick + reload, data, project, region, zone, run: exec, runAll, runOut, paste: pasteCmd, go, openTab: go, refresh: () => setReload((x) => x + 1), creating, setCreating }
    : null;

  const results = useMemo(() => {
    const s = search.trim().toLowerCase();
    if (!s) return [];
    const out: { id: string; label: string; icon: string; sub?: string }[] = [];
    for (const p of PRODUCTS) {
      if (t(p.label).toLowerCase().includes(s)) out.push({ id: p.id, label: t(p.label), icon: p.icon });
      else for (const sp of p.pages) if (t(sp.label).toLowerCase().includes(s)) out.push({ id: sp.id, label: t(sp.label), sub: t(p.label), icon: p.icon });
    }
    return out;
  }, [search, t]);

  const prod = productOf(page);

  const body = () => {
    if (!ctx) return <p className="cc-empty">{err || t("Cargando…")}</p>;
    switch (page) {
      case "iam": return <P.IAM ctx={ctx} />;
      case "sa": return <P.ServiceAccounts ctx={ctx} />;
      case "vm": return <P.Instances ctx={ctx} />;
      case "disks": return <P.Disks ctx={ctx} />;
      case "vpc": return <P.Networks ctx={ctx} />;
      case "firewall": return <P.Firewall ctx={ctx} />;
      case "buckets": return <P.Buckets ctx={ctx} />;
      case "run": return <P.CloudRun ctx={ctx} />;
      case "sql": return <P.CloudSQL ctx={ctx} />;
      case "pubsub": return <P.PubSub ctx={ctx} />;
      case "bigquery": return <M.BigQueryStudio ctx={ctx} />;
      case "roles": return <M.Roles ctx={ctx} />;
      case "orgpolicies": return <M.OrgPolicies ctx={ctx} />;
      case "templates": return <M.InstanceTemplates ctx={ctx} />;
      case "groups": return <M.InstanceGroups ctx={ctx} />;
      case "healthchecks": return <M.HealthChecks ctx={ctx} />;
      case "snapshots": return <M.Snapshots ctx={ctx} />;
      case "workloads": return <M.Workloads ctx={ctx} />;
      case "k8sservices": return <M.K8sServices ctx={ctx} />;
      case "k8sconfig": return <M.K8sConfig ctx={ctx} />;
      case "addresses": return <M.IPAddresses ctx={ctx} />;
      case "routes": return <M.Routes ctx={ctx} />;
      case "lb": return <M.LoadBalancing ctx={ctx} />;
      case "dns": return <M.CloudDNS ctx={ctx} />;
      case "nat": return <M.CloudNAT ctx={ctx} />;
      case "kms": return <M.KMS ctx={ctx} />;
      case "artifacts": return <M.ArtifactRegistry ctx={ctx} />;
      case "build": return <M.CloudBuild ctx={ctx} />;
      case "logmetrics": return <M.LogMetrics ctx={ctx} />;
      case "alerting": return <M.Alerting ctx={ctx} />;
      case "gke": return <P.GKE ctx={ctx} />;
      case "secrets": return <P.Secrets ctx={ctx} />;
      case "apis": return <P.APIs ctx={ctx} />;
      case "logs": return <O.LogsExplorer ctx={ctx} />;
      case "metrics": return <O.MetricsExplorer ctx={ctx} />;
      case "billing": return <O.Billing ctx={ctx} />;
      case "topology": return <O.Topology ctx={ctx} />;
      case "activity": return <O.Activity ctx={ctx} />;
      default: return <P.Home ctx={ctx} history={history} />;
    }
  };

  const groups = [...new Set(PRODUCTS.map((p) => p.group))];
  const rows = shell === "min" ? "48px 1fr 36px" : shell === "max" ? "48px 0 1fr" : `48px 1fr ${shellH}px`;

  return (
    <div className="cc cc-full" ref={root} style={{ gridTemplateRows: rows }}>
      <header className="cc-top">
        <button type="button" className="cc-icon-btn" aria-label={t("Menú de navegación")} aria-expanded={menu} onClick={() => setMenu(!menu)}>
          <Icon name="menu" />
        </button>
        <a className="cc-brand" href="/dashboard" title={t("Volver al panel de Cloud Mastery")}>
          <span className="cc-brand-mark" aria-hidden="true" />
          Cloud Mastery
        </a>
        <div className="cc-popwrap">
          <button type="button" className="cc-project" aria-haspopup="dialog" aria-expanded={popup === "project"} onClick={() => setPopup(popup === "project" ? "" : "project")}>
            <Icon name="project" size={18} />
            <span>{project}</span>
            <Icon name="dropdown" size={18} />
          </button>
          {popup === "project" && (
            <div className="cc-pop wide" role="dialog" aria-label={t("Seleccionar un proyecto")}>
              <h2>{t("Seleccionar un proyecto")}</h2>
              <table className="cc-table">
                <thead><tr><th>{t("Nombre")}</th><th>ID</th></tr></thead>
                <tbody>
                  <tr className="selected"><td>✓ {data?.name ?? project}</td><td><code>{project}</code></td></tr>
                </tbody>
              </table>
              <p className="cc-help">{t("Este laboratorio trabaja en un único proyecto. En la terminal: gcloud config get-value project")}</p>
            </div>
          )}
        </div>
        <form
          className="cc-search"
          role="search"
          onSubmit={(e) => {
            e.preventDefault();
            if (results[0]) {
              go(results[0].id);
              setSearch("");
            }
          }}
        >
          <Icon name="search" />
          <input value={search} onChange={(e) => setSearch(e.target.value)} placeholder={t("Buscar recursos, documentos, productos y más")} aria-label={t("Buscar productos")} />
          {results.length > 0 && (
            <ul className="cc-results">
              {results.slice(0, 8).map((m) => (
                <li key={m.id}>
                  <button type="button" onClick={() => { go(m.id); setSearch(""); }}>
                    <Icon name={m.icon} size={18} />
                    <span>{m.label}</span>
                    {m.sub && <span className="cc-help">{m.sub}</span>}
                  </button>
                </li>
              ))}
            </ul>
          )}
        </form>
        <div className="cc-top-actions">
          <button type="button" className="cc-icon-btn" title={t("Activar Cloud Shell")} aria-label={t("Activar Cloud Shell")} onClick={openShell}>
            <Icon name="shell" />
          </button>
          <div className="cc-popwrap">
            <button type="button" className="cc-icon-btn" title={t("Notificaciones")} aria-label={t("Notificaciones")} aria-expanded={popup === "bell"} onClick={() => setPopup(popup === "bell" ? "" : "bell")}>
              <Icon name="bell" />
              {notes.some((x) => !x.ok) && <span className="cc-badge" aria-hidden="true" />}
            </button>
            {popup === "bell" && (
              <div className="cc-pop right" role="dialog" aria-label={t("Notificaciones")}>
                <h2>{t("Notificaciones")}</h2>
                {notes.length ? (
                  <ul className="cc-notes">
                    {notes.map((x, i) => (
                      <li key={i}>
                        <span className={`cc-dot ${x.ok ? "ok" : "bad"}`} aria-label={x.ok ? t("correcto") : t("con error")} />
                        <div>
                          <strong>{x.text}</strong> <span className="cc-help">{x.at.toLocaleTimeString()}</span>
                          <code className="cc-note-cmd">{x.cmd}</code>
                          {x.detail && <div className="cc-help">{x.detail}</div>}
                        </div>
                      </li>
                    ))}
                  </ul>
                ) : (
                  <p className="cc-help">{t("No hay operaciones recientes.")}</p>
                )}
              </div>
            )}
          </div>
          <div className="cc-popwrap">
            <button type="button" className="cc-icon-btn" title={t("Ayuda")} aria-label={t("Ayuda")} aria-expanded={popup === "help"} onClick={() => setPopup(popup === "help" ? "" : "help")}>
              <Icon name="help" />
            </button>
            {popup === "help" && (
              <div className="cc-pop right" role="dialog" aria-label={t("Ayuda")}>
                <h2>{t("Cómo funciona esta consola")}</h2>
                <ul className="cc-help-list">
                  <li>{t("Cada botón ejecuta el comando equivalente en Cloud Shell (abajo): míralo para aprender la línea de comandos.")}</li>
                  <li>{t("Los formularios muestran el «Código equivalente» mientras los rellenas; puedes pegarlo en la terminal para editarlo.")}</li>
                  <li>{t("Lo que hagas en la terminal aparece aquí al instante, y la evaluación tiene en cuenta ambos.")}</li>
                </ul>
              </div>
            )}
          </div>
          <button type="button" className={`cc-icon-btn ${asideOpen ? "on" : ""}`} title={t("Instrucciones del laboratorio")} aria-label={t("Instrucciones del laboratorio")} aria-pressed={asideOpen} onClick={() => onAside(!asideOpen)}>
            <Icon name="book" />
          </button>
          <span className="cc-avatar" title="student@gcplab.dev" aria-label="student@gcplab.dev">S</span>
        </div>
      </header>

      <div className={`cc-body ${prod ? "" : "no-side"} ${asideOpen ? "with-aside" : ""}`}>
        {prod && (
          <nav className="cc-side" aria-label={t(prod.label)}>
            <div className="cc-side-head">
              <Icon name={prod.icon} />
              <span>{t(prod.label)}</span>
            </div>
            {prod.pages.map((sp) => {
              const c = data && sp.count ? sp.count(data) : undefined;
              return (
                <button key={sp.id} type="button" className={`cc-side-item ${page === sp.id ? "active" : ""}`} aria-current={page === sp.id ? "page" : undefined} onClick={() => go(sp.id)}>
                  <span>{t(sp.label)}</span>
                  {c ? <span className="cc-count">{c}</span> : null}
                </button>
              );
            })}
          </nav>
        )}
        <main id="main" className="cc-main" aria-label={t("Contenido de la consola")}>{body()}</main>
        {asideOpen && <aside className="cc-aside" aria-label={t("Instrucciones del laboratorio")}>{aside}</aside>}
        <div className="cc-toast-area" aria-live="polite">
          {toast && (
            <div className={`cc-toast ${toast.ok ? "ok" : "bad"}`}>
              <span>
                {toast.ok ? "✓" : "✗"} {toast.text}
                {toast.detail && <span className="cc-toast-detail">{toast.detail}</span>}
              </span>
              <button type="button" className="cc-toast-btn" onClick={openShell}>{t("Ver en Cloud Shell")}</button>
              <button type="button" className="cc-toast-btn" aria-label={t("Cerrar")} onClick={() => setToast(null)}>
                <Icon name="close" size={18} />
              </button>
            </div>
          )}
        </div>
      </div>

      <section className={`cc-shell ${shell}`} aria-label="Cloud Shell">
        {shell === "open" && <div className="cc-shell-drag" onPointerDown={drag} aria-hidden="true" title={t("Arrastra para cambiar el tamaño")} />}
        <div className="cc-shell-head">
          <span className="cc-shell-brand">CLOUD SHELL</span>
          <span className="cc-shell-tab">
            {shellMode === "terminal" ? "Terminal" : t("Editor")} <span className="cc-shell-proj">({project})</span>
          </span>
          <span className="cc-shell-spacer" />
          <button type="button" className="cc-shell-btn" onClick={() => { setShellMode(shellMode === "terminal" ? "editor" : "terminal"); if (shell === "min") setShell("open"); }}>
            <Icon name={shellMode === "terminal" ? "edit" : "shell"} size={18} />
            {shellMode === "terminal" ? t("Abrir editor") : t("Abrir terminal")}
          </button>
          <button type="button" className="cc-shell-icon" aria-label={shell === "min" ? t("Restaurar") : t("Minimizar")} title={shell === "min" ? t("Restaurar") : t("Minimizar")} onClick={() => setShell(shell === "min" ? "open" : "min")}>
            <Icon name={shell === "min" ? "expand" : "minimize"} size={18} />
          </button>
          <button type="button" className="cc-shell-icon" aria-label={shell === "max" ? t("Restaurar") : t("Maximizar")} title={shell === "max" ? t("Restaurar") : t("Maximizar")} onClick={() => setShell(shell === "max" ? "open" : "max")}>
            <Icon name={shell === "max" ? "fullscreenExit" : "fullscreen"} size={18} />
          </button>
        </div>
        <div className="cc-shell-body" hidden={shell === "min"}>
          <div className="cc-shell-pane" hidden={shellMode !== "terminal"}>{terminal}</div>
          <div className="cc-shell-pane editor" hidden={shellMode !== "editor"}>{editor}</div>
        </div>
      </section>

      {menu && (
        <div className="cc-overlay left" onMouseDown={(e) => e.target === e.currentTarget && setMenu(false)}>
          <div className="cc-menu" role="dialog" aria-modal="true" aria-label={t("Menú de navegación")} ref={menuRef}>
            <button type="button" className={`cc-menu-item ${page === "home" ? "active" : ""}`} onClick={() => go("home")}>
              <Icon name="home" /> {t("Descripción general de Cloud")}
            </button>
            {groups.map((g) => (
              <div key={g}>
                <div className="cc-menu-group">{t(g)}</div>
                {PRODUCTS.filter((p) => p.group === g).map((p) => (
                  <button key={p.id} type="button" className={`cc-menu-item ${prod?.id === p.id ? "active" : ""}`} onClick={() => go(p.id)}>
                    <Icon name={p.icon} /> {t(p.label)}
                  </button>
                ))}
              </div>
            ))}
          </div>
        </div>
      )}
    </div>
  );
}
