# Risk register

| # | Risk | Likelihood | Impact | Mitigation | Owner / status |
|---|---|---|---|---|---|
| R1 | Simulator behaviour diverges from Google Cloud and teaches something wrong | Medium | High | Behaviour follows public docs; every lab has a CI solution run; F2 capstones on real projects; fidelity matrix documents limits | Content team · ongoing |
| R2 | Learners abuse F2 sandboxes (crypto-mining, exfiltration, public data) | Medium | High | Folder org policies (locations, no external IPs, no SA keys, PAP, domain restriction), per-project budgets, short leases with conditional IAM, impersonated identities, janitor, monthly quota | Platform · mitigated |
| R3 | Runaway cloud cost | Low | High | Budgets per sandbox, `F2_MONTHLY`, TTL-based janitor, quarantine instead of reuse when cleanup fails | Platform · mitigated |
| R4 | Grader can be gamed (e.g. state tampering, answer leakage) | Medium | Medium | Grader worker rebuilds labs from its own content; check descriptions never reveal expected values; findings view hidden from the API; variants per seed | Platform · mitigated |
| R5 | Answers shared between learners | High | Low | Seeded variants, generated incidents, process assessment (how, not only what), stealth retention checks | Learning · mitigated |
| R6 | Lab plane is a single instance (in-memory sessions) | Medium | Medium | Snapshots to the store and restore on access; Cloud Run min=max=1 with CPU always on; horizontal scaling would need session-affine routing | Platform · accepted |
| R7 | LLM mentor gives wrong or leaking advice | Low | Medium | Opt-in only, never the grader, reviews post-mortems after submission; deterministic `why`/`whatif` are the default help | Learning · mitigated |
| R8 | Personal data | Low | Medium | Minimal data (name, email, attempts); PostgreSQL in the platform project; signed transcripts contain only competence data | Platform · accepted |
| R9 | Content rot as Google Cloud changes | High | Medium | Content-as-code CI, `labctl curriculum` comparison with official learning paths, versioned labs | Content team · ongoing |
| R10 | Accessibility of the terminal-centric UI | Medium | Medium | Console views mirror terminal state; xterm screen-reader mode toggle; history tab with command output; ARIA tabs and progress bars; skip link; WCAG 2 AA axe audit with 0 violations in light and dark mode (re-run before releases) | Web · mitigated |
| R11 | A rubric rewards a wrong fix (the symptom disappears, the control is gone) | Medium | High | Labs declare shortcuts (disable the control, over-grant, restore over live data, deploy to 100%); content CI fails when any shortcut passes | Content team · mitigated |
