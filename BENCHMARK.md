# Caveman Browse efficiency benchmark

Measured 2026-08-14 with Google Chrome 151, `@playwright/test` 1.62.1, and
Caveman's offline `o200k_base` counter. Every number is an `inferred` token
count for one snapshot—not provider usage or billing.

All arms were measured the same day on the same machine and fixtures: Caveman
rows are medians of 3 integration runs; the Playwright ARIA baseline was
re-run fresh on 1.62.1 (15,709 on the dashboard vs 15,704 on 1.56.1 —
the earlier stale-baseline caveat is closed). A chrome-devtools-mcp
`take_snapshot` arm and the full three-way tables live in
[benchmarks/snapshot/RESULTS.md](benchmarks/snapshot/RESULTS.md); the
tool-catalog head-to-head lives in
[benchmarks/toolcatalog/RESULTS.md](benchmarks/toolcatalog/RESULTS.md).

## Results

Runs report median and `[min–max]`. Random CDP node ids and CCR handles explain
the small Caveman ranges. Playwright was stable across all five of its runs.

### Large operations table

| Representation | Tokens | Versus raw AX | Versus Playwright |
|---|---:|---:|---:|
| Raw `Accessibility.getFullAXTree` JSON | 398,494 `[398,493–398,497]` | — | — |
| Playwright `locator("body").ariaSnapshot()` | 15,709 | 96.06% less | — |
| Caveman full agent-visible result | 11,942 `[11,941–11,944]` | 97.00% less | 23.98% less |
| Caveman `interactive` result, no query | 5,091 `[5,090–5,093]` | 98.72% less | 67.59% less |
| Caveman focused result, query `ORD-0173` | 125 `[123–127]` | 99.97% less | 99.20% less / 125.7× smaller |

The JSON envelope removal is the delta between the 2026-08-10 and 2026-08-14
Caveman rows: full 13,368 → 11,942 (**-10.7%**)
(**-19.0%**), with no change to what the agent can see or act on.

The focused row includes the matched row's sibling cells (customer, amount,
action button), not just the matching cell: a 2026-08-14 agent-loop run showed
a cell-only focus answered an order lookup with "unavailable". Row context
costs ~26 tokens on this fixture and is correctness, not overhead.

`interactive` keeps uid-bearing nodes plus the ancestors that place them. On
this corpus it more than halves the queryless result (11,942 → 5,091) but does
not approach the 200–400 tokens `agent-browser snapshot -i` reports for typical
pages — because this fixture is 200 rows each carrying its own actionable
control, so the actionable set *is* most of the page. A page with a normal
handful of controls behaves much closer to that class. `query` remains the
strongest lever on a page this shape.

Corpus: [`testdata/order_dashboard.html`](testdata/order_dashboard.html), a
200-row operations table with one requested order action. Caveman's full result
contains compact AX text, UIDs, CCR handle, exact agent-visible token count, and
honesty metadata. Playwright baseline is only its ARIA text: no MCP envelope,
action refs, recovery handle, or accounting. That asymmetry favors Playwright.

### Small checkout form

| Representation | Tokens | Versus raw AX | Versus Playwright |
|---|---:|---:|---:|
| Raw `Accessibility.getFullAXTree` JSON | 4,186 `[4,183–4,188]` | — | — |
| Playwright `locator("body").ariaSnapshot()` | 67 | 98.40% less | — |
| Caveman full agent-visible result | 134 | 96.80% less | 2.00× larger |
| Caveman focused result, query `Email Plan Save order` | 92 | 97.80% less | 1.37× larger |

This small-page loss is important: Caveman's recovery handle, exact counters,
honesty basis, and action UIDs cost more than bare Playwright ARIA text when the
page itself is tiny. It still saves 97.85% versus raw AX and carries enough
state to type, select, click, verify, and recover bytes. No universal
snapshot-only win is claimed.

Smaller captured fixture also locks serializer regression:

- prior Caveman JSON-lines view: 380 tokens;
- compact indented view: 58 tokens (84.7% less than prior view);
- exact delivered payload including CCR/accounting: 105 tokens (126 with the
  JSON envelope). What remains at this size is mostly the recovery handle and
  the counters themselves;
- raw AX: 5,351 tokens;
- four-tool MCP catalog: 297 tokens, budget 300. The `interactive` property cost
  10 of the prior 13 tokens of headroom; a fifth tool does not fit, which is the
  point.

Escaping tax that motivated the text payload, measured with the same counter on
a synthetic 200-row view in Browse's exact render format: raw 6,800 tokens vs
7,650 JSON-wrapped (**+12.5%**), plus 49 tokens for the envelope keys. Cause:
`renderAXRecords` already `strconv.Quote`s every accessible name, and
`json.Marshal` escaped each one a second time.

## Reproduce

Run Caveman live-Chrome benchmark and functional loop:

```bash
CAVEMAN_BROWSE_CHROME="/path/to/Chrome" \
  go test -tags=integration -run 'TestCDPQueryScales|TestCDPFullTokenEfficient' -count=5 -v .
```

Count locked Playwright ARIA baseline with same tokenizer:

```bash
CAVEMAN_BROWSE_CHROME="/path/to/Chrome" \
  node scripts/playwright-aria-baseline.mjs |
  CAVEMAN_CCR_DB=/tmp/caveman-browse-bench.db \
  go run github.com/JuliusBrussee/caveman/engine/cmd/caveman-engine compress --type no-such-type >/dev/null
```

Pass `agent_checkout.html` after the baseline script to reproduce the small-form
row. The four-tool MCP catalog cost and the delivered-payload budget are locked
in `TestBrowserToolsExactlyFour` and
`TestSnapshotCachesUIDTargetsAndRecoversExactAXTree`; the interactive row is
locked in `TestCDPQueryScalesOnTwoHundredRowDashboard`.

Integration gates also prove type, select, offscreen auto-scroll click,
post-action focused verification, disabled-control rejection, stale-UID
rejection, byte-exact live recovery, fresh-home startup, cross-process direct
CLI reattachment, and explicit Chrome shutdown.

## Claim boundary

These results prove this corpus and toolchain, not universal open-web dominance.
Phase 1 remains scoped to same-origin, predictable controls. OOPIFs, dialogs,
downloads, and arbitrary-site actionability remain deferred. Query-focused
progressive disclosure is default best practice for large pages; full snapshots
remain available when task intent is unknown.
