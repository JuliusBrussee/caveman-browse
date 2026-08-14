# Tier 3 — full-task agent loop (head-to-head pilot)

Measured 2026-08-14 by `runner.mjs`: the **same headless agent** (`claude -p`,
model `sonnet`, default settings), the **same task prompt**, one browser MCP
server swapped per arm, one run per cell (**pilot, n=1 — indicative, not
statistical**). Token figures are provider-reported usage for the whole session
(observed-local); nothing here is `verified`. Exact-answer gates; all 9 runs
passed.

Arms: caveman-browse HEAD · `@playwright/mcp@0.0.79` (`--browser chrome
--headless --isolated`) · `chrome-devtools-mcp@1.7.0` (`--headless
--isolated`).

**Total input tokens** = input + cache creation + cache read (what the
provider processed across all turns).

| Task | Arm | Pass | Turns | Total input tok | Output tok | Wall |
|---|---|---|---:|---:|---:|---:|
| dashboard-lookup (find 1 row in 200) | **browse** | ✓ | **3** | **150,458** | 297 | 20.0s |
| | playwright | ✓ | 4 | 202,839 | 343 | 18.6s |
| | cdm | ✓ | 4 | 223,496 | 311 | 17.9s |
| dashboard-count (count 12 matching rows) | **browse** | ✓ | **3** | **167,630** | 517 | 19.0s |
| | playwright | ✓ | 7 | 357,781 | 1,063 | 37.1s |
| | cdm | ✓ | 4 | 223,436 | 426 | 20.9s |
| checkout-fill (type + select + click + verify) | browse | ✓ | 13 | 668,408 | 1,279 | 40.7s |
| | **playwright** | ✓ | 8 | 361,963 | 664 | 22.2s |
| | **cdm** | ✓ | **6** | **311,867** | 551 | 21.4s |

## Reading

- **Read-heavy tasks: browse wins.** Fewest turns and 26–53% fewer total input
  tokens on both dashboard tasks — the focused/compressed snapshot does the
  work of several bigger reads.
- **Action-heavy task: browse loses this pilot.** 13 turns vs 6–8. Two causes,
  both visible in the transcript: `browser_act` returns `settled:false` by
  design (the agent re-snapshots to prove each step), and the fixture's
  Save button sits 1400px below the fold. Playwright/CDM auto-wait and report
  action success directly. This is the honesty-model tax plus an actionability
  gap, printed here on purpose; treating dispatch as success would be the
  dishonest way to win the row.
- Session floor dominates absolute numbers: ~50k tokens of every arm's total is
  the harness system prompt re-read per turn, which mostly rewards fewer turns.
- Cost column omitted: single-run cache-creation pricing noise exceeds the
  between-arm deltas at this n.

## Reproduce

```bash
export CAVEMAN_BROWSE_CHROME="/path/to/Chrome"
go build -o bin/caveman-browse ./cmd/caveman-browse
node benchmarks/agentloop/runner.mjs            # all tasks × all arms
node benchmarks/agentloop/runner.mjs --arms browse --tasks dashboard-lookup
```

Raw per-run records: `results.json`.

## Claim boundary

One run per cell, one model, three same-origin fixture tasks. No open-web
tasks, no OOPIF/dialog/download coverage, no statistical interval. The
dashboard wins and the checkout loss are both single observations; re-run with
`--tasks`/`--arms` and more repetitions before quoting beyond "pilot".
