"use client";

// More console products: Compute (instance templates, instance groups,
// health checks, snapshots), network services (load balancing, Cloud NAT,
// Cloud DNS), VPC (IP addresses, routes), security (KMS), IAM (roles,
// organization policies), CI/CD (Artifact Registry, Cloud Build), Logging
// (log-based metrics), Monitoring (alerting, notification channels), GKE
// (workloads, services, configuration) and BigQuery Studio.

import { useMemo, useState, type ReactNode } from "react";
import { useI18n } from "@/lib/i18n";
import type { Ctx } from "./pages";
import { Detail, Page, Pill, Props, REGIONS, Section, Status, Table, Tool, ZONES, locFlag, q, type Col, type FormSpec } from "./ui";
import { useCtxHost, splitList, startupFlag, saveStartup } from "./details";

const vals = (m: any): any[] => (m ? Object.values(m) : []);
const regions = (r: string) => (REGIONS.includes(r) ? REGIONS : [r, ...REGIONS]);
const zones = (z: string) => (ZONES.includes(z) ? ZONES : [z, ...ZONES]);
const IMAGES = ["debian-12", "debian-11", "ubuntu-2204-lts", "rocky-linux-9", "cos-stable"];
const MACHINES = ["e2-micro", "e2-small", "e2-medium", "e2-standard-2", "e2-standard-4", "n2-standard-2"];

/** List is a list page with a create form, row actions and a details view. */
function List<T>({
  ctx,
  title,
  intro,
  rows,
  rowKey,
  cols,
  empty,
  create,
  del,
  extra,
  detail,
}: {
  ctx: Ctx;
  title: string;
  intro?: string;
  rows: T[];
  rowKey: (r: T) => string;
  cols: Col<T>[];
  empty: string;
  create?: { label: string; form: FormSpec };
  del?: { cmd: (r: T) => string; title: string; text: string; note: string };
  extra?: (h: ReturnType<typeof useCtxHost>, sel: T[]) => ReactNode;
  detail?: (r: T, close: () => void, h: ReturnType<typeof useCtxHost>) => ReactNode;
}) {
  const { t } = useI18n();
  const h = useCtxHost(ctx);
  const [sel, setSel] = useState<string[]>([]);
  const [open, setOpen] = useState("");
  if (h.page) return h.page;
  const chosen = rows.filter((r) => sel.includes(rowKey(r)));
  const openRow = open ? rows.find((r) => rowKey(r) === open) : undefined;
  if (detail && openRow) {
    return (
      <>
        {detail(openRow, () => setOpen(""), h)}
        {h.node}
      </>
    );
  }
  const shown = detail ? cols.map((c, i) => (i === 0 ? { ...c, render: (r: T) => <button type="button" className="cc-link cc-name" onClick={() => setOpen(rowKey(r))}>{c.render(r)}</button> } : c)) : cols;
  return (
    <Page
      title={title}
      intro={intro}
      actions={
        <>
          {create && <Tool icon="add" label={create.label} onClick={() => h.form(create.form)} />}
          <Tool icon="refresh" label={t("Actualizar")} onClick={ctx.refresh} />
          {extra?.(h, chosen)}
          {del && <Tool icon="delete" label={t("Eliminar")} danger disabled={!chosen.length} onClick={() => h.confirm({ title: del.title, text: del.text, cmds: chosen.map(del.cmd), button: t("Eliminar"), note: del.note })} />}
        </>
      }
    >
      <Table caption={title} cols={shown} rows={rows} rowKey={rowKey} selected={del || extra ? sel : undefined} onSelect={del || extra ? setSel : undefined} empty={empty} />
      {h.node}
    </Page>
  );
}

// ---------------------------------------------------------------- Compute

export function InstanceTemplates({ ctx }: { ctx: Ctx }) {
  const { t } = useI18n();
  const nets = Object.keys(ctx.data.networks ?? {});
  return (
    <List
      ctx={ctx}
      title={t("Plantillas de instancia")}
      intro={t("Definen cómo son las VM de un grupo de instancias gestionado.")}
      rows={vals(ctx.data.instanceTemplates)}
      rowKey={(r) => r.name}
      empty={t("No hay plantillas de instancia.")}
      cols={[
        { key: "name", label: t("Nombre"), render: (r) => r.name },
        { key: "mt", label: t("Tipo de máquina"), render: (r) => r.machineType },
        { key: "img", label: t("Imagen"), render: (r) => r.image },
        { key: "net", label: t("Red"), render: (r) => r.network },
        { key: "tags", label: t("Etiquetas de red"), render: (r) => (r.tags ?? []).join(", ") || "—" },
        { key: "ip", label: t("IP externa"), render: (r) => (r.noAddress ? t("Ninguna") : t("Efímera")) },
      ]}
      create={{
        label: t("Crear plantilla de instancia"),
        form: {
          title: t("Crear una plantilla de instancia"),
          submit: t("Crear"),
          note: t("Crear plantilla de instancia"),
          initial: { name: "web-template", machine: "e2-small", image: "debian-12", network: nets[0] ?? "default", tags: "http-server", ext: false, startup: "" },
          fields: [
            { id: "name", label: t("Nombre"), required: true },
            { id: "machine", label: t("Tipo de máquina"), type: "select", options: MACHINES },
            { id: "image", label: t("Imagen del disco de arranque"), type: "select", options: IMAGES },
            { id: "network", label: t("Red"), type: "select", options: nets },
            { id: "tags", label: t("Etiquetas de red") },
            { id: "ext", label: t("Asignar una IP externa"), type: "check" },
            { id: "startup", label: t("Secuencia de comandos de inicio"), type: "textarea" },
          ],
          build: (v) => {
            const base = [`gcloud compute instance-templates create ${q(v.name.trim())}`, `--machine-type=${v.machine}`, `--image-family=${v.image}`, v.network !== "default" ? `--network=${v.network}` : "", v.tags.trim() ? `--tags=${splitList(v.tags).join(",")}` : "", v.ext ? "" : "--no-address"].filter(Boolean).join(" ");
            return v.startup.trim() ? startupFlag(base, v.startup, v.name.trim()) : base;
          },
          before: (v) => saveStartup(ctx, v.startup, v.name.trim()),
        },
      }}
      del={{ cmd: (r) => `gcloud compute instance-templates delete ${r.name} --quiet`, title: t("¿Eliminar las plantillas?"), text: t("No se pueden borrar mientras un grupo de instancias las use."), note: t("Eliminar plantilla") }}
    />
  );
}

export function InstanceGroups({ ctx }: { ctx: Ctx }) {
  const { t } = useI18n();
  const tpls = Object.keys(ctx.data.instanceTemplates ?? {});
  const loc = (r: any) => (r.region ? `--region=${r.region}` : `--zone=${r.zone}`);
  return (
    <List
      ctx={ctx}
      title={t("Grupos de instancias")}
      rows={vals(ctx.data.instanceGroups)}
      rowKey={(r) => r.name}
      empty={t("No hay grupos de instancias.")}
      cols={[
        { key: "name", label: t("Nombre"), render: (r) => r.name },
        { key: "loc", label: t("Ubicación"), render: (r) => r.region || r.zone },
        { key: "type", label: t("Tipo"), render: (r) => (r.managed ? t("Gestionado") : t("No gestionado")) },
        { key: "tpl", label: t("Plantilla"), render: (r) => r.instanceTemplate || "—" },
        { key: "n", label: t("Instancias"), render: (r) => `${(r.instances ?? []).length} / ${r.targetSize}` },
        { key: "as", label: t("Autoescalado"), render: (r) => (r.autoscaler ? `${r.autoscaler.minNumReplicas}–${r.autoscaler.maxNumReplicas} · CPU ${Math.round(r.autoscaler.targetCpuUtilization * 100)}%` : t("Desactivado")) },
        { key: "ah", label: t("Reparación automática"), render: (r) => (r.autoHealing ? t("Activado") : t("Desactivado")) },
      ]}
      create={{
        label: t("Crear grupo de instancias"),
        form: {
          title: t("Crear un grupo de instancias gestionado"),
          submit: t("Crear"),
          note: t("Crear grupo de instancias"),
          initial: { name: "web-mig", tpl: tpls[0] ?? "", size: "2", zone: ctx.zone },
          fields: [
            { id: "name", label: t("Nombre"), required: true },
            { id: "tpl", label: t("Plantilla de instancia"), type: "select", options: tpls, required: true, help: tpls.length ? undefined : t("Primero crea una plantilla de instancia.") },
            { id: "size", label: t("Número de instancias"), type: "number" },
            { id: "zone", label: t("Zona"), type: "select", options: zones(ctx.zone) },
          ],
          build: (v) => (v.tpl ? `gcloud compute instance-groups managed create ${q(v.name.trim())} --template=${v.tpl} --size=${v.size} --zone=${v.zone}` : ""),
        },
      }}
      extra={(h, sel) => (
        <>
          <Tool icon="edit" label={t("Cambiar tamaño")} disabled={sel.length !== 1} onClick={() => h.form({ kind: "drawer", title: t("Cambiar el tamaño de {g}", { g: sel[0].name }), submit: t("Guardar"), note: t("Cambiar tamaño del grupo"), initial: { size: String(sel[0].targetSize) }, fields: [{ id: "size", label: t("Número de instancias"), type: "number", required: true }], build: (v) => `gcloud compute instance-groups managed resize ${sel[0].name} ${loc(sel[0])} --size=${v.size}` })} />
          <Tool icon="monitoring" label={t("Autoescalado")} disabled={sel.length !== 1} onClick={() => h.form({ kind: "drawer", title: t("Autoescalado de {g}", { g: sel[0].name }), submit: t("Guardar"), note: t("Configurar autoescalado"), initial: { on: !!sel[0].autoscaler, min: String(sel[0].autoscaler?.minNumReplicas ?? 1), max: String(sel[0].autoscaler?.maxNumReplicas ?? 5), cpu: String(Math.round((sel[0].autoscaler?.targetCpuUtilization ?? 0.6) * 100)) }, fields: [{ id: "on", label: t("Activar el autoescalado"), type: "check" }, { id: "min", label: t("Mínimo de instancias"), type: "number", when: (v) => v.on }, { id: "max", label: t("Máximo de instancias"), type: "number", when: (v) => v.on }, { id: "cpu", label: t("Uso de CPU objetivo (%)"), type: "number", when: (v) => v.on }], build: (v) => (v.on ? `gcloud compute instance-groups managed set-autoscaling ${sel[0].name} ${loc(sel[0])} --min-num-replicas=${v.min} --max-num-replicas=${v.max} --target-cpu-utilization=${(Number(v.cpu) / 100).toFixed(2)}` : sel[0].autoscaler ? `gcloud compute instance-groups managed stop-autoscaling ${sel[0].name} ${loc(sel[0])}` : "") })} />
          <Tool icon="refresh" label={t("Sustituir instancias")} disabled={sel.length !== 1} onClick={() => h.confirm({ title: t("¿Sustituir las instancias de {g}?", { g: sel[0].name }), text: t("Se recrean poco a poco con la plantilla actual (actualización gradual)."), cmds: [`gcloud compute instance-groups managed rolling-action replace ${sel[0].name} ${loc(sel[0])}`], button: t("Sustituir"), note: t("Actualización gradual") })} />
        </>
      )}
      del={{ cmd: (r) => `gcloud compute instance-groups managed delete ${r.name} ${loc(r)} --quiet`, title: t("¿Eliminar los grupos de instancias?"), text: t("Se borrarán también sus VM."), note: t("Eliminar grupo de instancias") }}
    />
  );
}

