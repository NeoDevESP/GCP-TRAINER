# Cloud Mastery — GCP Lab Simulator

[Español](README.md) · **English**

Learn Google Cloud by doing the job: **Learn → Build → Break → Diagnose → Master**.

Learners work in a deterministic simulated Google Cloud through a real-feeling
terminal (`gcloud`, `gsutil`, `bq`, `kubectl`, `terraform`, Linux tools) and a
console that reads the same state. They answer tickets from people who don't
know the root cause, respond to generated incidents, run a persistent company,
and graduate to emulators and real sandbox projects. Grading looks at both the
outcome and how they got there: diagnosis before change, security, cost, risk
and communication.

## What's inside

| Area | Where | Highlights |
|---|---|---|
| Simulator (F0) | `internal/sim`, `internal/cli` | IAM with conditions and org policies, VPC/firewalls/NAT/LB, Compute and MIGs, Cloud Run, Cloud SQL, GCS, Pub/Sub, BigQuery, GKE/`kubectl`, Terraform, logging/metrics, billing, a guest-OS model for VMs; request paths record causal chains (`why`) |
| Scenarios | `internal/scenario`, `content/labs` | Declarative YAML labs with 45 fault types, templated variants per seed, hints, rubric validators, OPA/Rego policies |
| Incidents | `content/failures`, `internal/scenario/generator.go` | Failure library (systems, failure modes, symptoms, business contexts) and a generator with vague/misleading tickets, stacked failures, P1 war rooms and "unknown problem" mode |
| Service desk | `internal/desk` | INC/REQ/CHG/PRB/SEC/COST/MIG tickets, SLAs and simulated people who answer questions deterministically |
| Company | `internal/company`, `content/company` | Nebula Corporation: six projects that persist between missions; latent risks come back as incidents |
| Assessment | `internal/grader` | Rubric (100 pts) plus a process report: resolution, diagnosis, security, cost, efficiency, risk, communication, documentation, autonomy |
| Learning | `internal/learning` | Skill graph with prerequisites (82 skills), 9-dimension student model, forgetting curve, autonomy ladder, adaptive plan, XP/levels/badges/leagues, career stages and signed transcripts |
| Advanced modes | `internal/archsim`, `internal/cli` | Architecture simulator, chaos experiments, FinOps billing and recommender, interviews, "teach me why" and "what if" |
| Fidelity | `internal/fidelity` | F0 simulator, F1 emulators (Pub/Sub, GCS, kind), F2 real projects from a guarded sandbox pool with a janitor |
| Planes | `internal/api`, `internal/orchestrator` | Learning plane (users, progress) separated from the lab plane (sessions) and an isolated grader worker |
| Web | `web/` | Next.js static export served by the Go binary: dashboard, catalog, lab workspace with a graphical console modelled on the Google Cloud console and Cloud Shell docked below (every click runs and shows the equivalent `gcloud` command), xterm, Monaco, Mermaid, incidents, company, skill graph, leagues, instructor console; WCAG 2 AA (axe-audited, light and dark), terminal screen-reader mode |

Content today: 54 labs in 14 tracks (a 30-day Associate Cloud Engineer
programme, foundations, Linux, GKE, security, network, data, ML, architect,
DevOps, SRE, FinOps, capstones and career), 13 company missions and consequence
incidents, 20 failure modes in 3 systems (three-tier, serverless shop, GKE
shop), 16 badges, 6 career stages and 10 specialisations. Labs declare the
tempting wrong fixes ("shortcuts") and CI proves they fail.

## Languages

Spanish is the primary language and English is a translation
([ADR 0009](docs/adr/0009-spanish-first-i18n.md)). Everything the platform
says is written in Spanish first: the web client, server messages, the
terminal's own commands (`help`, `ticket`, `why`, `whatif`, `chaos`,
`interview`, `arch`), labs, missions, the failure library and the curriculum.
English lives in `en:` blocks in the content, in `i18n.P(lang, "es", "en")`
in the code and in `web/lib/en.ts` for the web client. Output of real tools
(`gcloud`, `kubectl`, `terraform`…) stays in English, as at work.

Learners switch language with the ES/EN buttons; the choice is saved on their
account. A lab session keeps the language it started with, and answers are
accepted in either language (keywords are matched through a bilingual
glossary). CI fails when a translation is missing (`labctl validate`,
`npm run check:i18n`).

## Quick start

Requirements: Go 1.25 and Node 22 (only to build the web client).

```sh
# web client (static files in web/out)
(cd web && npm ci && npm run build)

# server: API + lab plane + grader in one process, JSON file store
go run ./cmd/gcplab
# → http://localhost:8080  (create an account, open the catalog)
```

Play a lab directly in your terminal:

```sh
go run ./cmd/labctl play -lab ace-d01-context
go run ./cmd/labctl generate -system shop-platform -difficulty 4 -mode production -play
```

On the web, a lab opens in the **console**: a replica of the Google Cloud
console (product menu, project picker, search, filterable lists, creation
forms with their "Equivalent code") with Cloud Shell below. Every button types
and runs the equivalent `gcloud`, `gsutil` or `bq` command in Cloud Shell, so
learners pick up the command line while clicking, and grading sees console
and terminal work alike. It has details pages with tabs and editing for IAM,
service accounts, roles, Compute Engine (VMs, templates, instance groups,
health checks, disks and snapshots), GKE (clusters, node pools, workloads and
services), Cloud Run (revisions, traffic and security), Cloud Storage (object
upload and download, permissions), Cloud SQL (users, databases, authorized
networks and backups), BigQuery Studio, Pub/Sub, VPC (subnets, IPs, routes,
firewall), load balancing, Cloud DNS, Cloud NAT, Secret Manager, Cloud KMS,
Artifact Registry, Cloud Build, Logging (explorer and metrics), Monitoring
(metrics and alerting), billing, topology and the activity log. The lab
instructions sit in a right-hand panel and Cloud Shell can be resized,
maximized or switched to its editor.

