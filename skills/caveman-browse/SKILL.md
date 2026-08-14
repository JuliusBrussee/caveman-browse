---
name: caveman-browse
description: Drive web pages through the caveman-browse MCP tools (browser_snapshot, browser_act, browser_eval, browser_recover). Use whenever interacting with a web page, dashboard, or form through caveman-browse — it teaches the snapshot→act→verify loop, query-first discipline on large pages, and when to recover exact bytes.
---

# Driving pages with caveman-browse

Four tools. The loop is: snapshot → act on a uid → focused re-snapshot to prove
the action settled → recover exact bytes only when compression hid something you
need.

## The loop

1. **`browser_snapshot(url, query?)`** returns a compact accessibility tree:
   `[uid] role "name" = "value" {state}` lines plus one trailing accounting line
   (`caveman before=… after=… ratio=… basis=inferred handle=ccr_…`). uids are
   your action handles. They are valid for the current snapshot generation only.
2. **`browser_act(action, uid, text?, option?)`** — `click`, `type`, `select`,
   `scroll`, `wait`. Non-wait actions return `settled:false`: that is CDP
   dispatch acknowledgement, NOT proof the app updated.
3. **Prove settlement**: re-run `browser_snapshot` with a `query` focused on the
   expected outcome (e.g. the success toast text, the new row). Never chain a
   second action on top of an unproven first one.
4. **`browser_recover(handle, query?)`** returns the byte-exact original AX
   payload for a snapshot handle (or BM25-narrowed sections with `query`). Use
   it when the compressed view seems to be missing something — never guess.

## Token discipline

- **Large or unknown pages: pass `query`.** `browser_snapshot(url, query)`
  keeps at most 12 best-matching nodes plus ancestors. On a 200-row dashboard
  that is ~98 tokens instead of ~12k. Phrase the query with the words you
  expect on the target control ("Save order ORD-0173", "Email Plan").
- **`interactive: true`** keeps only uid-bearing lines plus placing ancestors —
  cheaper than a full snapshot, but it hides page text you may need to read.
  Use it when you already know the page and only need controls. It is never
  the default.
- Full snapshot (no query) is for genuinely unknown task intent only.
- After navigation, old uids are stale; a failed/uncompressed snapshot keeps
  the prior page's uid map and returns `cave_browser_snapshot_uncompressed` —
  do not act on uids after that error until a fresh snapshot succeeds.

## Boundaries (fail closed, don't fight them)

- Navigation allows `http(s)`, `about:blank`, bounded `data:text/html`.
  `file:`, `javascript:`, and privileged Chrome schemes are denied.
- Unknown uids/actions return `cave_snake_code` errors — re-snapshot instead of
  retrying blind.
- Disabled controls are rejected; make the control enabled first (fill the
  form, dismiss the overlay).
- Scope is same-origin, predictable controls. Cross-origin iframes appear as
  leaf nodes; OOPIF content, dialogs, downloads are out of scope — say so
  rather than looping.
- `browser_eval(expression)` is the JS escape hatch for reading state the AX
  tree cannot express. Prefer snapshot queries first.
- Every reported saving is `inferred` (offline BPE count). Never present these
  numbers as provider-verified.
