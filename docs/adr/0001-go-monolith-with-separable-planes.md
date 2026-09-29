# ADR 0001 — One Go binary, three separable planes

- Status: accepted
- Context: the programme needs a learning platform (accounts, progress,
  gamification), an environment that runs untrusted learner commands, and a
  grader that probes environments learners may have tampered with.
- Decision: a single Go module and binary with `MODE=all|api|labplane`, plus a
  separate `grader-worker`. The API talks to the lab plane through the
  `orchestrator.LabPlane` interface, implemented in-process or over HTTP with a
  shared token. The grader worker rebuilds labs from its own copy of the
  content (and regenerates generated incidents and company missions from their
  specs) instead of trusting definitions sent by the lab plane.
- Consequences: simple local development (one process, JSON store) and a
  hardened production topology (public API, internal lab plane and grader)
  without two codebases. The lab plane keeps live sessions in memory and must
  run as one instance until session-affine routing exists (risk R6).