export function HealthChecks({ ctx }: { ctx: Ctx }) {
  const { t } = useI18n();
  return (
    <List
      ctx={ctx}
      title={t("Comprobaciones de estado")}
      rows={vals(ctx.data.healthChecks)}
      rowKey={(r) => r.name}
      empty={t("No hay comprobaciones de estado.")}
      cols={[
        { key: "name", label: t("Nombre"), render: (r) => r.name },
        { key: "p", label: t("Protocolo"), render: (r) => r.type },
        { key: "port", label: t("Puerto"), render: (r) => r.port },
        { key: "path", label: t("Ruta de la solicitud"), render: (r) => r.requestPath || "—" },
        { key: "i", label: t("Intervalo"), render: (r) => (r.checkIntervalSec ? `${r.checkIntervalSec} s` : "—") },
      ]}
      create={{
        label: t("Crear comprobación de estado"),
        form: {
          kind: "drawer",
          title: t("Crear una comprobación de estado"),
          submit: t("Crear"),
          note: t("Crear comprobación de estado"),
          initial: { name: "http-80", proto: "http", port: "80", path: "/" },
          fields: [
            { id: "name", label: t("Nombre"), required: true },
            { id: "proto", label: t("Protocolo"), type: "select", options: ["http", "https", "tcp"] },
            { id: "port", label: t("Puerto"), type: "number" },
            { id: "path", label: t("Ruta de la solicitud"), when: (v) => v.proto !== "tcp" },
          ],
          build: (v) => `gcloud compute health-checks create ${v.proto} ${q(v.name.trim())} --port=${v.port}${v.proto !== "tcp" && v.path ? ` --request-path=${q(v.path)}` : ""}`,
        },
      }}
      del={{ cmd: (r) => `gcloud compute health-checks delete ${r.name} --quiet`, title: t("¿Eliminar las comprobaciones de estado?"), text: t("Los servicios de backend que las usen dejarán de comprobar sus instancias."), note: t("Eliminar comprobación de estado") }}
    />
  );
}

export function Snapshots({ ctx }: { ctx: Ctx }) {
  const { t } = useI18n();
  const disks = vals(ctx.data.disks);
  return (
    <List
      ctx={ctx}
      title={t("Instantáneas")}
      rows={vals(ctx.data.snapshots)}
      rowKey={(r) => r.name}
      empty={t("No hay instantáneas.")}
      cols={[
        { key: "name", label: t("Nombre"), render: (r) => r.name },
        { key: "d", label: t("Disco de origen"), render: (r) => r.sourceDisk },
        { key: "s", label: t("Tamaño"), render: (r) => `${r.diskSizeGb} GB` },
        { key: "l", label: t("Ubicación"), render: (r) => r.storageLocation || "—" },
        { key: "c", label: t("Creada"), render: (r) => String(r.creationTimestamp ?? "").replace("T", " ").replace(/Z$/, "") },
      ]}
      create={{
        label: t("Crear instantánea"),
        form: { kind: "drawer", title: t("Crear una instantánea"), submit: t("Crear"), note: t("Crear instantánea"), initial: { name: "snapshot-1", disk: disks[0] ? `${disks[0].zone}/${disks[0].name}` : "" }, fields: [{ id: "name", label: t("Nombre"), required: true }, { id: "disk", label: t("Disco de origen"), type: "select", required: true, options: disks.map((d) => [`${d.zone}/${d.name}`, `${d.name} (${d.zone})`] as [string, string]) }], build: (v) => { const [z, d] = String(v.disk).split("/"); return d ? `gcloud compute disks snapshot ${d} --zone=${z} --snapshot-names=${q(v.name.trim())}` : ""; } },
      }}
      extra={(h, sel) => <Tool icon="add" label={t("Crear disco a partir de la instantánea")} disabled={sel.length !== 1} onClick={() => h.form({ kind: "drawer", title: t("Crear un disco a partir de {s}", { s: sel[0].name }), submit: t("Crear"), note: t("Crear disco desde instantánea"), initial: { name: `${sel[0].name}-disk`, zone: ctx.zone }, fields: [{ id: "name", label: t("Nombre"), required: true }, { id: "zone", label: t("Zona"), type: "select", options: zones(ctx.zone) }], build: (v) => `gcloud compute disks create ${q(v.name.trim())} --zone=${v.zone} --source-snapshot=${sel[0].name}` })} />}
      del={{ cmd: (r) => `gcloud compute snapshots delete ${r.name} --quiet`, title: t("¿Eliminar las instantáneas?"), text: t("No podrás restaurar los discos desde ellas."), note: t("Eliminar instantánea") }}
    />
  );
}

// ---------------------------------------------------------------- Network services

