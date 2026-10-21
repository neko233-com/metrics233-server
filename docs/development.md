# Development

Product scope and the Grafana dependency boundary are defined in the [project goals](project-goals.md).

## Requirements

- Go 1.27
- Node.js 24 and npm
- Docker Desktop or Docker Engine for the Docker-backed integration suite

Install the web dependencies once:

```sh
cd web
npm ci
```

## Fast local loop

From the web directory, run:

```sh
npm run dev:workspace
```

The command starts go run ./cmd/metricspanel serve and Vite together. Open the UI at http://127.0.0.1:5173; the API listens on http://127.0.0.1:7333. Vite proxies API, metrics, Prometheus and plugin asset requests to the Go service. React and CSS edits use HMR without rebuilding the embedded bundle. Press Ctrl+C to stop both processes.

For a frontend-only server, use npm run dev; it expects the Go API to be running separately. The production executable embeds the frontend, so rebuild the web bundle before a webui-tagged Go build.

## Validation levels

Start with the smallest check covering the changed behavior:

```sh
# Frontend unit tests and type-check/build
cd web
npm run test:unit
npm run build

# Go unit and package tests
cd ..
go test ./... -count=1
go vet ./...
```

The full CI workflow additionally runs Go race tests, a production webui binary build, Playwright browser tests and Docker integration tests twice. Run Docker integration when the change affects real database/exporter behavior or container orchestration:

```sh
go test -tags=integration ./tests/integration -count=2 -v -timeout=25m
```

Playwright tests need a production frontend bundle and CLI binary first; see the [CI workflow](../.github/workflows/ci.yml) and the root README's automated verification section for the complete setup.

For focused browser checks after a webui build:

```sh
cd web
node scripts/ui-smoke.mjs     # legacy token mode: shell, theme, modal, icon assets
node scripts/auth-smoke.mjs   # local accounts: setup, roles, 403, session, logout
```

`scripts/auth-smoke.mjs` starts the server with `--auth=local` in a temporary directory, reads the one-time setup token from stdout and verifies the full browser flow.

## Linux container verification

```powershell
pwsh -File scripts/dockerfile_test.ps1 -Repeat 2 -Browser -IncludeDatabaseIntegration
```

The script builds the formal production runtime and validates confined token/local containers, business ingest,
query, sessions/roles, restart and offline encrypted backup recovery. Optional browser checks include real SQL
against the database fixture. It removes only resources created by that run; shared build caches and other browser
outputs remain. See [Docker validation](docker-validation.md) for the measured size/memory and their boundaries.

## Change workflow

1. Identify the public route, CLI command or UI action and trace it to the owning package.
2. Check existing tests and focused contract pages before changing persistence, security or Grafana compatibility behavior.
3. Make the smallest change that preserves existing data and API compatibility.
4. Add or update focused tests when behavior changes, and update docs when public behavior or a compatibility claim changes.
5. Run the relevant validation level and report exactly what it covers.

Product UI builds on first-party primitives in `web/src/ui`; reuse them. Grafana plugin hosting and resource conversion stay behind the compatibility
boundary described in the project goals.

## Release binaries

`serve` is the supported single-node deployment path. Build self-contained archives for the common targets with:

```powershell
pwsh -File scripts/build-release.ps1 -Version 0.1.1
```

```sh
VERSION=0.1.1 ./scripts/build-release.sh
```

Both wrappers call `scripts/build-release.mjs`: one `npm ci`/frontend build, then cross-compile with `CGO_ENABLED=0`,
`webui`, readonly modules and trimmed paths. They produce deterministic ZIP archives for all five targets under
`dist/<version>/`, with `SHA256SUMS`, `PROVENANCE.json` and per-archive `BUILDINFO.json`. Output must be a new directory;
existing archives are never deleted. `-OutputDirectory` / `OUTPUT_DIRECTORY` selects another new destination.
See [release checklist](release-checklist.md) for reproducibility, upgrade and external-release gates. Docker is not
required to build archives. Target-platform execution remains a separate acceptance layer from cross-compilation.

Do not commit local databases, encryption keys, .env files, plugin packages, generated web/dist assets or compiled binaries.
