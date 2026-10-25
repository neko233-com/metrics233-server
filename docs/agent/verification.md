# Verification

Run the narrowest check that covers the change; do not infer browser or Docker acceptance from a unit test.

## Fast loop

```sh
cd web
npm run test:unit
npm run build
cd ..
go vet ./...
go test ./internal/localauth ./internal/server ./internal/store ./cmd/metricspanel -count=1
```

Targeted browser checks after a webui build:

```sh
cd web
node scripts/auth-smoke.mjs
node scripts/ui-smoke.mjs
node scripts/render-bench.mjs
```

## Full browser suite

```sh
cd web
npx playwright test
```

The suite is intentionally not triggered by every push. It covers dashboards, variables, alerts, plugins, OTLP/
collector forms, Import, Table and TimeSeries contracts, plus responsive containment at 390/768/1280 widths and
dynamic panel height. Fix persistent failures; a single retry is configured for known long-dialog timing sensitivity.

## Docker Linux

```sh
pwsh -File scripts/dockerfile_test.ps1 -Repeat 2 -Browser -IncludeDatabaseIntegration
```

The script builds the formal `runtime` target, verifies hardened token/local containers, business metric protocols,
roles, restart and backup/restore, and can run the database integration suite. `docker-verify.ps1` remains a compatible
entry point. It removes only resources created by that run; it never prunes global images or build caches, or deletes
other browser test results. See [Docker validation](../docker-validation.md) for budgets and evidence boundaries.

## Release CI

The [real Tempo contract](../tempo-live-validation.md) pins an upstream daemon and checks actual OTLP ingestion,
TraceQL and trace-ID detail. `METRICSPANEL_TEMPO_BROWSER_TEST=1` with the `webui` build tag also drives the dashboard
without retries. The test job runs it twice after browser setup; Docker integration separately covers the API path.
No production datasource or credentials are needed.

`.github/workflows/ci.yml` runs only on `v*` tags or manual dispatch. Push/PR events do not start the expensive race,
browser and Docker jobs. Tag releases still run the full matrix.

## Cleanup expectations

- `web/test-results/` and `web/playwright-report/` are local only.
- `bin/`, `web/dist/`, `data/`, `.env`, `*.db`, `identity.db` and `.codely-cli/` stay out of git.
- After Docker verification, labeled containers/volumes/images must be zero.