export function LoadBalancing({ ctx }: { ctx: Ctx }) {
  const { t } = useI18n();
  const h = useCtxHost(ctx);
  if (h.page) return h.page;
  const d = ctx.data;
  const frs = vals(d.forwardingRules);
  const bes = vals(d.backendServices);
  const health: any[] = d.backendHealth ?? [];
  const igs = vals(d.instanceGroups);
  const hcs = Object.keys(d.healthChecks ?? {});
  const beOf = (fr: any) => {
    if (fr.backendService) return fr.backendService;
    const tp = d.targetProxies?.[fr.target];
    return tp ? d.urlMaps?.[tp.urlMap]?.defaultService : undefined;
  };
  const wizard: FormSpec = {
    title: t("Crear un balanceador de carga de aplicaciones externo"),
    submit: t("Crear"),
    note: t("Crear balanceador de carga"),
    initial: { name: "web", ig: igs[0] ? `${igs[0].zone}/${igs[0].name}` : "", hc: hcs[0] ?? "", newhc: hcs.length === 0, port: "80", ip: false },
    fields: [
      { id: "name", label: t("Nombre del balanceador (prefijo)"), required: true, section: t("Configuración del frontend") },
      { id: "ip", label: t("Reservar una IP estática global"), type: "check" },
      { id: "ig", label: t("Grupo de instancias del backend"), type: "select", required: true, options: igs.map((g) => [`${g.zone}/${g.name}`, `${g.name} (${g.zone || g.region})`] as [string, string]), section: t("Configuración del backend"), help: igs.length ? undefined : t("Primero crea un grupo de instancias.") },
      { id: "newhc", label: t("Crear una comprobación de estado HTTP nueva"), type: "check" },
      { id: "hc", label: t("Comprobación de estado"), type: "select", options: hcs, when: (v) => !v.newhc },
      { id: "port", label: t("Puerto"), type: "number" },
    ],
    build: (v) => {
      if (!v.ig) return "";
      const [zone, ig] = String(v.ig).split("/");
      const n = String(v.name).trim();
      const hc = v.newhc ? `${n}-hc` : v.hc;
      return [
        v.newhc ? `gcloud compute health-checks create http ${hc} --port=${v.port} --request-path=/` : "",
        v.ip ? `gcloud compute addresses create ${n}-ip --global` : "",
        `gcloud compute backend-services create ${n}-backend --protocol=HTTP --health-checks=${hc} --global`,
        `gcloud compute backend-services add-backend ${n}-backend --instance-group=${ig} --instance-group-zone=${zone} --global`,
        `gcloud compute url-maps create ${n}-map --default-service=${n}-backend`,
        `gcloud compute target-http-proxies create ${n}-proxy --url-map=${n}-map`,
        `gcloud compute forwarding-rules create ${n}-frontend --global --target-http-proxy=${n}-proxy --ports=${v.port}${v.ip ? ` --address=${n}-ip` : ""}`,
      ].filter(Boolean).join(" && ");
    },
  };
  return (
    <>
      <Page
        title={t("Balanceo de carga")}
        actions={
          <>
            <Tool icon="add" label={t("Crear balanceador de carga")} onClick={() => h.form(wizard)} />
            <Tool icon="refresh" label={t("Actualizar")} onClick={ctx.refresh} />
          </>
        }
      >
        <Table
          caption={t("Balanceadores de carga")}
          rows={frs}
          rowKey={(r: any) => r.name}
          empty={t("No hay balanceadores de carga.")}
          cols={[
            { key: "n", label: t("Nombre (frontend)"), render: (r: any) => r.name },
            { key: "ip", label: t("IP:puerto"), render: (r: any) => <code>{r.IPAddress}:{(r.ports ?? []).join(",")}</code> },
            { key: "s", label: t("Ámbito"), render: (r: any) => (r.global ? t("Global") : r.region) },
            { key: "sch", label: t("Esquema"), render: (r: any) => r.loadBalancingScheme },
            { key: "be", label: t("Backend"), render: (r: any) => beOf(r) ?? "—" },
            {
              key: "h",
              label: t("Estado de los backends"),
              render: (r: any) => {
                const hs = health.filter((x) => x.backendService === beOf(r));
                const ok = hs.filter((x) => x.healthy).length;
                return hs.length ? <Pill tone={ok === hs.length ? "ok" : ok ? "warn" : "bad"}>{ok}/{hs.length} {t("en buen estado")}</Pill> : "—";
              },
            },
            { key: "x", label: "", render: (r: any) => <button type="button" className="cc-link danger" onClick={() => h.confirm({ title: t("¿Eliminar el frontend {f}?", { f: r.name }), text: t("El balanceador dejará de recibir tráfico. El backend se conserva."), cmds: [`gcloud compute forwarding-rules delete ${r.name} ${r.global ? "--global" : `--region=${r.region}`} --quiet`], button: t("Eliminar"), note: t("Eliminar frontend") })}>{t("Eliminar")}</button> },
          ]}
        />
      </Page>
      <Page title={t("Servicios de backend")}>
        <Table
          caption={t("Servicios de backend")}
          rows={bes}
          rowKey={(r: any) => r.name}
          empty={t("No hay servicios de backend.")}
          cols={[
            { key: "n", label: t("Nombre"), render: (r: any) => r.name },
            { key: "p", label: t("Protocolo"), render: (r: any) => r.protocol },
            { key: "b", label: t("Backends"), render: (r: any) => (r.backends ?? []).map((b: any) => b.group || b.neg).join(", ") || "—" },
            { key: "hc", label: t("Comprobación de estado"), render: (r: any) => (r.healthChecks ?? []).join(", ") || "—" },
            { key: "cdn", label: "Cloud CDN", render: (r: any) => (r.enableCDN ? t("Activado") : t("Desactivado")) },
            { key: "arm", label: "Cloud Armor", render: (r: any) => r.securityPolicy || "—" },
            {
              key: "h",
              label: t("Estado"),
              render: (r: any) => (
                <span className="cc-chips">
                  {health.filter((x) => x.backendService === r.name).map((x) => (
                    <span key={x.instance} className="cc-chip plain" title={x.reason}>
                      <Status state={x.healthy ? "RUNNING" : "FAILED"} /> {x.instance}
                    </span>
                  ))}
                </span>
              ),
            },
          ]}
        />
      </Page>
      {h.node}
    </>
  );
}

export function CloudNAT({ ctx }: { ctx: Ctx }) {
  const { t } = useI18n();
  const routers = vals(ctx.data.routers);
  const nats = routers.flatMap((r) => (r.nats ?? []).map((n: any) => ({ ...n, router: r.name, region: r.region, network: r.network })));
  const nets = Object.keys(ctx.data.networks ?? {});
  return (
    <List
      ctx={ctx}
      title={t("Cloud NAT")}
      intro={t("Da salida a internet a las VM sin IP externa, sin exponerlas a conexiones entrantes.")}
      rows={nats}
      rowKey={(r: any) => `${r.region}/${r.router}/${r.name}`}
      empty={t("No hay pasarelas de Cloud NAT.")}
      cols={[
        { key: "name", label: t("Nombre de la pasarela"), render: (r: any) => r.name },
        { key: "net", label: t("Red"), render: (r: any) => r.network },
        { key: "reg", label: t("Región"), render: (r: any) => r.region },
        { key: "rt", label: "Cloud Router", render: (r: any) => r.router },
        { key: "sub", label: t("Subredes"), render: (r: any) => (r.allSubnets ? t("Todas las subredes") : (r.subnets ?? []).join(", ")) },
        { key: "ip", label: t("Direcciones IP"), render: (r: any) => (r.autoAllocateIps ? t("Automáticas") : t("Manuales")) },
        { key: "log", label: t("Registros"), render: (r: any) => (r.logErrors ? t("Errores") : t("Desactivado")) },
      ]}
      create={{
        label: t("Crear pasarela de Cloud NAT"),
        form: {
          title: t("Crear una pasarela de Cloud NAT"),
          submit: t("Crear"),
          note: t("Crear Cloud NAT"),
          initial: { name: "nat-1", network: nets.includes("default") ? "default" : nets[0] ?? "", region: ctx.region, router: "" },
          fields: [
            { id: "name", label: t("Nombre de la pasarela"), required: true },
            { id: "network", label: t("Red"), type: "select", options: nets },
            { id: "region", label: t("Región"), type: "select", options: regions(ctx.region) },
            { id: "router", label: t("Cloud Router existente (vacío = crear uno)"), type: "select", options: [["", t("Crear un router nuevo")], ...routers.map((r) => r.name)] },
          ],
          build: (v) => {
            const r = v.router || `${v.name}-router`;
            return [v.router ? "" : `gcloud compute routers create ${r} --network=${v.network} --region=${v.region}`, `gcloud compute routers nats create ${q(v.name.trim())} --router=${r} --region=${v.region} --auto-allocate-nat-external-ips --nat-all-subnet-ip-ranges`].filter(Boolean).join(" && ");
          },
        },
      }}
      del={{ cmd: (r: any) => `gcloud compute routers nats delete ${r.name} --router=${r.router} --region=${r.region} --quiet`, title: t("¿Eliminar las pasarelas NAT?"), text: t("Las VM sin IP externa perderán la salida a internet."), note: t("Eliminar Cloud NAT") }}
    />
  );
}

