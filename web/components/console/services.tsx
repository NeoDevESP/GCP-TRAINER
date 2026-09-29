"use client";

// Console pages of the newer managed services: Cloud Functions, Cloud
// Scheduler, Cloud Tasks, App Engine, Firestore, Memorystore, Spanner,
// Dataflow, Dataproc, Cloud Composer and Filestore. As everywhere in the
// console, every action runs the equivalent command in Cloud Shell.

import { useState } from "react";
import { api } from "@/lib/api";
import { useI18n } from "@/lib/i18n";
import type { Ctx } from "./pages";
import { List } from "./more";
import { Detail, Pill, Props, REGIONS, Status, Table, Tool, ZONES, q } from "./ui";

const vals = (m: any): any[] => (m ? Object.values(m) : []);
const regions = (r: string) => (REGIONS.includes(r) ? REGIONS : [r, ...REGIONS]);
const zones = (z: string) => (ZONES.includes(z) ? ZONES : [z, ...ZONES]);
const when = (s?: string) => String(s ?? "").replace("T", " ").replace(/Z$/, "");
const sh = (s: string) => `'${s.replace(/'/g, `'\\''`)}'`;

const tone = (s: string) => (/^(ACTIVE|READY|RUNNING|ENABLED|SERVING|DONE|JOB_STATE_RUNNING|JOB_STATE_DONE|success)$/.test(s) ? "ok" : /FAIL|ERROR|CANCEL/.test(s) ? "bad" : "warn");
const pill = (s: string) => <Pill tone={tone(s)}>{s}</Pill>;

async function saveFile(ctx: Ctx, path: string, content: string) {
  await api(`/api/sessions/${ctx.sessionId}/files`, { method: "PUT", body: { path, content } });
}

// ---------------------------------------------------------------- Cloud Functions

const SAMPLE: Record<string, [string, string]> = {
  python312: ["main.py", "import functions_framework\n\n@functions_framework.http\ndef hello(request):\n    return 'Hello World!'\n"],
  nodejs20: ["index.js", "const functions = require('@google-cloud/functions-framework');\n\nfunctions.http('hello', (req, res) => {\n  res.send('Hello World!');\n});\n"],
  go122: ["function.go", "package hello\n\nimport (\n\t\"fmt\"\n\t\"net/http\"\n)\n\nfunc hello(w http.ResponseWriter, r *http.Request) {\n\tfmt.Fprint(w, \"Hello World!\")\n}\n"],
};

export function Functions({ ctx }: { ctx: Ctx }) {
  const { t } = useI18n();
  return (
    <List
      ctx={ctx}
      title={t("Cloud Functions")}
      intro={t("Funciones que se ejecutan por una petición HTTP, un mensaje de Pub/Sub o una subida a un bucket.")}
      rows={vals(ctx.data.functions)}
      rowKey={(f: any) => f.name}
      empty={t("No hay funciones.")}
      cols={[
        { key: "name", label: t("Nombre"), render: (f: any) => f.name },
        { key: "st", label: t("Estado"), render: (f: any) => <Status state={f.state === "ACTIVE" ? "RUNNING" : f.state} /> },
        { key: "env", label: t("Entorno"), render: (f: any) => (f.gen2 ? "2nd gen" : "1st gen") },
        { key: "reg", label: t("Región"), render: (f: any) => f.region },
        { key: "tr", label: t("Activador"), render: (f: any) => (f.trigger === "http" ? "HTTP" : f.trigger.replace("topic:", "Pub/Sub: ").replace("bucket:", "Cloud Storage: ")) },
        { key: "rt", label: t("Entorno de ejecución"), render: (f: any) => f.runtime },
        { key: "inv", label: t("Invocaciones"), render: (f: any) => f.invocations },
      ]}
      create={{
        label: t("Crear función"),
        form: {
          title: t("Crear función"),
          submit: t("Desplegar"),
          note: t("Desplegar función"),
          initial: { name: "", region: ctx.region, runtime: "python312", trigger: "http", topic: "", bucket: "", entry: "hello", code: SAMPLE.python312[1], pub: false },
          fields: [
            { id: "name", label: t("Nombre de la función"), required: true },
            { id: "region", label: t("Región"), type: "select", options: regions(ctx.region) },
            { id: "trigger", label: t("Tipo de activador"), type: "select", options: [["http", "HTTPS"], ["topic", "Cloud Pub/Sub"], ["bucket", "Cloud Storage"]] },
            { id: "pub", label: t("Permitir invocaciones sin autenticar"), type: "check", when: (v) => v.trigger === "http" },
            { id: "topic", label: t("Tema de Pub/Sub"), required: true, when: (v) => v.trigger === "topic" },
            { id: "bucket", label: t("Bucket"), required: true, when: (v) => v.trigger === "bucket" },
            { id: "runtime", label: t("Entorno de ejecución"), type: "select", section: t("Código"), options: Object.keys(SAMPLE) },
            { id: "entry", label: t("Punto de entrada"), required: true },
            { id: "code", label: t("Código fuente"), type: "textarea", help: t("Se guarda en Cloud Shell y se despliega con --source.") },
          ],
          before: async (v) => saveFile(ctx, `${v.name.trim()}/${SAMPLE[v.runtime][0]}`, v.code),
          build: (v) => {
            const n = v.name.trim();
            if (!n) return "";
            const trig = v.trigger === "http" ? `--trigger-http${v.pub ? " --allow-unauthenticated" : ""}` : v.trigger === "topic" ? `--trigger-topic=${q(v.topic.trim())}` : `--trigger-bucket=${q(v.bucket.trim())}`;
            return `gcloud functions deploy ${q(n)} --gen2 --runtime=${v.runtime} --region=${v.region} --source=./${n} --entry-point=${q(v.entry.trim())} ${trig}`;
          },
        },
      }}
      del={{ cmd: (f: any) => `gcloud functions delete ${f.name} --region=${f.region} --quiet`, title: t("¿Eliminar las funciones?"), text: t("Dejarán de responder a sus activadores."), note: t("Eliminar función") }}
      detail={(f: any, close, h) => (
        <Detail
          title={f.name}
          onBack={close}
          status={<Status state={f.state === "ACTIVE" ? "RUNNING" : f.state} />}
          actions={
            <>
              <Tool icon="play" label={t("Probar la función")} onClick={() => h.show(t("Resultado"), `gcloud functions call ${f.name} --region=${f.region} --data='{"name":"Cloud"}'`, t("Probar función"))} />
              <Tool icon="logs" label={t("Registros")} onClick={() => h.show(t("Registros"), `gcloud functions logs read ${f.name} --region=${f.region} --limit=20`, t("Leer registros"))} />
            </>
          }
          tabs={[
            {
              id: "d",
              label: t("Detalles"),
              render: () => (
                <Props
                  rows={[
                    [t("URL"), f.url ? <code key="u">{f.url}</code> : "—"],
                    [t("Activador"), f.trigger],
                    [t("Entorno de ejecución"), f.runtime],
                    [t("Punto de entrada"), f.entryPoint],
                    [t("Memoria"), f.memory],
                    [t("Tiempo de espera"), `${f.timeoutSeconds}s`],
                    [t("Instancias"), `${f.minInstances} – ${f.maxInstances}`],
                    [t("Cuenta de servicio"), f.serviceAccount],
                    [t("Variables de entorno"), Object.entries(f.env ?? {}).map(([k, v]) => `${k}=${v}`).join(", ") || "—"],
                    [t("Invocaciones"), `${f.invocations} (${f.errors} ${t("errores")})`],
                  ]}
                />
              ),
            },
            {
              id: "perm",
              label: t("Permisos"),
              render: () => (
                <>
                  <Table caption={t("Permisos")} rows={(f.iamPolicy?.bindings ?? []).flatMap((b: any) => b.members.map((m: string) => ({ m, r: b.role })))} rowKey={(x: any) => x.m + x.r} empty={t("Solo los principales con permiso en el proyecto pueden invocarla.")} cols={[{ key: "m", label: t("Principal"), render: (x: any) => x.m }, { key: "r", label: t("Rol"), render: (x: any) => x.r }]} />
                  <p>
                    <button type="button" className="cc-btn" onClick={() => ctx.run(`gcloud functions add-invoker-policy-binding ${f.name} --region=${f.region} --member=allUsers`, t("Permitir acceso público"))}>{t("Permitir acceso público")}</button>
                  </p>
                </>
              ),
            },
          ]}
        />
      )}
    />
  );
}

// ---------------------------------------------------------------- Cloud Scheduler / Cloud Tasks

export function Scheduler({ ctx }: { ctx: Ctx }) {
  const { t } = useI18n();
  return (
    <List
      ctx={ctx}
      title={t("Cloud Scheduler")}
      intro={t("Trabajos cron que publican en Pub/Sub o llaman a una URL. Se ejecutan según avanza el tiempo simulado.")}
      rows={vals(ctx.data.schedulerJobs)}
      rowKey={(j: any) => j.name}
      empty={t("No hay trabajos programados.")}
      cols={[
        { key: "name", label: t("Nombre"), render: (j: any) => j.name },
        { key: "st", label: t("Estado"), render: (j: any) => pill(j.state) },
        { key: "sc", label: t("Frecuencia"), render: (j: any) => <code>{j.schedule}</code> },
        { key: "tg", label: t("Destino"), render: (j: any) => (j.targetType === "pubsub" ? `Pub/Sub: ${j.topic}` : `${j.httpMethod} ${j.uri}`) },
        { key: "last", label: t("Última ejecución"), render: (j: any) => (j.lastAttemptTime ? `${when(j.lastAttemptTime)} (${j.lastStatus})` : "—") },
        { key: "runs", label: t("Ejecuciones"), render: (j: any) => j.runs },
      ]}
      create={{
        label: t("Crear trabajo"),
        form: {
          title: t("Crear un trabajo programado"),
          submit: t("Crear"),
          note: t("Crear trabajo de Scheduler"),
          initial: { name: "", loc: ctx.region, schedule: "*/5 * * * *", target: "pubsub", topic: "", body: "", uri: "", method: "POST" },
          fields: [
            { id: "name", label: t("Nombre"), required: true },
            { id: "loc", label: t("Región"), type: "select", options: regions(ctx.region) },
            { id: "schedule", label: t("Frecuencia (formato cron)"), required: true, help: t("Ejemplo: 0 9 * * 1 = los lunes a las 9:00") },
            { id: "target", label: t("Tipo de destino"), type: "select", options: [["pubsub", "Pub/Sub"], ["http", "HTTP"]] },
            { id: "topic", label: t("Tema"), required: true, when: (v) => v.target === "pubsub" },
            { id: "body", label: t("Mensaje"), required: true, when: (v) => v.target === "pubsub" },
            { id: "uri", label: "URL", required: true, when: (v) => v.target === "http" },
            { id: "method", label: t("Método HTTP"), type: "select", options: ["POST", "GET", "PUT"], when: (v) => v.target === "http" },
          ],
          build: (v) =>
            v.target === "pubsub"
              ? `gcloud scheduler jobs create pubsub ${q(v.name.trim())} --location=${v.loc} --schedule=${sh(v.schedule)} --topic=${q(v.topic.trim())} --message-body=${sh(v.body)}`
              : `gcloud scheduler jobs create http ${q(v.name.trim())} --location=${v.loc} --schedule=${sh(v.schedule)} --uri=${sh(v.uri.trim())} --http-method=${v.method}`,
        },
      }}
      extra={(h, sel) => (
        <>
          <Tool icon="play" label={t("Forzar ejecución")} disabled={sel.length !== 1} onClick={() => ctx.run(`gcloud scheduler jobs run ${sel[0].name} --location=${sel[0].location}`, t("Ejecutar trabajo"))} />
          <Tool icon="stop" label={t("Pausar")} disabled={!sel.length} onClick={() => ctx.runAll(sel.map((j: any) => `gcloud scheduler jobs pause ${j.name} --location=${j.location}`), t("Pausar trabajo"))} />
          <Tool icon="power" label={t("Reanudar")} disabled={!sel.length} onClick={() => ctx.runAll(sel.map((j: any) => `gcloud scheduler jobs resume ${j.name} --location=${j.location}`), t("Reanudar trabajo"))} />
        </>
      )}
      del={{ cmd: (j: any) => `gcloud scheduler jobs delete ${j.name} --location=${j.location} --quiet`, title: t("¿Eliminar los trabajos?"), text: t("Dejarán de ejecutarse."), note: t("Eliminar trabajo") }}
    />
  );
}

export function Tasks({ ctx }: { ctx: Ctx }) {
  const { t } = useI18n();
  return (
    <List
      ctx={ctx}
      title={t("Colas de Cloud Tasks")}
      rows={vals(ctx.data.taskQueues)}
      rowKey={(x: any) => x.name}
      empty={t("No hay colas.")}
      cols={[
        { key: "name", label: t("Nombre"), render: (x: any) => x.name },
        { key: "st", label: t("Estado"), render: (x: any) => pill(x.state) },
        { key: "loc", label: t("Región"), render: (x: any) => x.location },
        { key: "n", label: t("Tareas en cola"), render: (x: any) => (x.tasks ?? []).length },
        { key: "d", label: t("Entregadas"), render: (x: any) => x.dispatched },
        { key: "r", label: t("Frecuencia máxima"), render: (x: any) => `${x.maxDispatchesPerSecond}/s` },
      ]}
      create={{ label: t("Crear cola"), form: { title: t("Crear cola"), submit: t("Crear"), note: t("Crear cola"), initial: { name: "", loc: ctx.region }, fields: [{ id: "name", label: t("Nombre"), required: true }, { id: "loc", label: t("Región"), type: "select", options: regions(ctx.region) }], build: (v) => `gcloud tasks queues create ${q(v.name.trim())} --location=${v.loc}` } }}
      extra={(h, sel) => (
        <>
          <Tool icon="add" label={t("Añadir tarea HTTP")} disabled={sel.length !== 1} onClick={() => h.form({ title: t("Añadir tarea HTTP"), submit: t("Crear"), note: t("Crear tarea"), kind: "drawer", initial: { url: "", body: "" }, fields: [{ id: "url", label: "URL", required: true }, { id: "body", label: t("Cuerpo") }], build: (v) => `gcloud tasks create-http-task --queue=${sel[0].name} --location=${sel[0].location} --url=${sh(v.url.trim())}${v.body ? ` --body-content=${sh(v.body)}` : ""}` })} />
          <Tool icon="stop" label={t("Pausar")} disabled={!sel.length} onClick={() => ctx.runAll(sel.map((x: any) => `gcloud tasks queues pause ${x.name} --location=${x.location}`), t("Pausar cola"))} />
          <Tool icon="power" label={t("Reanudar")} disabled={!sel.length} onClick={() => ctx.runAll(sel.map((x: any) => `gcloud tasks queues resume ${x.name} --location=${x.location}`), t("Reanudar cola"))} />
        </>
      )}
      del={{ cmd: (x: any) => `gcloud tasks queues delete ${x.name} --location=${x.location} --quiet`, title: t("¿Eliminar las colas?"), text: t("Se perderán sus tareas pendientes."), note: t("Eliminar cola") }}
    />
  );
}

// ---------------------------------------------------------------- App Engine

export function AppEngine({ ctx }: { ctx: Ctx }) {
  const { t } = useI18n();
  const app = ctx.data.appEngine;
  const [region, setRegion] = useState("europe-west");
  if (!app) {
    return (
      <section className="cc-page">
        <div className="cc-page-head"><h1>{t("App Engine")}</h1></div>
        <p>{t("Este proyecto todavía no tiene aplicación de App Engine. La región no se puede cambiar después.")}</p>
        <p>
          <label htmlFor="ae-region">{t("Región")} </label>
          <select id="ae-region" value={region} onChange={(e) => setRegion(e.target.value)}>
            {["europe-west", "europe-west3", "us-central", "us-east1"].map((r) => <option key={r}>{r}</option>)}
          </select>{" "}
          <button type="button" className="cc-btn primary" onClick={() => ctx.run(`gcloud app create --region=${region}`, t("Crear aplicación"))}>{t("Crear aplicación")}</button>
        </p>
        <p className="cc-help">{t("Después despliega con gcloud app deploy desde un directorio con app.yaml.")}</p>
      </section>
    );
  }
  const versions = vals(app.services).flatMap((s: any) => vals(s.versions).map((v: any) => ({ ...v, svc: s.name, split: s.split?.[v.id] ?? 0 })));
  return (
    <>
      <section className="cc-page">
        <div className="cc-page-head"><h1>{t("App Engine")}</h1></div>
        <Props rows={[[t("URL"), <code key="h">https://{app.defaultHostname}</code>], [t("Región"), app.locationId], [t("Estado"), app.servingStatus]]} />
      </section>
      <List
        ctx={ctx}
        title={t("Versiones")}
        rows={versions}
        rowKey={(v: any) => `${v.svc}/${v.id}`}
        empty={t("No hay versiones. Crea app.yaml y ejecuta gcloud app deploy.")}
        cols={[
          { key: "id", label: t("Versión"), render: (v: any) => v.id },
          { key: "svc", label: t("Servicio"), render: (v: any) => v.svc },
          { key: "st", label: t("Estado"), render: (v: any) => pill(v.servingStatus) },
          { key: "tr", label: t("Asignación de tráfico"), render: (v: any) => `${Math.round(v.split * 100)}%` },
          { key: "rt", label: t("Entorno de ejecución"), render: (v: any) => `${v.runtime} (${v.env})` },
          { key: "c", label: t("Desplegada"), render: (v: any) => when(v.createTime) },
        ]}
        extra={(h, sel) => (
          <>
            <Tool icon="refresh" label={t("Dividir el tráfico")} disabled={sel.length < 1 || new Set(sel.map((v: any) => v.svc)).size !== 1} onClick={() => h.form({ title: t("Dividir el tráfico"), submit: t("Guardar"), note: t("Dividir tráfico"), kind: "drawer", initial: Object.fromEntries(sel.map((v: any) => [v.id, String(Math.round(100 / sel.length))]).concat([["by", "random"]])), fields: [...sel.map((v: any) => ({ id: v.id, label: `${v.id} (%)`, type: "number" as const })), { id: "by", label: t("Dividir por"), type: "select", options: ["random", "ip", "cookie"] }], build: (v) => `gcloud app services set-traffic ${sel[0].svc} --splits=${sel.map((x: any) => `${x.id}=${Number(v[x.id]) / 100}`).join(",")} --split-by=${v.by} --quiet` })} />
            <Tool icon="stop" label={t("Detener")} disabled={!sel.length} onClick={() => ctx.runAll(sel.map((v: any) => `gcloud app versions stop ${v.id} --service=${v.svc} --quiet`), t("Detener versión"))} />
            <Tool icon="play" label={t("Iniciar")} disabled={!sel.length} onClick={() => ctx.runAll(sel.map((v: any) => `gcloud app versions start ${v.id} --service=${v.svc} --quiet`), t("Iniciar versión"))} />
          </>
        )}
        del={{ cmd: (v: any) => `gcloud app versions delete ${v.id} --service=${v.svc} --quiet`, title: t("¿Eliminar las versiones?"), text: t("Una versión que recibe tráfico no se puede eliminar."), note: t("Eliminar versión") }}
      />
    </>
  );
}

// ---------------------------------------------------------------- Firestore

export function Firestore({ ctx }: { ctx: Ctx }) {
  const { t } = useI18n();
  const dbs = vals(ctx.data.firestore);
  const [coll, setColl] = useState("");
  const base = `https://firestore.googleapis.com/v1/projects/${ctx.project}/databases/(default)/documents`;
  const auth = `-H "Authorization: Bearer $(gcloud auth print-access-token)"`;
  return (
    <List
      ctx={ctx}
      title={t("Firestore")}
      intro={t("Base de datos de documentos. Los documentos se leen y escriben con la API REST (curl) o las bibliotecas cliente.")}
      rows={dbs}
      rowKey={(d: any) => d.name}
      empty={t("No hay bases de datos.")}
      cols={[
        { key: "name", label: t("Base de datos"), render: (d: any) => d.name },
        { key: "loc", label: t("Ubicación"), render: (d: any) => d.locationId },
        { key: "type", label: t("Modo"), render: (d: any) => d.type },
        { key: "docs", label: t("Documentos"), render: (d: any) => Object.keys(d.documents ?? {}).length },
        { key: "dp", label: t("Protección contra eliminación"), render: (d: any) => (d.deleteProtection ? t("Activada") : t("Desactivada")) },
      ]}
      create={{ label: t("Crear base de datos"), form: { title: t("Crear base de datos de Firestore"), submit: t("Crear"), note: t("Crear base de datos"), initial: { loc: "eur3", type: "firestore-native" }, fields: [{ id: "loc", label: t("Ubicación"), type: "select", options: ["eur3", "nam5", "europe-west1", "us-central1"] }, { id: "type", label: t("Modo"), type: "select", options: [["firestore-native", t("Modo nativo")], ["datastore-mode", t("Modo Datastore")]] }], build: (v) => `gcloud firestore databases create --location=${v.loc} --type=${v.type}` } }}
      detail={(d: any, close, h) => {
        const docs = Object.entries(d.documents ?? {}) as [string, any][];
        const colls = [...new Set(docs.map(([k]) => k.split("/")[0]))];
        const cur = coll || colls[0] || "";
        return (
          <Detail
            title={d.name}
            onBack={close}
            actions={
              <>
                <Tool icon="add" label={t("Añadir documento")} onClick={() => h.form({ title: t("Añadir documento"), submit: t("Guardar"), note: t("Crear documento"), kind: "drawer", initial: { coll: cur, id: "", json: '{"name": "Ana", "age": 31}' }, fields: [{ id: "coll", label: t("Colección"), required: true }, { id: "id", label: t("ID del documento"), required: true }, { id: "json", label: t("Campos (JSON)"), type: "textarea" }], build: (v) => { let f: Record<string, any> = {}; try { f = JSON.parse(v.json || "{}"); } catch { return ""; } const fields = Object.fromEntries(Object.entries(f).map(([k, x]) => [k, typeof x === "number" ? (Number.isInteger(x) ? { integerValue: String(x) } : { doubleValue: x }) : typeof x === "boolean" ? { booleanValue: x } : { stringValue: String(x) }])); return `curl -s -X POST ${auth} "${base}/${v.coll.trim()}?documentId=${v.id.trim()}" -d ${sh(JSON.stringify({ fields }))}`; } })} />
                <Tool icon="download" label={t("Exportar")} onClick={() => h.form({ title: t("Exportar a Cloud Storage"), submit: t("Exportar"), note: t("Exportar Firestore"), kind: "drawer", initial: { dst: "" }, fields: [{ id: "dst", label: t("Destino (gs://bucket/carpeta)"), required: true }], build: (v) => `gcloud firestore export ${v.dst.trim()}` })} />
              </>
            }
            tabs={[
              {
                id: "data",
                label: t("Datos"),
                render: () => (
                  <div className="cc-fs">
                    <nav aria-label={t("Colecciones")}>
                      {colls.length ? colls.map((c) => <button key={c} type="button" className={`cc-link${c === cur ? " on" : ""}`} onClick={() => setColl(c)}>{c}</button>) : <p className="cc-empty">{t("Sin colecciones.")}</p>}
                    </nav>
                    <Table caption={t("Documentos")} rows={docs.filter(([k]) => k.startsWith(cur + "/")).map(([k, f]) => ({ k, f }))} rowKey={(x: any) => x.k} empty={t("Sin documentos.")} cols={[{ key: "id", label: t("ID"), render: (x: any) => <button type="button" className="cc-link" onClick={() => h.show(x.k, `curl -s ${auth} ${base}/${x.k}`, t("Leer documento"))}>{x.k.split("/").slice(1).join("/")}</button> }, { key: "f", label: t("Campos"), render: (x: any) => <code>{JSON.stringify(x.f)}</code> }]} />
                  </div>
                ),
              },
              { id: "idx", label: t("Índices"), render: () => <Table caption={t("Índices")} rows={d.indexes ?? []} rowKey={(x: any) => x.id} empty={t("Sin índices compuestos.")} cols={[{ key: "c", label: t("Colección"), render: (x: any) => x.collectionGroup }, { key: "f", label: t("Campos"), render: (x: any) => x.fields.join(", ") }, { key: "s", label: t("Estado"), render: (x: any) => pill(x.state) }]} /> },
            ]}
          />
        );
      }}
    />
  );
}

