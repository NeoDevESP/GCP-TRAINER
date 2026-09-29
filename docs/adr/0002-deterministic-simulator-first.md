# ADR 0002 — Deterministic simulator first (F0), real cloud as a fidelity level

- Status: accepted
- Context: real Google Cloud projects are slow to provision, cost money, need
  cleanup and make incidents hard to reproduce. Learners also need to break
  things safely.
- Decision: every lab runs on F0, a deterministic model of Google Cloud driven
  by a seed. F1 (emulators) and F2 (real sandbox projects) are optional
  fidelity levels chosen per lab by a router that falls back and explains why.
  Grading always reads a simulator state: F0 directly, F1 via mirrored calls,
  F2 via a snapshot of the real project.
- Consequences: instant labs, reproducible incidents, CI that proves every lab
  is solvable, and a causal-chain recorder (`why`) that a real cloud cannot
  offer. The cost is maintaining the model; the fidelity matrix and lab CI keep
  its scope explicit and tested.