export function CloudDNS({ ctx }: { ctx: Ctx }) {
  const { t } = useI18n();
  const nets = Object.keys(ctx.data.networks ?? {});
  return (
    <List
      ctx={ctx}
      title={t("Cloud DNS")}
      rows={vals(ctx.data.dnsZones)}
      rowKey={(r) => r.name}
      empty={t("No hay zonas.")}
      cols={[
        { key: "name", label: t("Nombre de la zona"), render: (r) => r.name },
        { key: "dns", label: t("Nombre de DNS"), render: (r) => r.dnsName },
        { key: "v", label: t("Tipo"), render: (r) => (r.visibility === "private" ? t("Privada") : t("Pública")) },
        { key: "n", label: t("Registros"), render: (r) => (r.records ?? []).length },
        { key: "sec", label: "DNSSEC", render: (r) => (r.dnssec ? t("Activado") : t("Desactivado")) },
      ]}
      create={{
        label: t("Crear zona"),
        form: {
          title: t("Crear una zona DNS"),
          submit: t("Crear"),
          note: t("Crear zona DNS"),
          initial: { name: "", dns: "", vis: "public", nets: nets[0] ?? "", desc: "" },
          fields: [
            { id: "vis", label: t("Tipo de zona"), type: "select", options: [["public", t("Pública")], ["private", t("Privada")]] },
            { id: "name", label: t("Nombre de la zona"), required: true },
            { id: "dns", label: t("Nombre de DNS"), placeholder: "example.com.", required: true },
            { id: "nets", label: t("Redes (zona privada)"), when: (v) => v.vis === "private" },
            { id: "desc", label: t("Descripción") },
          ],
          build: (v) => `gcloud dns managed-zones create ${q(v.name.trim())} --dns-name=${q(v.dns.trim().replace(/([^.])$/, "$1."))} --description=${q(v.desc || v.name)}${v.vis === "private" ? ` --visibility=private --networks=${splitList(v.nets).join(",")}` : ""}`,
        },
      }}
      del={{ cmd: (r) => `gcloud dns managed-zones delete ${r.name} --quiet`, title: t("¿Eliminar las zonas?"), text: t("Los nombres de la zona dejarán de resolverse."), note: t("Eliminar zona DNS") }}
      detail={(z, close, h) => (
        <Detail
          title={z.name}
          onBack={close}
          actions={<Tool icon="add" label={t("Añadir conjunto de registros estándar")} onClick={() => h.form({ kind: "drawer", title: t("Crear un conjunto de registros"), submit: t("Crear"), note: t("Crear registro DNS"), initial: { name: "", type: "A", ttl: "300", data: "" }, fields: [{ id: "name", label: t("Nombre de DNS"), help: t("Vacío = el propio dominio {d}", { d: z.dnsName }) }, { id: "type", label: t("Tipo de registro"), type: "select", options: ["A", "AAAA", "CNAME", "MX", "TXT", "NS"] }, { id: "ttl", label: "TTL", type: "number" }, { id: "data", label: t("Datos (separados por comas)"), required: true, placeholder: "34.1.2.3" }], build: (v) => `gcloud dns record-sets create ${q(v.name.trim() ? `${v.name.trim().replace(/\.$/, "")}.${z.dnsName}` : z.dnsName)} --zone=${z.name} --type=${v.type} --ttl=${v.ttl} --rrdatas=${q(splitList(v.data).join(","))}` })} />}
          tabs={[
            {
              id: "rec",
              label: t("Registros"),
              render: () => (
                <Table
                  caption={t("Conjuntos de registros")}
                  rows={z.records ?? []}
                  rowKey={(r: any) => `${r.name}|${r.type}`}
                  empty={t("No hay registros.")}
                  cols={[
                    { key: "n", label: t("Nombre de DNS"), render: (r: any) => r.name },
                    { key: "t", label: t("Tipo"), render: (r: any) => r.type },
                    { key: "ttl", label: "TTL", render: (r: any) => r.ttl },
                    { key: "d", label: t("Datos"), render: (r: any) => <code>{(r.rrdatas ?? []).join(", ")}</code> },
                    { key: "x", label: "", render: (r: any) => (["NS", "SOA"].includes(r.type) && r.name === z.dnsName ? null : <button type="button" className="cc-link danger" onClick={() => h.confirm({ title: t("¿Eliminar el registro {n} {t}?", { n: r.name, t: r.type }), text: t("El nombre dejará de resolverse a estos datos."), cmds: [`gcloud dns record-sets delete ${r.name} --zone=${z.name} --type=${r.type}`], button: t("Eliminar"), note: t("Eliminar registro DNS") })}>{t("Eliminar")}</button>) },
                  ]}
                />
              ),
            },
            { id: "d", label: t("Detalles"), render: () => <Props rows={[[t("Nombre de DNS"), z.dnsName], [t("Tipo"), z.visibility], [t("Redes"), (z.networks ?? []).join(", ")], ["DNSSEC", z.dnssec ? t("Activado") : t("Desactivado")], [t("Descripción"), z.description]]} /> },
          ]}
        />
      )}
    />
  );
}

export function IPAddresses({ ctx }: { ctx: Ctx }) {
  const { t } = useI18n();
  return (
    <List
      ctx={ctx}
      title={t("Direcciones IP")}
      intro={t("Las IP estáticas reservadas se cobran también cuando no están en uso.")}
      rows={vals(ctx.data.addresses)}
      rowKey={(r) => `${r.region || "global"}/${r.name}`}
      empty={t("No hay direcciones IP estáticas.")}
      cols={[
        { key: "name", label: t("Nombre"), render: (r) => r.name },
        { key: "ip", label: t("Dirección IP"), render: (r) => <code>{r.address}{r.prefixLength ? `/${r.prefixLength}` : ""}</code> },
        { key: "t", label: t("Tipo de acceso"), render: (r) => (r.addressType === "INTERNAL" ? t("Interna") : t("Externa")) },
        { key: "reg", label: t("Región"), render: (r) => r.region || t("Global") },
        { key: "p", label: t("Finalidad"), render: (r) => r.purpose || "—" },
        { key: "u", label: t("En uso por"), render: (r) => r.user || <Pill tone="warn">{t("Sin usar")}</Pill> },
      ]}
      create={{
        label: t("Reservar dirección IP estática externa"),
        form: { kind: "drawer", title: t("Reservar una dirección estática"), submit: t("Reservar"), note: t("Reservar IP estática"), initial: { name: "", scope: "regional", region: ctx.region }, fields: [{ id: "name", label: t("Nombre"), required: true }, { id: "scope", label: t("Tipo"), type: "select", options: [["regional", t("Regional")], ["global", t("Global")]] }, { id: "region", label: t("Región"), type: "select", options: regions(ctx.region), when: (v) => v.scope === "regional" }], build: (v) => `gcloud compute addresses create ${q(v.name.trim())} ${v.scope === "global" ? "--global" : `--region=${v.region}`}` },
      }}
      del={{ cmd: (r) => `gcloud compute addresses delete ${r.name} ${r.region ? `--region=${r.region}` : "--global"} --quiet`, title: t("¿Liberar las direcciones?"), text: t("No podrás recuperar la misma IP."), note: t("Liberar IP estática") }}
    />
  );
}

export function Routes({ ctx }: { ctx: Ctx }) {
  const { t } = useI18n();
  const nets = Object.keys(ctx.data.networks ?? {});
  return (
    <List
      ctx={ctx}
      title={t("Rutas")}
      rows={vals(ctx.data.routes)}
      rowKey={(r) => r.name}
      empty={t("No hay rutas.")}
      cols={[
        { key: "name", label: t("Nombre"), render: (r) => r.name },
        { key: "net", label: t("Red"), render: (r) => r.network },
        { key: "dst", label: t("Rango de IP de destino"), render: (r) => <code>{r.destRange}</code> },
        { key: "p", label: t("Prioridad"), render: (r) => r.priority },
        { key: "nh", label: t("Siguiente salto"), render: (r) => r.nextHopGateway || r.nextHopInstance || r.nextHopIp || r.nextHopPeering || "—" },
        { key: "tags", label: t("Etiquetas de instancia"), render: (r) => (r.tags ?? []).join(", ") || "—" },
        { key: "sys", label: t("Tipo"), render: (r) => (r.system ? t("Del sistema") : t("Estática")) },
      ]}
      create={{
        label: t("Crear ruta"),
        form: { kind: "drawer", title: t("Crear una ruta"), submit: t("Crear"), note: t("Crear ruta"), initial: { name: "", network: nets[0] ?? "default", dst: "", hop: "gateway", instance: "", priority: "1000", tags: "" }, fields: [{ id: "name", label: t("Nombre"), required: true }, { id: "network", label: t("Red"), type: "select", options: nets }, { id: "dst", label: t("Rango de IP de destino"), required: true, placeholder: "10.50.0.0/16" }, { id: "hop", label: t("Siguiente salto"), type: "select", options: [["gateway", t("Pasarela de internet predeterminada")], ["instance", t("Instancia")]] }, { id: "instance", label: t("Instancia"), type: "select", options: Object.keys(ctx.data.instances ?? {}), when: (v) => v.hop === "instance" }, { id: "priority", label: t("Prioridad"), type: "number" }, { id: "tags", label: t("Etiquetas de instancia (opcional)") }], build: (v) => `gcloud compute routes create ${q(v.name.trim())} --network=${v.network} --destination-range=${q(v.dst.trim())} ${v.hop === "gateway" ? "--next-hop-gateway=default-internet-gateway" : `--next-hop-instance=${v.instance} --next-hop-instance-zone=${ctx.data.instances?.[v.instance]?.zone ?? ctx.zone}`} --priority=${v.priority}${v.tags.trim() ? ` --tags=${splitList(v.tags).join(",")}` : ""}` },
      }}
      del={{ cmd: (r) => `gcloud compute routes delete ${r.name} --quiet`, title: t("¿Eliminar las rutas?"), text: t("El tráfico hacia esos destinos seguirá otra ruta o se perderá."), note: t("Eliminar ruta") }}
    />
  );
}

// ---------------------------------------------------------------- Security