// ---------------------------------------------------------------- Memorystore / Filestore

export function Redis({ ctx }: { ctx: Ctx }) {
  const { t } = useI18n();
  return (
    <List
      ctx={ctx}
      title={t("Memorystore para Redis")}
      intro={t("Las instancias solo tienen IP privada: conéctate desde una VM de la misma red con redis-cli.")}
      rows={vals(ctx.data.redis)}
      rowKey={(r: any) => r.name}
      empty={t("No hay instancias.")}
      cols={[
        { key: "name", label: t("ID de instancia"), render: (r: any) => r.name },
        { key: "st", label: t("Estado"), render: (r: any) => pill(r.state) },
        { key: "v", label: t("Versión"), render: (r: any) => r.redisVersion },
        { key: "tier", label: t("Nivel"), render: (r: any) => r.tier },
        { key: "size", label: t("Capacidad"), render: (r: any) => `${r.memorySizeGb} GB` },
        { key: "ip", label: t("Endpoint principal"), render: (r: any) => <code>{r.host}:{r.port}</code> },
        { key: "nw", label: t("Red"), render: (r: any) => r.authorizedNetwork },
      ]}
      create={{ label: t("Crear instancia"), form: { title: t("Crear instancia de Redis"), submit: t("Crear"), note: t("Crear instancia de Redis"), initial: { name: "", region: ctx.region, tier: "basic", size: "1", network: "default", auth: false }, fields: [{ id: "name", label: t("ID de instancia"), required: true }, { id: "tier", label: t("Nivel"), type: "select", options: [["basic", t("Básico (sin réplica)")], ["standard", t("Estándar (alta disponibilidad)")]] }, { id: "size", label: t("Capacidad (GB)"), type: "number" }, { id: "region", label: t("Región"), type: "select", options: regions(ctx.region) }, { id: "network", label: t("Red"), type: "select", options: Object.keys(ctx.data.networks ?? { default: 1 }) }, { id: "auth", label: t("Habilitar AUTH"), type: "check" }], build: (v) => `gcloud redis instances create ${q(v.name.trim())} --region=${v.region} --tier=${v.tier} --size=${v.size} --network=${v.network}${v.auth ? " --enable-auth" : ""}` } }}
      del={{ cmd: (r: any) => `gcloud redis instances delete ${r.name} --region=${r.region} --quiet`, title: t("¿Eliminar las instancias?"), text: t("Se perderán sus datos."), note: t("Eliminar instancia de Redis") }}
    />
  );
}

