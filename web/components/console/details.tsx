"use client";

// Resource details pages of the console, with their tabs, edit forms and
// actions. Like everything in the console, every change runs the equivalent
// command in Cloud Shell.

import { useEffect, useRef, useState } from "react";
import { api } from "@/lib/api";
import { useI18n } from "@/lib/i18n";
import type { Ctx } from "./pages";
import { Detail, Pill, Props, Section, Status, Table, Tool, locFlag, q, useHost, type FormSpec } from "./ui";
import { Icon as MIcon } from "./icons";

const vals = (m: any): any[] => (m ? Object.values(m) : []);
export const kvText = (m?: Record<string, string> | null) => Object.entries(m ?? {}).map(([k, v]) => `${k}=${v}`).join(", ");
export const parseKV = (s: string): Record<string, string> =>
  Object.fromEntries(
    String(s ?? "")
      .split(/[,\n]/)
      .map((x) => x.trim())
      .filter(Boolean)
      .map((x) => {
        const i = x.indexOf("=");
        return i < 0 ? [x, ""] : [x.slice(0, i).trim(), x.slice(i + 1).trim()];
      }),
  );
export const splitList = (s: string) => String(s ?? "").split(/[,\s]+/).map((x) => x.trim()).filter(Boolean);
const and = (cmds: string[]) => cmds.filter(Boolean).join(" && ");

/** Multi-line startup scripts go through a file in Cloud Shell, as at work. */
export const startupFile = (name: string) => `${name}-startup.sh`;
export function startupFlag(base: string, script: string, name: string) {
  if (!String(script ?? "").includes("\n")) return `${base} --metadata=startup-script=${q(script ?? "")}`;
  return `${base} --metadata-from-file=startup-script=${startupFile(name)}`;
}
export async function saveStartup(ctx: Ctx, script: string, name: string) {
  if (String(script ?? "").includes("\n")) await api(`/api/sessions/${ctx.sessionId}/files`, { method: "PUT", body: { path: startupFile(name), content: script } });
}

function Chips({ items }: { items?: string[] | null }) {
  if (!items?.length) return <span className="cc-muted">—</span>;
  return <span className="cc-chips">{items.map((i) => <span key={i} className="cc-chip plain">{i}</span>)}</span>;
}

function Labels({ m }: { m?: Record<string, string> | null }) {
  const e = Object.entries(m ?? {});
  if (!e.length) return <span className="cc-muted">—</span>;
  return <span className="cc-chips">{e.map(([k, v]) => <span key={k} className="cc-chip plain">{k}: {v}</span>)}</span>;
}

export function useCtxHost(ctx: Ctx) {
  return useHost(ctx.run, ctx.runAll, ctx.paste, ctx.runOut);
}

/** Metrics shows the charts of the session metrics whose name contains `match`. */
function MetricCharts({ ctx, match }: { ctx: Ctx; match: string }) {
  const { t } = useI18n();
  const [data, setData] = useState<Record<string, { t: string; v: number }[]>>({});
  useEffect(() => {
    api<Record<string, { t: string; v: number }[]>>(`/api/sessions/${ctx.sessionId}/views/metrics`).then((d) => setData(d ?? {})).catch(() => {});
  }, [ctx.sessionId, ctx.tick]);
  const keys = Object.keys(data).filter((k) => k.includes(match));
  if (!keys.length) return <p className="cc-empty">{t("Todavía no hay métricas de este recurso. Aparecen a medida que avanza el tiempo simulado.")}</p>;
  return (
    <div className="cc-grid">
      {keys.map((k) => {
        const pts = data[k].slice(-60);
        const vs = pts.map((p) => p.v);
        const max = Math.max(...vs, 1);
        const d = pts.map((p, i) => `${i ? "L" : "M"}${((i / Math.max(1, pts.length - 1)) * 300).toFixed(1)},${(78 - (p.v / max) * 70).toFixed(1)}`).join(" ");
        return (
          <div key={k} className="card">
            <div className="cc-mono-sm" style={{ wordBreak: "break-all" }}>{k}</div>
            <div className="kpi">{pts.length ? Math.round(pts[pts.length - 1].v * 100) / 100 : "—"}</div>
            <svg viewBox="0 0 300 80" className="cc-chart" role="img" aria-label={k}>
              <path d={d} className="line" />
            </svg>
          </div>
        );
      })}
    </div>
  );
}

// ---------------------------------------------------------------- VM

export function VMDetail({ ctx, vm, close }: { ctx: Ctx; vm: any; close: () => void }) {
  const { t } = useI18n();
  const h = useCtxHost(ctx);
  if (h.page) return h.page;
  const z = `--zone=${vm.zone}`;
  const running = vm.status === "RUNNING";
  const disks = (vm.disks ?? []).map((d: string) => ({ name: d, boot: d === vm.bootDisk, ...(ctx.data.disks?.[d] ?? {}) }));
  const free = vals(ctx.data.disks).filter((d) => d.zone === vm.zone && !(d.users ?? []).length);
  const edit: FormSpec = {
    title: t("Editar {name}", { name: vm.name }),
    submit: t("Guardar"),
    note: t("Editar instancia de VM"),
    initial: { machine: vm.machineType, tags: (vm.tags ?? []).join(", "), labels: kvText(vm.labels), protect: !!vm.deletionProtection, startup: vm.metadata?.["startup-script"] ?? "" },
    fields: [
      { id: "machine", label: t("Tipo de máquina"), type: "select", options: [...new Set([vm.machineType, "e2-micro", "e2-small", "e2-medium", "e2-standard-2", "e2-standard-4", "n2-standard-2", "n2-standard-4"])], help: running ? t("Para cambiar el tipo de máquina, la VM debe estar detenida.") : undefined },
      { id: "tags", label: t("Etiquetas de red"), help: t("Separadas por comas.") },
      { id: "labels", label: t("Etiquetas (clave=valor)"), placeholder: "env=prod, team=web" },
      { id: "protect", label: t("Habilitar la protección contra eliminación"), type: "check" },
      { id: "startup", label: t("Secuencia de comandos de inicio (startup-script)"), type: "textarea" },
    ],
    build: (v) => {
      const cmds: string[] = [];
      if (v.machine !== vm.machineType) cmds.push(`gcloud compute instances set-machine-type ${vm.name} ${z} --machine-type=${v.machine}`);
      const nt = splitList(v.tags);
      const ot: string[] = vm.tags ?? [];
      const addT = nt.filter((x) => !ot.includes(x));
      const rmT = ot.filter((x) => !nt.includes(x));
      if (addT.length) cmds.push(`gcloud compute instances add-tags ${vm.name} ${z} --tags=${addT.join(",")}`);
      if (rmT.length) cmds.push(`gcloud compute instances remove-tags ${vm.name} ${z} --tags=${rmT.join(",")}`);
      const nl = parseKV(v.labels);
      const ol: Record<string, string> = vm.labels ?? {};
      const addL = Object.entries(nl).filter(([k, val]) => ol[k] !== val);
      const rmL = Object.keys(ol).filter((k) => !(k in nl));
      if (addL.length) cmds.push(`gcloud compute instances add-labels ${vm.name} ${z} --labels=${q(addL.map(([k, val]) => `${k}=${val}`).join(","))}`);
      if (rmL.length) cmds.push(`gcloud compute instances remove-labels ${vm.name} ${z} --labels=${rmL.join(",")}`);
      if (!!v.protect !== !!vm.deletionProtection) cmds.push(`gcloud compute instances update ${vm.name} ${z} ${v.protect ? "--deletion-protection" : "--no-deletion-protection"}`);
      if ((v.startup ?? "") !== (vm.metadata?.["startup-script"] ?? "")) cmds.push(startupFlag(`gcloud compute instances add-metadata ${vm.name} ${z}`, v.startup, vm.name));
      return and(cmds);
    },
    before: (v) => saveStartup(ctx, v.startup, vm.name),
  };
  return (
    <>
      <Detail
        title={vm.name}
        onBack={close}
        status={<Status state={vm.status} />}
        actions={
          <>
            <Tool icon="edit" label={t("Editar")} onClick={() => h.form(edit)} />
            <Tool icon="refresh" label={t("Restablecer")} disabled={!running} onClick={() => ctx.run(`gcloud compute instances reset ${vm.name} ${z}`, t("Restablecer VM"))} />
            {running ? (
              <Tool icon="stop" label={t("Detener")} onClick={() => ctx.run(`gcloud compute instances stop ${vm.name} ${z}`, t("Detener VM"))} />
            ) : (
              <Tool icon="play" label={t("Iniciar/Reanudar")} onClick={() => ctx.run(`gcloud compute instances start ${vm.name} ${z}`, t("Iniciar VM"))} />
            )}
            <Tool icon="shell" label="SSH" disabled={!running} onClick={() => ctx.paste(`gcloud compute ssh ${vm.name} ${z} --command='hostname'`)} />
            <Tool icon="delete" label={t("Eliminar")} danger onClick={() => h.confirm({ title: t("¿Eliminar la instancia {name}?", { name: vm.name }), text: vm.deletionProtection ? t("Tiene la protección contra eliminación activada: el comando fallará hasta que la quites.") : t("Se borrará la VM y su disco de arranque."), cmds: [`gcloud compute instances delete ${vm.name} ${z} --quiet`], button: t("Eliminar"), note: t("Eliminar VM") })} />
          </>
        }
        tabs={[
          {
            id: "details",
            label: t("Detalles"),
            render: () => (
              <>
                <Section title={t("Información básica")}>
                  <Props rows={[
                    [t("Nombre"), vm.name],
                    [t("Estado"), vm.status],
                    [t("Zona"), vm.zone],
                    [t("Tipo de máquina"), `${vm.machineType}${vm.spot ? " · Spot" : ""}`],
                    [t("Creado por"), vm.createdBy],
                    [t("Protección contra eliminación"), vm.deletionProtection ? t("Habilitada") : t("Inhabilitada")],
                    [t("Etiquetas"), <Labels key="l" m={vm.labels} />],
                  ]} />
                </Section>
                <Section title={t("Interfaces de red")}>
                  <Props rows={[
                    [t("Red"), vm.network],
                    [t("Subred"), vm.subnetwork],
                    [t("IP interna principal"), <code key="i">{vm.networkIP}</code>],
                    [t("IP externa"), vm.natIP ? <code key="e">{vm.natIP}</code> : t("Ninguna")],
                    [t("Etiquetas de red"), <Chips key="t" items={vm.tags} />],
                  ]} />
                </Section>
                <Section title={t("Almacenamiento")} actions={<Tool icon="add" label={t("Conectar disco")} onClick={() => h.form({ kind: "drawer", title: t("Conectar un disco a {name}", { name: vm.name }), submit: t("Conectar"), note: t("Conectar disco"), initial: { disk: free[0]?.name ?? "" }, fields: [{ id: "disk", label: t("Disco"), type: "select", required: true, options: free.map((d) => d.name), help: free.length ? t("Discos libres de la zona {z}.", { z: vm.zone }) : t("No hay discos libres en la zona {z}: crea uno en Discos.", { z: vm.zone }) }], build: (v) => (v.disk ? `gcloud compute instances attach-disk ${vm.name} ${z} --disk=${v.disk}` : "") })} />}>
                  <Table
                    caption={t("Discos de {name}", { name: vm.name })}
                    filter={false}
                    rows={disks}
                    rowKey={(d: any) => d.name}
                    empty={t("No hay discos.")}
                    cols={[
                      { key: "n", label: t("Nombre"), render: (d: any) => d.name },
                      { key: "b", label: t("Tipo"), render: (d: any) => (d.boot ? t("Disco de arranque") : t("Disco adicional")) },
                      { key: "s", label: t("Tamaño"), render: (d: any) => (d.sizeGb ? `${d.sizeGb} GB` : "—") },
                      { key: "t", label: t("Tipo de disco"), render: (d: any) => d.type ?? "—" },
                      { key: "i", label: t("Imagen"), render: (d: any) => d.sourceImage ?? "—" },
                      { key: "a", label: "", render: (d: any) => (d.boot ? null : <button type="button" className="cc-link" onClick={() => h.confirm({ title: t("¿Desconectar {d}?", { d: d.name }), text: t("El disco seguirá existiendo y podrás volver a conectarlo."), cmds: [`gcloud compute instances detach-disk ${vm.name} ${z} --disk=${d.name}`], button: t("Desconectar"), note: t("Desconectar disco") })}>{t("Desconectar")}</button>) },
                    ]}
                  />
                </Section>
                <Section title={t("Seguridad y acceso")}>
                  <Props rows={[
                    [t("Cuenta de servicio"), vm.serviceAccount ? <code key="s">{vm.serviceAccount}</code> : t("Ninguna")],
                    [t("Permisos de acceso a la API de Cloud"), <Chips key="sc" items={vm.scopes} />],
                    [t("VM blindada"), vm.shieldedVm ? t("Activado") : t("Desactivado")],
                    [t("OS Login"), vm.osLogin ? t("Activado") : t("Desactivado")],
                  ]} />
                </Section>
                <Section title={t("Gestión")}>
                  <Props rows={[
                    [t("Metadatos"), <Chips key="m" items={Object.keys(vm.metadata ?? {})} />],
                    [t("Secuencia de comandos de inicio"), vm.metadata?.["startup-script"] ? <pre key="p" className="cc-pre">{vm.metadata["startup-script"]}</pre> : ""],
                    [t("Modelo de aprovisionamiento"), vm.spot ? "Spot" : t("Estándar")],
                  ]} />
                </Section>
              </>
            ),
          },
          { id: "obs", label: t("Observabilidad"), render: () => <MetricCharts ctx={ctx} match={vm.name} /> },
          {
            id: "logs",
            label: t("Registros"),
            render: () => (
              <div className="cc-actions-row">
                <button type="button" className="cc-btn" onClick={() => h.show(t("Consola en serie (puerto 1)"), `gcloud compute instances get-serial-port-output ${vm.name} ${z}`, t("Consola en serie"))}>{t("Ver la consola en serie")}</button>
                <button type="button" className="cc-btn" onClick={() => ctx.go("logs")}>{t("Abrir el Explorador de registros")}</button>
              </div>
            ),
          },
        ]}
      />
      {h.node}
    </>
  );
}