export function KMS({ ctx }: { ctx: Ctx }) {
  const { t } = useI18n();
  const rings = vals(ctx.data.keyRings);
  return (
    <List
      ctx={ctx}
      title={t("Gestión de claves")}
      rows={rings}
      rowKey={(r) => `${r.location}/${r.name}`}
      empty={t("No hay llaveros.")}
      cols={[
        { key: "name", label: t("Llavero"), render: (r) => r.name },
        { key: "l", label: t("Ubicación"), render: (r) => r.location },
        { key: "k", label: t("Claves"), render: (r) => Object.keys(r.keys ?? {}).length },
      ]}
      create={{ label: t("Crear llavero"), form: { kind: "drawer", title: t("Crear un llavero"), submit: t("Crear"), note: t("Crear llavero"), initial: { name: "", loc: ctx.region }, fields: [{ id: "name", label: t("Nombre del llavero"), required: true }, { id: "loc", label: t("Ubicación"), type: "select", options: [...regions(ctx.region), "global", "europe", "us"] }], build: (v) => `gcloud kms keyrings create ${q(v.name.trim())} --location=${v.loc}` } }}
      detail={(kr, close, h) => (
        <Detail
          title={kr.name}
          onBack={close}
          actions={<Tool icon="add" label={t("Crear clave")} onClick={() => h.form({ kind: "drawer", title: t("Crear una clave en {k}", { k: kr.name }), submit: t("Crear"), note: t("Crear clave KMS"), initial: { name: "", purpose: "encryption", rot: "90d" }, fields: [{ id: "name", label: t("Nombre de la clave"), required: true }, { id: "purpose", label: t("Finalidad"), type: "select", options: [["encryption", t("Cifrado/descifrado simétrico")], ["asymmetric-signing", t("Firma asimétrica")]] }, { id: "rot", label: t("Periodo de rotación"), type: "select", options: [["", t("Nunca (rotación manual)")], "30d", "90d", "365d"], when: (v) => v.purpose === "encryption" }], build: (v) => `gcloud kms keys create ${q(v.name.trim())} --keyring=${kr.name} --location=${kr.location} --purpose=${v.purpose}${v.purpose === "encryption" && v.rot ? ` --rotation-period=${v.rot} --next-rotation-time=$(date -u -d '+${v.rot.replace("d", "")} days' +%Y-%m-%dT%H:%M:%SZ)` : ""}` })} />}
          tabs={[{
            id: "keys",
            label: t("Claves"),
            render: () => (
              <Table
                caption={t("Claves de {k}", { k: kr.name })}
                rows={vals(kr.keys)}
                rowKey={(k: any) => k.name}
                empty={t("No hay claves.")}
                cols={[
                  { key: "n", label: t("Nombre"), render: (k: any) => k.name },
                  { key: "p", label: t("Finalidad"), render: (k: any) => k.purpose },
                  { key: "r", label: t("Periodo de rotación"), render: (k: any) => k.rotationPeriod || t("Nunca") },
                  { key: "pl", label: t("Nivel de protección"), render: (k: any) => k.protectionLevel },
                  { key: "v", label: t("Versiones"), render: (k: any) => (k.versions ?? []).length },
                  { key: "x", label: "", render: (k: any) => <button type="button" className="cc-link" onClick={() => ctx.run(`gcloud kms keys versions create --key=${k.name} --keyring=${kr.name} --location=${kr.location} --primary`, t("Rotar clave"))}>{t("Rotar ahora")}</button> },
                ]}
              />
            ),
          }]}
        />
      )}
    />
  );
}

// ---------------------------------------------------------------- IAM

export function Roles({ ctx }: { ctx: Ctx }) {
  const { t } = useI18n();
  const roles = Object.entries(ctx.data.customRoles ?? {}).map(([id, r]: [string, any]) => ({ id, ...r }));
  return (
    <List
      ctx={ctx}
      title={t("Roles")}
      intro={t("Roles personalizados del proyecto: solo los permisos que hacen falta.")}
      rows={roles}
      rowKey={(r: any) => r.id}
      empty={t("No hay roles personalizados.")}
      cols={[
        { key: "t", label: t("Título"), render: (r: any) => r.title || r.id },
        { key: "id", label: "ID", render: (r: any) => <code>projects/{ctx.project}/roles/{r.id}</code> },
        { key: "p", label: t("Permisos"), render: (r: any) => (r.includedPermissions ?? r.permissions ?? []).length },
        { key: "s", label: t("Fase"), render: (r: any) => r.stage || "GA" },
      ]}
      create={{ label: t("Crear rol"), form: { title: t("Crear un rol personalizado"), submit: t("Crear"), note: t("Crear rol personalizado"), initial: { id: "", title: "", perms: "" }, fields: [{ id: "id", label: "ID", required: true, placeholder: "storageReader" }, { id: "title", label: t("Título"), required: true }, { id: "perms", label: t("Permisos (separados por comas)"), type: "textarea", required: true, placeholder: "storage.objects.get, storage.objects.list" }], build: (v) => `gcloud iam roles create ${q(v.id.trim())} --project=${ctx.project} --title=${q(v.title)} --permissions=${splitList(v.perms).join(",")}` } }}
      del={{ cmd: (r: any) => `gcloud iam roles delete ${r.id} --project=${ctx.project} --quiet`, title: t("¿Eliminar los roles?"), text: t("Quien los tenga perderá esos permisos."), note: t("Eliminar rol") }}
    />
  );
}

export function OrgPolicies({ ctx }: { ctx: Ctx }) {
  const { t } = useI18n();
  const rows = Object.entries(ctx.data.orgPolicies ?? {}).map(([k, p]: [string, any]) => ({ k, ...p }));
  return (
    <Page title={t("Políticas de organización")} intro={t("Restricciones heredadas de la organización o la carpeta. Se aplican aunque tengas el rol de propietario.")}>
      <Table
        caption={t("Políticas de organización")}
        rows={rows}
        rowKey={(r: any) => r.k}
        empty={t("No se aplica ninguna política de organización a este proyecto.")}
        cols={[
          { key: "c", label: t("Restricción"), render: (r: any) => <code>{r.constraint || r.k}</code> },
          { key: "e", label: t("Aplicación"), render: (r: any) => (r.enforce ? <Pill tone="warn">{t("Aplicada")}</Pill> : t("Lista")) },
          { key: "a", label: t("Valores permitidos"), render: (r: any) => (r.allowedValues ?? []).join(", ") || "—" },
          { key: "d", label: t("Valores denegados"), render: (r: any) => (r.deniedValues ?? []).join(", ") || "—" },
        ]}
      />
    </Page>
  );
}

// ---------------------------------------------------------------- CI/CD

export function ArtifactRegistry({ ctx }: { ctx: Ctx }) {
  const { t } = useI18n();
  return (
    <List
      ctx={ctx}
      title={t("Repositorios")}
      rows={vals(ctx.data.artifactRepos)}
      rowKey={(r) => r.name}
      empty={t("No hay repositorios.")}
      cols={[
        { key: "name", label: t("Nombre"), render: (r) => r.name },
        { key: "f", label: t("Formato"), render: (r) => r.format },
        { key: "l", label: t("Ubicación"), render: (r) => r.location },
        { key: "i", label: t("Imágenes"), render: (r) => Object.keys(r.images ?? {}).length },
        { key: "s", label: t("Análisis de vulnerabilidades"), render: (r) => (r.vulnerabilityScanning ? t("Activado") : t("Desactivado")) },
        { key: "c", label: t("Políticas de limpieza"), render: (r) => (r.cleanupPolicies ? t("Sí") : t("No")) },
      ]}
      create={{ label: t("Crear repositorio"), form: { title: t("Crear un repositorio"), submit: t("Crear"), note: t("Crear repositorio"), initial: { name: "", format: "docker", loc: ctx.region }, fields: [{ id: "name", label: t("Nombre"), required: true }, { id: "format", label: t("Formato"), type: "select", options: ["docker", "python", "npm", "maven", "go"] }, { id: "loc", label: t("Región"), type: "select", options: regions(ctx.region) }], build: (v) => `gcloud artifacts repositories create ${q(v.name.trim())} --repository-format=${v.format} --location=${v.loc}` } }}
      del={{ cmd: (r) => `gcloud artifacts repositories delete ${r.name} --location=${r.location} --quiet`, title: t("¿Eliminar los repositorios?"), text: t("Se borrarán todas sus imágenes."), note: t("Eliminar repositorio") }}
      detail={(r, close) => (
        <Detail
          title={r.name}
          onBack={close}
          tabs={[{
            id: "img",
            label: t("Imágenes"),
            render: () => (
              <>
                <p className="cc-help">{t("Ruta del repositorio")}: <code>{r.location}-docker.pkg.dev/{ctx.project}/{r.name}</code></p>
                <Table caption={t("Imágenes")} rows={Object.entries(r.images ?? {}).map(([i, tags]: [string, any]) => ({ i, tags }))} rowKey={(x: any) => x.i} empty={t("No hay imágenes. Súbelas con docker push o gcloud builds submit.")} cols={[{ key: "i", label: t("Imagen"), render: (x: any) => <code>{x.i}</code> }, { key: "t", label: t("Etiquetas"), render: (x: any) => (x.tags ?? []).join(", ") || "—" }]} />
              </>
            ),
          }]}
        />
      )}
    />
  );
}