export function Filestore({ ctx }: { ctx: Ctx }) {
  const { t } = useI18n();
  return (
    <List
      ctx={ctx}
      title={t("Filestore")}
      intro={t("Servidores NFS gestionados para montar desde VMs y GKE.")}
      rows={vals(ctx.data.filestore)}
      rowKey={(f: any) => f.name}
      empty={t("No hay instancias.")}
      cols={[
        { key: "name", label: t("ID de instancia"), render: (f: any) => f.name },
        { key: "st", label: t("Estado"), render: (f: any) => pill(f.state) },
        { key: "tier", label: t("Nivel"), render: (f: any) => f.tier },
        { key: "loc", label: t("Ubicación"), render: (f: any) => f.location },
        { key: "share", label: t("Recurso compartido"), render: (f: any) => <code>{f.ipAddress}:/{f.fileShare}</code> },
        { key: "cap", label: t("Capacidad"), render: (f: any) => `${f.capacityGb} GB` },
      ]}
      create={{ label: t("Crear instancia"), form: { title: t("Crear instancia de Filestore"), submit: t("Crear"), note: t("Crear Filestore"), initial: { name: "", zone: ctx.zone, tier: "BASIC_HDD", share: "vol1", cap: "1TB", network: "default" }, fields: [{ id: "name", label: t("ID de instancia"), required: true }, { id: "tier", label: t("Nivel"), type: "select", options: ["BASIC_HDD", "BASIC_SSD", "ZONAL", "REGIONAL"] }, { id: "zone", label: t("Zona"), type: "select", options: zones(ctx.zone) }, { id: "share", label: t("Nombre del recurso compartido"), required: true }, { id: "cap", label: t("Capacidad"), help: t("Mínimo 1TB en BASIC_HDD") }, { id: "network", label: t("Red"), type: "select", options: Object.keys(ctx.data.networks ?? { default: 1 }) }], build: (v) => `gcloud filestore instances create ${q(v.name.trim())} --zone=${v.zone} --tier=${v.tier} --file-share=name=${v.share.trim()},capacity=${v.cap.trim()} --network=name=${v.network}` } }}
      del={{ cmd: (f: any) => `gcloud filestore instances delete ${f.name} --zone=${f.location} --quiet`, title: t("¿Eliminar las instancias?"), text: t("Se perderán sus archivos."), note: t("Eliminar Filestore") }}
    />
  );
}

