# Cloud Mastery — GCP Lab Simulator

**Español** · [English](README.en.md)

Aprende Google Cloud haciendo el trabajo: **Aprende → Construye → Rompe → Diagnostica → Domina**.

El alumnado trabaja en un Google Cloud simulado y determinista a través de una
terminal realista (`gcloud`, `gsutil`, `bq`, `kubectl`, `terraform`,
herramientas de Linux) y de una consola que lee el mismo estado. Atiende
tickets de personas que no conocen la causa raíz, responde a incidentes
generados, gestiona una empresa persistente y da el salto a emuladores y a
proyectos reales en sandbox. La evaluación mira tanto el resultado como el
camino: diagnóstico antes del cambio, seguridad, coste, riesgo y comunicación.

## Qué incluye

| Área | Dónde | Lo más destacado |
|---|---|---|
| Simulador (F0) | `internal/sim`, `internal/cli` | IAM con condiciones y políticas de organización, VPC/cortafuegos/NAT/balanceadores, Compute y MIG, Cloud Run, Cloud SQL, GCS, Pub/Sub, BigQuery, GKE/`kubectl`, Terraform, registros y métricas, facturación y un modelo del sistema operativo de las VM; los caminos de las peticiones registran cadenas causales (`why`) |
| Escenarios | `internal/scenario`, `content/labs` | Laboratorios declarativos en YAML con 45 tipos de fallo, variantes por semilla, pistas, validadores de rúbrica y políticas OPA/Rego |
| Incidentes | `content/failures`, `internal/scenario/generator.go` | Biblioteca de fallos (sistemas, modos de fallo, síntomas y contextos de negocio) y un generador con tickets vagos o engañosos, fallos acumulados, salas de crisis P1 y modo «problema desconocido» |
| Mesa de servicio | `internal/desk` | Tickets INC/REQ/CHG/PRB/SEC/COST/MIG, SLA y personas simuladas que responden a las preguntas de forma determinista |
| Empresa | `internal/company`, `content/company` | Nebula Corporation: seis proyectos que persisten entre misiones; los riesgos latentes vuelven como incidentes |
| Evaluación | `internal/grader` | Rúbrica (100 puntos) más un informe del proceso: resolución, diagnóstico, seguridad, coste, eficiencia, riesgo, comunicación, documentación y autonomía |
| Aprendizaje | `internal/learning` | Grafo de habilidades con prerrequisitos (82 habilidades), modelo del alumno en 9 dimensiones, curva del olvido, escalera de autonomía, plan adaptativo, XP/niveles/insignias/ligas, etapas profesionales y expedientes firmados |
| Modos avanzados | `internal/archsim`, `internal/cli` | Simulador de arquitectura, experimentos de caos, facturación y recomendador de FinOps, entrevistas, «explícame por qué» (`why`) y «¿qué pasa si…?» (`whatif`) |
| Fidelidad | `internal/fidelity` | F0 simulador, F1 emuladores (Pub/Sub, GCS, kind), F2 proyectos reales de un pool protegido con limpieza automática |
| Planos | `internal/api`, `internal/orchestrator` | Plano de aprendizaje (usuarios, progreso) separado del plano de laboratorios (sesiones) y un evaluador aislado |
| Web | `web/` | Exportación estática de Next.js servida por el binario de Go: panel, catálogo, espacio de trabajo del laboratorio con una consola gráfica al estilo de la de Google Cloud y Cloud Shell acoplado debajo (cada clic ejecuta y muestra el comando `gcloud` equivalente), xterm, Monaco, Mermaid, incidentes, empresa, grafo de habilidades, ligas y consola del instructor; WCAG 2 AA (auditado con axe, modo claro y oscuro) y modo lector de pantalla en la terminal |

Contenido actual: 54 laboratorios en 14 rutas (un programa de 30 días de
Associate Cloud Engineer, fundamentos, Linux, GKE, seguridad, redes, datos, ML,
arquitectura, DevOps, SRE, FinOps, proyectos finales y carrera), 13 misiones de
empresa con sus incidentes por consecuencias, 20 modos de fallo en 3 sistemas
(tres capas, tienda serverless y tienda en GKE), 16 insignias, 6 etapas
profesionales y 10 especializaciones. Los laboratorios declaran las
correcciones erróneas tentadoras («atajos») y CI demuestra que fallan.