export function CloudBuild({ ctx }: { ctx: Ctx }) {
  const { t } = useI18n();
  const builds = [...(ctx.data.builds ?? [])].reverse();
  const triggers = vals(ctx.data.buildTriggers);
  return (
    <>
      <List
        ctx={ctx}
        title={t("Historial de compilaciones")}
        rows={builds}
        rowKey={(b: any) => b.id}
        empty={t("No hay compilaciones. Lanza una con gcloud builds submit.")}
        cols={[
          { key: "id", label: "ID", render: (b: any) => <code>{String(b.id).slice(0, 8)}</code> },
          { key: "st", label: t("Estado"), render: (b: any) => <Pill tone={b.status === "SUCCESS" ? "ok" : b.status === "FAILURE" ? "bad" : undefined}>{b.status}</Pill> },
          { key: "src", label: t("Origen"), render: (b: any) => b.source },
          { key: "tr", label: t("Activador"), render: (b: any) => b.buildTriggerId || "—" },
          { key: "img", label: t("Imágenes"), render: (b: any) => (b.images ?? []).join(", ") || "—" },
          { key: "c", label: t("Creada"), render: (b: any) => String(b.createTime ?? "").replace("T", " ").replace(/Z$/, "") },
        ]}
        detail={(b, close) => (
          <Detail
            title={t("Compilación {id}", { id: String(b.id).slice(0, 8) })}
            onBack={close}
            status={<Status state={b.status === "SUCCESS" ? "RUNNING" : b.status === "FAILURE" ? "FAILED" : b.status} />}
            tabs={[
              { id: "steps", label: t("Pasos"), render: () => <ol className="cc-steps">{(b.steps ?? []).map((s: string, i: number) => <li key={i}><code>{s}</code></li>)}</ol> },
              { id: "log", label: t("Registro de compilación"), render: () => <pre className="cc-output">{(b.log ?? []).join("\n")}</pre> },
              { id: "d", label: t("Detalles"), render: () => <Props rows={[["ID", <code key="i">{b.id}</code>], [t("Estado"), b.status], [t("Origen"), b.source], ["Commit", b.commitSha], [t("Cuenta de servicio"), b.serviceAccount], [t("Imágenes"), (b.images ?? []).join(", ")]]} /> },
            ]}
          />
        )}
      />
      <List
        ctx={ctx}
        title={t("Activadores")}
        rows={triggers}
        rowKey={(r: any) => r.name}
        empty={t("No hay activadores.")}
        cols={[
          { key: "name", label: t("Nombre"), render: (r: any) => r.name },
          { key: "repo", label: t("Repositorio"), render: (r: any) => r.repo },
          { key: "br", label: t("Rama"), render: (r: any) => <code>{r.branchPattern}</code> },
          { key: "cfg", label: t("Configuración"), render: (r: any) => r.filename },
          { key: "sa", label: t("Cuenta de servicio"), render: (r: any) => r.serviceAccount || t("Predeterminada") },
        ]}
        extra={(h, sel) => <Tool icon="play" label={t("Ejecutar")} disabled={sel.length !== 1} onClick={() => ctx.run(`gcloud builds triggers run ${sel[0].name} --branch=main`, t("Ejecutar activador"))} />}
        del={{ cmd: (r: any) => `gcloud builds triggers delete ${r.name} --quiet`, title: t("¿Eliminar los activadores?"), text: t("Dejarán de lanzar compilaciones."), note: t("Eliminar activador") }}
      />
    </>
  );
}

// ---------------------------------------------------------------- Logging / Monitoring

export function LogMetrics({ ctx }: { ctx: Ctx }) {
  const { t } = useI18n();
  return (
    <List
      ctx={ctx}
      title={t("Métricas basadas en registros")}
      rows={vals(ctx.data.logMetrics)}
      rowKey={(r) => r.name}
      empty={t("No hay métricas definidas por el usuario.")}
      cols={[
        { key: "name", label: t("Nombre"), render: (r) => r.name },
        { key: "d", label: t("Descripción"), render: (r) => r.description || "—" },
        { key: "f", label: t("Filtro"), render: (r) => <code>{r.filter}</code> },
        { key: "m", label: t("Métrica en Monitoring"), render: (r) => <code>logging.googleapis.com/user/{r.name}</code> },
      ]}
      create={{ label: t("Crear métrica"), form: { title: t("Crear una métrica basada en registros"), submit: t("Crear métrica"), note: t("Crear métrica basada en registros"), initial: { name: "", desc: "", filter: 'resource.type="cloud_run_revision" AND httpRequest.status>=500' }, fields: [{ id: "name", label: t("Nombre de la métrica"), required: true }, { id: "desc", label: t("Descripción") }, { id: "filter", label: t("Filtro de registros"), type: "textarea", required: true }], build: (v) => `gcloud logging metrics create ${q(v.name.trim())} --description=${q(v.desc || v.name)} --log-filter=${q(v.filter.replace(/\n/g, " "))}` } }}
      del={{ cmd: (r) => `gcloud logging metrics delete ${r.name} --quiet`, title: t("¿Eliminar las métricas?"), text: t("Las alertas que las usen dejarán de funcionar."), note: t("Eliminar métrica") }}
    />
  );
}

export function Alerting({ ctx }: { ctx: Ctx }) {
  const { t } = useI18n();
  const channels = vals(ctx.data.notificationChannels);
  const metrics = Object.keys(ctx.data.logMetrics ?? {});
  return (
    <>
      <List
        ctx={ctx}
        title={t("Políticas de alertas")}
        rows={vals(ctx.data.alertPolicies)}
        rowKey={(r) => r.name}
        empty={t("No hay políticas de alertas.")}
        cols={[
          { key: "name", label: t("Nombre"), render: (r) => r.displayName || r.name },
          { key: "st", label: t("Estado"), render: (r) => (r.firing ? <Pill tone="bad">{t("Incidente abierto")}</Pill> : r.enabled ? <Pill tone="ok">{t("Habilitada")}</Pill> : <Pill>{t("Inhabilitada")}</Pill>) },
          { key: "c", label: t("Condición"), render: (r) => (r.conditions ?? []).map((c: any) => `${c.filter} ${c.comparison === "COMPARISON_LT" ? "<" : ">"} ${c.thresholdValue} (${c.duration})`).join("; ") },
          { key: "ch", label: t("Canales de notificación"), render: (r) => (r.notificationChannels ?? []).map((c: string) => ctx.data.notificationChannels?.[c]?.displayName ?? c).join(", ") || "—" },
        ]}
        create={{
          label: t("Crear política"),
          form: {
            title: t("Crear una política de alertas"),
            submit: t("Crear política"),
            note: t("Crear política de alertas"),
            initial: { name: "", filter: metrics[0] ? `metric.type="logging.googleapis.com/user/${metrics[0]}"` : "", cmp: ">", value: "5", dur: "60s", ch: channels[0]?.name ?? "", doc: "" },
            fields: [
              { id: "filter", label: t("Filtro de la métrica"), required: true, section: t("Condición"), help: metrics.length ? t("Métricas de registros disponibles: {m}", { m: metrics.join(", ") }) : t("Crea antes una métrica basada en registros o usa una métrica del sistema.") },
              { id: "cmp", label: t("Activador"), type: "select", options: [[">", t("Por encima del umbral")], ["<", t("Por debajo del umbral")]] },
              { id: "value", label: t("Valor del umbral"), type: "number" },
              { id: "dur", label: t("Durante"), type: "select", options: ["0s", "60s", "300s", "600s"] },
              { id: "ch", label: t("Canal de notificación"), type: "select", options: [["", t("Ninguno")], ...channels.map((c) => [c.name, c.displayName] as [string, string])], section: t("Notificaciones") },
              { id: "name", label: t("Nombre de la política"), required: true },
              { id: "doc", label: t("Documentación (qué hacer cuando salte)"), type: "textarea" },
            ],
            build: (v) => `gcloud alpha monitoring policies create --display-name=${q(v.name.trim())} --condition-display-name=${q(v.name.trim())} --condition-filter=${q(v.filter)} --if=${q(`${v.cmp} ${v.value}`)} --duration=${v.dur}${v.ch ? ` --notification-channels=${v.ch}` : ""}${v.doc.trim() ? ` --documentation=${q(v.doc)}` : ""}`,
          },
        }}
      />
      <List
        ctx={ctx}
        title={t("Canales de notificación")}
        rows={channels}
        rowKey={(r: any) => r.name}
        empty={t("No hay canales de notificación.")}
        cols={[
          { key: "n", label: t("Nombre"), render: (r: any) => r.displayName },
          { key: "t", label: t("Tipo"), render: (r: any) => r.type },
          { key: "l", label: t("Destino"), render: (r: any) => Object.values(r.labels ?? {}).join(", ") },
        ]}
        create={{ label: t("Añadir canal de correo"), form: { kind: "drawer", title: t("Añadir un canal de correo electrónico"), submit: t("Guardar"), note: t("Crear canal de notificación"), initial: { name: "", email: "" }, fields: [{ id: "name", label: t("Nombre visible"), required: true }, { id: "email", label: t("Correo electrónico"), required: true }], build: (v) => `gcloud alpha monitoring channels create --display-name=${q(v.name.trim())} --type=email --channel-labels=email_address=${q(v.email.trim())}` } }}
      />
    </>
  );
}

// ---------------------------------------------------------------- GKE workloads

type K8sRow = { cluster: any; ns: string; name: string; obj: any };
function k8s(ctx: Ctx, kind: "deployments" | "services" | "configMaps" | "secrets" | "ingresses") {
  const out: K8sRow[] = [];
  for (const c of vals(ctx.data.clusters)) {
    for (const [ns, n] of Object.entries(c.k8s?.namespaces ?? {}) as [string, any][]) {
      for (const [name, obj] of Object.entries(n[kind] ?? {})) out.push({ cluster: c, ns, name, obj });
    }
  }
  return out;
}
const creds = (c: any) => `gcloud container clusters get-credentials ${c.name} ${locFlag(c.location)}`;