// ---------------------------------------------------------------- Spanner

export function Spanner({ ctx }: { ctx: Ctx }) {
  const { t } = useI18n();
  const [sql, setSql] = useState("SELECT 1");
  const [ddl, setDdl] = useState("CREATE TABLE Singers (SingerId INT64 NOT NULL, Name STRING(100)) PRIMARY KEY (SingerId)");
  return (
    <List
      ctx={ctx}
      title={t("Spanner")}
      intro={t("Base de datos relacional distribuida globalmente con SQL real.")}
      rows={vals(ctx.data.spanner)}
      rowKey={(i: any) => i.name}
      empty={t("No hay instancias.")}
      cols={[
        { key: "name", label: t("Instancia"), render: (i: any) => i.name },
        { key: "st", label: t("Estado"), render: (i: any) => pill(i.state) },
        { key: "cfg", label: t("Configuración"), render: (i: any) => i.config },
        { key: "pu", label: t("Unidades de procesamiento"), render: (i: any) => i.processingUnits },
        { key: "db", label: t("Bases de datos"), render: (i: any) => Object.keys(i.databases ?? {}).length },
      ]}
      create={{ label: t("Crear instancia"), form: { title: t("Crear instancia de Spanner"), submit: t("Crear"), note: t("Crear instancia de Spanner"), initial: { name: "", cfg: `regional-${ctx.region}`, pu: "100" }, fields: [{ id: "name", label: t("ID de instancia"), required: true }, { id: "cfg", label: t("Configuración"), type: "select", options: [`regional-${ctx.region}`, "regional-europe-west1", "regional-us-central1", "eur3", "nam6"] }, { id: "pu", label: t("Unidades de procesamiento"), type: "select", options: ["100", "200", "500", "1000", "2000"] }], build: (v) => `gcloud spanner instances create ${q(v.name.trim())} --config=${v.cfg} --processing-units=${v.pu} --description=${q(v.name.trim())}` } }}
      del={{ cmd: (i: any) => `gcloud spanner instances delete ${i.name} --quiet`, title: t("¿Eliminar las instancias?"), text: t("Se eliminarán todas sus bases de datos."), note: t("Eliminar instancia de Spanner") }}
      detail={(i: any, close, h) => (
        <Detail
          title={i.name}
          onBack={close}
          actions={<Tool icon="add" label={t("Crear base de datos")} onClick={() => h.form({ title: t("Crear base de datos"), submit: t("Crear"), note: t("Crear base de datos de Spanner"), kind: "drawer", initial: { name: "", ddl: "" }, fields: [{ id: "name", label: t("Nombre"), required: true }, { id: "ddl", label: t("Esquema (DDL, opcional)"), type: "textarea" }], build: (v) => `gcloud spanner databases create ${q(v.name.trim())} --instance=${i.name}${v.ddl.trim() ? ` --ddl=${sh(v.ddl.trim())}` : ""}` })} />}
          tabs={vals(i.databases).length ? vals(i.databases).map((d: any) => ({
            id: d.name,
            label: d.name,
            render: () => (
              <>
                <h2>{t("Esquema")}</h2>
                <pre className="cc-output">{(d.ddl ?? []).join(";\n\n") || t("Sin tablas.")}</pre>
                <label htmlFor={`ddl-${d.name}`}>{t("Actualizar el esquema (DDL)")}</label>
                <textarea id={`ddl-${d.name}`} className="bq-editor" rows={3} value={ddl} onChange={(e) => setDdl(e.target.value)} />
                <p><button type="button" className="cc-btn" onClick={() => ctx.run(`gcloud spanner databases ddl update ${d.name} --instance=${i.name} --ddl=${sh(ddl)}`, t("Actualizar esquema"))}>{t("Aplicar DDL")}</button></p>
                <h2>{t("Spanner Studio")}</h2>
                <label htmlFor={`sql-${d.name}`}>{t("Consulta (GoogleSQL)")}</label>
                <textarea id={`sql-${d.name}`} className="bq-editor" rows={4} value={sql} onChange={(e) => setSql(e.target.value)} />
                <p><button type="button" className="cc-btn primary" onClick={() => h.show(t("Resultados"), `gcloud spanner databases execute-sql ${d.name} --instance=${i.name} --sql=${sh(sql.replace(/\s+/g, " ").trim())}`, t("Ejecutar consulta"))}>{t("Ejecutar")}</button></p>
              </>
            ),
          })) : [{ id: "none", label: t("Bases de datos"), render: () => <p className="cc-empty">{t("Sin bases de datos.")}</p> }]}
        />
      )}
    />
  );
}

