# Roadmap

Status of both programmes this repository implements: the original *GCP Lab
Simulator* practice programme (including its 12-month roadmap) and the *Cloud
Mastery Blueprint v1*. Details and test evidence per phase are in the
[master implementation blueprint](blueprint/01-master-implementation-blueprint.md).

## 12-month roadmap of the practice programme

| Month | Deliverable | Where |
|---|---|---|
| 1 | ADRs, UX, skill taxonomy, content schema | `docs/adr`, `web/`, `content/skills.yaml`, `internal/scenario/lab.go` |
| 2 | Accounts, labs on F0, terminal, first grader | `internal/api`, `internal/cli`, `internal/grader` |
| 3 | IAM / Compute / Storage / Network simulator and labs | `internal/sim`, `content/labs/ace-30` |
| 4 | Project pool, fault engine, observability | `internal/fidelity`, `internal/scenario/faults.go`, logs and metrics views |
| 5 | Gamification, skill graph, incident engine | `internal/learning`, `content/failures` |
| 6 | 30-day ACE track, capstones | `content/labs/ace-30`, `content/labs/capstones` |
| 7 | GKE and Kubernetes incidents | `gke` track, failure-library system `gke-shop` (7 failure modes), `gke-k01`, `gke-k02` |
| 8 | DevOps / SRE and Professional Architect foundations | `devops`, `sre`, `architect` tracks |
| 9 | Security and Network Engineer tracks | `security` (incl. `sec-s01-leaked-key`), `network` (incl. `net-n02-shared-services-peering`) |
| 10 | Data and databases | `data` track (incl. `data-d01-sql-recovery`) |
| 11 | ML/AI and professional capstones | `ml` track (incl. `ml-m01-canary-model`), `cap-data-ml-platform` |
| 12 | Hardening, accessibility, load testing, analytics | WCAG 2 AA audit with axe (0 violations, light and dark), terminal screen-reader mode, `labctl loadtest`, instructor KPIs, shortcut gate in content CI |

## Done

- F0 simulator and terminal (gcloud, gsutil, bq, kubectl, terraform, Linux tools)
- Declarative scenario engine with faults, variants, hints, rubric validators, OPA policies and declared shortcuts that CI proves fail
- Learning plane: XP, levels, mastery formula, badges, leagues, certification readiness, KPIs, classes
- Fidelity router with F1 emulators and F2 sandbox pool, janitor and quotas
- 54 labs in 14 tracks: 30-day ACE, foundations, Linux, GKE, security, network, data, ML, architect, DevOps, SRE, FinOps, capstones and career
- Skill graph with prerequisites, 9-dimension student model, autonomy ladder, retention and adaptive plan
- Service desk tickets and simulated people; failure library (3 systems, 20 failure modes), symptom graph and incident generator with P1 mode
- Nebula Corporation company simulation with consequences
- Architecture simulator, chaos, FinOps, what-if, teach-me-why
- Career stages, specialisations, capstones, signed transcripts
- Web client (WCAG 2 AA), split planes, Docker/Compose, Terraform, Cloud Build, GitHub Actions

## Next

| Item | Why |
|---|---|
| More failure-library systems (data pipeline, hybrid network) | More variety in generated incidents |
| Case-study packs per professional certification | Exam-style reasoning on top of hands-on labs |
| Session-affine lab plane scaling | Remove the single-instance constraint (R6) |
| Collaborative incidents (two learners, one ticket) | Communication and hand-off skills |
| Provider adapters for AWS and Azure | The engines are provider-agnostic; only the simulator adapter is GCP-specific |
| Instructor authoring UI with live `labctl test` | Lower the barrier to content contributions |
| Spanner, Firestore, Bigtable and Dataflow in the simulator | Complete the Data Engineer and Database Engineer paths |