export function Workloads({ ctx }: { ctx: Ctx }) {
  const { t } = useI18n();
  const clusters = vals(ctx.data.clusters);
  const rows = k8s(ctx, "deployments").filter((r) => r.ns !== "kube-system");
  return (
    <List
      ctx={ctx}
      title={t("Cargas de trabajo")}
      rows={rows}
      rowKey={(r) => `${r.cluster.name}/${r.ns}/${r.name}`}
      empty={t("No hay cargas de trabajo. Despliega una o usa kubectl apply -f.")}
      cols={[
        { key: "n", label: t("Nombre"), render: (r) => r.name },
        { key: "st", label: t("Estado"), render: (r) => (r.obj.availableReplicas >= r.obj.replicas ? <Pill tone="ok">OK</Pill> : <Pill tone="warn">{t("Pods pendientes")}</Pill>) },
        { key: "p", label: t("Pods"), render: (r) => `${r.obj.availableReplicas ?? 0}/${r.obj.replicas}` },
        { key: "img", label: t("Imagen"), render: (r) => (r.obj.template?.containers ?? []).map((c: any) => c.image).join(", ") },
        { key: "ns", label: t("Espacio de nombres"), render: (r) => r.ns },
        { key: "cl", label: t("Clúster"), render: (r) => r.cluster.name },
      ]}
      create={{
        label: t("Implementar"),
        form: {
          title: t("Crear una implementación"),
          submit: t("Implementar"),
          note: t("Implementar carga de trabajo"),
          initial: { name: "web", image: "nginx:1.27", replicas: "2", cluster: clusters[0]?.name ?? "", ns: "default", expose: false, port: "80" },
          fields: [
            { id: "image", label: t("Imagen del contenedor"), required: true, section: t("Contenedor") },
            { id: "name", label: t("Nombre de la aplicación"), required: true, section: t("Configuración") },
            { id: "replicas", label: t("Réplicas"), type: "number" },
            { id: "ns", label: t("Espacio de nombres") },
            { id: "cluster", label: t("Clúster"), type: "select", options: clusters.map((c) => c.name), required: true, help: clusters.length ? undefined : t("Primero crea un clúster.") },
            { id: "expose", label: t("Exponer con un balanceador de carga"), type: "check", section: t("Exponer (opcional)") },
            { id: "port", label: t("Puerto"), type: "number", when: (v) => v.expose },
          ],
          build: (v) => {
            const c = clusters.find((x) => x.name === v.cluster);
            if (!c) return "";
            return [creds(c), `kubectl create deployment ${q(v.name.trim())} --image=${q(v.image.trim())} --replicas=${v.replicas} -n ${v.ns}`, v.expose ? `kubectl expose deployment ${q(v.name.trim())} --port=${v.port} --type=LoadBalancer -n ${v.ns}` : ""].filter(Boolean).join(" && ");
          },
        },
      }}
      extra={(h, sel) => (
        <Tool icon="edit" label={t("Escalar")} disabled={sel.length !== 1} onClick={() => h.form({ kind: "drawer", title: t("Escalar {d}", { d: sel[0].name }), submit: t("Escalar"), note: t("Escalar implementación"), initial: { n: String(sel[0].obj.replicas) }, fields: [{ id: "n", label: t("Réplicas"), type: "number", required: true }], build: (v) => `${creds(sel[0].cluster)} && kubectl scale deployment ${sel[0].name} --replicas=${v.n} -n ${sel[0].ns}` })} />
      )}
      del={{ cmd: (r) => `${creds(r.cluster)} && kubectl delete deployment ${r.name} -n ${r.ns}`, title: t("¿Eliminar las cargas de trabajo?"), text: t("Se borrarán sus pods."), note: t("Eliminar carga de trabajo") }}
    />
  );
}

export function K8sServices({ ctx }: { ctx: Ctx }) {
  const { t } = useI18n();
  const svcs = k8s(ctx, "services").filter((r) => r.ns !== "kube-system");
  const ings = k8s(ctx, "ingresses");
  const deps = k8s(ctx, "deployments").filter((r) => r.ns !== "kube-system");
  return (
    <>
      <List
        ctx={ctx}
        title={t("Servicios")}
        rows={svcs}
        rowKey={(r) => `${r.cluster.name}/${r.ns}/${r.name}`}
        empty={t("No hay servicios.")}
        cols={[
          { key: "n", label: t("Nombre"), render: (r) => r.name },
          { key: "t", label: t("Tipo"), render: (r) => r.obj.type },
          { key: "e", label: t("Endpoints"), render: (r) => <code>{r.obj.externalIP ? `${r.obj.externalIP}:` : ""}{(r.obj.ports ?? []).map((p: any) => p.port).join(",")}</code> },
          { key: "ip", label: "Cluster IP", render: (r) => <code>{r.obj.clusterIP}</code> },
          { key: "sel", label: t("Selector"), render: (r) => Object.entries(r.obj.selector ?? {}).map(([k, v]) => `${k}=${v}`).join(", ") },
          { key: "ns", label: t("Espacio de nombres"), render: (r) => r.ns },
          { key: "cl", label: t("Clúster"), render: (r) => r.cluster.name },
        ]}
        create={{
          label: t("Exponer una implementación"),
          form: {
            kind: "drawer",
            title: t("Exponer una implementación"),
            submit: t("Exponer"),
            note: t("Exponer implementación"),
            initial: { dep: deps[0] ? `${deps[0].cluster.name}/${deps[0].ns}/${deps[0].name}` : "", type: "LoadBalancer", port: "80", target: "" },
            fields: [
              { id: "dep", label: t("Implementación"), type: "select", required: true, options: deps.map((d) => `${d.cluster.name}/${d.ns}/${d.name}`) },
              { id: "type", label: t("Tipo de servicio"), type: "select", options: ["ClusterIP", "NodePort", "LoadBalancer"] },
              { id: "port", label: t("Puerto"), type: "number" },
              { id: "target", label: t("Puerto de destino (vacío = el mismo)"), type: "number" },
            ],
            build: (v) => {
              const d = deps.find((x) => `${x.cluster.name}/${x.ns}/${x.name}` === v.dep);
              return d ? `${creds(d.cluster)} && kubectl expose deployment ${d.name} --port=${v.port}${v.target ? ` --target-port=${v.target}` : ""} --type=${v.type} -n ${d.ns}` : "";
            },
          },
        }}
        del={{ cmd: (r) => `${creds(r.cluster)} && kubectl delete service ${r.name} -n ${r.ns}`, title: t("¿Eliminar los servicios?"), text: t("Dejarán de enrutar tráfico a sus pods."), note: t("Eliminar servicio") }}
      />
      <Page title="Ingress">
        <Table caption="Ingress" rows={ings} rowKey={(r) => `${r.cluster.name}/${r.ns}/${r.name}`} empty={t("No hay Ingress.")} cols={[{ key: "n", label: t("Nombre"), render: (r) => r.name }, { key: "ip", label: t("Dirección IP"), render: (r) => <code>{r.obj.ip || "—"}</code> }, { key: "c", label: t("Clase"), render: (r) => r.obj.class || "—" }, { key: "ns", label: t("Espacio de nombres"), render: (r) => r.ns }, { key: "cl", label: t("Clúster"), render: (r) => r.cluster.name }]} />
      </Page>
    </>
  );
}

export function K8sConfig({ ctx }: { ctx: Ctx }) {
  const { t } = useI18n();
  const rows = [...k8s(ctx, "configMaps").map((r) => ({ ...r, kind: "ConfigMap" })), ...k8s(ctx, "secrets").map((r) => ({ ...r, kind: "Secret" }))].filter((r) => r.ns !== "kube-system");
  return (
    <Page title={t("Secretos y ConfigMaps")}>
      <Table
        caption={t("Secretos y ConfigMaps")}
        rows={rows}
        rowKey={(r) => `${r.kind}/${r.cluster.name}/${r.ns}/${r.name}`}
        empty={t("No hay ConfigMaps ni secretos.")}
        cols={[
          { key: "n", label: t("Nombre"), render: (r) => r.name },
          { key: "k", label: t("Tipo"), render: (r) => r.kind },
          { key: "keys", label: t("Claves"), render: (r) => Object.keys(r.obj ?? {}).join(", ") || "—" },
          { key: "ns", label: t("Espacio de nombres"), render: (r) => r.ns },
          { key: "cl", label: t("Clúster"), render: (r) => r.cluster.name },
        ]}
      />
    </Page>
  );
}

// ---------------------------------------------------------------- BigQuery Studio