// ---------------------------------------------------------------- Dataflow / Dataproc / Composer

const TEMPLATES: [string, string][] = [
  ["Word_Count", "Word Count"],
  ["PubSub_Subscription_to_BigQuery", "Pub/Sub Subscription to BigQuery"],
  ["PubSub_to_BigQuery", "Pub/Sub Topic to BigQuery"],
  ["Cloud_PubSub_to_GCS_Text", "Pub/Sub to Text Files on Cloud Storage"],
];

export function Dataflow({ ctx }: { ctx: Ctx }) {
  const { t } = useI18n();
  return (
    <List
      ctx={ctx}
      title={t("Trabajos de Dataflow")}
      intro={t("Crea trabajos desde plantillas de Google. Los trabajos de streaming procesan mensajes a medida que avanza el tiempo.")}
      rows={[...(ctx.data.dataflowJobs ?? [])].reverse()}
      rowKey={(j: any) => j.id}
      empty={t("No hay trabajos.")}
      cols={[
        { key: "name", label: t("Nombre"), render: (j: any) => j.name },
        { key: "type", label: t("Tipo"), render: (j: any) => (j.type === "JOB_TYPE_STREAMING" ? t("Streaming") : t("Lote")) },
        { key: "st", label: t("Estado"), render: (j: any) => pill(j.currentState.replace("JOB_STATE_", "")) },
        { key: "tpl", label: t("Plantilla"), render: (j: any) => j.template },
        { key: "el", label: t("Elementos procesados"), render: (j: any) => j.elementsProcessed },
        { key: "c", label: t("Inicio"), render: (j: any) => when(j.createTime) },
      ]}
      create={{
        label: t("Crear trabajo a partir de una plantilla"),
        form: {
          title: t("Crear trabajo a partir de una plantilla"),
          submit: t("Ejecutar trabajo"),
          note: t("Ejecutar trabajo de Dataflow"),
          initial: { name: "", region: ctx.region, tpl: "Word_Count", input: "gs://dataflow-samples/shakespeare/kinglear.txt", output: "", sub: "", topic: "", table: "", dir: "" },
          fields: [
            { id: "name", label: t("Nombre del trabajo"), required: true },
            { id: "region", label: t("Región"), type: "select", options: regions(ctx.region) },
            { id: "tpl", label: t("Plantilla"), type: "select", options: TEMPLATES },
            { id: "input", label: t("Archivo de entrada"), required: true, when: (v) => v.tpl === "Word_Count" },
            { id: "output", label: t("Prefijo de salida (gs://...)"), required: true, when: (v) => v.tpl === "Word_Count" },
            { id: "sub", label: t("Suscripción de entrada"), required: true, when: (v) => v.tpl === "PubSub_Subscription_to_BigQuery" },
            { id: "topic", label: t("Tema de entrada"), required: true, when: (v) => v.tpl === "PubSub_to_BigQuery" || v.tpl === "Cloud_PubSub_to_GCS_Text" },
            { id: "table", label: t("Tabla de BigQuery (proyecto:dataset.tabla)"), required: true, when: (v) => v.tpl.endsWith("BigQuery") },
            { id: "dir", label: t("Directorio de salida (gs://...)"), required: true, when: (v) => v.tpl === "Cloud_PubSub_to_GCS_Text" },
          ],
          build: (v) => {
            const p: Record<string, string> =
              v.tpl === "Word_Count" ? { inputFile: v.input.trim(), output: v.output.trim() }
              : v.tpl === "PubSub_Subscription_to_BigQuery" ? { inputSubscription: `projects/${ctx.project}/subscriptions/${v.sub.trim()}`, outputTableSpec: v.table.trim() }
              : v.tpl === "PubSub_to_BigQuery" ? { inputTopic: `projects/${ctx.project}/topics/${v.topic.trim()}`, outputTableSpec: v.table.trim() }
              : { inputTopic: `projects/${ctx.project}/topics/${v.topic.trim()}`, outputDirectory: v.dir.trim() };
            return `gcloud dataflow jobs run ${q(v.name.trim())} --gcs-location=gs://dataflow-templates-${v.region}/latest/${v.tpl} --region=${v.region} --parameters=${Object.entries(p).map(([a, b]) => `${a}=${b}`).join(",")}`;
          },
        },
      }}
      extra={(h, sel) => (
        <>
          <Tool icon="stop" label={t("Cancelar")} disabled={!sel.length} onClick={() => ctx.runAll(sel.map((j: any) => `gcloud dataflow jobs cancel ${j.id} --region=${j.region}`), t("Cancelar trabajo"))} />
          <Tool icon="download" label={t("Drenar")} disabled={!sel.length} onClick={() => ctx.runAll(sel.map((j: any) => `gcloud dataflow jobs drain ${j.id} --region=${j.region}`), t("Drenar trabajo"))} />
        </>
      )}
    />
  );
}