// ---------------------------------------------------------------- Disk

export function DiskDetail({ ctx, d, close }: { ctx: Ctx; d: any; close: () => void }) {
  const { t } = useI18n();
  const h = useCtxHost(ctx);
  const z = `--zone=${d.zone}`;
  return (
    <>
      <Detail
        title={d.name}
        onBack={close}
        actions={
          <>
            <Tool icon="copy" label={t("Crear instantánea")} onClick={() => h.form({ kind: "drawer", title: t("Crear una instantánea de {d}", { d: d.name }), submit: t("Crear"), note: t("Crear instantánea"), initial: { name: `${d.name}-snap` }, fields: [{ id: "name", label: t("Nombre"), required: true }], build: (v) => `gcloud compute disks snapshot ${d.name} ${z} --snapshot-names=${q(v.name.trim())}` })} />
            <Tool icon="edit" label={t("Cambiar tamaño")} onClick={() => h.form({ kind: "drawer", title: t("Cambiar el tamaño de {d}", { d: d.name }), submit: t("Guardar"), note: t("Cambiar tamaño del disco"), initial: { size: String(d.sizeGb) }, fields: [{ id: "size", label: t("Tamaño (GB)"), type: "number", required: true, help: t("Solo se puede aumentar; después hay que ampliar el sistema de archivos dentro de la VM.") }], build: (v) => (Number(v.size) > d.sizeGb ? `gcloud compute disks resize ${d.name} ${z} --size=${v.size}GB --quiet` : "") })} />
            <Tool icon="delete" label={t("Eliminar")} danger onClick={() => h.confirm({ title: t("¿Eliminar el disco {d}?", { d: d.name }), text: t("Los datos del disco se perderán."), cmds: [`gcloud compute disks delete ${d.name} ${z} --quiet`], button: t("Eliminar"), note: t("Eliminar disco") })} />
          </>
        }
        tabs={[{ id: "d", label: t("Detalles"), render: () => <Props rows={[[t("Tipo"), d.type], [t("Tamaño"), `${d.sizeGb} GB`], [t("Zona"), d.zone], [t("Imagen de origen"), d.sourceImage], [t("En uso por"), <Chips key="u" items={d.users} />]]} /> }]}
      />
      {h.node}
    </>
  );
}

// ---------------------------------------------------------------- Storage

