"use client";

// Service pages of the graphical console. They read the project snapshot
// (views/console) and change it only through gcloud, gsutil and bq commands
// run in the learner's Cloud Shell, exactly as the grader sees them.

import { useState, type ReactNode } from "react";
import { useI18n } from "@/lib/i18n";
import { Col, Confirm, CreatePage, Drawer, Field, Page, Pill, REGIONS, Status, Table, Tool, ZONES, locFlag, q } from "./ui";

export type Ctx = {
  data: any;
  project: string;
  region: string;
  zone: string;
  run: (cmd: string, note: string) => Promise<boolean>;
  runAll: (cmds: string[], note: string) => Promise<void>;
  paste: (cmd: string) => void;
  go: (page: string) => void;
  openTab: (tab: string) => void;
  refresh: () => void;
  /** creating is the page's open creation form, if any (full-page forms). */
  creating: string;
  setCreating: (id: string) => void;
};

const vals = (m: any): any[] => (m ? Object.values(m) : []);
const nameHelp = (t: (s: string) => string) => t("Minúsculas, números y guiones; empieza por una letra.");

type Action<T> = {
  icon: string;
  label: string;
  danger?: boolean;
  note: (rows: T[]) => string;
  cmds: (rows: T[]) => string[];
  confirm?: { title: string; text: string; button: string };
  single?: boolean;
};

type Create = { label: string; title: string; fields: Field[]; initial: Record<string, any>; build: (v: Record<string, any>) => string; submit: string; note: (v: Record<string, any>) => string };

/** Resource renders a list page with selection actions and a creation form. */
function Resource<T>({
  ctx,
  title,
  intro,
  caption,
  rows,
  rowKey,
  cols,
  create,
  actions = [],
  empty,
  children,
}: {
  ctx: Ctx;
  title: string;
  intro?: ReactNode;
  caption: string;
  rows: T[];
  rowKey: (r: T) => string;
  cols: Col<T>[];
  create?: Create;
  actions?: Action<T>[];
  empty: string;
  children?: ReactNode;
}) {
  const { t } = useI18n();
  const [sel, setSel] = useState<string[]>([]);
  const [confirm, setConfirm] = useState<Action<T> | null>(null);
  const chosen = rows.filter((r) => sel.includes(rowKey(r)));
  const act = async (a: Action<T>) => {
    await ctx.runAll(a.cmds(chosen), a.note(chosen));
    setSel([]);
  };
  if (create && ctx.creating === create.title) {
    return (
      <CreatePage
        title={create.title}
        fields={create.fields}
        initial={create.initial}
        build={create.build}
        submit={create.submit}
        onClose={() => ctx.setCreating("")}
        onRun={(cmd) => ctx.run(cmd, create.note({}))}
        onPaste={ctx.paste}
      />
    );
  }
  if (ctx.creating) return null;
  return (
    <Page
      title={title}
      intro={intro}
      actions={
        <>
          {create && <Tool icon="＋" label={create.label} onClick={() => ctx.setCreating(create.title)} />}
          <Tool icon="↻" label={t("Actualizar")} onClick={ctx.refresh} />
          {actions.map((a) => (
            <Tool
              key={a.label}
              icon={a.icon}
              label={a.label}
              danger={a.danger}
              disabled={!chosen.length || (a.single && chosen.length !== 1)}
              onClick={() => (a.confirm ? setConfirm(a) : act(a))}
            />
          ))}
        </>
      }
    >
      <Table caption={caption} cols={cols} rows={rows} rowKey={rowKey} selected={actions.length ? sel : undefined} onSelect={actions.length ? setSel : undefined} empty={empty} />
      {children}
      {confirm && (
        <Confirm
          title={confirm.confirm!.title}
          text={confirm.confirm!.text}
          cmds={confirm.cmds(chosen)}
          confirm={confirm.confirm!.button}
          onClose={() => setConfirm(null)}
          onRun={() => act(confirm)}
        />
      )}
    </Page>
  );
}

function List({ items, max = 3 }: { items?: string[] | null; max?: number }) {
  const { t } = useI18n();
  if (!items?.length) return <span className="muted">—</span>;
  return (
    <span className="cc-chips">
      {items.slice(0, max).map((i) => <code key={i}>{i}</code>)}
      {items.length > max && <span className="muted small">{t("y {n} más", { n: items.length - max })}</span>}
    </span>
  );
}

function PageButton({ onClick, children }: { onClick: () => void; children: ReactNode }) {
  return <button type="button" className="cc-link" onClick={onClick}>{children}</button>;
}

// ---------------------------------------------------------------- home

export function Home({ ctx, history }: { ctx: Ctx; history: any[] }) {
  const { t } = useI18n();
  const d = ctx.data;
  const cards: [string, string, number][] = [
    ["vm", t("Instancias de VM"), vals(d.instances).length],
    ["buckets", t("Buckets de Cloud Storage"), vals(d.buckets).length],
    ["run", t("Servicios de Cloud Run"), vals(d.runServices).length],
    ["sql", t("Instancias de Cloud SQL"), vals(d.sqlInstances).length],
    ["gke", t("Clústeres de GKE"), vals(d.clusters).length],
    ["pubsub", t("Temas de Pub/Sub"), vals(d.topics).length],
    ["bigquery", t("Conjuntos de datos de BigQuery"), vals(d.datasets).length],
    ["firewall", t("Reglas de cortafuegos"), vals(d.firewalls).length],
    ["secrets", t("Secretos"), vals(d.secrets).length],
  ];
  const cost = d.cost?.monthlyEur ?? 0;
  return (
    <Page title={t("Panel")}>
      <div className="cc-grid">
        <div className="card cc-card">
          <h3>{t("Información del proyecto")}</h3>
          <dl className="cc-dl">
            <dt>{t("Nombre del proyecto")}</dt><dd>{d.name}</dd>
            <dt>{t("ID del proyecto")}</dt><dd><code>{d.projectId}</code></dd>
            <dt>{t("Número del proyecto")}</dt><dd><code>{d.projectNumber}</code></dd>
            <dt>{t("Región predeterminada")}</dt><dd>{ctx.region}</dd>
          </dl>
        </div>
        <div className="card cc-card">
          <h3>{t("Facturación")}</h3>
          <div className="kpi">€{cost.toFixed(2)}</div>
          <p className="small muted">{t("Coste mensual estimado de los recursos actuales.")}</p>
          <PageButton onClick={() => ctx.openTab("cost")}>{t("Ver el detalle de costes →")}</PageButton>
        </div>
        <div className="card cc-card">
          <h3>{t("APIs")}</h3>
          <div className="kpi">{Object.values(d.services ?? {}).filter(Boolean).length}</div>
          <p className="small muted">{t("APIs habilitadas en el proyecto.")}</p>
          <PageButton onClick={() => ctx.go("apis")}>{t("Ir a APIs y servicios →")}</PageButton>
        </div>
      </div>
      <h3>{t("Recursos")}</h3>
      <div className="cc-grid small-cards">
        {cards.map(([id, label, n]) => (
          <button key={id} type="button" className="card cc-res" onClick={() => ctx.go(id)}>
            <span className="kpi">{n}</span>
            <span>{label}</span>
          </button>
        ))}
      </div>
      <h3>{t("Actividad")}</h3>
      {history.length ? (
        <ul className="cc-activity">
          {history.slice(-8).reverse().map((h, i) => (
            <li key={i}>
              <span className={`cc-dot ${h.exit ? "bad" : "ok"}`} aria-label={h.exit ? t("con error") : t("correcto")} />
              <code>{h.line}</code>
            </li>
          ))}
        </ul>
      ) : (
        <p className="small muted">{t("Todavía no has hecho cambios. Lo que hagas aquí o en Cloud Shell aparecerá en esta lista.")}</p>
      )}
    </Page>
  );
}

// ---------------------------------------------------------------- IAM

const ROLES = ["roles/viewer", "roles/editor", "roles/owner", "roles/browser", "roles/compute.admin", "roles/compute.viewer", "roles/storage.admin", "roles/storage.objectViewer", "roles/storage.objectAdmin", "roles/run.admin", "roles/run.invoker", "roles/cloudsql.client", "roles/pubsub.publisher", "roles/pubsub.subscriber", "roles/bigquery.dataViewer", "roles/bigquery.jobUser", "roles/secretmanager.secretAccessor", "roles/iam.serviceAccountUser", "roles/logging.viewer", "roles/monitoring.viewer", "roles/container.developer"];

