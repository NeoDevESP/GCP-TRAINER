# ADR 0005 — Derive the student model from attempts

- Status: accepted
- Context: the Blueprint adds dimensions, retention, autonomy and career stages
  on top of an existing XP/mastery system with stored attempts.
- Decision: store only attempts (with grader results) and derive everything
  else — mastery, dimensions, forgetting curve, autonomy, career stage,
  adaptive plan — on read. Formulas live in `internal/learning` and are tested.
- Consequences: no data migrations when models change, historical data is
  re-interpreted consistently, and transcripts can be regenerated. Profile
  computation cost grows with attempts; acceptable at course scale and
  cacheable later.
