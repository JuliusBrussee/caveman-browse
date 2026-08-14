# Agent orientation

Read [CLAUDE.md](CLAUDE.md) — same content applies to every coding agent.
Key invariants: savings are always `inferred`; lossy snapshots require stored
byte-exact originals (CCR-or-passthrough); unknown handles fail closed with
`cave_snake_code` errors; the four-tool catalog stays within its 300-token
budget (locked in tests).
