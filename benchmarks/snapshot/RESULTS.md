# Tier 2 — snapshot token cost, same fixtures, same day (head-to-head)

Measured 2026-08-14, all arms on the same machine, same fixtures
(`testdata/order_dashboard.html`, `testdata/agent_checkout.html`), same Google
Chrome 151 binary, one tokenizer (offline `o200k_base` via
`benchmarks/counttext`). Every number is `inferred`. Caveman rows are the
median of 3 integration runs; each competitor arm is what the agent actually
receives from that tool.

Arms:
- **caveman-browse** — full delivered `browser_snapshot` payload (tree + uids +
  CCR handle + accounting line), from
  `go test -tags=integration -run 'TestCDPQueryScales|TestCDPFullTokenEfficient' -v .`
- **Playwright 1.62.1** — `locator("body").ariaSnapshot()` text only
  (`scripts/playwright-aria-baseline.mjs`); no MCP envelope, no action refs, no
  recovery. This asymmetry favors Playwright.
- **chrome-devtools-mcp 1.7.0** — full `take_snapshot` tool-result text
  (uids included), `benchmarks/snapshot/cdm-snapshot.mjs`.

## 200-row operations dashboard

| Representation | Tokens | vs raw AX |
|---|---:|---:|
| Raw `Accessibility.getFullAXTree` JSON | 398,494 | — |
| Playwright `ariaSnapshot()` (text only) | 15,709 | 96.1% less |
| chrome-devtools-mcp `take_snapshot` (full result) | 13,544 | 96.6% less |
| **caveman-browse full result** | **11,942** | 97.0% less |
| **caveman-browse `interactive`** | **5,091** | 98.7% less |
| **caveman-browse focused, query `ORD-0173`** | **125** | 99.97% less — 108× under chrome-devtools-mcp, 126× under Playwright |

## Small checkout form

| Representation | Tokens |
|---|---:|
| Raw `Accessibility.getFullAXTree` JSON | 4,185 |
| **Playwright `ariaSnapshot()` (text only)** | **67** |
| caveman-browse focused, query | 92 |
| caveman-browse full result | 131 |
| chrome-devtools-mcp `take_snapshot` (full result) | 180 |

Playwright wins the tiny page: bare ARIA text carries no uids, recovery handle,
or accounting, and at this size that overhead exceeds the compression win.
caveman-browse still beats the other full tool result (chrome-devtools-mcp,
180) while carrying strictly more capability (byte-exact recovery + exact
accounting). No universal snapshot-only win is claimed.

## Reproduce

```bash
export CAVEMAN_BROWSE_CHROME="/path/to/Chrome"
go test -tags=integration -run 'TestCDPQueryScales|TestCDPFullTokenEfficient' -count=3 -v .
node scripts/playwright-aria-baseline.mjs order_dashboard.html | go run ./benchmarks/counttext
node benchmarks/snapshot/cdm-snapshot.mjs order_dashboard.html | go run ./benchmarks/counttext
# repeat both with agent_checkout.html
```

## Claim boundary

These results prove these fixtures and versions, not universal open-web
dominance. The Playwright arm is snapshot *text* (its cheapest possible
representation); its MCP tool result would be larger. chrome-devtools-mcp and
Playwright both offer capabilities caveman-browse deliberately lacks. Scope
remains same-origin, predictable controls.
