# ADR 0006 — A persistent company world with consequences

- Status: accepted
- Context: isolated labs never show learners the long-term cost of shortcuts.
- Decision: Career Mode keeps one simulator world per learner (Nebula
  Corporation, six projects in three folders). Missions start from the saved
  world and save it back. After each mission, Rego policies and resource checks
  detect risks left in the world (ignoring those already present in the
  baseline where marked `newOnly`) and schedule consequence incidents a
  few simulated days later; fixing the risk first cancels them. Company metrics
  (availability, security, cost, satisfaction) and a journal track the story.
- Consequences: learners live with their decisions; the world is serialised
  state, so it is portable across lab-plane restarts and to the grader worker.