export function BigQueryStudio({ ctx }: { ctx: Ctx }) {
  const { t } = useI18n();
  const h = useCtxHost(ctx);
  const [sql, setSql] = useState("");
  const [res, setRes] = useState<{ cmd: string; output: string; exit: number } | null>(null);
  const [busy, setBusy] = useState(false);
  const [sel, setSel] = useState<{ ds: string; tb?: string } | null>(null);
  const datasets = vals(ctx.data.datasets);
  const cur = useMemo(() => {
    if (!sel) return null;
    const d = ctx.data.datasets?.[sel.ds];
    return d ? { d, tb: sel.tb ? d.tables?.[sel.tb] : undefined } : null;
  }, [sel, ctx.data]);
  if (h.page) return h.page;
  const exec = async (dry: boolean) => {
    if (!sql.trim()) return;
    setBusy(true);
    const cmd = `bq query --use_legacy_sql=false${dry ? " --dry_run" : ""} ${q(sql.trim().replace(/\s*\n\s*/g, " "))}`;
    const r = await ctx.runOut(cmd, dry ? t("Validar consulta") : t("Ejecutar consulta"));
    setRes({ cmd, ...r });
    setBusy(false);
  };
  return (
    <section className="cc-page bq" aria-label={t("Estudio de BigQuery")}>
      <div className="cc-page-head">
        <h1>{t("Estudio de BigQuery")}</h1>
        <div className="cc-toolbar">
          <Tool icon="add" label={t("Crear conjunto de datos")} onClick={() => h.form({ kind: "drawer", title: t("Crear un conjunto de datos"), submit: t("Crear conjunto de datos"), note: t("Crear conjunto de datos"), initial: { id: "", loc: "EU" }, fields: [{ id: "id", label: t("ID del conjunto de datos"), required: true }, { id: "loc", label: t("Ubicación"), type: "select", options: ["EU", "US", ...REGIONS] }], build: (v) => `bq mk --dataset --location=${v.loc} ${ctx.project}:${v.id.trim()}` })} />
          <Tool icon="refresh" label={t("Actualizar")} onClick={ctx.refresh} />
        </div>
      </div>
      <div className="bq-body">
        <nav className="bq-explorer" aria-label={t("Explorador")}>
          <div className="bq-title">{t("Explorador")}</div>
          <div className="bq-node root">{ctx.project}</div>
          {datasets.map((d) => (
            <div key={d.datasetId}>
              <button type="button" className={`bq-node ds ${sel?.ds === d.datasetId && !sel?.tb ? "on" : ""}`} onClick={() => setSel({ ds: d.datasetId })}>▦ {d.datasetId}</button>
              {Object.keys(d.tables ?? {}).map((tb) => (
                <button key={tb} type="button" className={`bq-node tb ${sel?.ds === d.datasetId && sel?.tb === tb ? "on" : ""}`} onClick={() => { setSel({ ds: d.datasetId, tb }); if (!sql.trim()) setSql(`SELECT *\nFROM \`${ctx.project}.${d.datasetId}.${tb}\`\nLIMIT 100`); }}>
                  ▤ {tb}
                </button>
              ))}
            </div>
          ))}
          {!datasets.length && <p className="cc-help">{t("No hay conjuntos de datos.")}</p>}
        </nav>
        <div className="bq-main">
          {cur?.tb ? (
            <Detail
              title={`${sel!.ds}.${sel!.tb}`}
              onBack={() => setSel(null)}
              actions={
                <>
                  <Tool icon="search" label={t("Consulta")} onClick={() => { setSql(`SELECT *\nFROM \`${ctx.project}.${sel!.ds}.${sel!.tb}\`\nLIMIT 100`); setSel(null); }} />
                  <Tool icon="delete" label={t("Eliminar")} danger onClick={() => h.confirm({ title: t("¿Eliminar la tabla {t}?", { t: sel!.tb ?? "" }), text: t("Se perderán sus datos."), cmds: [`bq rm -f -t ${ctx.project}:${sel!.ds}.${sel!.tb}`], button: t("Eliminar"), note: t("Eliminar tabla") })} />
                </>
              }
              tabs={[
                { id: "schema", label: t("Esquema"), render: () => <Table caption={t("Esquema")} filter={false} rows={cur.tb.schema ?? []} rowKey={(f: any) => f.name} empty={t("Sin esquema.")} cols={[{ key: "n", label: t("Nombre del campo"), render: (f: any) => f.name }, { key: "t", label: t("Tipo"), render: (f: any) => f.type }]} /> },
                { id: "details", label: t("Detalles"), render: () => <Props rows={[[t("Tipo"), cur.tb.type], [t("Número de filas"), cur.tb.numRows ?? 0], [t("Partición"), cur.tb.partitionField ? `${cur.tb.partitionField} (${cur.tb.partitionType ?? "DAY"})` : t("Sin particiones")], [t("Requerir filtro de partición"), cur.tb.requirePartitionFilter ? t("Sí") : t("No")], [t("Agrupamiento"), (cur.tb.clustering ?? []).join(", ")], [t("Consulta de la vista"), cur.tb.query ? <pre key="q" className="cc-pre">{cur.tb.query}</pre> : ""]]} /> },
                { id: "preview", label: t("Vista previa"), render: () => <button type="button" className="cc-btn" onClick={() => h.show(t("Vista previa de {t}", { t: sel!.tb ?? "" }), `bq head -n 10 ${ctx.project}:${sel!.ds}.${sel!.tb}`, t("Vista previa de la tabla"))}>{t("Ver las primeras filas")}</button> },
              ]}
            />
          ) : cur ? (
            <Detail
              title={cur.d.datasetId}
              onBack={() => setSel(null)}
              actions={
                <>
                  <Tool icon="add" label={t("Crear tabla")} onClick={() => h.form({ kind: "drawer", title: t("Crear una tabla en {d}", { d: cur.d.datasetId }), submit: t("Crear tabla"), note: t("Crear tabla"), initial: { name: "", schema: "id:INTEGER,name:STRING,created:TIMESTAMP", part: "" }, fields: [{ id: "name", label: t("Tabla"), required: true }, { id: "schema", label: t("Esquema (campo:TIPO, separados por comas)"), required: true }, { id: "part", label: t("Particionar por el campo (opcional)") }], build: (v) => `bq mk --table${v.part.trim() ? ` --time_partitioning_field=${v.part.trim()}` : ""} ${ctx.project}:${cur.d.datasetId}.${v.name.trim()} ${splitList(v.schema).join(",")}` })} />
                  <Tool icon="delete" label={t("Eliminar")} danger onClick={() => h.confirm({ title: t("¿Eliminar el conjunto de datos {d}?", { d: cur.d.datasetId }), text: t("Se borrarán con todas sus tablas."), cmds: [`bq rm -r -f -d ${ctx.project}:${cur.d.datasetId}`], button: t("Eliminar"), note: t("Eliminar conjunto de datos") })} />
                </>
              }
              tabs={[{ id: "d", label: t("Detalles"), render: () => <Props rows={[[t("ID del conjunto de datos"), `${ctx.project}:${cur.d.datasetId}`], [t("Ubicación"), cur.d.location], [t("Tablas"), Object.keys(cur.d.tables ?? {}).length], [t("Caducidad predeterminada de las tablas"), cur.d.defaultTableExpirationDays ? t("{d} días", { d: cur.d.defaultTableExpirationDays }) : t("Nunca")], [t("Acceso"), (cur.d.access?.bindings ?? []).map((b: any) => `${b.role}: ${(b.members ?? []).join(", ")}`).join("; ")]]} /> }]}
            />
          ) : (
            <>
              <div className="bq-editor">
                <div className="bq-editor-bar">
                  <span className="bq-tab">{t("Consulta sin título")}</span>
                  <span className="se-spacer" />
                  <button type="button" className="cc-btn primary" disabled={busy || !sql.trim()} onClick={() => exec(false)}>▶ {t("Ejecutar")}</button>
                  <button type="button" className="cc-btn" disabled={busy || !sql.trim()} onClick={() => exec(true)}>{t("Validar")}</button>
                </div>
                <textarea className="cc-code-input bq-sql" value={sql} onChange={(e) => setSql(e.target.value)} onKeyDown={(e) => { if (e.key === "Enter" && (e.ctrlKey || e.metaKey)) exec(false); }} aria-label={t("Editor de consultas SQL")} spellCheck={false} placeholder={"SELECT name, COUNT(*) AS n\nFROM `dataset.tabla`\nGROUP BY name"} />
              </div>
              <Section title={t("Resultados de la consulta")}>
                {res ? (
                  <>
                    <p className={`cc-help ${res.exit ? "error" : ""}`}>{res.exit ? t("La consulta ha fallado.") : t("Consulta completada.")} {bytesLine(res.output)}</p>
                    {!res.exit && parseGrid(res.output) ? <ResultGrid grid={parseGrid(res.output)!} /> : <pre className="cc-output">{res.output}</pre>}
                  </>
                ) : (
                  <p className="cc-empty">{t("Escribe una consulta y pulsa Ejecutar (Ctrl+Intro). Se ejecuta con bq query en Cloud Shell. Usa Validar para ver cuántos datos procesará antes de pagar por ella.")}</p>
                )}
              </Section>
            </>
          )}
        </div>
      </div>
      {h.node}
    </section>
  );
}

/** parseGrid reads the boxed table printed by bq query. */
export function parseGrid(out: string): { cols: string[]; rows: string[][] } | null {
  const lines = out.split("\n").filter((l) => l.startsWith("|"));
  if (!lines.length) return null;
  const cells = lines.map((l) => l.slice(1, l.lastIndexOf("|")).split("|").map((c) => c.trim()));
  return { cols: cells[0], rows: cells.slice(1) };
}

function bytesLine(out: string) {
  const m = out.match(/Bytes processed: ([^(\n]+)/);
  return m ? `· ${m[1].trim()}` : "";
}

function ResultGrid({ grid }: { grid: { cols: string[]; rows: string[][] } }) {
  const { t } = useI18n();
  return (
    <div className="cc-table-wrap">
      <div className="cc-table-scroll">
        <table className="cc-table">
          <caption className="sr-only">{t("Resultados de la consulta")}</caption>
          <thead>
            <tr><th scope="col">{t("Fila")}</th>{grid.cols.map((c, i) => <th key={i} scope="col">{c}</th>)}</tr>
          </thead>
          <tbody>
            {grid.rows.map((r, i) => (
              <tr key={i}><td className="cc-muted">{i + 1}</td>{r.map((v, j) => <td key={j}>{v === "NULL" ? <span className="cc-muted">null</span> : v}</td>)}</tr>
            ))}
          </tbody>
        </table>
        {!grid.rows.length && <p className="cc-empty">{t("La consulta no ha devuelto filas.")}</p>}
      </div>
    </div>
  );
}