export function Dataproc({ ctx }: { ctx: Ctx }) {
  const { t } = useI18n();
  const jobs = [...(ctx.data.dataprocJobs ?? [])].reverse();
  return (
    <>
      <List
        ctx={ctx}
        title={t("Clústeres de Dataproc")}
        rows={vals(ctx.data.dataprocClusters)}
        rowKey={(c: any) => c.clusterName}
        empty={t("No hay clústeres.")}
        cols={[
          { key: "name", label: t("Nombre"), render: (c: any) => c.clusterName },
          { key: "st", label: t("Estado"), render: (c: any) => <Status state={c.state} /> },
          { key: "reg", label: t("Región"), render: (c: any) => c.region },
          { key: "w", label: t("Nodos de trabajo"), render: (c: any) => (c.singleNode ? t("Un solo nodo") : `${c.numWorkers} + ${c.numSecondaryWorkers}`) },
          { key: "img", label: t("Imagen"), render: (c: any) => c.imageVersion },
        ]}
        create={{ label: t("Crear clúster"), form: { title: t("Crear un clúster de Dataproc"), submit: t("Crear"), note: t("Crear clúster de Dataproc"), initial: { name: "", region: ctx.region, mode: "standard", workers: "2", type: "n2-standard-4" }, fields: [{ id: "name", label: t("Nombre"), required: true }, { id: "region", label: t("Región"), type: "select", options: regions(ctx.region) }, { id: "mode", label: t("Tipo de clúster"), type: "select", options: [["standard", t("Estándar (1 maestro, N trabajadores)")], ["single", t("Un solo nodo")]] }, { id: "workers", label: t("Nodos de trabajo"), type: "number", when: (v) => v.mode === "standard" }, { id: "type", label: t("Tipo de máquina"), type: "select", options: ["n2-standard-2", "n2-standard-4", "e2-standard-4"] }], build: (v) => `gcloud dataproc clusters create ${q(v.name.trim())} --region=${v.region} ${v.mode === "single" ? "--single-node" : `--num-workers=${v.workers} --worker-machine-type=${v.type}`} --master-machine-type=${v.type}` } }}
        extra={(h, sel) => (
          <>
            <Tool icon="play" label={t("Enviar trabajo")} disabled={sel.length !== 1} onClick={() => h.form({ title: t("Enviar un trabajo"), submit: t("Enviar"), note: t("Enviar trabajo de Dataproc"), kind: "drawer", initial: { type: "spark", py: "", args: "1000" }, fields: [{ id: "type", label: t("Tipo de trabajo"), type: "select", options: [["spark", "Spark (SparkPi)"], ["pyspark", "PySpark"]] }, { id: "py", label: t("Archivo .py (gs://...)"), required: true, when: (v) => v.type === "pyspark" }, { id: "args", label: t("Argumentos") }], build: (v) => (v.type === "spark" ? `gcloud dataproc jobs submit spark --cluster=${sel[0].clusterName} --region=${sel[0].region} --class=org.apache.spark.examples.SparkPi --jars=file:///usr/lib/spark/examples/jars/spark-examples.jar -- ${v.args}` : `gcloud dataproc jobs submit pyspark ${v.py.trim()} --cluster=${sel[0].clusterName} --region=${sel[0].region}${v.args.trim() ? ` -- ${v.args.trim()}` : ""}`) })} />
            <Tool icon="stop" label={t("Detener")} disabled={!sel.length} onClick={() => ctx.runAll(sel.map((c: any) => `gcloud dataproc clusters stop ${c.clusterName} --region=${c.region}`), t("Detener clúster"))} />
            <Tool icon="power" label={t("Iniciar")} disabled={!sel.length} onClick={() => ctx.runAll(sel.map((c: any) => `gcloud dataproc clusters start ${c.clusterName} --region=${c.region}`), t("Iniciar clúster"))} />
          </>
        )}
        del={{ cmd: (c: any) => `gcloud dataproc clusters delete ${c.clusterName} --region=${c.region} --quiet`, title: t("¿Eliminar los clústeres?"), text: t("Se borrarán sus VMs y discos."), note: t("Eliminar clúster") }}
      />
      <List
        ctx={ctx}
        title={t("Trabajos")}
        rows={jobs}
        rowKey={(j: any) => j.jobId}
        empty={t("No hay trabajos.")}
        cols={[
          { key: "id", label: "ID", render: (j: any) => <code>{String(j.jobId).slice(0, 12)}</code> },
          { key: "type", label: t("Tipo"), render: (j: any) => j.type },
          { key: "cl", label: t("Clúster"), render: (j: any) => j.clusterName },
          { key: "st", label: t("Estado"), render: (j: any) => pill(j.state) },
          { key: "out", label: t("Salida"), render: (j: any) => <code>{String(j.driverOutput ?? "").split("\n")[0]}</code> },
        ]}
      />
    </>
  );
}

