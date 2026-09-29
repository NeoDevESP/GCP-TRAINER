# Guía de autoría

**Español** · [English](authoring.en.md)

El contenido vive en `content/` y es código: se valida y se prueba en CI, y un
laboratorio solo se acepta cuando su solución de referencia lo lleva de
suspenso a aprobado con varias semillas. Se escribe en español, el idioma
principal de la plataforma, con su traducción al inglés en un bloque `en:`
(ver [Traducciones](#traducciones-primero-español-después-inglés)).

```
content/
  skills.yaml          grafo de habilidades: ramas, bloques, habilidades y prerrequisitos
  tracks.yaml          rutas de aprendizaje (laboratorios ordenados)
  career.yaml          etapas profesionales, especializaciones y proyectos finales
  badges.yaml, certs.yaml
  labs/<ruta>/<lab>/lab.yaml   (+ solution.sh opcional)
  baselines/*.sh       entornos reutilizables (shop-platform, three-tier-app, ...)
  policies/*.rego      políticas OPA que usan las comprobaciones `policy`
  failures/            biblioteca de fallos para los incidentes generados
  company/             Nebula Corporation: nebula.yaml y missions/<id>/lab.yaml
```

## Anatomía de un laboratorio

```yaml
id: ace-d03-secure-bucket          # único y estable
title: "Día 3 · Un bucket de documentos seguro"
track: ace-30
day: 3
level: basic                        # basic | intermediate | professional | expert
branch: storage                     # rama de habilidades a la que pertenece
type: challenge                     # guided | challenge | incident | boss | capstone | interview ...
mode: lab                           # learn | lab | production | architecture | career | unknown
skills: [storage.buckets, storage.lifecycle]
difficulty: 2                       # 1-5
minutes: 45
params:                             # una semilla elige un valor por clave → variantes
  region: [europe-west1, europe-west3]
baseline: [default-network]         # scripts de content/baselines
setup: |                            # comandos extra que ejecuta el administrador de la plataforma
  gcloud storage buckets create gs://tmp-{{.project}} --location={{.region}}
faults:                             # rotura declarativa (45 tipos, ver labctl validate)
  - {type: firewall_delete, rule: allow-health}
story: |                            # Markdown que ve el alumnado; plantillas {{.param}}
objectives: [...]
constraints: [no-downtime, no-public, "protect:billing-batch"]
hints:                              # progresivas; cada una cuesta puntos
  - text: "..."
solution: |                         # solución de referencia que usa CI (nunca se muestra)
rubric:                             # 100 puntos
  - name: Seguridad
    validator: security             # functional | state | security | diagnosis | cost | reliability | evidence | quiz
    points: 30
    critical: true                  # si falla, el laboratorio no se aprueba
    checks:
      - {type: exists, path: 'buckets["docs-{{.project}}"]', expect: {publicAccessPrevention: enforced}, desc: "PAP aplicada"}
      - {type: policy, package: gcplab.security, ids: [PUBLIC_BUCKET], desc: "Sin permisos públicos"}
en:                                 # traducción al inglés (obligatoria)
  title: "Day 3 · A secure documents bucket"
  ...
```

`desc` es lo que ve el alumnado en *Comprobar* cuando una comprobación falla,
así que escríbelo como el requisito, nunca como la respuesta.

### Atajos: demuestra que las correcciones erróneas fallan

Una buena rúbrica premia el cambio correcto, no cualquier cambio que haga
desaparecer el síntoma. Declara las correcciones erróneas tentadoras y CI
aplicará cada una a un entorno nuevo y romperá la build si el laboratorio
sigue aprobándose:

```yaml
shortcuts:
  - name: dar acceso al bucket a la identidad de los nodos
    run: |
      gcloud storage buckets add-iam-policy-binding gs://reports-{{.project}} \
        --member=serviceAccount:gke-nodes@{{.project}}.iam.gserviceaccount.com --role=roles/storage.objectViewer
```

Todos los pasos deben funcionar (prefija con `!` un paso que se espera que
falle), así que un atajo no puede aprobar por accidente. Atajos típicos:
desactivar el control (borrar la NetworkPolicy, quitar la sonda), conceder de
más (editor, roles a nivel de proyecto, `0.0.0.0/0`), destruir evidencias,
restaurar sobre datos vivos o desplegar directamente al 100 %. Si un atajo
aprueba, corrige la rúbrica (normalmente haciendo `critical` el criterio que
lo protege).

### Comprobaciones

| Familia | Tipos |
|---|---|
| Estado | `exists`, `absent`, `count` (con los operadores de `expect` `eq`, `ne`, `gt`, `gte`, `lt`, `lte`, `contains`, `in`, `regex`, `len`, `empty`) |
| Funcionales | `http`, `tcp`, `egress`, `google_api`, `pubsub`, `sql_healthy`, `rollout`, `hpa_scaled`, `build`, `chaos` |
| Identidad | `iam` (permiso efectivo, con condiciones y políticas de organización), `no_basic_roles`, `org_policy`, `k8s_rbac` |
| Seguridad y coste | `policy` (Rego), `forbid_firewall`, `finding_absent`/`finding_present`, `cost_max`/`cost_min`, `bq_bytes_max`, `secret_rotated` |
| Operaciones | `log_metric`, `alert_policy`, `log_contains`, `terraform_clean`, `tf_state`, `vm_file`, `vm_disk`, `subnet_plan`, `design` |
| Proceso y comunicación | `command` (lo que ejecutó el alumno; `min`, o `max` para prohibir un comando), `session_config`, `file_contains`, `ticket_update`, `ticket_resolved`, `asked`, `evidence` (palabras clave en los campos del post-mortem), `quiz` |

Cualquier comprobación puede apuntar a otro proyecto con `on: <clave-de-proyecto>` (misiones de empresa).

### Incidentes: tickets y personas

```yaml
ticket:
  id: INC-2041
  kind: INC                 # INC | REQ | CHG | PRB | SEC | COST | MIG
  priority: P2
  sla: 4h
  summary: "El checkout va lento"   # lo que cree quien lo abre, no la causa raíz
  reporter: Soporte de la tienda
actors:
  - role: developer
    name: Leo (backend)
    fallback: "El código no ha cambiado desde el viernes."
    facts:                  # todos los grupos de palabras clave deben coincidir con la pregunta
      - {keywords: [[deploy, despliegue, release], [today, hoy, yesterday, ayer]], answer: "No hay despliegues desde el viernes.", evidence: deploy.log, content: "..."}
```

El alumnado usa `ticket show|comment|update|resolve|escalate`, `team` y
`ask <quién> "pregunta"`. La evaluación del proceso premia preguntar antes de
cambiar y penaliza los reinicios a ciegas, los permisos arriesgados y el
incumplimiento de las restricciones.

## Biblioteca de fallos

`content/failures/systems/<sistema>.yaml` describe una arquitectura sana
(`baseline`, `setup`, `topology`, `traffic`, comprobaciones `healthy` y
`safety`, `actors`, `noise` y recursos señuelo) y sus `failures`. Cada modo de
fallo tiene un `symptom` (de `symptoms.yaml`), la `layer` donde vive la causa,
`faults`, `facts` extra para las personas, `solution`, `checks` y `variants`
opcionales. `contexts.yaml` contiene los contextos de negocio (empresa,
servicio, impacto, prioridad).

El generador combina sistema + fallos + contexto + dificultad + modo en un
laboratorio normal: más dificultad añade ruido, tickets vagos o engañosos y
fallos acumulados; el modo `production` lo convierte en un P1 con cronología de
alertas y post-mortem obligatorio; el modo `unknown` solo indica el impacto en
el negocio. Cada incidente se genera en español y en inglés con la misma
semilla, y las dos versiones se emparejan para formar su bloque `en:`.

```sh
go run ./cmd/labctl failures                                   # grafo de síntomas
go run ./cmd/labctl generate -system three-tier -difficulty 5 -mode production
```

CI genera un incidente por modo de fallo, además de combinaciones acumuladas y
en modo producción, y verifica que todos se pueden resolver.

## Misiones de empresa

Las misiones viven en `content/company/missions/<id>/lab.yaml` y llevan un
bloque `company`:

```yaml
company: {stage: junior, project: prod-web, kind: REQ, after: [nb-m01-orientation]}
```

Se ejecutan sobre el mundo de Nebula que conserva cada persona, así que los ids
de proyecto son plantillas (`{{.prod_web}}`, `{{.prod_data}}`, ...). Las
consecuencias de `company/nebula.yaml` convierten los riesgos que se dejan
atrás (SSH abierto, datos públicos, sin copias de seguridad, sin presupuesto)
en misiones de incidente unos días después; mitigar el riesgo antes las
cancela. Las personas de la empresa (`actors`) se traducen con `en.text` en
`nebula.yaml` y se incorporan a todas las misiones.

## Traducciones: primero español, después inglés

El contenido se escribe en español, el idioma principal de la plataforma
([ADR 0009](adr/0009-spanish-first-i18n.md)). Cada laboratorio, misión y
elemento del temario lleva un bloque `en:` con su traducción al inglés:

```yaml
title: "Día 3 · Un bucket de documentos seguro con ciclo de vida"
story: |
  El departamento legal necesita un bucket **docs-{{.project}}** ...
objectives: [...]
hints:
  - text: "`gcloud storage buckets create` admite ..."
rubric:
  - name: Seguridad
    checks:
      - {type: policy, ..., desc: "Sin permisos públicos"}
en:
  title: "Day 3 · A secure documents bucket with lifecycle"
  story: |
    Legal needs a bucket **docs-{{.project}}** ...
  objectives: [...]            # mismo orden y longitud que la lista en español
  hints: [...]
  evidencePrompt: "..."
  ticket: {summary: "...", impact: "..."}
  timeline: [...]              # el texto de cada evento, en orden
  text:                        # todo lo demás, con el texto exacto en español como clave
    Seguridad: Security
    Sin permisos públicos: No public bindings
```

- `en.text` traduce los nombres de la rúbrica, las descripciones de las
  comprobaciones, las preguntas y opciones del test, las personas (nombre,
  perfil, respuesta por defecto y respuestas), quien abre el ticket y sus
  comentarios, y los remitentes de la cronología. Los textos iguales en los dos
  idiomas (comandos, `ALLOW`) también llevan su entrada.
- Los archivos del espacio de trabajo, los adjuntos de los tickets y los
  archivos que comparten las personas suelen ser código o salida de
  herramientas: traducirlos es opcional.
- Se puede responder en cualquiera de los dos idiomas. Los grupos de palabras
  clave (`evidence`, `keywords` en las comprobaciones de test y de la mesa de
  servicio, `facts` de las personas) se comparan con un glosario bilingüe,
  pero añade igualmente variantes en español a cada grupo
  (`[[firewall, cortafuegos], [priority, prioridad]]`).
- Las respuestas de referencia (`evidence.sample`) están en español, así que CI
  demuestra que se aprueba respondiendo en español.
- Los elementos del temario (`skills.yaml`, `tracks.yaml`, `badges.yaml`,
  `certs.yaml`, `career.yaml`) usan `en: {name, title, description,
  observable, scenario, guide}` según corresponda; los de la biblioteca de
  fallos usan `en:` con sus propios campos más `text` para personas, hechos y
  descripciones de comprobaciones.
- La salida de las herramientas reales (`gcloud`, `kubectl`, `terraform`) no
  se traduce: sigue en inglés, como en el trabajo real.
- `go run ./cmd/labctl i18n` lista lo que falta; `labctl validate` (CI) falla
  si falta algo.

La interfaz web sigue la misma regla: los textos se escriben en español dentro
de `t("…")` y `web/lib/en.ts` guarda su traducción; `npm run check:i18n` (que
se ejecuta antes de cada build) falla si falta alguna.

## Lista de comprobación antes de abrir una PR

1. `go run ./cmd/labctl validate` (incluye las traducciones al inglés)
2. `go run ./cmd/labctl test -lab <id> -seeds 1,7,42`: antes < nota de aprobado ≤ después
3. Declara como `shortcuts` las correcciones erróneas obvias (CI demuestra que
   fallan) y usa `go run ./cmd/labctl play -lab <id>` para probar alguna más a mano
4. Las pistas van de un empujón a casi la respuesta; la historia nunca nombra
   la causa raíz