Inside a lab, type `help`. Useful commands: `why <resource>` (causal chain of a
request), `whatif <command>` (impact preview), `ticket show`, `team`,
`ask <person> "question"`.

### Full local stack

```sh
docker compose -f deploy/docker-compose.yml up --build
```

This runs the API, lab plane and grader as separate services with PostgreSQL
and the Pub/Sub and GCS emulators (F1).

## Configuration

| Variable | Default | Purpose |
|---|---|---|
| `ADDR` | `:8080` | listen address |
| `MODE` | `all` | `all`, `api` (learning plane) or `labplane` |
| `CONTENT_DIR` | `content` | labs, skills, tracks, failures, company |
| `WEB_DIR` | `web/out` | static web client |
| `DATA_FILE` | `data/gcplab.json` | JSON store when `DATABASE_URL` is unset |
| `DATABASE_URL` | — | PostgreSQL DSN |
| `JWT_SECRET` | random per start | token signing and transcript signatures |
| `LABPLANE_URL`, `LABPLANE_TOKEN` | — | API → lab plane (split mode) |
| `GRADER_URL` | — | isolated grader worker (`cmd/grader-worker`) |
| `PUBSUB_EMULATOR_HOST`, `STORAGE_EMULATOR_HOST`, `KIND_KUBECONFIG` | — | enable F1 |
| `F2_PROJECTS` | — | comma-separated sandbox project ids (enables F2) |
| `F2_DRIVER` | `sim` | `gcloud` to drive real projects |
| `F2_ADMIN_SA`, `F2_LAB_SA` | — | pool administrator and learner identities (impersonated) |
| `F2_DRY_RUN` | — | `1` logs gcloud commands instead of running them |
| `F2_MONTHLY` | `10` | real-cloud sessions per learner per month |
| `OIDC_ISSUER`, `OIDC_CLIENT_ID`, `OIDC_CLIENT_SECRET`, `OIDC_REDIRECT_URL` | — | Google Workspace / OIDC sign-in |
| `MENTOR_LLM` | `off` | `on` enables the Claude post-mortem reviewer (needs `ANTHROPIC_API_KEY`) |
| `CORS_ORIGIN` | — | allow a separately hosted web client |

## Windows installer

An MSI with everything included (simulator, web client and labs), offline and
without dependencies: download it from *Releases* or *Actions →
windows-installer*. Guide (Spanish): [docs/instalador-windows.md](docs/instalador-windows.md).

## Online for practice

One service running the repository's Docker image, with the database in
Supabase. Step-by-step guides (Spanish):

| Where | How | Guide |
|---|---|---|
| **Google Cloud Run** (recommended for ACE preparation) | Script for Cloud Shell or PowerShell (`deploy/cloudrun/`) | [docs/despliegue-cloud-run.md](docs/despliegue-cloud-run.md) |
| **Render** | [`render.yaml`](render.yaml) Blueprint | [docs/despliegue-render-supabase.md](docs/despliegue-render-supabase.md) |
| **Koyeb** | Docker service from GitHub | [docs/despliegue-koyeb.md](docs/despliegue-koyeb.md) |

## Full Google Cloud deployment (organisation and sandbox pool)

`deploy/terraform` creates the sandbox folder with org-policy guardrails
(allowed locations, no service-account keys, public access prevention, no VM
external IPs, domain-restricted sharing), the pool projects with budgets, and
the platform: Cloud Run services for the API (public), lab plane and grader
(internal only), Cloud SQL, Secret Manager and Artifact Registry. See
[docs/operations.md](docs/operations.md).

```sh
cd deploy/terraform
cp terraform.tfvars.example terraform.tfvars   # edit
terraform init && terraform apply
gcloud builds submit --config deploy/cloudbuild.yaml ..
```

## Authoring content

Labs are content-as-code. Every lab must go from failing to passing when its
reference solution is applied, for several seeds:

```sh
go run ./cmd/labctl validate          # schema, references and English translations
go run ./cmd/labctl i18n              # translation gaps only
go run ./cmd/labctl test              # all labs, several seeds
go run ./cmd/labctl test -lab ace-d05-subnet-isolation -seeds 1,7,42
go run ./cmd/labctl variants -lab ace-d05-subnet-isolation -n 3
```

See [docs/authoring.md](docs/authoring.md) for the lab DSL, failure library
and company missions.

## Development

```sh
go vet ./... && go test -race ./...
go run ./cmd/labctl validate && go run ./cmd/labctl test
(cd web && npm run build)
```

CI (`.github/workflows/ci.yml`) runs the same checks plus `terraform validate`
and an image build.

## Documentation

- [Master implementation blueprint](docs/blueprint/01-master-implementation-blueprint.md): architecture, contracts, schema, DSL, phases, changelog
- [Repository audit](docs/blueprint/00-repository-audit.md)
- [Architecture decision records](docs/adr/)
- [Fidelity matrix](docs/fidelity-matrix.md)
- [Authoring guide](docs/authoring.md)
- [Operations](docs/operations.md)
- [Risk register](docs/risk-register.md)
- [Roadmap](docs/roadmap.md)
