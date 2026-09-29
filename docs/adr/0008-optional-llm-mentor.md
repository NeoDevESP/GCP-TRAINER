# ADR 0008 — Deterministic help first, LLM mentor optional

- Status: accepted
- Context: learners need explanations ("teach me why", "what if"), but grading
  and help must be reproducible and must not leak answers.
- Decision: `why` explains the recorded causal chain of a request and `whatif`
  runs a command on a clone of the world and reports the impact; Socratic
  mentor messages are rule-based. An LLM (Claude, through the official SDK) is
  opt-in (`MENTOR_LLM=on`) and only reviews post-mortems after submission. It
  never grades.
- Consequences: the platform works fully offline; enabling the LLM adds
  richer feedback without changing scores.
