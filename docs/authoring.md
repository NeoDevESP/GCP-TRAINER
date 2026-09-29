# Authoring guide

Content lives in `content/` and is code: it is validated and tested in CI, and
a lab is only accepted when its reference solution takes it from failing to
passing for several seeds.

```
content/
  skills.yaml          skill graph: branches, blocks, skills and prerequisites
  tracks.yaml          learning paths (ordered labs)
  career.yaml          career stages, specialisations, capstones
  badges.yaml, certs.yaml
  labs/<track>/<lab>/lab.yaml   (+ optional solution.sh)
  baselines/*.sh       reusable environments (shop-platform, three-tier-app, ...)
  policies/*.rego      OPA policies used by `policy` checks
  failures/            failure library for generated incidents
  company/             Nebula Corporation: nebula.yaml and missions/<id>/lab.yaml
```

## Lab anatomy

```yaml
id: ace-d03-secure-bucket          # unique, stable
title: "Day 3 · A secure documents bucket"
track: ace-30
day: 3
level: basic                        # basic | intermediate | professional | expert
branch: storage                     # skill branch the lab belongs to
type: challenge                     # guided | challenge | incident | boss | capstone | interview ...
mode: lab                           # learn | lab | production | architecture | career | unknown
skills: [storage.buckets, storage.lifecycle]
difficulty: 2                       # 1-5
minutes: 45
params:                             # a seed picks one value per key → variants
  region: [europe-west1, europe-west3]
baseline: [default-network]         # scripts from content/baselines
setup: |                            # extra commands run as the platform admin
  gcloud storage buckets create gs://tmp-{{.project}} --location={{.region}}
faults:                             # declarative breakage (45 types, see labctl validate)
  - {type: firewall_delete, rule: allow-health}
story: |                            # Markdown shown to the learner; {{.param}} templates
objectives: [...]
constraints: [no-downtime, no-public, "protect:billing-batch"]
hints:                              # progressive, each one costs points
  - text: "..."
solution: |                         # reference solution used by CI (never shown)
rubric:                             # 100 points
  - name: Security
    validator: security             # functional | state | security | diagnosis | cost | reliability | evidence | quiz
    points: 30
    critical: true                  # failing it fails the lab
    checks:
      - {type: exists, path: 'buckets["docs-{{.project}}"]', expect: {publicAccessPrevention: enforced}, desc: "PAP enforced"}
      - {type: policy, package: gcplab.security, ids: [PUBLIC_BUCKET], desc: "No public bindings"}
```

`desc` is what the learner sees in *Check work* when a check fails, so write
it as the requirement, never as the answer.

### Checks

| Family | Types |
|---|---|
| State | `exists`, `absent`, `count` (with `expect` operators `eq`, `ne`, `gt`, `gte`, `lt`, `lte`, `contains`, `in`, `regex`, `len`, `empty`) |
| Functional | `http`, `tcp`, `egress`, `google_api`, `pubsub`, `sql_healthy`, `rollout`, `hpa_scaled`, `build`, `chaos` |
| Identity | `iam` (effective permission, conditions and org policies included), `no_basic_roles`, `org_policy`, `k8s_rbac` |
| Security and cost | `policy` (Rego), `forbid_firewall`, `finding_absent`/`finding_present`, `cost_max`/`cost_min`, `bq_bytes_max`, `secret_rotated` |
| Operations | `log_metric`, `alert_policy`, `log_contains`, `terraform_clean`, `tf_state`, `vm_file`, `vm_disk`, `subnet_plan`, `design` |
| Process and communication | `command` (what the learner ran; `min`, or `max` to forbid a command), `session_config`, `file_contains`, `ticket_update`, `ticket_resolved`, `asked`, `evidence` (keywords in the post-mortem fields), `quiz` |

Any check can target another project with `on: <project-key>` (company missions).

### Incidents: tickets and people

```yaml
ticket:
  id: INC-2041
  kind: INC                 # INC | REQ | CHG | PRB | SEC | COST | MIG
  priority: P2
  sla: 4h
  summary: "Checkout is slow"   # what the reporter believes, not the root cause
  reporter: Store support
actors:
  - role: developer
    name: Leo (backend)
    fallback: "The code hasn't changed since Friday."
    facts:                  # every keyword group must match the question
      - {keywords: [[deploy, release], [today, yesterday]], answer: "No deploys since Friday.", evidence: deploy.log, content: "..."}
```

Learners use `ticket show|comment|update|resolve|escalate`, `team` and
`ask <who> "question"`. The process assessment rewards asking before changing
and penalises blind restarts, risky grants and constraint violations.

## Failure library

`content/failures/systems/<system>.yaml` describes a healthy architecture
(`baseline`, `setup`, `topology`, `traffic`, `healthy` and `safety` checks,
`actors`, `noise`, decoy resources) and its `failures`. Each failure mode has a
`symptom` (from `symptoms.yaml`), the `layer` where the cause lives, `faults`,
extra actor `facts`, `solution`, `checks` and optional `variants`.
`contexts.yaml` holds business contexts (company, service, impact, priority).

The generator composes system + failures + context + difficulty + mode into an
ordinary lab: higher difficulty adds noise, vague or misleading tickets and
stacked failures; `production` mode makes it a P1 with an alert timeline and a
mandatory post-mortem; `unknown` mode only states the business impact.

```sh
go run ./cmd/labctl failures                                   # symptom graph
go run ./cmd/labctl generate -system three-tier -difficulty 5 -mode production
```

CI generates one incident per failure mode plus stacked and production-mode
combinations and verifies each is solvable.

## Company missions

Missions live in `content/company/missions/<id>/lab.yaml` and carry a
`company` block:

```yaml
company: {stage: junior, project: prod-web, kind: REQ, after: [nb-m01-orientation]}
```

They run on the learner's persisted Nebula world, so project ids are
templated (`{{.prod_web}}`, `{{.prod_data}}`, ...). Consequences in
`company/nebula.yaml` turn risks left behind (open SSH, public data, no
backups, no budget) into incident missions a few days later; mitigating the
risk before then cancels them.

## Checklist before opening a PR

1. `go run ./cmd/labctl validate`
2. `go run ./cmd/labctl test -lab <id> -seeds 1,7,42` — before < pass mark ≤ after
3. `go run ./cmd/labctl play -lab <id>` and try the obvious wrong fixes: they
   must not pass (e.g. opening a port to 0.0.0.0/0 must fail the security criterion)
4. Hints go from a nudge to a near answer; the story never names the root cause
