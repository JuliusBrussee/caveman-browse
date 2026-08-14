# caveman-browse

**A token-efficient alternative to Playwright MCP for coding agents.** One Go
binary that attaches to the Chrome you already run, reads the real
accessibility tree, compresses it, and gives your agent four tools that cost
**297 tokens of catalog** — versus ~13,700 for Playwright MCP and ~17,000 for
Chrome DevTools MCP.

| | caveman-browse | Playwright MCP | Chrome DevTools MCP |
|---|---|---|---|
| Tool-definition cost | **297 tokens** | ~13.7k tokens | ~17k tokens |
| Page representation | compressed a11y tree | aria-YAML (lossy) | a11y snapshot |
| Focused query on a 200-row page | **~98 tokens** | ~15.7k (full ARIA) | full snapshot |
| Byte-exact recovery of the original tree | **yes (CCR)** | no | no |
| Runtime | 1 static Go binary + your Chrome | Node + Chromium | Node + Puppeteer + Chromium |

Numbers are `inferred` token counts measured with an offline `o200k_base`
counter on pinned fixtures — see [BENCHMARK.md](BENCHMARK.md) for methodology,
reproduction commands, and the cases where Playwright's bare ARIA text is
*smaller* (tiny pages; we print those too).

## Install

**Claude Code (plugin — server + usage skill):**

```
/plugin marketplace add JuliusBrussee/caveman-browse
/plugin install caveman-browse@caveman-browse
```

**Claude Code (MCP only):**

```bash
claude mcp add caveman-browse -- npx -y caveman-browse
```

**Cursor / any MCP host** (`.mcp.json` / `mcp.json`):

```json
{ "mcpServers": { "caveman-browse": { "command": "npx", "args": ["-y", "caveman-browse"] } } }
```

**Gemini CLI:**

```bash
gemini extensions install https://github.com/JuliusBrussee/caveman-browse
```

The npm shim downloads a signature-verified prebuilt binary on first run. No
Node runtime is involved after launch; no Chromium is bundled — it drives your
installed Chrome with a dedicated automation profile.

## The four tools

- `browser_snapshot(url?, wait?, query?, interactive?)` — compact accessibility
  tree with `[uid]` action handles plus one accounting line
  (`caveman before=… after=… ratio=… basis=inferred handle=ccr_…`).
  `query` keeps the ≤12 best-matching nodes plus ancestors — the
  token-efficient path on large pages. `interactive: true` keeps only
  actionable nodes.
- `browser_act(action, uid, text?, option?)` — `click`, `type`, `select`,
  `scroll`, `wait`. Returns `settled:false` until a focused re-snapshot proves
  the app updated; the driver never claims success it hasn't seen.
- `browser_eval(expression)` — JS escape hatch.
- `browser_recover(recovery_handle, query?)` — the **byte-exact** original
  `Accessibility.getFullAXTree` payload behind any snapshot (CCR-backed), or
  BM25-narrowed sections with a query. Compression here is never a one-way
  door.

## Direct CLI

```bash
npx caveman-browse snapshot http://127.0.0.1:3000
npx caveman-browse snapshot http://127.0.0.1:3000 "save settings"   # focused
npx caveman-browse snapshot -i http://127.0.0.1:3000                # actionable only
npx caveman-browse act <uid> click
npx caveman-browse recover <handle>
npx caveman-browse close
```

Direct commands share one detached, isolated Chrome until `close`. Navigation
allows `http(s)`, `about:blank`, and bounded `data:text/html`; `file:`,
`javascript:`, and privileged schemes fail closed with `cave_snake_code`
errors.

## Honesty model

- Every savings figure is **`inferred`** (offline BPE count), never provider
  usage, never billing, never re-projected to a period.
- Lossy snapshots are emitted **only** when the byte-exact original is stored
  for recovery (CCR-or-passthrough).
- Unknown roles, uids, or CDP responses fail closed — no fabricated handles,
  no claimed success without settlement proof.
- Scope: same-origin pages with predictable controls. Cross-origin iframes
  appear as leaves; OOPIFs, dialogs, downloads, and arbitrary-open-web
  actionability are out of scope (see the claim boundary in BENCHMARK.md).

## Build from source

```bash
go build ./cmd/caveman-browse
CAVEMAN_BROWSE_CHROME="/path/to/Chrome" go test -tags=integration ./...
```

## License

Source and binaries ship under [Business Source License 1.1](LICENSE)
(source-available; first-party self-hosted production permitted; third-party
hosted/managed/embedded use requires a commercial license). Third-party
notices: [NOTICE](NOTICE). Part of the [Caveman](https://github.com/JuliusBrussee/caveman)
efficiency stack; the a11y compressor and CCR live in the Caveman engine.
