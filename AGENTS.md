# caveman-browse — token-efficient browser driver

Standalone Apache-2.0 repository (`JuliusBrussee/caveman-browse`). Serves stdio
MCP tools that read a real Chrome accessibility tree, compress it with the
Caveman engine's forced-only `a11y` compressor, act on `uid` handles, and
recover the byte-exact original AX payload through CCR. Every saving here is
`inferred`; Browse never emits `verified`.

`CLAUDE.md` is a symlink to this file.

## Layout

- `session.go` — MCP tool handlers, engine/CCR integration, UID target cache.
- `cdp.go` — chromedp-backed Mode-A dedicated Chrome driver and bounded actionability recipe.
- `cmd/caveman-browse/` — stdio MCP binary + direct CLI subcommands.
- `bin/` — npm shim (launcher + generated installer); `scripts/gen-installer.mjs` regenerates generated files from `BINARY_RELEASE` + `BINARY_SIGNING_PUBKEY.pub`.
- `.claude-plugin/`, `.codex-plugin/`, `gemini-extension.json`, `skills/` — installable plugin surfaces; `.mcp.json` is the server entry they share.
- `benchmarks/` — head-to-head suite vs Playwright MCP / Chrome DevTools MCP (`BENCHMARK.md`).
- `tests/` — packaging and integration test harnesses.

## Module boundary

- This module pins `github.com/JuliusBrussee/caveman` (engine, engine/ccr, engine/tokens, mcp framing) in `go.mod`. Bumps are explicit commits. The `a11y` compressor (`engine/compressors/axtree.go`) lives in that repo, not here.
- Signed binaries are published on Caveman releases (`scripts/gen-installer.mjs` release base); repo-owned signed releases are tracked separately.

## Checks

```bash
go vet ./...
go test ./...
# With a real Chrome binary:
CAVEMAN_BROWSE_CHROME=<chrome> go test -tags=integration ./...
```

## Invariants & gotchas

- The actionability layer is intentionally bounded: same-origin dashboards and predictable design-system controls, not arbitrary-open-web parity.
- Unknown handles/actions fail closed with `cave_snake_code` errors.
- Snapshot output is compact indented text, not JSON-lines. `query` keeps at most 12 highest-scoring task matches plus ancestors; recovery metadata exposes only UIDs actually shown. Keep `tokens_after` equal to exact agent-visible result cost and `view_tokens` equal to serializer output cost.
- **The whole payload is text, not a JSON envelope:** the AX view plus one trailing `caveman before= view= after= ratio= basis= handle=` line (`renderSnapshot`). Re-wrapping it in JSON escapes every already-`strconv.Quote`d accessible name a second time (measured 12.5% tax plus 49 envelope tokens). `finalizeSnapshotPayload` settles the self-reference; `ratio` is rounded to 4 decimals, `before`/`after` stay exact.
- `interactive: true` keeps uid-bearing lines plus the ancestors that place them and nothing else. It is a flag and never a default — it hides text an agent must read — and fails open to the full view when no uid survives. It never narrows the uid map: the target cache always comes from the full curate pass.
- The four-tool catalog is at 297 of its 300-token budget (locked in tests). A fifth tool does not fit, and neither does a verbose description. Always re-measure external MCP baselines rather than quoting stale numbers (`benchmarks/toolcatalog/`).
- Direct CLI Chrome is detached so separate `snapshot`/`act`/`eval` processes can share a target. `close` must terminate it. Fresh `CAVEMAN_HOME` must work; state writes stay atomic and mode `0600`.
- CDP action acknowledgement is not application settlement. Non-wait actions return `settled:false` and require a focused resnapshot for proof.
- Navigation denies `file:`, `javascript:`, and privileged Chrome schemes.
- **The uid map is `browser_snapshot`'s contract, not a side effect of compression.** On engine pass-through (no recovery handle) `snapshotTool` fails closed with `cave_browser_snapshot_uncompressed` and returns the prior page's uids intact; it must never dump the raw AX tree into `uids` nor wipe the target cache.
- **An `<iframe>` is a leaf, not a broken tree.** `Accessibility.getFullAXTree` returns one frame at a time; the `a11y` compressor treats an unresolvable `childId` as a leaf and curates frame-visible nodes. Keep Chrome's default Site Isolation (`site-per-process`).
