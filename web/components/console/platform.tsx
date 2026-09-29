"use client";

// Console pages for services that were only reachable from Cloud Shell:
// uptime checks, log sinks, budgets, Cloud Armor, VPC peering, Cloud VPN,
// Cloud Deploy, Source Repositories, Vertex AI, Security Command Center and
// Recommender.

import { useI18n } from "@/lib/i18n";
import type { Ctx } from "./pages";
import { List } from "./more";
import { Detail, Page, Pill, Props, REGIONS, Table, Tool, q } from "./ui";
import { useCtxHost } from "./details";

const vals = (m: any): any[] => (m ? Object.values(m) : []);
const regions = (r: string) => (REGIONS.includes(r) ? REGIONS : [r, ...REGIONS]);
const sh = (s: string) => `'${s.replace(/'/g, `'\\''`)}'`;

// ---------------------------------------------------------------- Monitoring / Logging / Billing

export function Uptime({ ctx }: { ctx: Ctx }) {
  const { t } = useI18n();
  return (
    <List
      ctx={ctx}
      title={t("Comprobaciones de disponibilidad")}
      intro={t("Sondas externas que piden una URL cada pocos minutos y avisan si deja de responder.")}
      rows={vals(ctx.data.uptimeChecks)}
      rowKey={(u: any) => u.name}
      empty={t("No hay comprobaciones de disponibilidad.")}
      cols={[
        { key: "name", label: t("Nombre"), render: (u: any) => u.name },
        { key: "t", label: t("Destino"), render: (u: any) => <code>{u.host}:{u.port}{u.path}</code> },
        { key: "p", label: t("Frecuencia"), render: (u: any) => `${u.period} min` },
        { key: "s", label: t("Estado"), render: (u: any) => <Pill tone={u.passing ? "ok" : "bad"}>{u.passing ? t("Correcta") : t("Fallando")}</Pill> },
      ]}
      create={{ label: t("Crear comprobación"), form: { title: t("Crear comprobación de disponibilidad"), submit: t("Crear"), note: t("Crear comprobación de disponibilidad"), initial: { name: "", host: "", path: "/", port: "80", period: "1" }, fields: [{ id: "name", label: t("Nombre"), required: true }, { id: "host", label: t("Host o IP"), required: true }, { id: "path", label: t("Ruta") }, { id: "port", label: t("Puerto"), type: "number" }, { id: "period", label: t("Frecuencia (minutos)"), type: "select", options: ["1", "5", "10", "15"] }], build: (v) => `gcloud monitoring uptime create ${q(v.name.trim())} --resource-type=uptime-url --resource-labels=host=${v.host.trim()},project_id=${ctx.project} --path=${q(v.path || "/")} --port=${v.port} --period=${v.period}` } }}
      del={{ cmd: (u: any) => `gcloud monitoring uptime delete ${q(u.name)} --quiet`, title: t("¿Eliminar las comprobaciones?"), text: t("Dejarás de saber si el servicio responde."), note: t("Eliminar comprobación") }}
    />
  );
}

export function LogSinks({ ctx }: { ctx: Ctx }) {
  const { t } = useI18n();
  return (
    <List
      ctx={ctx}
      title={t("Enrutador de registros")}
      intro={t("Los sumideros copian los registros que cumplen un filtro a Cloud Storage, BigQuery o Pub/Sub. La identidad del sumidero necesita permiso en el destino.")}
      rows={vals(ctx.data.logSinks)}
      rowKey={(s: any) => s.name}
      empty={t("No hay sumideros.")}
      cols={[
        { key: "name", label: t("Nombre"), render: (s: any) => s.name },
        { key: "d", label: t("Destino"), render: (s: any) => <code>{s.destination}</code> },
        { key: "f", label: t("Filtro"), render: (s: any) => <code>{s.filter || "—"}</code> },
        { key: "w", label: t("Identidad de escritura"), render: (s: any) => <code>{s.writerIdentity}</code> },
      ]}
      create={{ label: t("Crear sumidero"), form: { title: t("Crear sumidero"), submit: t("Crear"), note: t("Crear sumidero"), initial: { name: "", kind: "storage", target: "", filter: "severity>=ERROR" }, fields: [{ id: "name", label: t("Nombre"), required: true }, { id: "kind", label: t("Servicio de destino"), type: "select", options: [["storage", "Cloud Storage"], ["bigquery", "BigQuery"], ["pubsub", "Pub/Sub"]] }, { id: "target", label: t("Bucket, dataset o tema"), required: true }, { id: "filter", label: t("Filtro de inclusión"), type: "textarea" }], build: (v) => { const d = v.kind === "storage" ? `storage.googleapis.com/${v.target.trim()}` : v.kind === "bigquery" ? `bigquery.googleapis.com/projects/${ctx.project}/datasets/${v.target.trim()}` : `pubsub.googleapis.com/projects/${ctx.project}/topics/${v.target.trim()}`; return `gcloud logging sinks create ${q(v.name.trim())} ${d}${v.filter.trim() ? ` --log-filter=${sh(v.filter.trim())}` : ""}`; } } }}
      del={{ cmd: (s: any) => `gcloud logging sinks delete ${s.name} --quiet`, title: t("¿Eliminar los sumideros?"), text: t("Los registros dejarán de copiarse."), note: t("Eliminar sumidero") }}
    />
  );
}

export function Budgets({ ctx }: { ctx: Ctx }) {
  const { t } = useI18n();
  return (
    <List
      ctx={ctx}
      title={t("Presupuestos y alertas")}
      rows={vals(ctx.data.budgets)}
      rowKey={(b: any) => b.name}
      empty={t("No hay presupuestos.")}
      cols={[
        { key: "name", label: t("Nombre"), render: (b: any) => b.name },
        { key: "a", label: t("Importe"), render: (b: any) => `${b.amount} EUR` },
        { key: "th", label: t("Umbrales"), render: (b: any) => (b.thresholds ?? []).map((x: number) => `${Math.round(x * 100)}%`).join(", ") || "—" },
        { key: "tp", label: t("Tema de Pub/Sub"), render: (b: any) => b.pubsubTopic || "—" },
      ]}
      create={{ label: t("Crear presupuesto"), form: { title: t("Crear presupuesto"), submit: t("Crear"), note: t("Crear presupuesto"), initial: { name: "", amount: "100", t1: "0.5", t2: "0.9", t3: "1.0" }, fields: [{ id: "name", label: t("Nombre"), required: true }, { id: "amount", label: t("Importe mensual (EUR)"), type: "number" }, { id: "t1", label: t("Umbral 1"), type: "select", options: ["0.5", "0.75"] }, { id: "t2", label: t("Umbral 2"), type: "select", options: ["0.9", "0.8"] }, { id: "t3", label: t("Umbral 3"), type: "select", options: ["1.0", "1.2"] }], build: (v) => `gcloud billing budgets create --billing-account=0X0X0X-0X0X0X-0X0X0X --display-name=${q(v.name.trim())} --budget-amount=${v.amount}EUR --threshold-rule=percent=${v.t1} --threshold-rule=percent=${v.t2} --threshold-rule=percent=${v.t3}` } }}
      del={{ cmd: (b: any) => `gcloud billing budgets delete ${q(b.name)} --billing-account=0X0X0X-0X0X0X-0X0X0X --quiet`, title: t("¿Eliminar los presupuestos?"), text: t("Dejarás de recibir alertas de gasto."), note: t("Eliminar presupuesto") }}
    />
  );
}

// ---------------------------------------------------------------- Network security

export function CloudArmor({ ctx }: { ctx: Ctx }) {
  const { t } = useI18n();
  const bss = Object.keys(ctx.data.backendServices ?? {});
  return (
    <List
      ctx={ctx}
      title={t("Políticas de Cloud Armor")}
      intro={t("Reglas de cortafuegos de aplicación (L7) que se aplican a los servicios de backend de un balanceador externo.")}
      rows={vals(ctx.data.securityPolicies)}
      rowKey={(p: any) => p.name}
      empty={t("No hay políticas.")}
      cols={[
        { key: "name", label: t("Nombre"), render: (p: any) => p.name },
        { key: "r", label: t("Reglas"), render: (p: any) => (p.rules ?? []).length },
        { key: "u", label: t("Usada por"), render: (p: any) => vals(ctx.data.backendServices).filter((b: any) => b.securityPolicy === p.name).map((b: any) => b.name).join(", ") || "—" },
      ]}
      create={{ label: t("Crear política"), form: { title: t("Crear política de seguridad"), submit: t("Crear"), note: t("Crear política de Cloud Armor"), initial: { name: "" }, fields: [{ id: "name", label: t("Nombre"), required: true }], build: (v) => `gcloud compute security-policies create ${q(v.name.trim())}` } }}
      del={{ cmd: (p: any) => `gcloud compute security-policies delete ${p.name} --quiet`, title: t("¿Eliminar las políticas?"), text: t("Antes hay que quitarlas de los servicios de backend."), note: t("Eliminar política") }}
      detail={(p: any, close, h) => (
        <Detail
          title={p.name}
          onBack={close}
          actions={
            <>
              <Tool icon="add" label={t("Añadir regla")} onClick={() => h.form({ title: t("Añadir regla"), submit: t("Añadir"), note: t("Añadir regla de Cloud Armor"), kind: "drawer", initial: { prio: "1000", action: "deny-403", mode: "ip", ranges: "", expr: "", preview: false }, fields: [{ id: "prio", label: t("Prioridad"), type: "number", required: true }, { id: "action", label: t("Acción"), type: "select", options: ["allow", "deny-403", "deny-404", "deny-502", "throttle"] }, { id: "mode", label: t("Condición"), type: "select", options: [["ip", t("Rangos de IP")], ["expr", t("Expresión avanzada")]] }, { id: "ranges", label: t("Rangos de IP de origen"), required: true, when: (v) => v.mode === "ip" }, { id: "expr", label: t("Expresión"), required: true, placeholder: "origin.region_code == 'RU'", when: (v) => v.mode === "expr" }, { id: "preview", label: t("Modo vista previa (solo registrar)"), type: "check" }], build: (v) => `gcloud compute security-policies rules create ${v.prio} --security-policy=${p.name} --action=${v.action} ${v.mode === "ip" ? `--src-ip-ranges=${v.ranges.replace(/\s+/g, "")}` : `--expression=${sh(v.expr)}`}${v.preview ? " --preview" : ""}` })} />
              <Tool icon="shield" label={t("Aplicar a un backend")} disabled={!bss.length} onClick={() => h.form({ title: t("Aplicar a un servicio de backend"), submit: t("Aplicar"), note: t("Aplicar política"), kind: "drawer", initial: { bs: bss[0] }, fields: [{ id: "bs", label: t("Servicio de backend"), type: "select", options: bss }], build: (v) => `gcloud compute backend-services update ${v.bs} --security-policy=${p.name} --global` })} />
            </>
          }
          tabs={[{
            id: "rules",
            label: t("Reglas"),
            render: () => (
              <Table caption={t("Reglas")} rows={[...(p.rules ?? [])].sort((a: any, b: any) => a.priority - b.priority)} rowKey={(r: any) => String(r.priority)} empty="—" cols={[
                { key: "p", label: t("Prioridad"), render: (r: any) => r.priority },
                { key: "a", label: t("Acción"), render: (r: any) => <Pill tone={r.action === "allow" ? "ok" : "bad"}>{r.action}</Pill> },
                { key: "m", label: t("Condición"), render: (r: any) => <code>{r.expression || (r.srcIpRanges ?? []).join(", ")}</code> },
                { key: "v", label: t("Vista previa"), render: (r: any) => (r.preview ? t("Sí") : t("No")) },
                { key: "x", label: "", render: (r: any) => (r.priority === 2147483647 ? null : <button type="button" className="cc-link" onClick={() => ctx.run(`gcloud compute security-policies rules delete ${r.priority} --security-policy=${p.name} --quiet`, t("Eliminar regla"))}>{t("Eliminar")}</button>) },
              ]} />
            ),
          }]}
        />
      )}
    />
  );
}

export function Peering({ ctx }: { ctx: Ctx }) {
  const { t } = useI18n();
  const nets = Object.keys(ctx.data.networks ?? {});
  const rows = vals(ctx.data.networks).flatMap((n: any) => (n.peerings ?? []).map((p: any) => ({ ...p, from: n.name })));
  return (
    <List
      ctx={ctx}
      title={t("Emparejamiento de redes de VPC")}
      intro={t("Conecta dos VPC por IP privada. Tiene que crearse en los dos lados para quedar ACTIVE y no es transitivo.")}
      rows={rows}
      rowKey={(p: any) => `${p.from}/${p.name}`}
      empty={t("No hay emparejamientos.")}
      cols={[
        { key: "name", label: t("Nombre"), render: (p: any) => p.name },
        { key: "from", label: t("Red"), render: (p: any) => p.from },
        { key: "to", label: t("Red emparejada"), render: (p: any) => p.network },
        { key: "st", label: t("Estado"), render: (p: any) => <Pill tone={p.state === "ACTIVE" ? "ok" : "warn"}>{p.state}</Pill> },
      ]}
      create={{ label: t("Crear conexión"), form: { title: t("Crear emparejamiento"), submit: t("Crear"), note: t("Crear emparejamiento de VPC"), initial: { name: "", net: nets[0] ?? "default", peer: "", proj: ctx.project }, fields: [{ id: "name", label: t("Nombre"), required: true }, { id: "net", label: t("Tu red de VPC"), type: "select", options: nets }, { id: "peer", label: t("Red emparejada"), required: true }, { id: "proj", label: t("Proyecto de la red emparejada") }], build: (v) => `gcloud compute networks peerings create ${q(v.name.trim())} --network=${v.net} --peer-network=${q(v.peer.trim())}${v.proj && v.proj !== ctx.project ? ` --peer-project=${v.proj}` : ""}` } }}
      del={{ cmd: (p: any) => `gcloud compute networks peerings delete ${p.name} --network=${p.from} --quiet`, title: t("¿Eliminar los emparejamientos?"), text: t("Se cortará la conectividad privada entre las redes."), note: t("Eliminar emparejamiento") }}
    />
  );
}

export function VPN({ ctx }: { ctx: Ctx }) {
  const { t } = useI18n();
  const tunnels = vals(ctx.data.vpnGateways).flatMap((g: any) => (g.tunnels ?? []).map((x: any) => ({ ...x, gw: g.name, region: g.region, network: g.network })));
  return (
    <List
      ctx={ctx}
      title={t("Túneles de Cloud VPN")}
      rows={tunnels}
      rowKey={(x: any) => x.name}
      empty={t("No hay túneles. Crea una pasarela con gcloud compute vpn-gateways create.")}
      cols={[
        { key: "name", label: t("Nombre"), render: (x: any) => x.name },
        { key: "gw", label: t("Pasarela"), render: (x: any) => x.gw },
        { key: "nw", label: t("Red"), render: (x: any) => x.network },
        { key: "peer", label: t("IP del par"), render: (x: any) => <code>{x.peerIp}</code> },
        { key: "r", label: t("Cloud Router"), render: (x: any) => x.router || "—" },
        { key: "st", label: t("Estado"), render: (x: any) => <Pill tone={x.status === "ESTABLISHED" ? "ok" : "bad"}>{x.status}</Pill> },
      ]}
    />
  );
}

// ---------------------------------------------------------------- CI/CD

export function CloudDeploy({ ctx }: { ctx: Ctx }) {
  const { t } = useI18n();
  const releases = [...(ctx.data.releases ?? [])].reverse();
  return (
    <>
      <List
        ctx={ctx}
        title={t("Flujos de entrega")}
        intro={t("Cloud Deploy promociona cada versión por los destinos en orden (p. ej. staging → prod), con aprobación y reversión.")}
        rows={vals(ctx.data.deliveryPipelines)}
        rowKey={(p: any) => p.name}
        empty={t("No hay flujos. Créalos con gcloud deploy apply --file=clouddeploy.yaml.")}
        cols={[
          { key: "name", label: t("Nombre"), render: (p: any) => p.name },
          { key: "r", label: t("Región"), render: (p: any) => p.region },
          { key: "s", label: t("Destinos"), render: (p: any) => (p.stages ?? []).map((s: any) => s.targetId + (s.canaryPercentages?.length ? ` (canary ${s.canaryPercentages.join("/")}%)` : "")).join(" → ") },
          { key: "rb", label: t("Reversión automática"), render: (p: any) => (p.automaticRollback ? t("Sí") : t("No")) },
        ]}
        extra={(h, sel) => <Tool icon="add" label={t("Crear versión")} disabled={sel.length !== 1} onClick={() => h.form({ title: t("Crear versión"), submit: t("Crear"), note: t("Crear versión de Cloud Deploy"), kind: "drawer", initial: { name: "", image: "" }, fields: [{ id: "name", label: t("Nombre de la versión"), required: true }, { id: "image", label: t("Imagen (app=REGION-docker.pkg.dev/...)"), required: true }], build: (v) => `gcloud deploy releases create ${q(v.name.trim())} --delivery-pipeline=${sel[0].name} --region=${sel[0].region} --images=${v.image.trim()}` })} />}
      />
      <List
        ctx={ctx}
        title={t("Versiones")}
        rows={releases}
        rowKey={(r: any) => r.name}
        empty={t("No hay versiones.")}
        cols={[
          { key: "name", label: t("Versión"), render: (r: any) => r.name },
          { key: "p", label: t("Flujo"), render: (r: any) => r.deliveryPipeline },
          { key: "i", label: t("Imagen"), render: (r: any) => <code>{r.image}</code> },
          { key: "ro", label: t("Despliegues"), render: (r: any) => (r.rollouts ?? []).map((x: any) => `${x.target}: ${x.state}`).join(" · ") || "—" },
        ]}
        extra={(h, sel) => <Tool icon="play" label={t("Promocionar")} disabled={sel.length !== 1} onClick={() => ctx.run(`gcloud deploy releases promote --release=${sel[0].name} --delivery-pipeline=${sel[0].deliveryPipeline} --region=${vals(ctx.data.deliveryPipelines).find((p: any) => p.name === sel[0].deliveryPipeline)?.region ?? ctx.region} --quiet`, t("Promocionar versión"))} />}
      />
    </>
  );
}

export function SourceRepos({ ctx }: { ctx: Ctx }) {
  const { t } = useI18n();
  return (
    <List
      ctx={ctx}
      title={t("Repositorios de código")}
      rows={vals(ctx.data.sourceRepos)}
      rowKey={(r: any) => r.name}
      empty={t("No hay repositorios.")}
      cols={[
        { key: "name", label: t("Nombre"), render: (r: any) => r.name },
        { key: "c", label: t("Confirmaciones"), render: (r: any) => (r.commits ?? []).length },
        { key: "l", label: t("Última confirmación"), render: (r: any) => { const c = (r.commits ?? []).slice(-1)[0]; return c ? <span><code>{String(c.sha).slice(0, 7)}</code> {c.message}</span> : "—"; } },
        { key: "u", label: t("Clonar"), render: (r: any) => <code>gcloud source repos clone {r.name}</code> },
      ]}
      create={{ label: t("Crear repositorio"), form: { title: t("Crear repositorio"), submit: t("Crear"), note: t("Crear repositorio de código"), initial: { name: "" }, fields: [{ id: "name", label: t("Nombre"), required: true }], build: (v) => `gcloud source repos create ${q(v.name.trim())}` } }}
      del={{ cmd: (r: any) => `gcloud source repos delete ${r.name} --quiet`, title: t("¿Eliminar los repositorios?"), text: t("Se perderá su historial."), note: t("Eliminar repositorio") }}
    />
  );
}

// ---------------------------------------------------------------- Vertex AI

export function VertexAI({ ctx }: { ctx: Ctx }) {
  const { t } = useI18n();
  const models = vals(ctx.data.aiModels);
  return (
    <>
      <List
        ctx={ctx}
        title={t("Registro de modelos")}
        rows={models}
        rowKey={(m: any) => m.id}
        empty={t("No hay modelos. Súbelos con gcloud ai models upload.")}
        cols={[
          { key: "n", label: t("Nombre"), render: (m: any) => m.displayName },
          { key: "id", label: "ID", render: (m: any) => <code>{m.id}</code> },
          { key: "r", label: t("Región"), render: (m: any) => m.region },
          { key: "i", label: t("Contenedor"), render: (m: any) => <code>{m.containerImageUri}</code> },
        ]}
      />
      <List
        ctx={ctx}
        title={t("Endpoints")}
        rows={vals(ctx.data.aiEndpoints)}
        rowKey={(e: any) => e.id}
        empty={t("No hay endpoints.")}
        cols={[
          { key: "n", label: t("Nombre"), render: (e: any) => e.displayName },
          { key: "r", label: t("Región"), render: (e: any) => e.region },
          { key: "d", label: t("Modelos desplegados"), render: (e: any) => (e.deployedModels ?? []).map((d: any) => `${d.modelDisplayName || d.model} (${d.trafficPercent ?? 0}%, ${d.machineType})`).join(", ") || "—" },
          { key: "p", label: t("Predicciones"), render: (e: any) => e.predictionCount },
          { key: "m", label: t("Monitorización"), render: (e: any) => (e.modelMonitoring ? t("Activada") : t("Desactivada")) },
        ]}
        create={{ label: t("Crear endpoint"), form: { title: t("Crear endpoint"), submit: t("Crear"), note: t("Crear endpoint de Vertex AI"), initial: { name: "", region: ctx.region }, fields: [{ id: "name", label: t("Nombre"), required: true }, { id: "region", label: t("Región"), type: "select", options: regions(ctx.region) }], build: (v) => `gcloud ai endpoints create --display-name=${q(v.name.trim())} --region=${v.region}` } }}
        extra={(h, sel) => <Tool icon="add" label={t("Desplegar modelo")} disabled={sel.length !== 1 || !models.length} onClick={() => h.form({ title: t("Desplegar modelo"), submit: t("Desplegar"), note: t("Desplegar modelo"), kind: "drawer", initial: { model: models[0]?.id, mt: "n1-standard-2", min: "1", max: "2" }, fields: [{ id: "model", label: t("Modelo"), type: "select", options: models.map((m: any) => [m.id, m.displayName] as [string, string]) }, { id: "mt", label: t("Tipo de máquina"), type: "select", options: ["n1-standard-2", "n1-standard-4", "e2-standard-4"] }, { id: "min", label: t("Réplicas mínimas"), type: "number" }, { id: "max", label: t("Réplicas máximas"), type: "number" }], build: (v) => `gcloud ai endpoints deploy-model ${sel[0].id} --region=${sel[0].region} --model=${v.model} --display-name=${q(models.find((m: any) => m.id === v.model)?.displayName ?? "model")} --machine-type=${v.mt} --min-replica-count=${v.min} --max-replica-count=${v.max} --traffic-split=0=100` })} />}
      />
    </>
  );
}

// ---------------------------------------------------------------- Security Command Center / Recommender

export function SecurityCenter({ ctx }: { ctx: Ctx }) {
  const { t } = useI18n();
  const h = useCtxHost(ctx);
  return (
    <Page title={t("Security Command Center")} intro={t("Hallazgos de configuración insegura y recomendaciones del proyecto.")} actions={<Tool icon="refresh" label={t("Actualizar")} onClick={ctx.refresh} />}>
      <Props rows={[[t("Proyecto"), ctx.project]]} />
      <p className="cc-toolbar">
        <button type="button" className="cc-btn primary" onClick={() => h.show(t("Hallazgos"), `gcloud scc findings list projects/${ctx.project} --filter='state="ACTIVE"'`, t("Listar hallazgos"))}>{t("Ver hallazgos activos")}</button>
        <button type="button" className="cc-btn" onClick={() => h.show(t("Recomendaciones de IAM"), `gcloud recommender recommendations list --project=${ctx.project} --location=global --recommender=google.iam.policy.Recommender`, t("Listar recomendaciones"))}>{t("Recomendaciones de IAM")}</button>
        <button type="button" className="cc-btn" onClick={() => h.show(t("VMs inactivas"), `gcloud recommender recommendations list --project=${ctx.project} --location=${ctx.zone} --recommender=google.compute.instance.IdleResourceRecommender`, t("Listar recomendaciones"))}>{t("VMs inactivas")}</button>
      </p>
      {h.node}
    </Page>
  );
}
