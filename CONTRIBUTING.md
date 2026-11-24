# Contributing to caveman-browse

Thank you for contributing to `JuliusBrussee/caveman-browse`. Read
[`AGENTS.md`](AGENTS.md) for invariants and token-budget locks before opening a
pull request.

## Prerequisites

- Go 1.23+ (for unit tests; Chrome/Chromium required only for `-tags=integration`)
- Node.js 20+ (for npm shim / installer tests)

## Verification

```bash
go vet ./...
go test ./...
```

To run real Chrome CDP integration tests:

```bash
CAVEMAN_BROWSE_CHROME=/path/to/chrome go test -tags=integration ./...
```

## Guidelines

- Preserve the 300-token four-tool catalog budget and CCR byte-exact recovery invariants.
- Keep `inferred` savings labels honest; never emit `verified`.
- Report security issues privately per [`SECURITY.md`](SECURITY.md).