export function BucketDetail({ ctx, b, close }: { ctx: Ctx; b: any; close: () => void }) {
  const { t } = useI18n();
  const h = useCtxHost(ctx);
  const file = useRef<HTMLInputElement>(null);
  const [sel, setSel] = useState<string[]>([]);
  if (h.page) return h.page;
  const objs = Object.entries(b.objects ?? {}).map(([name, o]: [string, any]) => ({ name, ...o }));
  const gs = `gs://${b.name}`;
  const upload = async (files: FileList | null) => {
    for (const f of Array.from(files ?? [])) {
      const text = await f.text();
      const path = `uploads/${f.name}`;
      await api(`/api/sessions/${ctx.sessionId}/files`, { method: "PUT", body: { path, content: text } });
      await ctx.run(`gcloud storage cp ${q(path)} ${gs}/${q(f.name)}`, t("Subir archivo"));
    }
    if (file.current) file.current.value = "";
  };
  const bindings: { member: string; role: string }[] = (b.iamPolicy?.bindings ?? []).flatMap((x: any) => (x.members ?? []).map((m: string) => ({ member: m, role: x.role })));
  const edit: FormSpec = {
    title: t("Editar la configuración de {b}", { b: b.name }),
    submit: t("Guardar"),
    note: t("Editar bucket"),
    initial: { cls: b.storageClass, versioning: !!b.versioning, ubla: !!b.uniformBucketLevelAccess, pap: b.publicAccessPrevention === "enforced", labels: kvText(b.labels) },
    fields: [
      { id: "cls", label: t("Clase de almacenamiento predeterminada"), type: "select", options: ["STANDARD", "NEARLINE", "COLDLINE", "ARCHIVE"] },
      { id: "versioning", label: t("Versiones de objetos"), type: "check" },
      { id: "ubla", label: t("Acceso uniforme a nivel de bucket"), type: "check" },
      { id: "pap", label: t("Aplicar la prevención de acceso público"), type: "check" },
      { id: "labels", label: t("Etiquetas (clave=valor)") },
    ],
    build: (v) => {
      const f: string[] = [];
      if (v.cls !== b.storageClass) f.push(`--default-storage-class=${v.cls}`);
      if (!!v.versioning !== !!b.versioning) f.push(v.versioning ? "--versioning" : "--no-versioning");
      if (!!v.ubla !== !!b.uniformBucketLevelAccess) f.push(v.ubla ? "--uniform-bucket-level-access" : "--no-uniform-bucket-level-access");
      if (!!v.pap !== (b.publicAccessPrevention === "enforced")) f.push(v.pap ? "--public-access-prevention" : "--no-public-access-prevention");
      if (v.labels !== kvText(b.labels) && v.labels.trim()) f.push(`--update-labels=${q(Object.entries(parseKV(v.labels)).map(([k, x]) => `${k}=${x}`).join(","))}`);
      return f.length ? `gcloud storage buckets update ${gs} ${f.join(" ")}` : "";
    },
  };
  return (
    <>
      <Detail
        title={b.name}
        onBack={close}
        actions={<Tool icon="delete" label={t("Eliminar")} danger onClick={() => h.confirm({ title: t("¿Eliminar el bucket {b}?", { b: b.name }), text: t("Se borrarán los buckets y todos sus objetos."), cmds: [objs.length ? `gcloud storage rm --recursive ${gs}` : `gcloud storage buckets delete ${gs} --quiet`], button: t("Eliminar"), note: t("Eliminar bucket") })} />}
        tabs={[
          {
            id: "objects",
            label: t("Objetos"),
            render: () => (
              <>
                <div className="cc-crumbs"><span>{t("Buckets")}</span> › <strong>{b.name}</strong></div>
                <div className="cc-toolbar" style={{ margin: "8px 0" }}>
                  <input ref={file} type="file" multiple hidden onChange={(e) => upload(e.target.files)} aria-label={t("Subir archivos")} />
                  <Tool icon="upload" label={t("Subir archivos")} onClick={() => file.current?.click()} />
                  <Tool icon="download" label={t("Descargar")} disabled={sel.length !== 1} onClick={() => h.show(sel[0], `gcloud storage cat ${gs}/${q(sel[0])}`, t("Descargar objeto"))} />
                  <Tool icon="delete" label={t("Eliminar")} danger disabled={!sel.length} onClick={() => h.confirm({ title: t("¿Eliminar {n} objeto(s)?", { n: sel.length }), text: b.versioning ? t("Con versiones activadas se conserva una versión no actual.") : t("Los objetos se borrarán definitivamente."), cmds: sel.map((o) => `gcloud storage rm ${gs}/${q(o)}`), button: t("Eliminar"), note: t("Eliminar objeto") })} />
                </div>
                <Table
                  caption={t("Objetos de {b}", { b: b.name })}
                  rows={objs}
                  rowKey={(o: any) => o.name}
                  selected={sel}
                  onSelect={setSel}
                  empty={t("No hay objetos en este bucket. Sube archivos o usa gcloud storage cp.")}
                  cols={[
                    { key: "name", label: t("Nombre"), render: (o: any) => <span className="cc-obj"><MIcon name="file" size={18} /> {o.name}</span> },
                    { key: "size", label: t("Tamaño"), render: (o: any) => (o.size != null ? `${o.size} B` : "—") },
                    { key: "type", label: t("Tipo"), render: (o: any) => o.contentType ?? "—" },
                    { key: "class", label: t("Clase de almacenamiento"), render: (o: any) => o.storageClass ?? b.storageClass },
                    { key: "upd", label: t("Actualizado"), render: (o: any) => String(o.updated ?? o.created ?? "—").replace("T", " ").replace(/Z$/, "") },
                  ]}
                />
              </>
            ),
          },
          {
            id: "config",
            label: t("Configuración"),
            render: () => (
              <Section title={t("Visión general")} actions={<Tool icon="edit" label={t("Editar")} onClick={() => h.form(edit)} />}>
                <Props rows={[
                  [t("Ubicación"), `${b.location} (${b.locationType})`],
                  [t("Clase de almacenamiento predeterminada"), b.storageClass],
                  [t("URL pública"), <code key="u">https://storage.googleapis.com/{b.name}</code>],
                  [t("URI de gsutil"), <code key="g">{gs}</code>],
                  [t("Etiquetas"), <Labels key="l" m={b.labels} />],
                  [t("Reglas de ciclo de vida"), (b.lifecycle?.rule ?? b.lifecycle ?? []).length ? <pre key="lc" className="cc-pre">{JSON.stringify(b.lifecycle, null, 2)}</pre> : t("Ninguna")],
                ]} />
              </Section>
            ),
          },
          {
            id: "perms",
            label: t("Permisos"),
            render: () => (
              <>
                <Section title={t("Acceso público")}>
                  <Props rows={[
                    [t("Acceso público"), isPublicPolicy(b.iamPolicy) ? <Pill tone="bad">{t("Público en internet")}</Pill> : t("No público")],
                    [t("Prevención de acceso público"), b.publicAccessPrevention === "enforced" ? t("Aplicada") : t("Heredada")],
                    [t("Control de acceso"), b.uniformBucketLevelAccess ? t("Uniforme") : t("Detallado")],
                  ]} />
                </Section>
                <Section title={t("Principales con acceso")} actions={<Tool icon="add" label={t("Conceder acceso")} onClick={() => h.form(grant(t, `gcloud storage buckets add-iam-policy-binding ${gs}`, ["roles/storage.objectViewer", "roles/storage.objectCreator", "roles/storage.objectAdmin", "roles/storage.admin", "roles/storage.legacyBucketReader"], t("Conceder acceso a {b}", { b: b.name })))} />}>
                  <Bindings rows={bindings} onRemove={(r) => h.confirm({ title: t("¿Quitar el rol?"), text: t("{who} dejará de tener {role}.", { who: r.member, role: r.role }), cmds: [`gcloud storage buckets remove-iam-policy-binding ${gs} --member=${q(r.member)} --role=${r.role}`], button: t("Quitar"), note: t("Quitar un rol") })} />
                </Section>
              </>
            ),
          },
          {
            id: "protect",
            label: t("Protección"),
            render: () => (
              <Props rows={[
                [t("Versiones de objetos"), b.versioning ? t("Activado") : t("Desactivado")],
                [t("Eliminación no definitiva"), b.softDeleteRetentionSec ? t("{d} días", { d: Math.round(b.softDeleteRetentionSec / 86400) }) : t("Desactivado")],
                [t("Política de retención"), b.retentionPolicy ? JSON.stringify(b.retentionPolicy) : t("Ninguna")],
                [t("Cifrado"), b.defaultKmsKey || b.kmsKey ? t("Clave gestionada por el cliente") : t("Gestionado por Google")],
              ]} />
            ),
          },
        ]}
      />
      {h.node}
    </>
  );
}

export const isPublicPolicy = (policy: any) => (policy?.bindings ?? []).some((b: any) => (b.members ?? []).some((m: string) => m === "allUsers" || m === "allAuthenticatedUsers"));

function grant(t: (s: string, v?: any) => string, base: string, roles: string[], title: string): FormSpec {
  return {
    kind: "drawer",
    title,
    submit: t("Guardar"),
    note: t("Conceder acceso"),
    initial: { kind: "user", who: "", role: roles[0], custom: "" },
    fields: [
      { id: "kind", label: t("Tipo de principal"), type: "select", options: [["user", t("Usuario")], ["serviceAccount", t("Cuenta de servicio")], ["group", t("Grupo")], ["allUsers", t("Cualquiera en internet")]] },
      { id: "who", label: t("Principal"), placeholder: "ana@example.com", required: true, when: (v) => v.kind !== "allUsers" },
      { id: "role", label: t("Rol"), type: "select", options: [...roles, ["custom", t("Otro rol…")]] },
      { id: "custom", label: t("Rol personalizado o predefinido"), placeholder: "roles/…", when: (v) => v.role === "custom", required: true },
    ],
    build: (v) => `${base} --member=${q(v.kind === "allUsers" ? "allUsers" : `${v.kind}:${String(v.who).trim()}`)} --role=${q(v.role === "custom" ? v.custom.trim() : v.role)}`,
  };
}

function Bindings({ rows, onRemove }: { rows: { member: string; role: string }[]; onRemove: (r: { member: string; role: string }) => void }) {
  const { t } = useI18n();
  return (
    <Table
      caption={t("Principales con acceso")}
      filter={false}
      rows={rows}
      rowKey={(r) => `${r.member}|${r.role}`}
      empty={t("Nadie tiene acceso directo a este recurso.")}
      cols={[
        { key: "m", label: t("Principal"), render: (r) => <code>{r.member}</code> },
        { key: "r", label: t("Rol"), render: (r) => r.role },
        { key: "x", label: "", render: (r) => <button type="button" className="cc-link" onClick={() => onRemove(r)}>{t("Quitar")}</button> },
      ]}
    />
  );
}

// ---------------------------------------------------------------- Cloud Run

export function RunDetail({ ctx, s, close }: { ctx: Ctx; s: any; close: () => void }) {
  const { t } = useI18n();
  const h = useCtxHost(ctx);
  if (h.page) return h.page;
  const r = `--region=${s.region}`;
  const pub = isPublicPolicy(s.iamPolicy);
  const sas = Object.keys(ctx.data.serviceAccounts ?? {});
  const revs = [...(s.revisions ?? [])].reverse();
  const edit: FormSpec = {
    title: t("Editar e implementar una revisión nueva de {s}", { s: s.name }),
    submit: t("Implementar"),
    note: t("Implementar revisión de Cloud Run"),
    initial: { image: s.image, port: String(s.port ?? 8080), memory: s.memory ?? "512Mi", cpu: s.cpu ?? "1", concurrency: String(s.concurrency ?? 80), min: String(s.minInstances ?? 0), max: String(s.maxInstances ?? 100), ingress: s.ingress ?? "all", env: Object.entries(s.env ?? {}).map(([k, v]) => `${k}=${v}`).join("\n"), secrets: Object.entries(s.secrets ?? {}).map(([k, v]) => `${k}=${v}`).join("\n"), sa: s.serviceAccount ?? "" },
    fields: [
      { id: "image", label: t("URL de la imagen del contenedor"), required: true, section: t("Contenedor") },
      { id: "port", label: t("Puerto del contenedor"), type: "number" },
      { id: "memory", label: t("Memoria"), type: "select", options: ["256Mi", "512Mi", "1Gi", "2Gi", "4Gi"] },
      { id: "cpu", label: t("CPU"), type: "select", options: ["1", "2", "4"] },
      { id: "concurrency", label: t("Número máximo de solicitudes simultáneas por instancia"), type: "number", section: t("Escalado") },
      { id: "min", label: t("Número mínimo de instancias"), type: "number" },
      { id: "max", label: t("Número máximo de instancias"), type: "number" },
      { id: "env", label: t("Variables de entorno (una por línea, NOMBRE=valor)"), type: "textarea", section: t("Variables y secretos") },
      { id: "secrets", label: t("Secretos como variables (NOMBRE=secreto:versión)"), type: "textarea" },
      { id: "ingress", label: t("Entrada"), type: "select", options: [["all", t("Todo")], ["internal", t("Interno")], ["internal-and-cloud-load-balancing", t("Interno y Cloud Load Balancing")]], section: t("Redes y seguridad") },
      { id: "sa", label: t("Cuenta de servicio"), type: "select", options: [["", t("Predeterminada de Compute Engine")], ...sas] },
    ],
    build: (v) => {
      const env = Object.entries(parseKV(v.env));
      const sec = Object.entries(parseKV(v.secrets));
      return [
        `gcloud run deploy ${s.name} --image=${q(v.image.trim())} ${r}`,
        `--port=${v.port}`,
        `--memory=${v.memory} --cpu=${v.cpu}`,
        `--concurrency=${v.concurrency} --min-instances=${v.min} --max-instances=${v.max}`,
        `--ingress=${v.ingress}`,
        env.length ? `--set-env-vars=${q(env.map(([k, x]) => `${k}=${x}`).join(","))}` : Object.keys(s.env ?? {}).length ? "--clear-env-vars" : "",
        sec.length ? `--set-secrets=${q(sec.map(([k, x]) => `${k}=${x}`).join(","))}` : "",
        v.sa ? `--service-account=${v.sa}` : "",
      ].filter(Boolean).join(" ");
    },
  };
  return (
    <>
      <Detail
        title={s.name}
        onBack={close}
        status={<Status state="READY" />}
        actions={
          <>
            <Tool icon="edit" label={t("Editar e implementar una revisión nueva")} onClick={() => h.form(edit)} />
            <Tool icon="delete" label={t("Eliminar")} danger onClick={() => h.confirm({ title: t("¿Eliminar el servicio {s}?", { s: s.name }), text: t("Sus URL dejarán de responder."), cmds: [`gcloud run services delete ${s.name} ${r} --quiet`], button: t("Eliminar"), note: t("Eliminar servicio de Cloud Run") })} />
          </>
        }
        tabs={[
          { id: "metrics", label: t("Métricas"), render: () => <MetricCharts ctx={ctx} match={s.name} /> },
          {
            id: "revisions",
            label: t("Revisiones"),
            render: () => (
              <>
                <div className="cc-toolbar" style={{ marginBottom: 8 }}>
                  <Tool icon="refresh" label={t("Enviar todo el tráfico a la última revisión")} onClick={() => ctx.run(`gcloud run services update-traffic ${s.name} ${r} --to-latest`, t("Administrar tráfico"))} />
                </div>
                <Table
                  caption={t("Revisiones de {s}", { s: s.name })}
                  filter={false}
                  rows={revs}
                  rowKey={(x: any) => x.name}
                  empty={t("No hay revisiones.")}
                  cols={[
                    { key: "n", label: t("Nombre"), render: (x: any) => <code>{x.name}</code> },
                    { key: "tr", label: t("Tráfico"), render: (x: any) => `${x.traffic ?? 0}%` },
                    { key: "i", label: t("Imagen"), render: (x: any) => <span className="cc-mono-sm">{x.image}</span> },
                    { key: "e", label: t("Variables"), render: (x: any) => Object.keys(x.env ?? {}).join(", ") || "—" },
                    { key: "a", label: "", render: (x: any) => (x.traffic === 100 ? null : <button type="button" className="cc-link" onClick={() => ctx.run(`gcloud run services update-traffic ${s.name} ${r} --to-revisions=${x.name}=100`, t("Administrar tráfico"))}>{t("Enviar el 100 % del tráfico")}</button>) },
                  ]}
                />
              </>
            ),
          },
          {
            id: "details",
            label: t("Detalles"),
            render: () => (
              <>
                <Props rows={[
                  ["URL", <a key="u" href="#" onClick={(e) => { e.preventDefault(); ctx.paste(`curl -s ${pub ? "" : '-H "Authorization: Bearer $(gcloud auth print-identity-token)" '}${s.url}`); }}>{s.url}</a>],
                  [t("Región"), s.region],
                  [t("Imagen"), <code key="i">{s.image}</code>],
                  [t("Puerto"), s.port],
                  [t("Memoria / CPU"), `${s.memory} / ${s.cpu}`],
                  [t("Simultaneidad"), s.concurrency],
                  [t("Instancias mín./máx."), `${s.minInstances} / ${s.maxInstances}`],
                  [t("Cuenta de servicio"), s.serviceAccount ? <code key="s">{s.serviceAccount}</code> : t("Predeterminada de Compute Engine")],
                  [t("Conexiones de Cloud SQL"), <Chips key="c" items={s.cloudSqlInstances} />],
                  [t("Conector de VPC / salida"), s.vpcConnector || s.network ? `${s.vpcConnector || s.network} (${s.vpcEgress ?? ""})` : ""],
                ]} />
                <Section title={t("Variables de entorno")}>
                  <Table caption={t("Variables de entorno")} filter={false} rows={Object.entries(s.env ?? {}).map(([k, v]) => ({ k, v }))} rowKey={(x: any) => x.k} empty={t("Sin variables de entorno.")} cols={[{ key: "k", label: t("Nombre"), render: (x: any) => <code>{x.k}</code> }, { key: "v", label: t("Valor"), render: (x: any) => <code>{String(x.v)}</code> }]} />
                </Section>
                <Section title={t("Secretos")}>
                  <Table caption={t("Secretos")} filter={false} rows={Object.entries(s.secrets ?? {}).map(([k, v]) => ({ k, v }))} rowKey={(x: any) => x.k} empty={t("Sin secretos.")} cols={[{ key: "k", label: t("Nombre"), render: (x: any) => <code>{x.k}</code> }, { key: "v", label: t("Secreto"), render: (x: any) => <code>{String(x.v)}</code> }]} />
                </Section>
              </>
            ),
          },
          {
            id: "security",
            label: t("Seguridad"),
            render: () => (
              <>
                <Section title={t("Autenticación")}>
                  <div className="cc-radio">
                    <label className="cc-check">
                      <input type="radio" name="auth" checked={pub} onChange={() => !pub && ctx.run(`gcloud run services add-iam-policy-binding ${s.name} ${r} --member=allUsers --role=roles/run.invoker`, t("Permitir acceso público"))} />
                      <span>{t("Permitir el acceso público")}<span className="cc-help">{t("Cualquiera en internet podrá llamar al servicio.")}</span></span>
                    </label>
                    <label className="cc-check">
                      <input type="radio" name="auth" checked={!pub} onChange={() => pub && ctx.run(`gcloud run services remove-iam-policy-binding ${s.name} ${r} --member=allUsers --role=roles/run.invoker`, t("Requerir autenticación"))} />
                      <span>{t("Requerir autenticación")}<span className="cc-help">{t("Solo las identidades con roles/run.invoker podrán llamarlo.")}</span></span>
                    </label>
                  </div>
                </Section>
                <Section title={t("Principales con acceso")} actions={<Tool icon="add" label={t("Conceder acceso")} onClick={() => h.form(grant(t, `gcloud run services add-iam-policy-binding ${s.name} ${r}`, ["roles/run.invoker", "roles/run.developer", "roles/run.admin"], t("Conceder acceso a {s}", { s: s.name })))} />}>
                  <Bindings rows={(s.iamPolicy?.bindings ?? []).flatMap((x: any) => (x.members ?? []).map((m: string) => ({ member: m, role: x.role })))} onRemove={(b) => h.confirm({ title: t("¿Quitar el rol?"), text: t("{who} dejará de tener {role}.", { who: b.member, role: b.role }), cmds: [`gcloud run services remove-iam-policy-binding ${s.name} ${r} --member=${q(b.member)} --role=${b.role}`], button: t("Quitar"), note: t("Quitar un rol") })} />
                </Section>
              </>
            ),
          },
          { id: "logs", label: t("Registros"), render: () => <button type="button" className="cc-btn" onClick={() => ctx.go("logs")}>{t("Abrir el Explorador de registros")}</button> },
        ]}
      />
      {h.node}
    </>
  );
}

// ---------------------------------------------------------------- Cloud SQL

export function SQLDetail({ ctx, i, close }: { ctx: Ctx; i: any; close: () => void }) {
  const { t } = useI18n();
  const h = useCtxHost(ctx);
  if (h.page) return h.page;
  const nets: string[] = i.authorizedNetworks ?? [];
  const edit: FormSpec = {
    title: t("Editar la instancia {i}", { i: i.name }),
    submit: t("Guardar"),
    note: t("Editar instancia de Cloud SQL"),
    initial: { tier: i.tier, ha: i.availabilityType === "REGIONAL", backup: i.backupStartTime ?? "", pitr: !!i.pointInTimeRecovery, ssl: !!i.requireSsl, protect: !!i.deletionProtection },
    fields: [
      { id: "tier", label: t("Tipo de máquina"), type: "select", options: [...new Set([i.tier, "db-f1-micro", "db-g1-small", "db-custom-2-7680", "db-custom-4-15360", "db-custom-8-30720"])], section: t("Configuración de la máquina") },
      { id: "ha", label: t("Alta disponibilidad (varias zonas)"), type: "check" },
      { id: "backup", label: t("Hora de inicio de las copias de seguridad (HH:MM)"), placeholder: "02:00", section: t("Protección de datos") },
      { id: "pitr", label: t("Recuperación a un momento dado"), type: "check" },
      { id: "protect", label: t("Protección contra eliminación"), type: "check" },
      { id: "ssl", label: t("Permitir solo conexiones SSL"), type: "check", section: t("Seguridad") },
    ],
    build: (v) => {
      const f: string[] = [];
      if (v.tier !== i.tier) f.push(`--tier=${v.tier}`);
      if (!!v.ha !== (i.availabilityType === "REGIONAL")) f.push(`--availability-type=${v.ha ? "REGIONAL" : "ZONAL"}`);
      if ((v.backup ?? "") !== (i.backupStartTime ?? "") && v.backup) f.push(`--backup-start-time=${v.backup}`);
      if (!!v.pitr !== !!i.pointInTimeRecovery) f.push(v.pitr ? "--enable-point-in-time-recovery" : "--no-enable-point-in-time-recovery");
      if (!!v.protect !== !!i.deletionProtection) f.push(v.protect ? "--deletion-protection" : "--no-deletion-protection");
      if (!!v.ssl !== !!i.requireSsl) f.push(v.ssl ? "--require-ssl" : "--no-require-ssl");
      return f.length ? `gcloud sql instances patch ${i.name} ${f.join(" ")} --quiet` : "";
    },
  };
  const conn = `${ctx.project}:${i.region}:${i.name}`;
  return (
    <>
      <Detail
        title={i.name}
        onBack={close}
        status={<Status state={i.state} />}
        actions={
          <>
            <Tool icon="edit" label={t("Editar")} onClick={() => h.form(edit)} />
            <Tool icon="shell" label={t("Conectar con Cloud Shell")} onClick={() => ctx.paste(`gcloud sql connect ${i.name} --user=postgres`)} />
            <Tool icon="refresh" label={t("Reiniciar")} onClick={() => ctx.run(`gcloud sql instances restart ${i.name} --quiet`, t("Reiniciar Cloud SQL"))} />
            <Tool icon="delete" label={t("Eliminar")} danger onClick={() => h.confirm({ title: t("¿Eliminar la instancia {i}?", { i: i.name }), text: i.deletionProtection ? t("Tiene la protección contra eliminación activada: el comando fallará hasta que la quites.") : t("Se perderán las bases de datos y sus copias de seguridad."), cmds: [`gcloud sql instances delete ${i.name} --quiet`], button: t("Eliminar"), note: t("Eliminar instancia de Cloud SQL") })} />
          </>
        }
        tabs={[
          {
            id: "overview",
            label: t("Descripción general"),
            render: () => (
              <>
                <Section title={t("Conectarse a esta instancia")}>
                  <Props rows={[
                    [t("Nombre de conexión"), <code key="c">{conn}</code>],
                    [t("Dirección IP pública"), i.publicIp ? <code key="p">{i.publicIp}</code> : t("Ninguna")],
                    [t("Dirección IP privada"), i.privateIp ? <code key="pr">{i.privateIp}</code> : t("Ninguna")],
                    [t("Solo SSL"), i.requireSsl ? t("Sí") : t("No")],
                  ]} />
                </Section>
                <Section title={t("Configuración")}>
                  <Props rows={[
                    [t("Versión de la base de datos"), i.databaseVersion],
                    [t("Tipo de máquina"), i.tier],
                    [t("Región"), i.region],
                    [t("Alta disponibilidad"), i.availabilityType === "REGIONAL" ? t("Sí (regional)") : t("No (zonal)")],
                    [t("Copias de seguridad automáticas"), i.backupEnabled ? `${t("Activadas")} · ${i.backupStartTime ?? ""}` : t("Desactivadas")],
                    [t("Recuperación a un momento dado"), i.pointInTimeRecovery ? t("Activado") : t("Desactivado")],
                    [t("Protección contra eliminación"), i.deletionProtection ? t("Habilitada") : t("Inhabilitada")],
                    [t("Marcas de la base de datos"), kvText(i.databaseFlags)],
                    [t("Réplicas"), <Chips key="r" items={i.replicaNames} />],
                  ]} />
                </Section>
                <Section title={t("Monitorización")}>
                  <Props rows={[[t("Uso de CPU"), `${Math.round((i.cpuUtilization ?? 0) * 100) / 100}%`], [t("Conexiones activas"), i.activeConnections ?? 0]]} />
                </Section>
              </>
            ),
          },
          {
            id: "connections",
            label: t("Conexiones"),
            render: () => (
              <Section title={t("Redes autorizadas")} actions={<Tool icon="add" label={t("Añadir una red")} onClick={() => h.form({ kind: "drawer", title: t("Nueva red autorizada"), submit: t("Guardar"), note: t("Autorizar red"), initial: { cidr: "" }, fields: [{ id: "cidr", label: t("Red (CIDR)"), required: true, placeholder: "203.0.113.0/24", help: t("Nunca autorices 0.0.0.0/0: expondrías la base de datos a todo internet.") }], build: (v) => (v.cidr.trim() ? `gcloud sql instances patch ${i.name} --authorized-networks=${[...nets, v.cidr.trim()].join(",")} --quiet` : "") })} />}>
                <Table
                  caption={t("Redes autorizadas")}
                  filter={false}
                  rows={nets.map((n) => ({ n }))}
                  rowKey={(x: any) => x.n}
                  empty={t("No hay redes autorizadas. Conéctate con el conector de Cloud SQL o por IP privada.")}
                  cols={[
                    { key: "n", label: t("Red"), render: (x: any) => <code>{x.n}</code> },
                    { key: "x", label: "", render: (x: any) => <button type="button" className="cc-link" onClick={() => { const rest = nets.filter((y) => y !== x.n); h.confirm({ title: t("¿Quitar la red {n}?", { n: x.n }), text: t("Dejará de poder conectarse por la IP pública."), cmds: [rest.length ? `gcloud sql instances patch ${i.name} --authorized-networks=${rest.join(",")} --quiet` : `gcloud sql instances patch ${i.name} --clear-authorized-networks --quiet`], button: t("Quitar"), note: t("Quitar red autorizada") }); }}>{t("Quitar")}</button> },
                  ]}
                />
              </Section>
            ),
          },
          {
            id: "users",
            label: t("Usuarios"),
            render: () => (
              <Section title={t("Cuentas de usuario")} actions={<Tool icon="add" label={t("Añadir cuenta de usuario")} onClick={() => h.form({ kind: "drawer", title: t("Añadir una cuenta de usuario a {i}", { i: i.name }), submit: t("Añadir"), note: t("Crear usuario de Cloud SQL"), initial: { user: "", pw: "" }, fields: [{ id: "user", label: t("Nombre de usuario"), required: true }, { id: "pw", label: t("Contraseña"), type: "password", required: true, help: t("En un proyecto real, guárdala en Secret Manager.") }], build: (v) => `gcloud sql users create ${q(v.user.trim())} --instance=${i.name} --password=${q(v.pw)}` })} />}>
                <Table
                  caption={t("Cuentas de usuario")}
                  filter={false}
                  rows={Object.keys(i.users ?? {}).sort().map((u) => ({ u }))}
                  rowKey={(x: any) => x.u}
                  empty={t("No hay usuarios.")}
                  cols={[
                    { key: "u", label: t("Nombre de usuario"), render: (x: any) => x.u },
                    { key: "ty", label: t("Autenticación"), render: () => t("Integrada") },
                    {
                      key: "a",
                      label: "",
                      render: (x: any) => (
                        <span className="cc-chips">
                          <button type="button" className="cc-link" onClick={() => h.form({ kind: "drawer", title: t("Cambiar la contraseña de {u}", { u: x.u }), submit: t("Guardar"), note: t("Cambiar contraseña"), initial: { pw: "" }, fields: [{ id: "pw", label: t("Contraseña nueva"), type: "password", required: true }], build: (v) => `gcloud sql users set-password ${q(x.u)} --instance=${i.name} --password=${q(v.pw)}` })}>{t("Cambiar contraseña")}</button>
                          <button type="button" className="cc-link danger" onClick={() => h.confirm({ title: t("¿Eliminar el usuario {u}?", { u: x.u }), text: t("Las aplicaciones que lo usen dejarán de conectarse."), cmds: [`gcloud sql users delete ${q(x.u)} --instance=${i.name} --quiet`], button: t("Eliminar"), note: t("Eliminar usuario de Cloud SQL") })}>{t("Eliminar")}</button>
                        </span>
                      ),
                    },
                  ]}
                />
              </Section>
            ),
          },
          {
            id: "dbs",
            label: t("Bases de datos"),
            render: () => (
              <Section title={t("Bases de datos")} actions={<Tool icon="add" label={t("Crear base de datos")} onClick={() => h.form({ kind: "drawer", title: t("Crear una base de datos"), submit: t("Crear"), note: t("Crear base de datos"), initial: { db: "" }, fields: [{ id: "db", label: t("Nombre de la base de datos"), required: true }], build: (v) => `gcloud sql databases create ${q(v.db.trim())} --instance=${i.name}` })} />}>
                <Table
                  caption={t("Bases de datos")}
                  filter={false}
                  rows={(i.databases ?? []).map((d: string) => ({ d }))}
                  rowKey={(x: any) => x.d}
                  empty={t("No hay bases de datos.")}
                  cols={[
                    { key: "d", label: t("Nombre"), render: (x: any) => x.d },
                    { key: "a", label: "", render: (x: any) => (["postgres", "mysql", "sys"].includes(x.d) ? <span className="cc-muted">{t("del sistema")}</span> : <button type="button" className="cc-link danger" onClick={() => h.confirm({ title: t("¿Eliminar la base de datos {d}?", { d: x.d }), text: t("Se perderán sus datos."), cmds: [`gcloud sql databases delete ${q(x.d)} --instance=${i.name} --quiet`], button: t("Eliminar"), note: t("Eliminar base de datos") })}>{t("Eliminar")}</button>) },
                  ]}
                />
              </Section>
            ),
          },
          {
            id: "backups",
            label: t("Copias de seguridad"),
            render: () => (
              <Section title={t("Copias de seguridad")} actions={<Tool icon="add" label={t("Crear copia de seguridad")} onClick={() => ctx.run(`gcloud sql backups create --instance=${i.name}`, t("Crear copia de seguridad"))} />}>
                <Table
                  caption={t("Copias de seguridad")}
                  filter={false}
                  rows={i.backups ?? []}
                  rowKey={(x: any) => x.id}
                  empty={t("No hay copias de seguridad.")}
                  cols={[
                    { key: "id", label: "ID", render: (x: any) => <code>{x.id}</code> },
                    { key: "t", label: t("Hora"), render: (x: any) => String(x.windowStartTime ?? "").replace("T", " ").replace(/Z$/, "") },
                    { key: "s", label: t("Estado"), render: (x: any) => x.status },
                    { key: "r", label: "", render: (x: any) => <button type="button" className="cc-link" onClick={() => h.confirm({ title: t("¿Restaurar la copia {id}?", { id: x.id }), text: t("Se sobrescribirán los datos actuales de la instancia."), cmds: [`gcloud sql backups restore ${x.id} --restore-instance=${i.name} --quiet`], button: t("Restaurar"), note: t("Restaurar copia de seguridad") })}>{t("Restaurar")}</button> },
                  ]}
                />
              </Section>
            ),
          },
        ]}
      />
      {h.node}
    </>
  );
}

// ---------------------------------------------------------------- VPC

export function FirewallDetail({ ctx, r, close }: { ctx: Ctx; r: any; close: () => void }) {
  const { t } = useI18n();
  const h = useCtxHost(ctx);
  if (h.page) return h.page;
  const rules = (r.rules ?? []).map((x: any) => (x.ports?.length ? x.ports.map((p: string) => `${x.IPProtocol}:${p}`).join(",") : x.IPProtocol)).join(",");
  const egress = r.direction === "EGRESS";
  const ranges = (egress ? r.destinationRanges : r.sourceRanges) ?? [];
  const edit: FormSpec = {
    title: t("Editar la regla de cortafuegos {r}", { r: r.name }),
    submit: t("Guardar"),
    note: t("Editar regla de cortafuegos"),
    initial: { priority: String(r.priority), ranges: ranges.join(", "), rules, targets: (r.targetTags ?? []).join(", "), logging: !!r.logging, disabled: !!r.disabled },
    fields: [
      { id: "priority", label: t("Prioridad"), type: "number" },
      { id: "targets", label: t("Etiquetas de destino"), help: t("Vacío = todas las instancias de la red.") },
      { id: "ranges", label: egress ? t("Rangos de IP de destino") : t("Rangos de IP de origen"), required: true },
      { id: "rules", label: t("Protocolos y puertos"), required: true },
      { id: "logging", label: t("Registros de cortafuegos"), type: "check" },
      { id: "disabled", label: t("Inhabilitar la regla"), type: "check" },
    ],
    build: (v) => {
      const f: string[] = [];
      const nr = splitList(v.ranges);
      if (String(v.priority) !== String(r.priority)) f.push(`--priority=${v.priority}`);
      if (nr.join(",") !== ranges.join(",")) f.push(`${egress ? "--destination-ranges" : "--source-ranges"}=${nr.join(",")}`);
      if (String(v.rules).replace(/\s/g, "") !== rules) f.push(`--rules=${String(v.rules).replace(/\s/g, "")}`);
      const nt = splitList(v.targets);
      if (nt.join(",") !== (r.targetTags ?? []).join(",")) f.push(`--target-tags=${nt.join(",")}`);
      if (!!v.logging !== !!r.logging) f.push(v.logging ? "--enable-logging" : "--no-enable-logging");
      if (!!v.disabled !== !!r.disabled) f.push(v.disabled ? "--disabled" : "--no-disabled");
      return f.length ? `gcloud compute firewall-rules update ${r.name} ${f.join(" ")}` : "";
    },
  };
  return (
    <>
      <Detail
        title={r.name}
        onBack={close}
        actions={
          <>
            <Tool icon="edit" label={t("Editar")} onClick={() => h.form(edit)} />
            <Tool icon="delete" label={t("Eliminar")} danger onClick={() => h.confirm({ title: t("¿Eliminar la regla {r}?", { r: r.name }), text: t("El tráfico que permitían quedará bloqueado (o permitido, si eran de denegación)."), cmds: [`gcloud compute firewall-rules delete ${r.name} --quiet`], button: t("Eliminar"), note: t("Eliminar regla de cortafuegos") })} />
          </>
        }
        tabs={[{
          id: "d",
          label: t("Detalles"),
          render: () => (
            <Props rows={[
              [t("Red"), r.network],
              [t("Dirección"), egress ? t("Salida") : t("Entrada")],
              [t("Acción si hay coincidencia"), r.action === "DENY" ? t("Denegar") : t("Permitir")],
              [t("Prioridad"), r.priority],
              [t("Destinos"), r.targetTags?.length ? <Chips key="t" items={r.targetTags} /> : r.targetServiceAccounts?.length ? <Chips key="s" items={r.targetServiceAccounts} /> : t("Todas las instancias")],
              [egress ? t("Rangos de IP de destino") : t("Rangos de IP de origen"), <Chips key="r" items={ranges} />],
              [t("Etiquetas de origen"), <Chips key="st" items={r.sourceTags} />],
              [t("Protocolos y puertos"), <code key="p">{rules}</code>],
              [t("Registros"), r.logging ? t("Activado") : t("Desactivado")],
              [t("Estado"), r.disabled ? t("Inhabilitada") : t("Habilitada")],
              [t("Descripción"), r.description],
            ]} />
          ),
        }]}
      />
      {h.node}
    </>
  );
}

export function SubnetDetail({ ctx, s, close }: { ctx: Ctx; s: any; close: () => void }) {
  const { t } = useI18n();
  const h = useCtxHost(ctx);
  const rg = `--region=${s.region}`;
  const prefix = Number(String(s.ipCidrRange).split("/")[1] ?? 24);
  return (
    <>
      <Detail
        title={s.name}
        onBack={close}
        actions={
          <>
            <Tool icon="edit" label={t("Ampliar el rango de IP")} onClick={() => h.form({ kind: "drawer", title: t("Ampliar el rango de {s}", { s: s.name }), submit: t("Guardar"), note: t("Ampliar rango de subred"), initial: { prefix: String(prefix - 1) }, fields: [{ id: "prefix", label: t("Longitud del prefijo nuevo"), type: "number", required: true, help: t("Solo se puede ampliar (prefijo más corto) y no puede solaparse con otras subredes.") }], build: (v) => (Number(v.prefix) < prefix ? `gcloud compute networks subnets expand-ip-range ${s.name} ${rg} --prefix-length=${v.prefix} --quiet` : "") })} />
            <Tool icon="delete" label={t("Eliminar")} danger onClick={() => h.confirm({ title: t("¿Eliminar la subred {s}?", { s: s.name }), text: t("Las VM que las usan tienen que borrarse antes."), cmds: [`gcloud compute networks subnets delete ${s.name} ${rg} --quiet`], button: t("Eliminar"), note: t("Eliminar subred") })} />
          </>
        }
        tabs={[{
          id: "d",
          label: t("Detalles"),
          render: () => (
            <>
              <Props rows={[[t("Red"), s.network], [t("Región"), s.region], [t("Rango de IP"), <code key="r">{s.ipCidrRange}</code>], [t("Rangos secundarios"), <Chips key="sr" items={(s.secondaryRanges ?? []).map((x: any) => `${x.rangeName ?? x.name}: ${x.ipCidrRange ?? x.range}`)} />]]} />
              <Section title={t("Opciones")}>
                <label className="cc-check">
                  <input type="checkbox" checked={!!s.privateIpGoogleAccess} onChange={(e) => ctx.run(`gcloud compute networks subnets update ${s.name} ${rg} ${e.target.checked ? "--enable-private-ip-google-access" : "--no-enable-private-ip-google-access"}`, t("Acceso privado a Google"))} />
                  <span>{t("Acceso privado a Google")}<span className="cc-help">{t("Permite a las VM sin IP externa usar las APIs de Google.")}</span></span>
                </label>
                <label className="cc-check" style={{ marginTop: 12 }}>
                  <input type="checkbox" checked={!!s.flowLogs} onChange={(e) => ctx.run(`gcloud compute networks subnets update ${s.name} ${rg} ${e.target.checked ? "--enable-flow-logs" : "--no-enable-flow-logs"}`, t("Registros de flujo"))} />
                  <span>{t("Registros de flujo")}</span>
                </label>
              </Section>
            </>
          ),
        }]}
      />
      {h.node}
    </>
  );
}

// ---------------------------------------------------------------- Pub/Sub

export function TopicDetail({ ctx, tp, close }: { ctx: Ctx; tp: any; close: () => void }) {
  const { t } = useI18n();
  const h = useCtxHost(ctx);
  const subs = vals(ctx.data.subscriptions).filter((s) => s.topic === tp.name);
  return (
    <>
      <Detail
        title={tp.name}
        onBack={close}
        actions={
          <>
            <Tool icon="mail" label={t("Publicar mensaje")} onClick={() => h.form({ kind: "drawer", title: t("Publicar un mensaje en {t}", { t: tp.name }), submit: t("Publicar"), note: t("Publicar mensaje"), initial: { body: "", attrs: "" }, fields: [{ id: "body", label: t("Cuerpo del mensaje"), type: "textarea", required: true }, { id: "attrs", label: t("Atributos (clave=valor)"), placeholder: "origen=consola" }], build: (v) => `gcloud pubsub topics publish ${tp.name} --message=${q(v.body)}${v.attrs.trim() ? ` --attribute=${q(Object.entries(parseKV(v.attrs)).map(([k, x]) => `${k}=${x}`).join(","))}` : ""}` })} />
            <Tool icon="add" label={t("Crear suscripción")} onClick={() => h.form({ kind: "drawer", title: t("Crear una suscripción a {t}", { t: tp.name }), submit: t("Crear"), note: t("Crear suscripción"), initial: { name: `${tp.name}-sub`, push: "" }, fields: [{ id: "name", label: t("ID de la suscripción"), required: true }, { id: "push", label: t("Endpoint de push (vacío = pull)"), placeholder: "https://…" }], build: (v) => `gcloud pubsub subscriptions create ${q(v.name.trim())} --topic=${tp.name}${v.push.trim() ? ` --push-endpoint=${q(v.push.trim())}` : ""}` })} />
            <Tool icon="delete" label={t("Eliminar")} danger onClick={() => h.confirm({ title: t("¿Eliminar el tema {t}?", { t: tp.name }), text: t("Sus suscripciones dejarán de recibir mensajes."), cmds: [`gcloud pubsub topics delete ${tp.name} --quiet`], button: t("Eliminar"), note: t("Eliminar tema") })} />
          </>
        }
        tabs={[
          { id: "subs", label: t("Suscripciones"), render: () => <Table caption={t("Suscripciones")} filter={false} rows={subs} rowKey={(s: any) => s.name} empty={t("No hay suscripciones.")} cols={[{ key: "n", label: t("ID de la suscripción"), render: (s: any) => s.name }, { key: "ty", label: t("Tipo de entrega"), render: (s: any) => (s.pushEndpoint ? "Push" : "Pull") }, { key: "b", label: t("Mensajes sin confirmar"), render: (s: any) => (s.backlog ?? []).length }]} /> },
          { id: "d", label: t("Detalles"), render: () => <Props rows={[[t("Nombre del tema"), <code key="n">projects/{ctx.project}/topics/{tp.name}</code>], [t("Mensajes publicados"), tp.publishedCount ?? 0], [t("Retención de mensajes"), tp.messageRetentionDuration], [t("Cifrado"), tp.kmsKeyName ? t("Clave gestionada por el cliente") : t("Gestionado por Google")]]} /> },
        ]}
      />
      {h.node}
    </>
  );
}

export function SubscriptionDetail({ ctx, s, close }: { ctx: Ctx; s: any; close: () => void }) {
  const { t } = useI18n();
  const h = useCtxHost(ctx);
  if (h.page) return h.page;
  const topics = Object.keys(ctx.data.topics ?? {});
  return (
    <>
      <Detail
        title={s.name}
        onBack={close}
        actions={
          <>
            <Tool icon="edit" label={t("Editar")} onClick={() => h.form({ title: t("Editar la suscripción {s}", { s: s.name }), submit: t("Actualizar"), note: t("Editar suscripción"), initial: { ack: String(s.ackDeadlineSeconds), dlt: s.deadLetterTopic ?? "", max: String(s.maxDeliveryAttempts || 5) }, fields: [{ id: "ack", label: t("Plazo de confirmación (s)"), type: "number" }, { id: "dlt", label: t("Tema de mensajes fallidos"), type: "select", options: [["", t("Ninguno")], ...topics] }, { id: "max", label: t("Número máximo de intentos de entrega"), type: "number", when: (v) => !!v.dlt }], build: (v) => { const f: string[] = []; if (String(v.ack) !== String(s.ackDeadlineSeconds)) f.push(`--ack-deadline=${v.ack}`); if (v.dlt && (v.dlt !== s.deadLetterTopic || String(v.max) !== String(s.maxDeliveryAttempts))) f.push(`--dead-letter-topic=${v.dlt} --max-delivery-attempts=${v.max}`); if (!v.dlt && s.deadLetterTopic) f.push("--clear-dead-letter-policy"); return f.length ? `gcloud pubsub subscriptions update ${s.name} ${f.join(" ")}` : ""; } })} />
            <Tool icon="delete" label={t("Eliminar")} danger onClick={() => h.confirm({ title: t("¿Eliminar la suscripción {s}?", { s: s.name }), text: t("Los mensajes pendientes se perderán."), cmds: [`gcloud pubsub subscriptions delete ${s.name} --quiet`], button: t("Eliminar"), note: t("Eliminar suscripción") })} />
          </>
        }
        tabs={[
          {
            id: "messages",
            label: t("Mensajes"),
            render: () => (
              <>
                <div className="cc-actions-row">
                  <button type="button" className="cc-btn primary" disabled={!!s.pushEndpoint} onClick={() => h.show(t("Mensajes de {s}", { s: s.name }), `gcloud pubsub subscriptions pull ${s.name} --auto-ack --limit=10`, t("Extraer mensajes"))}>{t("Extraer")}</button>
                  <span className="cc-help">{s.pushEndpoint ? t("Es una suscripción push: los mensajes se envían a su endpoint.") : t("Extrae y confirma hasta 10 mensajes.")}</span>
                </div>
                <Table caption={t("Mensajes pendientes")} filter={false} rows={s.backlog ?? []} rowKey={(m: any) => m.messageId} empty={t("No hay mensajes sin confirmar.")} cols={[{ key: "i", label: "ID", render: (m: any) => <code>{m.messageId}</code> }, { key: "d", label: t("Datos"), render: (m: any) => m.data }, { key: "a", label: t("Intentos"), render: (m: any) => m.deliveryAttempt }, { key: "p", label: t("Publicado"), render: (m: any) => String(m.publishTime ?? "").replace("T", " ").replace(/Z$/, "") }]} />
              </>
            ),
          },
          { id: "d", label: t("Detalles"), render: () => <Props rows={[[t("Tema"), s.topic], [t("Tipo de entrega"), s.pushEndpoint ? `Push → ${s.pushEndpoint}` : "Pull"], [t("Plazo de confirmación"), `${s.ackDeadlineSeconds} s`], [t("Tema de mensajes fallidos"), s.deadLetterTopic ? `${s.deadLetterTopic} (${s.maxDeliveryAttempts})` : ""], [t("Filtro"), s.filter], [t("Retención"), s.messageRetentionDuration], [t("Entrega exactamente una vez"), s.enableExactlyOnceDelivery ? t("Sí") : t("No")], [t("Confirmados / en mensajes fallidos"), `${s.ackedCount ?? 0} / ${s.deadLetteredCount ?? 0}`]]} /> },
        ]}
      />
      {h.node}
    </>
  );
}

// ---------------------------------------------------------------- Secret Manager

export function SecretDetail({ ctx, s, close }: { ctx: Ctx; s: any; close: () => void }) {
  const { t } = useI18n();
  const h = useCtxHost(ctx);
  const versions = [...(s.versions ?? [])].reverse();
  const bindings = (s.iamPolicy?.bindings ?? []).flatMap((x: any) => (x.members ?? []).map((m: string) => ({ member: m, role: x.role })));
  return (
    <>
      <Detail
        title={s.name}
        onBack={close}
        actions={
          <>
            <Tool icon="add" label={t("Nueva versión")} onClick={() => h.form({ kind: "drawer", title: t("Añadir una versión a {s}", { s: s.name }), submit: t("Añadir"), note: t("Añadir versión del secreto"), initial: { value: "" }, fields: [{ id: "value", label: t("Valor del secreto"), type: "password", required: true }], build: (v) => `printf '%s' ${q(v.value)} | gcloud secrets versions add ${s.name} --data-file=-` })} />
            <Tool icon="delete" label={t("Eliminar")} danger onClick={() => h.confirm({ title: t("¿Eliminar el secreto {s}?", { s: s.name }), text: t("Las aplicaciones que los lean empezarán a fallar."), cmds: [`gcloud secrets delete ${s.name} --quiet`], button: t("Eliminar"), note: t("Eliminar secreto") })} />
          </>
        }
        tabs={[
          {
            id: "versions",
            label: t("Versiones"),
            render: () => (
              <Table
                caption={t("Versiones de {s}", { s: s.name })}
                filter={false}
                rows={versions}
                rowKey={(x: any) => String(x.id)}
                empty={t("No hay versiones.")}
                cols={[
                  { key: "id", label: t("Versión"), render: (x: any) => x.id },
                  { key: "st", label: t("Estado"), render: (x: any) => (x.state === "ENABLED" ? <Pill tone="ok">{t("Habilitada")}</Pill> : x.state === "DISABLED" ? <Pill tone="warn">{t("Inhabilitada")}</Pill> : <Pill>{t("Destruida")}</Pill>) },
                  {
                    key: "a",
                    label: t("Acciones"),
                    render: (x: any) => (
                      <span className="cc-chips">
                        {x.state === "ENABLED" && <button type="button" className="cc-link" onClick={() => h.show(t("Valor de la versión {v}", { v: x.id }), `gcloud secrets versions access ${x.id} --secret=${s.name}`, t("Ver valor del secreto"))}>{t("Ver el valor del secreto")}</button>}
                        {x.state === "ENABLED" && <button type="button" className="cc-link" onClick={() => ctx.run(`gcloud secrets versions disable ${x.id} --secret=${s.name}`, t("Inhabilitar versión"))}>{t("Inhabilitar")}</button>}
                        {x.state === "DISABLED" && <button type="button" className="cc-link" onClick={() => ctx.run(`gcloud secrets versions enable ${x.id} --secret=${s.name}`, t("Habilitar versión"))}>{t("Habilitar")}</button>}
                        {x.state !== "DESTROYED" && <button type="button" className="cc-link danger" onClick={() => h.confirm({ title: t("¿Destruir la versión {v}?", { v: x.id }), text: t("El valor se borrará para siempre."), cmds: [`gcloud secrets versions destroy ${x.id} --secret=${s.name} --quiet`], button: t("Destruir"), note: t("Destruir versión") })}>{t("Destruir")}</button>}
                      </span>
                    ),
                  },
                ]}
              />
            ),
          },
          {
            id: "perms",
            label: t("Permisos"),
            render: () => (
              <Section title={t("Principales con acceso")} actions={<Tool icon="add" label={t("Conceder acceso")} onClick={() => h.form(grant(t, `gcloud secrets add-iam-policy-binding ${s.name}`, ["roles/secretmanager.secretAccessor", "roles/secretmanager.viewer", "roles/secretmanager.admin"], t("Conceder acceso a {s}", { s: s.name })))} />}>
                <Bindings rows={bindings} onRemove={(b) => h.confirm({ title: t("¿Quitar el rol?"), text: t("{who} dejará de tener {role}.", { who: b.member, role: b.role }), cmds: [`gcloud secrets remove-iam-policy-binding ${s.name} --member=${q(b.member)} --role=${b.role}`], button: t("Quitar"), note: t("Quitar un rol") })} />
              </Section>
            ),
          },
          { id: "d", label: t("Descripción general"), render: () => <Props rows={[[t("Nombre"), <code key="n">projects/{ctx.project}/secrets/{s.name}</code>], [t("Replicación"), s.replication], [t("Cifrado"), s.kmsKeyName ? s.kmsKeyName : t("Gestionado por Google")], [t("Rotación"), s.rotationPeriod], [t("Etiquetas"), <Labels key="l" m={s.labels} />]]} /> },
        ]}
      />
      {h.node}
    </>
  );
}

// ---------------------------------------------------------------- IAM

export function SADetail({ ctx, sa, close }: { ctx: Ctx; sa: any; close: () => void }) {
  const { t } = useI18n();
  const h = useCtxHost(ctx);
  const bindings = (sa.iamPolicy?.bindings ?? []).flatMap((x: any) => (x.members ?? []).map((m: string) => ({ member: m, role: x.role })));
  const projectRoles = (ctx.data.iamPolicy?.bindings ?? []).filter((b: any) => (b.members ?? []).includes(`serviceAccount:${sa.email}`)).map((b: any) => b.role);
  return (
    <>
      <Detail
        title={sa.displayName || sa.email.split("@")[0]}
        onBack={close}
        status={<Status state={sa.disabled ? "DISABLED" : "ENABLED"} />}
        actions={
          <>
            <Tool icon="edit" label={t("Editar")} onClick={() => h.form({ kind: "drawer", title: t("Editar la cuenta de servicio"), submit: t("Guardar"), note: t("Editar cuenta de servicio"), initial: { name: sa.displayName ?? "" }, fields: [{ id: "name", label: t("Nombre visible"), required: true }], build: (v) => (v.name !== sa.displayName ? `gcloud iam service-accounts update ${sa.email} --display-name=${q(v.name)}` : "") })} />
            {sa.disabled ? (
              <Tool icon="play" label={t("Habilitar")} onClick={() => ctx.run(`gcloud iam service-accounts enable ${sa.email}`, t("Habilitar cuenta de servicio"))} />
            ) : (
              <Tool icon="power" label={t("Inhabilitar")} onClick={() => ctx.run(`gcloud iam service-accounts disable ${sa.email}`, t("Inhabilitar cuenta de servicio"))} />
            )}
            <Tool icon="delete" label={t("Eliminar")} danger onClick={() => h.confirm({ title: t("¿Eliminar la cuenta de servicio?"), text: t("Lo que dependa de ellas dejará de poder autenticarse."), cmds: [`gcloud iam service-accounts delete ${sa.email} --quiet`], button: t("Eliminar"), note: t("Eliminar cuenta de servicio") })} />
          </>
        }
        tabs={[
          { id: "d", label: t("Detalles"), render: () => <Props rows={[[t("Correo"), <code key="e">{sa.email}</code>], [t("Nombre visible"), sa.displayName], [t("Estado"), sa.disabled ? t("Inhabilitada") : t("Habilitada")], [t("Roles en el proyecto"), <Chips key="r" items={projectRoles} />]]} /> },
          {
            id: "perms",
            label: t("Principales con acceso"),
            render: () => (
              <Section title={t("Quién puede usar esta cuenta de servicio")} actions={<Tool icon="add" label={t("Conceder acceso")} onClick={() => h.form(grant(t, `gcloud iam service-accounts add-iam-policy-binding ${sa.email}`, ["roles/iam.serviceAccountUser", "roles/iam.serviceAccountTokenCreator", "roles/iam.workloadIdentityUser"], t("Conceder acceso a la cuenta de servicio")))} />}>
                <Bindings rows={bindings} onRemove={(b) => h.confirm({ title: t("¿Quitar el rol?"), text: t("{who} dejará de tener {role}.", { who: b.member, role: b.role }), cmds: [`gcloud iam service-accounts remove-iam-policy-binding ${sa.email} --member=${q(b.member)} --role=${b.role}`], button: t("Quitar"), note: t("Quitar un rol") })} />
              </Section>
            ),
          },
          {
            id: "keys",
            label: t("Claves"),
            render: () => (
              <Section title={t("Claves")} actions={<Tool icon="add" label={t("Añadir clave")} onClick={() => h.confirm({ title: t("¿Crear una clave JSON?"), text: t("Las claves descargables son un riesgo: si se filtran dan acceso a la cuenta. Usa Workload Identity o la suplantación siempre que puedas."), cmds: [`gcloud iam service-accounts keys create key.json --iam-account=${sa.email}`], button: t("Crear"), note: t("Crear clave") })} />}>
                <Table
                  caption={t("Claves")}
                  filter={false}
                  rows={sa.keys ?? []}
                  rowKey={(k: any) => k.id}
                  empty={t("Esta cuenta de servicio no tiene claves gestionadas por el usuario.")}
                  cols={[
                    { key: "id", label: t("ID de clave"), render: (k: any) => <code>{k.id}</code> },
                    { key: "c", label: t("Creada"), render: (k: any) => String(k.created ?? "").replace("T", " ").replace(/Z$/, "") },
                    { key: "l", label: "", render: (k: any) => (k.leaked ? <Pill tone="bad">{t("Expuesta")}</Pill> : null) },
                    { key: "x", label: "", render: (k: any) => <button type="button" className="cc-link danger" onClick={() => h.confirm({ title: t("¿Eliminar la clave?"), text: t("Quien la use dejará de poder autenticarse."), cmds: [`gcloud iam service-accounts keys delete ${k.id} --iam-account=${sa.email} --quiet`], button: t("Eliminar"), note: t("Eliminar clave") })}>{t("Eliminar")}</button> },
                  ]}
                />
              </Section>
            ),
          },
        ]}
      />
      {h.node}
    </>
  );
}

// ---------------------------------------------------------------- GKE

export function ClusterDetail({ ctx, c, close }: { ctx: Ctx; c: any; close: () => void }) {
  const { t } = useI18n();
  const h = useCtxHost(ctx);
  const loc = locFlag(c.location);
  const pools = c.nodePools ?? [];
  return (
    <>
      <Detail
        title={c.name}
        onBack={close}
        status={<Status state={c.status} />}
        actions={
          <>
            <Tool icon="shell" label={t("Conectar")} onClick={() => ctx.run(`gcloud container clusters get-credentials ${c.name} ${loc}`, t("Conectar con el clúster"))} />
            <Tool icon="delete" label={t("Eliminar")} danger onClick={() => h.confirm({ title: t("¿Eliminar el clúster {c}?", { c: c.name }), text: t("Se borrarán con todas sus cargas de trabajo."), cmds: [`gcloud container clusters delete ${c.name} ${loc} --quiet`], button: t("Eliminar"), note: t("Eliminar clúster") })} />
          </>
        }
        tabs={[
          {
            id: "d",
            label: t("Detalles"),
            render: () => (
              <Props rows={[
                [t("Modo"), c.autopilot ? "Autopilot" : t("Estándar")],
                [t("Ubicación"), c.location],
                [t("Canal de versiones"), c.releaseChannel],
                [t("Endpoint"), <code key="e">{c.endpoint}</code>],
                [t("Red / subred"), `${c.network}${c.subnetwork ? ` / ${c.subnetwork}` : ""}`],
                [t("Nodos privados"), c.privateNodes ? t("Sí") : t("No")],
                [t("Redes autorizadas del plano de control"), <Chips key="m" items={c.masterAuthorizedNetworks} />],
                [t("Workload Identity"), c.workloadPool ? <code key="w">{c.workloadPool}</code> : t("Inhabilitado")],
                [t("Nodos blindados"), c.shieldedNodes ? t("Activado") : t("Desactivado")],
                [t("Autorización binaria"), c.binaryAuthorization ? t("Activado") : t("Desactivado")],
              ]} />
            ),
          },
          {
            id: "nodes",
            label: t("Nodos"),
            render: () =>
              c.autopilot ? (
                <p className="cc-empty">{t("En Autopilot, Google gestiona los nodos: no hay grupos de nodos que administrar.")}</p>
              ) : (
                <Section title={t("Grupos de nodos")} actions={<Tool icon="add" label={t("Añadir grupo de nodos")} onClick={() => h.form({ kind: "drawer", title: t("Añadir un grupo de nodos a {c}", { c: c.name }), submit: t("Crear"), note: t("Crear grupo de nodos"), initial: { name: "pool-2", n: "1", mt: "e2-standard-4", spot: false }, fields: [{ id: "name", label: t("Nombre"), required: true }, { id: "n", label: t("Número de nodos"), type: "number" }, { id: "mt", label: t("Tipo de máquina"), type: "select", options: ["e2-medium", "e2-standard-2", "e2-standard-4", "n2-standard-4"] }, { id: "spot", label: t("VM Spot"), type: "check" }], build: (v) => `gcloud container node-pools create ${q(v.name.trim())} --cluster=${c.name} ${loc} --num-nodes=${v.n} --machine-type=${v.mt}${v.spot ? " --spot" : ""}` })} />}>
                  <Table
                    caption={t("Grupos de nodos")}
                    filter={false}
                    rows={pools}
                    rowKey={(p: any) => p.name}
                    empty={t("No hay grupos de nodos.")}
                    cols={[
                      { key: "n", label: t("Nombre"), render: (p: any) => p.name },
                      { key: "c", label: t("Nodos"), render: (p: any) => p.nodeCount },
                      { key: "m", label: t("Tipo de máquina"), render: (p: any) => p.machineType },
                      { key: "a", label: t("Autoescalado"), render: (p: any) => (p.autoscaling ? `${p.minNodeCount}–${p.maxNodeCount}` : t("Desactivado")) },
                      { key: "s", label: "Spot", render: (p: any) => (p.spot ? t("Sí") : t("No")) },
                      {
                        key: "x",
                        label: "",
                        render: (p: any) => (
                          <span className="cc-chips">
                            <button type="button" className="cc-link" onClick={() => h.form({ kind: "drawer", title: t("Cambiar el tamaño de {p}", { p: p.name }), submit: t("Cambiar tamaño"), note: t("Cambiar tamaño del grupo de nodos"), initial: { n: String(p.nodeCount) }, fields: [{ id: "n", label: t("Número de nodos"), type: "number", required: true }], build: (v) => (String(v.n) !== String(p.nodeCount) ? `gcloud container clusters resize ${c.name} ${loc} --node-pool=${p.name} --num-nodes=${v.n} --quiet` : "") })}>{t("Cambiar tamaño")}</button>
                            <button type="button" className="cc-link danger" onClick={() => h.confirm({ title: t("¿Eliminar el grupo de nodos {p}?", { p: p.name }), text: t("Los pods que se ejecutan en él se reprogramarán si hay sitio."), cmds: [`gcloud container node-pools delete ${p.name} --cluster=${c.name} ${loc} --quiet`], button: t("Eliminar"), note: t("Eliminar grupo de nodos") })}>{t("Eliminar")}</button>
                          </span>
                        ),
                      },
                    ]}
                  />
                </Section>
              ),
          },
        ]}
      />
      {h.node}
    </>
  );
}