export function IAM({ ctx }: { ctx: Ctx }) {
  const { t } = useI18n();
  const [drawer, setDrawer] = useState(false);
  const [remove, setRemove] = useState<{ member: string; role: string } | null>(null);
  const byMember = new Map<string, { role: string; cond?: any }[]>();
  for (const b of ctx.data.iamPolicy?.bindings ?? []) {
    for (const m of b.members ?? []) byMember.set(m, [...(byMember.get(m) ?? []), { role: b.role, cond: b.condition }]);
  }
  const rows = [...byMember.entries()].map(([member, roles]) => ({ member, roles })).sort((a, b) => a.member.localeCompare(b.member));
  const removeCmd = (r: { member: string; role: string }) => `gcloud projects remove-iam-policy-binding ${ctx.project} --member=${q(r.member)} --role=${r.role}`;
  const kind = (m: string) => {
    const k = m.split(":")[0];
    return ({ user: t("Usuario"), serviceAccount: t("Cuenta de servicio"), group: t("Grupo"), domain: t("Dominio"), allUsers: t("Cualquiera en internet"), allAuthenticatedUsers: t("Cualquier cuenta de Google") } as Record<string, string>)[k] ?? k;
  };
  return (
    <Page
      title={t("IAM")}
      intro={t("Permisos del proyecto {p}: quién (principal) tiene qué rol.", { p: ctx.project })}
      actions={<Tool icon="＋" label={t("Conceder acceso")} onClick={() => setDrawer(true)} />}
    >
      <Table
        caption={t("Principales y roles del proyecto")}
        rows={rows}
        rowKey={(r) => r.member}
        empty={t("La política del proyecto no tiene vinculaciones.")}
        cols={[
          { key: "type", label: t("Tipo"), render: (r) => <span className="small">{kind(r.member)}</span> },
          { key: "member", label: t("Principal"), render: (r) => <code>{r.member.replace(/^[a-zA-Z]+:/, "")}</code> },
          {
            key: "roles",
            label: t("Roles"),
            render: (r) => (
              <span className="cc-chips">
                {r.roles.map((x) => (
                  <span key={x.role + (x.cond?.title ?? "")} className="cc-chip">
                    {x.role.replace(/^roles\//, "")}
                    {x.cond ? (
                      <span className="pill" title={x.cond.expression}>{t("condicional")}</span>
                    ) : (
                      <button type="button" aria-label={t("Quitar {role} a {who}", { role: x.role, who: r.member })} onClick={() => setRemove({ member: r.member, role: x.role })}>✕</button>
                    )}
                  </span>
                ))}
              </span>
            ),
          },
        ]}
      />
      {drawer && (
        <Drawer
          title={t("Conceder acceso a {p}", { p: ctx.project })}
          submit={t("Guardar")}
          initial={{ kind: "user", who: "", role: "roles/viewer", custom: "" }}
          fields={[
            { id: "kind", label: t("Tipo de principal"), type: "select", options: [["user", t("Usuario")], ["serviceAccount", t("Cuenta de servicio")], ["group", t("Grupo")], ["domain", t("Dominio")]] },
            { id: "who", label: t("Principal"), required: true, placeholder: "ana@example.com" },
            { id: "role", label: t("Rol"), type: "select", options: [...ROLES, ["custom", t("Otro rol…")]] },
            { id: "custom", label: t("Rol personalizado o predefinido"), placeholder: "roles/…", when: (v) => v.role === "custom", required: true },
          ]}
          build={(v) => `gcloud projects add-iam-policy-binding ${ctx.project} --member=${q(`${v.kind}:${v.who.trim()}`)} --role=${q(v.role === "custom" ? v.custom.trim() : v.role)}`}
          onClose={() => setDrawer(false)}
          onRun={(cmd) => ctx.run(cmd, t("Conceder acceso"))}
          onPaste={ctx.paste}
        />
      )}
      {remove && (
        <Confirm
          title={t("¿Quitar el rol?")}
          text={t("{who} dejará de tener {role} en el proyecto.", { who: remove.member, role: remove.role })}
          cmds={[removeCmd(remove)]}
          confirm={t("Quitar")}
          onClose={() => setRemove(null)}
          onRun={async () => void (await ctx.run(removeCmd(remove), t("Quitar un rol")))}
        />
      )}
    </Page>
  );
}

export function ServiceAccounts({ ctx }: { ctx: Ctx }) {
  const { t } = useI18n();
  const rows = vals(ctx.data.serviceAccounts).sort((a, b) => a.email.localeCompare(b.email));
  return (
    <Resource
      ctx={ctx}
      title={t("Cuentas de servicio")}
      intro={t("Identidades que usan las aplicaciones y las VM para llamar a las APIs de Google Cloud.")}
      caption={t("Cuentas de servicio")}
      rows={rows}
      rowKey={(r) => r.email}
      empty={t("No hay cuentas de servicio.")}
      cols={[
        { key: "st", label: t("Estado"), render: (r) => <Status state={r.disabled ? "DISABLED" : "ENABLED"} /> },
        { key: "email", label: t("Correo"), render: (r) => <code>{r.email}</code> },
        { key: "name", label: t("Nombre"), render: (r) => r.displayName || <span className="muted">—</span> },
        { key: "keys", label: t("Claves"), render: (r) => (r.keys?.length ? <Pill tone="warn">{r.keys.length}</Pill> : <span className="muted">0</span>) },
      ]}
      create={{
        label: t("Crear cuenta de servicio"),
        title: t("Crear cuenta de servicio"),
        submit: t("Crear"),
        note: () => t("Crear cuenta de servicio"),
        initial: { id: "", display: "" },
        fields: [
          { id: "id", label: t("ID de la cuenta de servicio"), required: true, placeholder: "app-backend", help: t("De 6 a 30 caracteres: minúsculas, números y guiones.") },
          { id: "display", label: t("Nombre visible") },
        ],
        build: (v) => `gcloud iam service-accounts create ${q(v.id.trim())}${v.display ? ` --display-name=${q(v.display)}` : ""}`,
      }}
      actions={[
        { icon: "⏻", label: t("Inhabilitar"), note: () => t("Inhabilitar cuenta de servicio"), cmds: (rs) => rs.map((r) => `gcloud iam service-accounts disable ${r.email}`) },
        { icon: "▶", label: t("Habilitar"), note: () => t("Habilitar cuenta de servicio"), cmds: (rs) => rs.map((r) => `gcloud iam service-accounts enable ${r.email}`) },
        {
          icon: "🗑",
          label: t("Eliminar"),
          danger: true,
          note: () => t("Eliminar cuenta de servicio"),
          cmds: (rs) => rs.map((r) => `gcloud iam service-accounts delete ${r.email} --quiet`),
          confirm: { title: t("¿Eliminar las cuentas de servicio?"), text: t("Lo que dependa de ellas dejará de poder autenticarse."), button: t("Eliminar") },
        },
      ]}
    />
  );
}

// ---------------------------------------------------------------- Compute

const MACHINES = ["e2-micro", "e2-small", "e2-medium", "e2-standard-2", "e2-standard-4", "n2-standard-2", "n2-standard-4", "c3-standard-4"];
const IMAGES = ["debian-12", "debian-11", "ubuntu-2204-lts", "ubuntu-2404-lts-amd64", "rocky-linux-9", "cos-stable"];

export function Instances({ ctx }: { ctx: Ctx }) {
  const { t } = useI18n();
  const rows = vals(ctx.data.instances).sort((a, b) => a.name.localeCompare(b.name));
  const nets = Object.keys(ctx.data.networks ?? {});
  const sas = Object.keys(ctx.data.serviceAccounts ?? {});
  const each = (verb: string) => (rs: any[]) => rs.map((r) => `gcloud compute instances ${verb} ${r.name} --zone=${r.zone}`);
  const n = rows.length + 1;
  return (
    <Resource
      ctx={ctx}
      title={t("Instancias de VM")}
      caption={t("Instancias de VM")}
      rows={rows}
      rowKey={(r) => r.name}
      empty={t("No hay instancias de VM en este proyecto.")}
      cols={[
        { key: "st", label: t("Estado"), render: (r) => <Status state={r.status} /> },
        { key: "name", label: t("Nombre"), render: (r) => <strong>{r.name}</strong> },
        { key: "zone", label: t("Zona"), render: (r) => r.zone },
        { key: "mt", label: t("Tipo de máquina"), render: (r) => <span className="small">{r.machineType}{r.spot ? " · Spot" : ""}</span> },
        { key: "ip", label: t("IP interna"), render: (r) => <code>{r.networkIP}</code> },
        { key: "ext", label: t("IP externa"), render: (r) => (r.natIP ? <code>{r.natIP}</code> : <span className="muted">{t("Ninguna")}</span>) },
        { key: "tags", label: t("Etiquetas de red"), render: (r) => <List items={r.tags} /> },
        {
          key: "ssh",
          label: t("Conectar"),
          render: (r) => (
            <button type="button" className="cc-link" disabled={r.status !== "RUNNING"} onClick={() => ctx.paste(`gcloud compute ssh ${r.name} --zone=${r.zone} --command='hostname'`)}>
              SSH
            </button>
          ),
        },
      ]}
      create={{
        label: t("Crear instancia"),
        title: t("Crear una instancia"),
        submit: t("Crear"),
        note: () => t("Crear instancia de VM"),
        initial: { name: `instance-${n}`, zone: ctx.zone, machine: "e2-medium", image: "debian-12", network: nets.includes("default") ? "default" : nets[0] ?? "", subnet: "", tags: "", http: false, https: false, ext: true, spot: false, sa: "" },
        fields: [
          { id: "name", label: t("Nombre"), required: true, help: nameHelp(t) },
          { id: "zone", label: t("Zona"), type: "select", options: ZONES.includes(ctx.zone) ? ZONES : [ctx.zone, ...ZONES] },
          { id: "machine", label: t("Tipo de máquina"), type: "select", options: MACHINES },
          { id: "image", label: t("Imagen del disco de arranque"), type: "select", options: IMAGES },
          { id: "network", label: t("Red"), type: "select", options: nets },
          { id: "subnet", label: t("Subred"), type: "select", options: [["", t("Automática")], ...vals(ctx.data.subnets).map((s: any) => s.name)] },
          { id: "tags", label: t("Etiquetas de red"), placeholder: "web, ssh", help: t("Separadas por comas. Las reglas de cortafuegos se aplican por etiqueta.") },
          { id: "http", label: t("Permitir tráfico HTTP"), type: "check", help: t("Añade la etiqueta http-server. Hace falta una regla de cortafuegos que la permita.") },
          { id: "https", label: t("Permitir tráfico HTTPS"), type: "check", help: t("Añade la etiqueta https-server.") },
          { id: "ext", label: t("Asignar una IP externa"), type: "check", help: t("Sin IP externa la VM solo sale a internet a través de Cloud NAT.") },
          { id: "spot", label: t("VM Spot (más barata, puede detenerse en cualquier momento)"), type: "check" },
          { id: "sa", label: t("Cuenta de servicio"), type: "select", options: [["", t("Predeterminada de Compute Engine")], ...sas] },
        ],
        build: (v) => {
          const tags = [...String(v.tags).split(",").map((x) => x.trim()).filter(Boolean), ...(v.http ? ["http-server"] : []), ...(v.https ? ["https-server"] : [])];
          return [
            `gcloud compute instances create ${q(v.name.trim())}`,
            `--zone=${v.zone}`,
            `--machine-type=${v.machine}`,
            `--image-family=${v.image}`,
            v.network && v.network !== "default" ? `--network=${v.network}` : "",
            v.subnet ? `--subnet=${v.subnet}` : "",
            tags.length ? `--tags=${[...new Set(tags)].join(",")}` : "",
            v.ext ? "" : "--no-address",
            v.spot ? "--provisioning-model=SPOT" : "",
            v.sa ? `--service-account=${v.sa}` : "",
          ].filter(Boolean).join(" ");
        },
      }}
      actions={[
        { icon: "▶", label: t("Iniciar"), note: () => t("Iniciar VM"), cmds: each("start") },
        { icon: "■", label: t("Detener"), note: () => t("Detener VM"), cmds: each("stop") },
        { icon: "↻", label: t("Restablecer"), note: () => t("Restablecer VM"), cmds: each("reset") },
        {
          icon: "🗑",
          label: t("Eliminar"),
          danger: true,
          note: () => t("Eliminar VM"),
          cmds: (rs) => rs.map((r) => `gcloud compute instances delete ${r.name} --zone=${r.zone} --quiet`),
          confirm: { title: t("¿Eliminar las instancias?"), text: t("Se borrarán las VM seleccionadas y sus discos de arranque."), button: t("Eliminar") },
        },
      ]}
    />
  );
}

export function Disks({ ctx }: { ctx: Ctx }) {
  const { t } = useI18n();
  const rows = vals(ctx.data.disks).sort((a, b) => a.name.localeCompare(b.name));
  return (
    <Resource
      ctx={ctx}
      title={t("Discos")}
      caption={t("Discos persistentes")}
      rows={rows}
      rowKey={(r) => `${r.zone}/${r.name}`}
      empty={t("No hay discos.")}
      cols={[
        { key: "name", label: t("Nombre"), render: (r) => <strong>{r.name}</strong> },
        { key: "type", label: t("Tipo"), render: (r) => r.type },
        { key: "size", label: t("Tamaño"), render: (r) => `${r.sizeGb} GB` },
        { key: "zone", label: t("Zona"), render: (r) => r.zone },
        { key: "users", label: t("En uso por"), render: (r) => <List items={r.users} /> },
      ]}
      create={{
        label: t("Crear disco"),
        title: t("Crear un disco"),
        submit: t("Crear"),
        note: () => t("Crear disco"),
        initial: { name: `disk-${rows.length + 1}`, zone: ctx.zone, size: "20", type: "pd-balanced" },
        fields: [
          { id: "name", label: t("Nombre"), required: true, help: nameHelp(t) },
          { id: "zone", label: t("Zona"), type: "select", options: ZONES.includes(ctx.zone) ? ZONES : [ctx.zone, ...ZONES] },
          { id: "type", label: t("Tipo de disco"), type: "select", options: ["pd-balanced", "pd-ssd", "pd-standard"] },
          { id: "size", label: t("Tamaño (GB)"), type: "number", required: true },
        ],
        build: (v) => `gcloud compute disks create ${q(v.name.trim())} --zone=${v.zone} --size=${v.size}GB --type=${v.type}`,
      }}
      actions={[
        {
          icon: "🗑",
          label: t("Eliminar"),
          danger: true,
          note: () => t("Eliminar disco"),
          cmds: (rs) => rs.map((r) => `gcloud compute disks delete ${r.name} --zone=${r.zone} --quiet`),
          confirm: { title: t("¿Eliminar los discos?"), text: t("Los datos del disco se perderán."), button: t("Eliminar") },
        },
      ]}
    />
  );
}

// ---------------------------------------------------------------- VPC

export function Networks({ ctx }: { ctx: Ctx }) {
  const { t } = useI18n();
  const nets = vals(ctx.data.networks).sort((a, b) => a.name.localeCompare(b.name));
  const subnets = vals(ctx.data.subnets).sort((a, b) => (a.network + a.name).localeCompare(b.network + b.name));
  const [showAuto, setShowAuto] = useState(false);
  const autoNets = new Set(nets.filter((n) => n.subnetMode === "auto").map((n) => n.name));
  const shown = showAuto ? subnets : subnets.filter((s) => !autoNets.has(s.network) || s.region === ctx.region);
  return (
    <>
      <Resource
        ctx={ctx}
        title={t("Redes de VPC")}
        caption={t("Redes de VPC")}
        rows={nets}
        rowKey={(r) => r.name}
        empty={t("No hay redes de VPC.")}
        cols={[
          { key: "name", label: t("Nombre"), render: (r) => <strong>{r.name}</strong> },
          { key: "mode", label: t("Modo de subred"), render: (r) => (r.subnetMode === "auto" ? t("Automático") : t("Personalizado")) },
          { key: "subs", label: t("Subredes"), render: (r) => subnets.filter((s) => s.network === r.name).length },
          { key: "peer", label: t("Emparejamientos"), render: (r) => <List items={(r.peerings ?? []).map((p: any) => p.name ?? String(p))} /> },
          { key: "psa", label: t("Acceso a servicios privados"), render: (r) => (r.psaConnected ? <Pill tone="ok">{t("Sí")}</Pill> : <span className="muted">{t("No")}</span>) },
        ]}
        create={{
          label: t("Crear red de VPC"),
          title: t("Crear una red de VPC"),
          submit: t("Crear"),
          note: () => t("Crear red de VPC"),
          initial: { name: "vpc-1", mode: "custom" },
          fields: [
            { id: "name", label: t("Nombre"), required: true, help: nameHelp(t) },
            { id: "mode", label: t("Modo de creación de subredes"), type: "select", options: [["custom", t("Personalizado")], ["auto", t("Automático")]] },
          ],
          build: (v) => `gcloud compute networks create ${q(v.name.trim())} --subnet-mode=${v.mode}`,
        }}
        actions={[
          {
            icon: "🗑",
            label: t("Eliminar"),
            danger: true,
            note: () => t("Eliminar red de VPC"),
            cmds: (rs) => rs.map((r) => `gcloud compute networks delete ${r.name} --quiet`),
            confirm: { title: t("¿Eliminar las redes?"), text: t("Antes hay que borrar sus subredes, reglas y VM."), button: t("Eliminar") },
          },
        ]}
      />
      <Resource
        ctx={ctx}
        title={t("Subredes")}
        intro={
          <label className="small" style={{ display: "inline-flex", gap: 6, alignItems: "center", color: "var(--text)" }}>
            <input type="checkbox" style={{ width: "auto" }} checked={showAuto} onChange={(e) => setShowAuto(e.target.checked)} />
            {t("Mostrar las subredes automáticas de todas las regiones")}
          </label>
        }
        caption={t("Subredes")}
        rows={shown}
        rowKey={(r) => `${r.region}/${r.name}`}
        empty={t("No hay subredes.")}
        cols={[
          { key: "name", label: t("Nombre"), render: (r) => <strong>{r.name}</strong> },
          { key: "region", label: t("Región"), render: (r) => r.region },
          { key: "net", label: t("Red"), render: (r) => r.network },
          { key: "range", label: t("Rango de IP"), render: (r) => <code>{r.ipCidrRange}</code> },
          { key: "pga", label: t("Acceso privado a Google"), render: (r) => (r.privateIpGoogleAccess ? t("Activado") : t("Desactivado")) },
          { key: "flow", label: t("Registros de flujo"), render: (r) => (r.flowLogs ? t("Activado") : t("Desactivado")) },
        ]}
        create={{
          label: t("Añadir subred"),
          title: t("Añadir una subred"),
          submit: t("Añadir"),
          note: () => t("Añadir subred"),
          initial: { name: "subnet-1", network: nets.find((n) => n.subnetMode !== "auto")?.name ?? nets[0]?.name ?? "", region: ctx.region, range: "10.10.0.0/24", pga: true, flow: false },
          fields: [
            { id: "name", label: t("Nombre"), required: true, help: nameHelp(t) },
            { id: "network", label: t("Red"), type: "select", options: nets.map((n) => n.name) },
            { id: "region", label: t("Región"), type: "select", options: REGIONS.includes(ctx.region) ? REGIONS : [ctx.region, ...REGIONS] },
            { id: "range", label: t("Rango de IPv4"), required: true, help: t("En notación CIDR; no puede solaparse con otras subredes de la red.") },
            { id: "pga", label: t("Acceso privado a Google"), type: "check", help: t("Permite a las VM sin IP externa usar las APIs de Google.") },
            { id: "flow", label: t("Registros de flujo"), type: "check" },
          ],
          build: (v) =>
            `gcloud compute networks subnets create ${q(v.name.trim())} --network=${v.network} --region=${v.region} --range=${q(v.range.trim())}${v.pga ? " --enable-private-ip-google-access" : ""}${v.flow ? " --enable-flow-logs" : ""}`,
        }}
        actions={[
          {
            icon: "🗑",
            label: t("Eliminar"),
            danger: true,
            note: () => t("Eliminar subred"),
            cmds: (rs) => rs.map((r) => `gcloud compute networks subnets delete ${r.name} --region=${r.region} --quiet`),
            confirm: { title: t("¿Eliminar las subredes?"), text: t("Las VM que las usan tienen que borrarse antes."), button: t("Eliminar") },
          },
        ]}
      />
    </>
  );
}

export function Firewall({ ctx }: { ctx: Ctx }) {
  const { t } = useI18n();
  const rows = vals(ctx.data.firewalls).sort((a, b) => a.priority - b.priority || a.name.localeCompare(b.name));
  const nets = Object.keys(ctx.data.networks ?? {});
  const proto = (r: any) => (r.rules ?? []).map((x: any) => (x.ports?.length ? x.ports.map((p: string) => `${x.IPProtocol}:${p}`).join(", ") : x.IPProtocol)).join(", ");
  return (
    <Resource
      ctx={ctx}
      title={t("Reglas de cortafuegos")}
      intro={t("Se evalúan por prioridad: el número más bajo gana. Sin regla que lo permita, el tráfico entrante se bloquea.")}
      caption={t("Reglas de cortafuegos")}
      rows={rows}
      rowKey={(r) => r.name}
      empty={t("No hay reglas de cortafuegos.")}
      cols={[
        { key: "name", label: t("Nombre"), render: (r) => <strong>{r.name}</strong> },
        { key: "dir", label: t("Tipo"), render: (r) => (r.direction === "EGRESS" ? t("Salida") : t("Entrada")) },
        { key: "target", label: t("Destinos"), render: (r) => (r.targetTags?.length ? <List items={r.targetTags} /> : r.targetServiceAccounts?.length ? <List items={r.targetServiceAccounts} max={1} /> : <span className="small">{t("Todas las instancias")}</span>) },
        { key: "src", label: t("Filtros"), render: (r) => <List items={r.direction === "EGRESS" ? r.destinationRanges : [...(r.sourceRanges ?? []), ...(r.sourceTags ?? []).map((x: string) => `tag:${x}`)]} max={2} /> },
        { key: "proto", label: t("Protocolos/puertos"), render: (r) => <code>{proto(r)}</code> },
        { key: "action", label: t("Acción"), render: (r) => (r.action === "DENY" ? <Pill tone="bad">{t("Denegar")}</Pill> : <Pill tone="ok">{t("Permitir")}</Pill>) },
        { key: "prio", label: t("Prioridad"), render: (r) => r.priority },
        { key: "net", label: t("Red"), render: (r) => r.network },
        { key: "on", label: t("Estado"), render: (r) => (r.disabled ? <Pill tone="warn">{t("Inhabilitada")}</Pill> : t("Habilitada")) },
      ]}
      create={{
        label: t("Crear regla de cortafuegos"),
        title: t("Crear una regla de cortafuegos"),
        submit: t("Crear"),
        note: () => t("Crear regla de cortafuegos"),
        initial: { name: "allow-http", network: nets.includes("default") ? "default" : nets[0] ?? "", direction: "INGRESS", action: "ALLOW", priority: "1000", targets: "", ranges: "0.0.0.0/0", rules: "tcp:80", logging: false },
        fields: [
          { id: "name", label: t("Nombre"), required: true, help: nameHelp(t) },
          { id: "network", label: t("Red"), type: "select", options: nets },
          { id: "direction", label: t("Dirección del tráfico"), type: "select", options: [["INGRESS", t("Entrada")], ["EGRESS", t("Salida")]] },
          { id: "action", label: t("Acción si hay coincidencia"), type: "select", options: [["ALLOW", t("Permitir")], ["DENY", t("Denegar")]] },
          { id: "priority", label: t("Prioridad"), type: "number", help: t("De 0 a 65535; el número más bajo tiene más prioridad.") },
          { id: "targets", label: t("Etiquetas de destino"), placeholder: "web", help: t("Vacío = todas las instancias de la red.") },
          { id: "ranges", label: t("Rangos de IP"), required: true, help: t("Origen (entrada) o destino (salida), separados por comas. 0.0.0.0/0 es todo internet.") },
          { id: "rules", label: t("Protocolos y puertos"), required: true, placeholder: "tcp:80,tcp:443", help: t("p. ej. tcp:22, udp:53, icmp o all.") },
          { id: "logging", label: t("Registros de cortafuegos"), type: "check" },
        ],
        build: (v) =>
          [
            `gcloud compute firewall-rules create ${q(v.name.trim())}`,
            `--network=${v.network}`,
            `--direction=${v.direction}`,
            `--action=${v.action}`,
            `--rules=${q(String(v.rules).replace(/\s/g, ""))}`,
            `${v.direction === "EGRESS" ? "--destination-ranges" : "--source-ranges"}=${q(String(v.ranges).replace(/\s/g, ""))}`,
            v.targets.trim() ? `--target-tags=${q(String(v.targets).replace(/\s/g, ""))}` : "",
            `--priority=${v.priority}`,
            v.logging ? "--enable-logging" : "",
          ].filter(Boolean).join(" "),
      }}
      actions={[
        { icon: "⏻", label: t("Inhabilitar"), note: () => t("Inhabilitar regla"), cmds: (rs) => rs.map((r) => `gcloud compute firewall-rules update ${r.name} --disabled`) },
        { icon: "▶", label: t("Habilitar"), note: () => t("Habilitar regla"), cmds: (rs) => rs.map((r) => `gcloud compute firewall-rules update ${r.name} --no-disabled`) },
        {
          icon: "🗑",
          label: t("Eliminar"),
          danger: true,
          note: () => t("Eliminar regla de cortafuegos"),
          cmds: (rs) => rs.map((r) => `gcloud compute firewall-rules delete ${r.name} --quiet`),
          confirm: { title: t("¿Eliminar las reglas?"), text: t("El tráfico que permitían quedará bloqueado (o permitido, si eran de denegación)."), button: t("Eliminar") },
        },
      ]}
    />
  );
}

// ---------------------------------------------------------------- Storage

const isPublic = (policy: any) => (policy?.bindings ?? []).some((b: any) => (b.members ?? []).some((m: string) => m === "allUsers" || m === "allAuthenticatedUsers"));

export function Buckets({ ctx }: { ctx: Ctx }) {
  const { t } = useI18n();
  const rows = vals(ctx.data.buckets).sort((a, b) => a.name.localeCompare(b.name));
  const [open, setOpen] = useState("");
  const b = rows.find((r) => r.name === open);
  return (
    <Resource
      ctx={ctx}
      title={t("Buckets")}
      caption={t("Buckets de Cloud Storage")}
      rows={rows}
      rowKey={(r) => r.name}
      empty={t("No hay buckets en este proyecto.")}
      cols={[
        { key: "name", label: t("Nombre"), render: (r) => <PageButton onClick={() => setOpen(open === r.name ? "" : r.name)}>{r.name}</PageButton> },
        { key: "loc", label: t("Ubicación"), render: (r) => `${r.location} (${r.locationType === "region" ? t("región") : r.locationType === "dual-region" ? t("birregión") : t("multirregión")})` },
        { key: "class", label: t("Clase"), render: (r) => r.storageClass },
        { key: "pub", label: t("Acceso público"), render: (r) => (isPublic(r.iamPolicy) ? <Pill tone="bad">{t("Público en internet")}</Pill> : r.publicAccessPrevention === "enforced" ? t("Prevenido") : t("No público")) },
        { key: "acl", label: t("Control de acceso"), render: (r) => (r.uniformBucketLevelAccess ? t("Uniforme") : t("Detallado")) },
        { key: "ver", label: t("Versiones"), render: (r) => (r.versioning ? t("Activado") : t("Desactivado")) },
        { key: "objs", label: t("Objetos"), render: (r) => Object.keys(r.objects ?? {}).length },
      ]}
      create={{
        label: t("Crear bucket"),
        title: t("Crear un bucket"),
        submit: t("Crear"),
        note: () => t("Crear bucket"),
        initial: { name: `${ctx.project}-bucket`, location: ctx.region, cls: "STANDARD", ubla: true, pap: true },
        fields: [
          { id: "name", label: t("Nombre"), required: true, help: t("Único en todo el mundo: minúsculas, números, guiones y puntos.") },
          { id: "location", label: t("Ubicación"), type: "select", options: [...(REGIONS.includes(ctx.region) ? REGIONS : [ctx.region, ...REGIONS]), ["EU", t("EU (multirregión)")], ["US", t("US (multirregión)")]] },
          { id: "cls", label: t("Clase de almacenamiento"), type: "select", options: ["STANDARD", "NEARLINE", "COLDLINE", "ARCHIVE"] },
          { id: "ubla", label: t("Acceso uniforme a nivel de bucket"), type: "check", help: t("Solo IAM decide quién accede (sin ACL por objeto).") },
          { id: "pap", label: t("Aplicar la prevención de acceso público"), type: "check", help: t("Impide que el bucket se haga público por error.") },
        ],
        build: (v) =>
          `gcloud storage buckets create gs://${v.name.trim()} --location=${v.location}${v.cls !== "STANDARD" ? ` --default-storage-class=${v.cls}` : ""}${v.ubla ? " --uniform-bucket-level-access" : ""}${v.pap ? " --public-access-prevention" : ""}`,
      }}
      actions={[
        { icon: "⧉", label: t("Activar versiones"), note: () => t("Activar versiones de objetos"), cmds: (rs) => rs.map((r) => `gcloud storage buckets update gs://${r.name} --versioning`) },
        { icon: "🛡", label: t("Prevenir acceso público"), note: () => t("Prevenir acceso público"), cmds: (rs) => rs.map((r) => `gcloud storage buckets update gs://${r.name} --public-access-prevention`) },
        {
          icon: "🗑",
          label: t("Eliminar"),
          danger: true,
          note: () => t("Eliminar bucket"),
          cmds: (rs) => rs.map((r) => (Object.keys(r.objects ?? {}).length ? `gcloud storage rm --recursive gs://${r.name}` : `gcloud storage buckets delete gs://${r.name} --quiet`)),
          confirm: { title: t("¿Eliminar los buckets?"), text: t("Se borrarán los buckets y todos sus objetos."), button: t("Eliminar") },
        },
      ]}
    >
      {b && (
        <div className="card" style={{ marginTop: 12 }}>
          <div className="row" style={{ justifyContent: "space-between" }}>
            <h3 style={{ margin: 0 }}>gs://{b.name}</h3>
            <button type="button" className="btn secondary" onClick={() => ctx.paste(`gcloud storage cp ./archivo.txt gs://${b.name}/`)}>{t("Subir un archivo…")}</button>
          </div>
          <Table
            caption={t("Objetos de {b}", { b: b.name })}
            rows={Object.entries(b.objects ?? {}).map(([name, o]: [string, any]) => ({ name, ...o }))}
            rowKey={(o: any) => o.name}
            empty={t("El bucket está vacío.")}
            cols={[
              { key: "name", label: t("Nombre"), render: (o: any) => <code>{o.name}</code> },
              { key: "size", label: t("Tamaño"), render: (o: any) => (o.size != null ? `${o.size} B` : "—") },
              { key: "type", label: t("Tipo"), render: (o: any) => o.contentType ?? "—" },
            ]}
          />
        </div>
      )}
    </Resource>
  );
}

// ---------------------------------------------------------------- Cloud Run

export function CloudRun({ ctx }: { ctx: Ctx }) {
  const { t } = useI18n();
  const rows = vals(ctx.data.runServices).sort((a, b) => a.name.localeCompare(b.name));
  return (
    <Resource
      ctx={ctx}
      title={t("Servicios de Cloud Run")}
      caption={t("Servicios de Cloud Run")}
      rows={rows}
      rowKey={(r) => `${r.region}/${r.name}`}
      empty={t("No hay servicios de Cloud Run.")}
      cols={[
        { key: "st", label: t("Estado"), render: () => <Status state="READY" /> },
        { key: "name", label: t("Nombre"), render: (r) => <strong>{r.name}</strong> },
        { key: "region", label: t("Región"), render: (r) => r.region },
        { key: "url", label: "URL", render: (r) => <code className="small">{r.url}</code> },
        { key: "auth", label: t("Autenticación"), render: (r) => (isPublic(r.iamPolicy) ? <Pill tone="warn">{t("Permitir no autenticadas")}</Pill> : t("Requiere autenticación")) },
        { key: "ingress", label: t("Entrada"), render: (r) => r.ingress },
        { key: "rev", label: t("Última revisión"), render: (r) => <span className="small">{r.revisions?.[r.revisions.length - 1]?.name ?? "—"}</span> },
        { key: "img", label: t("Imagen"), render: (r) => <span className="small" style={{ wordBreak: "break-all" }}>{r.image}</span> },
      ]}
      create={{
        label: t("Desplegar contenedor"),
        title: t("Crear un servicio"),
        submit: t("Crear"),
        note: () => t("Desplegar en Cloud Run"),
        initial: { name: "hello", image: "us-docker.pkg.dev/cloudrun/container/hello", region: ctx.region, public: false, min: "0", max: "10" },
        fields: [
          { id: "name", label: t("Nombre del servicio"), required: true, help: nameHelp(t) },
          { id: "image", label: t("URL de la imagen del contenedor"), required: true },
          { id: "region", label: t("Región"), type: "select", options: REGIONS.includes(ctx.region) ? REGIONS : [ctx.region, ...REGIONS] },
          { id: "public", label: t("Permitir invocaciones no autenticadas"), type: "check", help: t("Cualquiera en internet podrá llamar al servicio.") },
          { id: "min", label: t("Número mínimo de instancias"), type: "number" },
          { id: "max", label: t("Número máximo de instancias"), type: "number" },
        ],
        build: (v) =>
          `gcloud run deploy ${q(v.name.trim())} --image=${q(v.image.trim())} --region=${v.region} ${v.public ? "--allow-unauthenticated" : "--no-allow-unauthenticated"}${v.min && v.min !== "0" ? ` --min-instances=${v.min}` : ""}${v.max ? ` --max-instances=${v.max}` : ""}`,
      }}
      actions={[
        {
          icon: "🗑",
          label: t("Eliminar"),
          danger: true,
          note: () => t("Eliminar servicio de Cloud Run"),
          cmds: (rs) => rs.map((r) => `gcloud run services delete ${r.name} --region=${r.region} --quiet`),
          confirm: { title: t("¿Eliminar los servicios?"), text: t("Sus URL dejarán de responder."), button: t("Eliminar") },
        },
      ]}
    />
  );
}

// ---------------------------------------------------------------- Cloud SQL

export function CloudSQL({ ctx }: { ctx: Ctx }) {
  const { t } = useI18n();
  const rows = vals(ctx.data.sqlInstances).sort((a, b) => a.name.localeCompare(b.name));
  return (
    <Resource
      ctx={ctx}
      title={t("Instancias de Cloud SQL")}
      caption={t("Instancias de Cloud SQL")}
      rows={rows}
      rowKey={(r) => r.name}
      empty={t("No hay instancias de Cloud SQL.")}
      cols={[
        { key: "st", label: t("Estado"), render: (r) => <Status state={r.state} /> },
        { key: "name", label: t("ID de instancia"), render: (r) => <strong>{r.name}</strong> },
        { key: "ver", label: t("Tipo"), render: (r) => r.databaseVersion },
        { key: "ip", label: t("IP pública"), render: (r) => (r.publicIp ? <code>{r.publicIp}</code> : <span className="muted">{t("Ninguna")}</span>) },
        { key: "priv", label: t("IP privada"), render: (r) => (r.privateIp ? <code>{r.privateIp}</code> : <span className="muted">—</span>) },
        { key: "ha", label: t("Alta disponibilidad"), render: (r) => (r.availabilityType === "REGIONAL" ? t("Sí (regional)") : t("No (zonal)")) },
        { key: "tier", label: t("Nivel"), render: (r) => r.tier },
        { key: "region", label: t("Región"), render: (r) => r.region },
        { key: "bk", label: t("Copias de seguridad"), render: (r) => (r.backupEnabled ? t("Activadas") : <Pill tone="warn">{t("Desactivadas")}</Pill>) },
      ]}
      create={{
        label: t("Crear instancia"),
        title: t("Crear una instancia de Cloud SQL"),
        submit: t("Crear"),
        note: () => t("Crear instancia de Cloud SQL"),
        initial: { name: "db-1", version: "POSTGRES_15", tier: "db-g1-small", region: ctx.region, ha: false },
        fields: [
          { id: "name", label: t("ID de instancia"), required: true, help: nameHelp(t) },
          { id: "version", label: t("Versión de la base de datos"), type: "select", options: ["POSTGRES_15", "POSTGRES_16", "MYSQL_8_0", "SQLSERVER_2022_STANDARD"] },
          { id: "tier", label: t("Tipo de máquina"), type: "select", options: ["db-f1-micro", "db-g1-small", "db-custom-2-7680", "db-custom-4-15360"] },
          { id: "region", label: t("Región"), type: "select", options: REGIONS.includes(ctx.region) ? REGIONS : [ctx.region, ...REGIONS] },
          { id: "ha", label: t("Alta disponibilidad (varias zonas)"), type: "check", help: t("Duplica el coste; sobrevive a la caída de una zona.") },
        ],
        build: (v) => `gcloud sql instances create ${q(v.name.trim())} --database-version=${v.version} --tier=${v.tier} --region=${v.region}${v.ha ? " --availability-type=REGIONAL" : ""}`,
      }}
      actions={[
        { icon: "↻", label: t("Reiniciar"), note: () => t("Reiniciar Cloud SQL"), cmds: (rs) => rs.map((r) => `gcloud sql instances restart ${r.name} --quiet`) },
        {
          icon: "🗑",
          label: t("Eliminar"),
          danger: true,
          note: () => t("Eliminar instancia de Cloud SQL"),
          cmds: (rs) => rs.map((r) => `gcloud sql instances delete ${r.name} --quiet`),
          confirm: { title: t("¿Eliminar las instancias?"), text: t("Se perderán las bases de datos y sus copias de seguridad."), button: t("Eliminar") },
        },
      ]}
    />
  );
}

// ---------------------------------------------------------------- Pub/Sub

export function PubSub({ ctx }: { ctx: Ctx }) {
  const { t } = useI18n();
  const topics = vals(ctx.data.topics).sort((a, b) => a.name.localeCompare(b.name));
  const subs = vals(ctx.data.subscriptions).sort((a, b) => a.name.localeCompare(b.name));
  return (
    <>
      <Resource
        ctx={ctx}
        title={t("Temas")}
        caption={t("Temas de Pub/Sub")}
        rows={topics}
        rowKey={(r) => r.name}
        empty={t("No hay temas.")}
        cols={[
          { key: "name", label: t("ID del tema"), render: (r) => <strong>{r.name}</strong> },
          { key: "subs", label: t("Suscripciones"), render: (r) => subs.filter((s) => s.topic === r.name).length },
          { key: "pub", label: t("Mensajes publicados"), render: (r) => r.publishedCount ?? 0 },
        ]}
        create={{
          label: t("Crear tema"),
          title: t("Crear un tema"),
          submit: t("Crear"),
          note: () => t("Crear tema de Pub/Sub"),
          initial: { name: "orders", sub: true },
          fields: [
            { id: "name", label: t("ID del tema"), required: true },
            { id: "sub", label: t("Añadir una suscripción predeterminada"), type: "check" },
          ],
          build: (v) => `gcloud pubsub topics create ${q(v.name.trim())}${v.sub ? ` && gcloud pubsub subscriptions create ${q(`${v.name.trim()}-sub`)} --topic=${q(v.name.trim())}` : ""}`,
        }}
        actions={[
          { icon: "✉", label: t("Publicar mensaje"), single: true, note: () => t("Publicar mensaje"), cmds: (rs) => rs.map((r) => `gcloud pubsub topics publish ${r.name} --message='hola'`) },
          {
            icon: "🗑",
            label: t("Eliminar"),
            danger: true,
            note: () => t("Eliminar tema"),
            cmds: (rs) => rs.map((r) => `gcloud pubsub topics delete ${r.name} --quiet`),
            confirm: { title: t("¿Eliminar los temas?"), text: t("Sus suscripciones dejarán de recibir mensajes."), button: t("Eliminar") },
          },
        ]}
      />
      <Resource
        ctx={ctx}
        title={t("Suscripciones")}
        caption={t("Suscripciones de Pub/Sub")}
        rows={subs}
        rowKey={(r) => r.name}
        empty={t("No hay suscripciones.")}
        cols={[
          { key: "name", label: t("ID de la suscripción"), render: (r) => <strong>{r.name}</strong> },
          { key: "topic", label: t("Tema"), render: (r) => r.topic },
          { key: "type", label: t("Tipo de entrega"), render: (r) => (r.pushEndpoint ? t("Push") : t("Pull")) },
          { key: "ack", label: t("Plazo de confirmación"), render: (r) => `${r.ackDeadlineSeconds} s` },
          { key: "backlog", label: t("Mensajes sin confirmar"), render: (r) => (Array.isArray(r.backlog) ? r.backlog.length : r.backlog ?? 0) },
          { key: "dl", label: t("Tema de mensajes fallidos"), render: (r) => r.deadLetterTopic || <span className="muted">—</span> },
        ]}
        create={{
          label: t("Crear suscripción"),
          title: t("Crear una suscripción"),
          submit: t("Crear"),
          note: () => t("Crear suscripción"),
          initial: { name: "", topic: topics[0]?.name ?? "", ack: "10" },
          fields: [
            { id: "name", label: t("ID de la suscripción"), required: true },
            { id: "topic", label: t("Tema"), type: "select", options: topics.map((x) => x.name), required: true },
            { id: "ack", label: t("Plazo de confirmación (s)"), type: "number" },
          ],
          build: (v) => `gcloud pubsub subscriptions create ${q(v.name.trim())} --topic=${q(v.topic)}${v.ack && v.ack !== "10" ? ` --ack-deadline=${v.ack}` : ""}`,
        }}
        actions={[
          { icon: "⇩", label: t("Extraer mensajes"), single: true, note: () => t("Extraer mensajes"), cmds: (rs) => rs.map((r) => `gcloud pubsub subscriptions pull ${r.name} --auto-ack --limit=5`) },
          {
            icon: "🗑",
            label: t("Eliminar"),
            danger: true,
            note: () => t("Eliminar suscripción"),
            cmds: (rs) => rs.map((r) => `gcloud pubsub subscriptions delete ${r.name} --quiet`),
            confirm: { title: t("¿Eliminar las suscripciones?"), text: t("Los mensajes pendientes se perderán."), button: t("Eliminar") },
          },
        ]}
      />
    </>
  );
}

// ---------------------------------------------------------------- BigQuery

export function BigQuery({ ctx }: { ctx: Ctx }) {
  const { t } = useI18n();
  const rows = vals(ctx.data.datasets).sort((a, b) => a.datasetId.localeCompare(b.datasetId));
  const tables = rows.flatMap((d) => vals(d.tables).map((tb: any) => ({ ...tb, dataset: d.datasetId })));
  return (
    <>
      <Resource
        ctx={ctx}
        title={t("BigQuery: conjuntos de datos")}
        caption={t("Conjuntos de datos de BigQuery")}
        rows={rows}
        rowKey={(r) => r.datasetId}
        empty={t("No hay conjuntos de datos.")}
        cols={[
          { key: "id", label: t("ID"), render: (r) => <strong>{r.datasetId}</strong> },
          { key: "loc", label: t("Ubicación"), render: (r) => r.location },
          { key: "tables", label: t("Tablas"), render: (r) => Object.keys(r.tables ?? {}).length },
          { key: "pub", label: t("Acceso público"), render: (r) => (isPublic(r.access) ? <Pill tone="bad">{t("Público")}</Pill> : t("No")) },
        ]}
        create={{
          label: t("Crear conjunto de datos"),
          title: t("Crear un conjunto de datos"),
          submit: t("Crear"),
          note: () => t("Crear conjunto de datos"),
          initial: { id: "analytics", location: "EU" },
          fields: [
            { id: "id", label: t("ID del conjunto de datos"), required: true, help: t("Letras, números y guiones bajos.") },
            { id: "location", label: t("Ubicación"), type: "select", options: ["EU", "US", ...REGIONS] },
          ],
          build: (v) => `bq mk --dataset --location=${v.location} ${ctx.project}:${v.id.trim()}`,
        }}
        actions={[
          {
            icon: "🗑",
            label: t("Eliminar"),
            danger: true,
            note: () => t("Eliminar conjunto de datos"),
            cmds: (rs) => rs.map((r) => `bq rm -r -f -d ${ctx.project}:${r.datasetId}`),
            confirm: { title: t("¿Eliminar los conjuntos de datos?"), text: t("Se borrarán con todas sus tablas."), button: t("Eliminar") },
          },
        ]}
      />
      {!ctx.creating && <Page title={t("Tablas")}>
        <Table
          caption={t("Tablas de BigQuery")}
          rows={tables}
          rowKey={(r: any) => `${r.dataset}.${r.tableId}`}
          empty={t("No hay tablas.")}
          cols={[
            { key: "id", label: t("Tabla"), render: (r: any) => <code>{r.dataset}.{r.tableId}</code> },
            { key: "type", label: t("Tipo"), render: (r: any) => r.type },
            { key: "rows", label: t("Filas"), render: (r: any) => r.numRows ?? 0 },
            { key: "part", label: t("Partición"), render: (r: any) => r.partitionField || <span className="muted">—</span> },
            {
              key: "q",
              label: t("Consultar"),
              render: (r: any) => (
                <button type="button" className="cc-link" onClick={() => ctx.paste(`bq query --use_legacy_sql=false --dry_run 'SELECT * FROM \`${ctx.project}.${r.dataset}.${r.tableId}\` LIMIT 10'`)}>
                  {t("Consulta")}
                </button>
              ),
            },
          ]}
        />
      </Page>}
    </>
  );
}

// ---------------------------------------------------------------- GKE

export function GKE({ ctx }: { ctx: Ctx }) {
  const { t } = useI18n();
  const rows = vals(ctx.data.clusters).sort((a, b) => a.name.localeCompare(b.name));
  const pods: any[] = ctx.data.pods ?? [];
  return (
    <>
      <Resource
        ctx={ctx}
        title={t("Clústeres de Kubernetes")}
        caption={t("Clústeres de GKE")}
        rows={rows}
        rowKey={(r) => r.name}
        empty={t("No hay clústeres.")}
        cols={[
          { key: "st", label: t("Estado"), render: (r) => <Status state={r.status} /> },
          { key: "name", label: t("Nombre"), render: (r) => <strong>{r.name}</strong> },
          { key: "loc", label: t("Ubicación"), render: (r) => r.location },
          { key: "mode", label: t("Modo"), render: (r) => (r.autopilot ? "Autopilot" : t("Estándar")) },
          { key: "nodes", label: t("Nodos"), render: (r) => (r.autopilot ? <span className="muted">{t("gestionados")}</span> : (r.nodePools ?? []).reduce((a: number, p: any) => a + (p.nodeCount ?? 0), 0)) },
          { key: "chan", label: t("Canal de versiones"), render: (r) => r.releaseChannel },
          { key: "priv", label: t("Nodos privados"), render: (r) => (r.privateNodes ? t("Sí") : t("No")) },
          {
            key: "conn",
            label: t("Conectar"),
            render: (r) => (
              <button type="button" className="cc-link" onClick={() => ctx.run(`gcloud container clusters get-credentials ${r.name} ${locFlag(r.location)}`, t("Conectar con el clúster"))}>
                {t("Conectar")}
              </button>
            ),
          },
        ]}
        create={{
          label: t("Crear clúster"),
          title: t("Crear un clúster"),
          submit: t("Crear"),
          note: () => t("Crear clúster de GKE"),
          initial: { name: "cluster-1", mode: "auto", region: ctx.region, zone: ctx.zone, nodes: "3", machine: "e2-medium" },
          fields: [
            { id: "name", label: t("Nombre"), required: true, help: nameHelp(t) },
            { id: "mode", label: t("Modo"), type: "select", options: [["auto", t("Autopilot (Google gestiona los nodos)")], ["standard", t("Estándar (tú gestionas los nodos)")]] },
            { id: "region", label: t("Región"), type: "select", options: REGIONS.includes(ctx.region) ? REGIONS : [ctx.region, ...REGIONS], when: (v) => v.mode === "auto" },
            { id: "zone", label: t("Zona"), type: "select", options: ZONES.includes(ctx.zone) ? ZONES : [ctx.zone, ...ZONES], when: (v) => v.mode !== "auto" },
            { id: "nodes", label: t("Número de nodos"), type: "number", when: (v) => v.mode !== "auto" },
            { id: "machine", label: t("Tipo de máquina de los nodos"), type: "select", options: MACHINES, when: (v) => v.mode !== "auto" },
          ],
          build: (v) =>
            v.mode === "auto"
              ? `gcloud container clusters create-auto ${q(v.name.trim())} --region=${v.region}`
              : `gcloud container clusters create ${q(v.name.trim())} --zone=${v.zone} --num-nodes=${v.nodes} --machine-type=${v.machine}`,
        }}
        actions={[
          {
            icon: "🗑",
            label: t("Eliminar"),
            danger: true,
            note: () => t("Eliminar clúster"),
            cmds: (rs) => rs.map((r) => `gcloud container clusters delete ${r.name} ${locFlag(r.location)} --quiet`),
            confirm: { title: t("¿Eliminar los clústeres?"), text: t("Se borrarán con todas sus cargas de trabajo."), button: t("Eliminar") },
          },
        ]}
      />
      {!ctx.creating && <Page title={t("Cargas de trabajo (pods)")}>
        <Table
          caption={t("Pods de los clústeres")}
          rows={pods}
          rowKey={(p: any) => `${p.cluster}/${p.namespace}/${p.name}`}
          empty={t("No hay pods. Despliega con kubectl apply -f desde la terminal.")}
          cols={[
            { key: "st", label: t("Estado"), render: (p: any) => <Status state={p.status === "Running" && p.ready ? "RUNNING" : p.status} /> },
            { key: "name", label: t("Nombre"), render: (p: any) => <code>{p.name}</code> },
            { key: "ns", label: t("Espacio de nombres"), render: (p: any) => p.namespace },
            { key: "cl", label: t("Clúster"), render: (p: any) => p.cluster },
            { key: "ready", label: t("Listo"), render: (p: any) => (p.ready ? t("Sí") : t("No")) },
            { key: "restarts", label: t("Reinicios"), render: (p: any) => p.restarts ?? 0 },
            { key: "reason", label: t("Motivo"), render: (p: any) => <span className="small">{p.reason || p.status}</span> },
          ]}
        />
      </Page>}
    </>
  );
}

// ---------------------------------------------------------------- Secrets

export function Secrets({ ctx }: { ctx: Ctx }) {
  const { t } = useI18n();
  const rows = vals(ctx.data.secrets).sort((a, b) => a.name.localeCompare(b.name));
  const [add, setAdd] = useState("");
  return (
    <Resource
      ctx={ctx}
      title={t("Secret Manager")}
      intro={t("Guarda contraseñas y claves fuera del código. Las aplicaciones leen el valor con el rol secretAccessor.")}
      caption={t("Secretos")}
      rows={rows}
      rowKey={(r) => r.name}
      empty={t("No hay secretos.")}
      cols={[
        { key: "name", label: t("Nombre"), render: (r) => <strong>{r.name}</strong> },
        { key: "rep", label: t("Replicación"), render: (r) => (r.replication === "automatic" ? t("Automática") : r.replication) },
        { key: "ver", label: t("Versiones"), render: (r) => (r.versions ?? []).length },
        { key: "cmek", label: t("Cifrado"), render: (r) => (r.kmsKey ? t("Clave gestionada por el cliente") : t("Gestionado por Google")) },
        { key: "add", label: t("Nueva versión"), render: (r) => <button type="button" className="cc-link" onClick={() => setAdd(r.name)}>{t("Añadir versión")}</button> },
      ]}
      create={{
        label: t("Crear secreto"),
        title: t("Crear un secreto"),
        submit: t("Crear"),
        note: () => t("Crear secreto"),
        initial: { name: "", value: "" },
        fields: [
          { id: "name", label: t("Nombre"), required: true },
          { id: "value", label: t("Valor del secreto"), type: "password", required: true, help: t("En la terminal se ve el valor: en un proyecto real usa un archivo (--data-file=ruta).") },
        ],
        build: (v) => `printf '%s' ${q(v.value)} | gcloud secrets create ${q(v.name.trim())} --replication-policy=automatic --data-file=-`,
      }}
      actions={[
        {
          icon: "🗑",
          label: t("Eliminar"),
          danger: true,
          note: () => t("Eliminar secreto"),
          cmds: (rs) => rs.map((r) => `gcloud secrets delete ${r.name} --quiet`),
          confirm: { title: t("¿Eliminar los secretos?"), text: t("Las aplicaciones que los lean empezarán a fallar."), button: t("Eliminar") },
        },
      ]}
    >
      {add && (
        <Drawer
          title={t("Añadir una versión a {s}", { s: add })}
          submit={t("Añadir")}
          initial={{ value: "" }}
          fields={[{ id: "value", label: t("Valor del secreto"), type: "password", required: true }]}
          build={(v) => `printf '%s' ${q(v.value)} | gcloud secrets versions add ${add} --data-file=-`}
          onClose={() => setAdd("")}
          onRun={(cmd) => ctx.run(cmd, t("Añadir versión del secreto"))}
          onPaste={ctx.paste}
        />
      )}
    </Resource>
  );
}

// ---------------------------------------------------------------- APIs

export function APIs({ ctx }: { ctx: Ctx }) {
  const { t } = useI18n();
  const [filter, setFilter] = useState("");
  const rows = Object.entries(ctx.data.services ?? {})
    .map(([name, on]) => ({ name, on: !!on }))
    .filter((r) => r.name.includes(filter.trim()))
    .sort((a, b) => a.name.localeCompare(b.name));
  return (
    <Resource
      ctx={ctx}
      title={t("APIs y servicios")}
      intro={
        <input value={filter} onChange={(e) => setFilter(e.target.value)} placeholder={t("Filtrar APIs")} aria-label={t("Filtrar APIs")} style={{ maxWidth: 320 }} />
      }
      caption={t("APIs del proyecto")}
      rows={rows}
      rowKey={(r) => r.name}
      empty={t("Ninguna API coincide.")}
      cols={[
        { key: "st", label: t("Estado"), render: (r) => <Status state={r.on ? "ENABLED" : "DISABLED"} /> },
        { key: "name", label: t("Servicio"), render: (r) => <code>{r.name}</code> },
        { key: "on", label: t("Habilitada"), render: (r) => (r.on ? t("Sí") : t("No")) },
      ]}
      create={{
        label: t("Habilitar API"),
        title: t("Habilitar una API"),
        submit: t("Habilitar"),
        note: () => t("Habilitar API"),
        initial: { name: "" },
        fields: [{ id: "name", label: t("Nombre del servicio"), required: true, placeholder: "run.googleapis.com" }],
        build: (v) => `gcloud services enable ${q(v.name.trim())}`,
      }}
      actions={[
        { icon: "▶", label: t("Habilitar"), note: () => t("Habilitar API"), cmds: (rs) => rs.map((r) => `gcloud services enable ${r.name}`) },
        {
          icon: "⏻",
          label: t("Inhabilitar"),
          danger: true,
          note: () => t("Inhabilitar API"),
          cmds: (rs) => rs.map((r) => `gcloud services disable ${r.name}`),
          confirm: { title: t("¿Inhabilitar las APIs?"), text: t("Los recursos y aplicaciones que las usan dejarán de funcionar."), button: t("Inhabilitar") },
        },
      ]}
    />
  );
}