## Idiomas

El español es el idioma principal y el inglés una traducción
([ADR 0009](docs/adr/0009-spanish-first-i18n.md)). Todo lo que dice la
plataforma se escribe primero en español: la web, los mensajes del servidor,
los comandos propios de la terminal (`help`, `ticket`, `why`, `whatif`,
`chaos`, `interview`, `arch`), los laboratorios, las misiones, la biblioteca de
fallos y el temario. El inglés vive en los bloques `en:` del contenido, en
`i18n.P(lang, "es", "en")` en el código y en `web/lib/en.ts` en la web. La
salida de las herramientas reales (`gcloud`, `kubectl`, `terraform`…) sigue en
inglés, como en el trabajo.

El alumnado cambia de idioma con los botones ES/EN y la elección se guarda en
su cuenta. Una sesión de laboratorio conserva el idioma con el que empezó y se
aceptan respuestas en cualquiera de los dos idiomas (las palabras clave se
comparan con un glosario bilingüe). CI falla si falta una traducción
(`labctl validate`, `npm run check:i18n`).

## Puesta en marcha rápida

Requisitos: Go 1.25 y Node 22 (solo para construir la web).

```sh
# cliente web (archivos estáticos en web/out)
(cd web && npm ci && npm run build)

# servidor: API + plano de laboratorios + evaluador en un proceso, almacén en un archivo JSON
go run ./cmd/gcplab
# → http://localhost:8080  (crea una cuenta y abre el catálogo)
```

Juega un laboratorio directamente en tu terminal:

```sh
go run ./cmd/labctl play -lab ace-d01-context
go run ./cmd/labctl generate -system shop-platform -difficulty 4 -mode production -play
```

En la web, el laboratorio se abre en la **consola**: una réplica de la
consola de Google Cloud (menú de productos, selector de proyecto, buscador,
listas con filtro, formularios de creación con su «Código equivalente») con
Cloud Shell debajo. Cada botón escribe y ejecuta en Cloud Shell el comando
`gcloud`, `gsutil` o `bq` equivalente, así que aprendes la línea de comandos
mientras haces clic, y la evaluación ve igual lo que haces en la consola y en
la terminal.

Dentro de un laboratorio, escribe `help`. Comandos útiles: `why <recurso>`
(cadena causal de una petición), `whatif <comando>` (vista previa del
impacto), `ticket show`, `team` y `ask <persona> "pregunta"`.

### Entorno local completo

```sh
docker compose -f deploy/docker-compose.yml up --build
```

Ejecuta la API, el plano de laboratorios y el evaluador como servicios
separados, con PostgreSQL y los emuladores de Pub/Sub y GCS (F1).

## Configuración

| Variable | Por defecto | Para qué sirve |
|---|---|---|
| `ADDR` | `:8080` | dirección de escucha |
| `MODE` | `all` | `all`, `api` (plano de aprendizaje) o `labplane` |
| `CONTENT_DIR` | `content` | laboratorios, habilidades, rutas, fallos y empresa |
| `WEB_DIR` | `web/out` | cliente web estático |
| `DATA_FILE` | `data/gcplab.json` | almacén JSON cuando no hay `DATABASE_URL` |
| `DATABASE_URL` | — | DSN de PostgreSQL |
| `JWT_SECRET` | aleatorio en cada arranque | firma de tokens y de expedientes |
| `LABPLANE_URL`, `LABPLANE_TOKEN` | — | API → plano de laboratorios (modo separado) |
| `GRADER_URL` | — | evaluador aislado (`cmd/grader-worker`) |
| `PUBSUB_EMULATOR_HOST`, `STORAGE_EMULATOR_HOST`, `KIND_KUBECONFIG` | — | activan F1 |
| `F2_PROJECTS` | — | ids de proyectos sandbox separados por comas (activa F2) |
| `F2_DRIVER` | `sim` | `gcloud` para manejar proyectos reales |
| `F2_ADMIN_SA`, `F2_LAB_SA` | — | identidades del administrador del pool y del alumnado (suplantadas) |
| `F2_DRY_RUN` | — | `1` registra los comandos de gcloud en lugar de ejecutarlos |
| `F2_MONTHLY` | `10` | sesiones en cloud real por persona y mes |
| `OIDC_ISSUER`, `OIDC_CLIENT_ID`, `OIDC_CLIENT_SECRET`, `OIDC_REDIRECT_URL` | — | inicio de sesión con Google Workspace / OIDC |
| `MENTOR_LLM` | `off` | `on` activa el revisor de post-mortems con Claude (necesita `ANTHROPIC_API_KEY`); responde en el idioma de la sesión |
| `CORS_ORIGIN` | — | permite un cliente web alojado aparte |

