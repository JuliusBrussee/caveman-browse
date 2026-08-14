# caveman-browse

Browser driver over MCP. Four tools: `browser_snapshot`, `browser_act`,
`browser_eval`, `browser_recover`.

Loop: snapshot → act on `[uid]` → focused re-snapshot to prove the action
settled → recover exact bytes only when the compressed view hid something.

- Large pages: always pass `query` to `browser_snapshot` (keeps ≤12 best
  matches plus ancestors; ~98 tokens instead of ~12k on a 200-row page).
- `settled:false` from `browser_act` = CDP dispatch only, not app state; prove
  outcomes with a focused re-snapshot before the next action.
- uids are per-snapshot; after navigation or a `cave_browser_snapshot_uncompressed`
  error, take a fresh snapshot before acting.
- Denied by design: `file:`/`javascript:`/privileged schemes, disabled
  controls, unknown uids — errors carry `cave_snake_code`, don't retry blind.
- All savings figures are `inferred` (offline token count), never verified.
