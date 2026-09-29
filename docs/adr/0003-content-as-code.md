# ADR 0003 — Content as code with solution-based CI

- Status: accepted
- Context: labs rot silently when the simulator changes, and hand-checked labs
  do not scale to generated incidents.
- Decision: labs, skills, tracks, career, failure library and company missions
  are YAML in the repository. `labctl validate` checks schema and references;
  `labctl test` provisions each lab for several seeds, grades it (must fail),
  runs the reference solution and grades again (must pass). Generated incidents
  are pure functions of a `GenSpec` and are tested the same way.
- Consequences: every change to the simulator or content is checked against
  all labs; the Docker image cannot be built with broken content.