## Instalarlo en Windows

Un instalador MSI con todo incluido (simulador, web y laboratorios), sin
conexión a la nube ni dependencias: descárgalo de *Releases* o de *Actions →
windows-installer*. Guía: [docs/instalador-windows.md](docs/instalador-windows.md).

## Publicarlo online para practicar

Un único servicio con la imagen Docker del repositorio y la base de datos en
Supabase. Elige dónde:

| Dónde | Cómo | Guía |
|---|---|---|
| **Google Cloud Run** (recomendado para preparar la ACE) | Script para Cloud Shell o PowerShell (`deploy/cloudrun/`) | [docs/despliegue-cloud-run.md](docs/despliegue-cloud-run.md) |
| **Render** | Blueprint [`render.yaml`](render.yaml) desde el panel | [docs/despliegue-render-supabase.md](docs/despliegue-render-supabase.md) |
| **Koyeb** | Servicio Docker desde GitHub en el panel | [docs/despliegue-koyeb.md](docs/despliegue-koyeb.md) |

## Despliegue completo en Google Cloud (organización y pool de sandboxes)

`deploy/terraform` crea la carpeta de sandbox con barreras de políticas de
organización (ubicaciones permitidas, sin claves de cuentas de servicio,
prevención de acceso público, sin IP externas en las VM, uso compartido
restringido al dominio), los proyectos del pool con presupuestos y la
plataforma: servicios de Cloud Run para la API (pública), el plano de
laboratorios y el evaluador (solo internos), Cloud SQL, Secret Manager y
Artifact Registry. Consulta [docs/operations.md](docs/operations.md).

```sh
cd deploy/terraform
cp terraform.tfvars.example terraform.tfvars   # edítalo
terraform init && terraform apply
gcloud builds submit --config deploy/cloudbuild.yaml ..
```

## Crear contenido

Los laboratorios son contenido como código y se escriben en español con su
bloque `en:`. Cada laboratorio debe pasar de suspenso a aprobado al aplicar su
solución de referencia, con varias semillas:

```sh
go run ./cmd/labctl validate          # esquema, referencias y traducciones al inglés
go run ./cmd/labctl i18n              # solo lo que falta por traducir
go run ./cmd/labctl test              # todos los laboratorios, varias semillas
go run ./cmd/labctl test -lab ace-d05-subnet-isolation -seeds 1,7,42
go run ./cmd/labctl variants -lab ace-d05-subnet-isolation -n 3
```

Consulta la [guía de autoría](docs/authoring.md) para el DSL de los
laboratorios, la biblioteca de fallos, las misiones de empresa y las
traducciones.

## Desarrollo

```sh
go vet ./... && go test -race ./...
go run ./cmd/labctl validate && go run ./cmd/labctl test
(cd web && npm run build)             # incluye npm run check:i18n
```

CI (`.github/workflows/ci.yml`) ejecuta las mismas comprobaciones más
`terraform validate` y la construcción de la imagen.

## Documentación

- [Blueprint maestro de implementación](docs/blueprint/01-master-implementation-blueprint.md): arquitectura, contratos, esquema, DSL, fases y registro de cambios
- [Auditoría del repositorio](docs/blueprint/00-repository-audit.md)
- [Registros de decisiones de arquitectura (ADR)](docs/adr/)
- [Matriz de fidelidad](docs/fidelity-matrix.md)
- [Guía de autoría](docs/authoring.md) ([English](docs/authoring.en.md))
- [Operaciones](docs/operations.md)
- [Registro de riesgos](docs/risk-register.md)
- [Hoja de ruta](docs/roadmap.md)