export function Composer({ ctx }: { ctx: Ctx }) {
  const { t } = useI18n();
  return (
    <List
      ctx={ctx}
      title={t("Entornos de Composer")}
      intro={t("Apache Airflow gestionado. Sube DAGs a la carpeta dags/ del bucket del entorno.")}
      rows={vals(ctx.data.composer)}
      rowKey={(e: any) => e.name}
      empty={t("No hay entornos.")}
      cols={[
        { key: "name", label: t("Nombre"), render: (e: any) => e.name },
        { key: "st", label: t("Estado"), render: (e: any) => <Status state={e.state} /> },
        { key: "loc", label: t("Ubicación"), render: (e: any) => e.location },
        { key: "v", label: t("Versión"), render: (e: any) => e.imageVersion },
        { key: "b", label: t("Carpeta de DAGs"), render: (e: any) => <code>{e.dagGcsPrefix}</code> },
        { key: "r", label: t("Ejecuciones"), render: (e: any) => (e.dagRuns ?? []).length },
      ]}
      create={{ label: t("Crear entorno"), form: { title: t("Crear entorno de Composer"), submit: t("Crear"), note: t("Crear entorno de Composer"), initial: { name: "", loc: ctx.region, size: "small" }, fields: [{ id: "name", label: t("Nombre"), required: true }, { id: "loc", label: t("Ubicación"), type: "select", options: regions(ctx.region) }, { id: "size", label: t("Tamaño"), type: "select", options: ["small", "medium", "large"] }], build: (v) => `gcloud composer environments create ${q(v.name.trim())} --location=${v.loc} --environment-size=${v.size}` } }}
      extra={(h, sel) => (
        <>
          <Tool icon="book" label={t("Listar DAGs")} disabled={sel.length !== 1} onClick={() => h.show(t("DAGs"), `gcloud composer environments run ${sel[0].name} --location=${sel[0].location} dags list`, t("Listar DAGs"))} />
          <Tool icon="play" label={t("Activar DAG")} disabled={sel.length !== 1} onClick={() => h.form({ title: t("Activar DAG"), submit: t("Activar"), note: t("Activar DAG"), kind: "drawer", initial: { dag: "" }, fields: [{ id: "dag", label: t("ID del DAG"), required: true }], build: (v) => `gcloud composer environments run ${sel[0].name} --location=${sel[0].location} dags trigger -- ${q(v.dag.trim())}` })} />
        </>
      )}
      del={{ cmd: (e: any) => `gcloud composer environments delete ${e.name} --location=${e.location} --quiet`, title: t("¿Eliminar los entornos?"), text: t("El bucket del entorno se conserva."), note: t("Eliminar entorno") }}
    />
  );
}
