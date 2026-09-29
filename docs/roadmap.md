# Roadmap

Status of both programmes this repository implements: the original *GCP Lab
Simulator* practice programme and the *Cloud Mastery Blueprint v1*. Details and
test evidence per phase are in the
[master implementation blueprint](blueprint/01-master-implementation-blueprint.md).

## Done

- F0 simulator and terminal (gcloud, gsutil, bq, kubectl, terraform, Linux tools)
- Declarative scenario engine with faults, variants, hints, rubric validators and OPA policies
- Learning plane: XP, levels, mastery formula, badges, leagues, certification readiness, KPIs, classes
- Fidelity router with F1 emulators and F2 sandbox pool, janitor and quotas
- 30-day ACE programme plus foundations, Linux, architecture, DevOps, SRE, FinOps, capstones and interview
- Skill graph with prerequisites, 9-dimension student model, autonomy ladder, retention and adaptive plan
- Service desk tickets and simulated people; failure library, symptom graph and incident generator with P1 mode
- Nebula Corporation company simulation with consequences
- Architecture simulator, chaos, FinOps, what-if, teach-me-why
- Career stages, specialisations, capstones, signed transcripts
- Web client, split planes, Docker/Compose, Terraform, Cloud Build, GitHub Actions

## Next

| Item | Why |
|---|---|
| More failure-library systems (GKE microservices, data pipeline, hybrid network) | More variety in generated incidents |
| Professional-level tracks (PCA, PCNE, PCSE, PCDE) with case studies | Readiness for professional certifications |
| Session-affine lab plane scaling | Remove the single-instance constraint (R6) |
| Collaborative incidents (two learners, one ticket) | Communication and hand-off skills |
| Accessibility pass on the lab workspace | R10 |
| Provider adapters for AWS and Azure | The engines are provider-agnostic; only the simulator adapter is GCP-specific |
| Instructor authoring UI with live `labctl test` | Lower the barrier to content contributions |
