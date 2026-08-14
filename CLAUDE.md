# caveman-browse — token-efficient browser driver

Standalone repo. Serves
stdio MCP tools that read a real Chrome accessibility tree, compress it with
the Caveman engine's forced-only `a11y` compressor, act on `uid` handles, and
recover the byte-exact original AX payload through CCR. Every saving here is
`inferred`; Browse never emits `verified`.

## Layout
- `session.go` — MCP tool handlers, engine/CCR integration, UID target cache.
- `cdp.go` — chromedp-backed Mode-A dedicated Chrome driver and bounded
  actionability recipe.
- `cmd/caveman-browse/` — stdio MCP binary + direct CLI subcommands.
- `bin/` — npm shim (launcher + generated installer); `scripts/gen-installer.mjs`
  regenerates the generated files from `BINARY_RELEASE` + `BINARY_SIGNING_PUBKEY.pub`.
- `.claude-plugin/`, `.codex-plugin/`, `gemini-extension.json`, `skills/` —
  installable plugin surfaces; `.mcp.json` is the server entry they share.
- `benchmarks/` — head-to-head suite vs Playwright MCP / Chrome DevTools MCP.

## Module boundary
- This module pins `github.com/JuliusBrussee/caveman` (engine, engine/ccr,
  engine/tokens, mcp framing) in `go.mod`. Bumps are explicit commits. The
  `a11y` compressor (`engine/compressors/axtree.go`) lives in that repo, not
  here.
- Signed binaries are still published on the caveman monorepo releases
  (`scripts/gen-installer.mjs` release base); repo-owned signed releases are a
  tracked follow-up.

## Gotchas
- `go test ./...` is setup-free. `CAVEMAN_BROWSE_CHROME=<chrome> go test
  -tags=integration ./...` runs the CDP contract across the package and direct
  CLI.
- The actionability layer is intentionally bounded: same-origin dashboards and
  predictable design-system controls, not arbitrary-open-web parity.
- Unknown handles/actions fail closed with `cave_snake_code` errors.
- Snapshot output is compact indented text, not JSON-lines. `query` keeps at most
  12 highest-scoring task matches plus ancestors; recovery metadata exposes only
  UIDs actually shown. Keep `tokens_after` equal to exact agent-visible result
  cost and `view_tokens` equal to serializer output cost.
- **The whole payload is text, not a JSON envelope**: the AX view plus one
  trailing `caveman before= view= after= ratio= basis= handle=` line
  (`renderSnapshot`). Re-wrapping it in JSON escapes every already-`strconv.Quote`d
  accessible name a second time — a measured 12.5% tax plus 49 envelope tokens.
  `finalizeSnapshotPayload` still settles the self-reference (the line states the
  count that stating it changes); `ratio` is rounded to the 4 decimals printed,
  `before`/`after` stay exact.
- `interactive: true` keeps uid-bearing lines plus the ancestors that place them
  and nothing else. It is a flag and never a default — it hides the text an agent
  has to READ — and fails open to the full view when no uid survives. It never
  narrows the uid map: the target cache always comes from the full curate pass.
- The four-tool catalog is at 297 of its 300-token budget. A fifth tool does not
  fit, and neither does a verbose description. That budget is the moat versus
  ~13.7k (Playwright MCP) and ~17k (Chrome DevTools MCP) tool-definition tokens.
- Direct CLI Chrome is detached so separate `snapshot`/`act`/`eval` processes
  can share a target. `close` must terminate it. Fresh `CAVEMAN_HOME` must work;
  state writes stay atomic and mode `0600`.
- CDP action acknowledgement is not application settlement. Non-wait actions
  return `settled:false` and require a focused resnapshot for proof.
- Navigation denies `file:`, `javascript:`, and privileged Chrome schemes.
- Benchmark contract and reproducible comparisons live in `BENCHMARK.md`; keep
  token budgets executable in tests.
- **The uid map is `browser_snapshot`'s contract, not a side effect of a
  compression ratio.** On engine pass-through (no recovery handle — a tree that
  did not get smaller, or no CCR store) `snapshotTool` fails closed with
  `cave_browser_snapshot_uncompressed` and returns the prior page's uids intact;
  it MUST NEVER dump the raw AX tree into `uids` (strictly worse than not using
  Browse) nor wipe the target cache. Regressing this reopens a
  known acting-dead failure mode.
- **An `<iframe>` is a leaf, not a broken tree.** `Accessibility.getFullAXTree`
  returns one frame at a time, so an iframe node's `childId` points at a child
  document absent from this payload; the `a11y` compressor
  (`engine/compressors/axtree.go`) treats an unresolvable `childId` as a leaf and
  still curates the frame-visible nodes. Because of that the CDP driver keeps
  Chrome's default Site Isolation (`site-per-process`) — do not disable it to
  "fix" cross-origin iframes.
