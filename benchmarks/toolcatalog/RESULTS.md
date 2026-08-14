# Tier 1 — tool-catalog token cost (head-to-head)

Measured 2026-08-14 by `run.sh`: each server's **live** `tools/list` response,
serialized identically (compact JSON `[{name, description, inputSchema}]` — the
field set every MCP host receives) and counted with Caveman's offline
`o200k_base` counter. Every figure is an `inferred` token count, not provider
usage. This is the context cost an agent pays on **every request** just for
having the server connected, before any page is visited.

| Server | Version measured | Tools | Catalog tokens | vs caveman-browse |
|---|---|---:|---:|---:|
| **caveman-browse** | HEAD (git 86f68d3) | 4 | **297** | — |
| @playwright/mcp | 0.0.79 (Playwright 1.63.0-alpha-2026-08-05) | 24 | 3,422 | 11.5× larger |
| chrome-devtools-mcp | 1.7.0 | 29 | 4,507 | 15.2× larger |

Per-tool caveman-browse breakdown: `browser_snapshot` 104 · `browser_act` 93 ·
`browser_eval` 44 · `browser_recover` 65. The catalog is budget-locked at 300
tokens by `TestBrowserToolsExactlyFour`.

## Claim boundary

- Counts are of the wire `tools/list` payload under one fixed serialization;
  hosts render tool definitions into context in slightly different shapes, so
  absolute in-context numbers vary by host. Relative ratios are the claim.
- Competitor catalogs shrink and grow between releases — earlier Playwright MCP
  builds measured ~13.7k tokens; 0.0.79 measures 3,422. Re-run `run.sh` before
  quoting; never cite this table for versions other than the ones listed.
- Fewer tools is a design choice with a cost: Playwright MCP and
  chrome-devtools-mcp cover capabilities caveman-browse deliberately lacks
  (tabs, network inspection, tracing, screenshots, file upload…). This table
  measures catalog cost, not feature parity.
