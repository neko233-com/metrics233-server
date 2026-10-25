# Real Tempo interoperability

Metrics233 queries a configured Tempo service through its authenticated datasource backend. Tempo owns trace
ingestion/storage; Metrics233 owns search, span hierarchy, waterfall, status and attributes. No external datasource
plugin is required. This does not add local trace storage, service graphs, TraceQL metrics or v2 partial traces.

## Reproduce

Build the first-party UI and install the repository's Playwright Chromium, then run from the server module:

```sh
cd web
npm ci
npm run build
npx playwright install chromium
cd ..
METRICSPANEL_TEMPO_BROWSER_TEST=1 go test -tags=integration,webui ./tests/integration \
  -run '^TestTempoLiveDataSource$' -count=2 -v -timeout=6m
```

PowerShell: set `$env:METRICSPANEL_TEMPO_BROWSER_TEST='1'` and quote `'-tags=integration,webui'`.
Docker is required. Without the browser flag the integration test still runs the real daemon/API path. The
standalone browser case explicitly skips without its owned fixture; that skip is not counted as acceptance.
The hosted test job runs the live browser contract twice after browser setup. Tag/manual triggering is unchanged.

The fixture pins official `grafana/tempo:3.1.1` to
`sha256:64e2fbd56d9c2b3412a4b430a58ea59e91a670b1e75f920a0c702213ad145044`.
The minimal configuration follows the [upstream v3.1.1 example](https://github.com/grafana/tempo/blob/v3.1.1/example/docker-compose/single-binary/tempo.yaml)
and [Tempo HTTP API](https://grafana.com/docs/tempo/latest/api_docs/).
`METRICSPANEL_TEST_TEMPO_IMAGE` permits an operator-controlled mirror of the same immutable image.

## Contract and isolation

- Actual OTLP/HTTP export writes three spans across two services into Tempo. Random trace IDs isolate runs.
  TraceQL search and trace-ID lookup go through Metrics233; no recorded query response replaces the daemon.
- Parent/child IDs, service names, 120.5/25.25/5 ms durations, database error/status message and missing-trace empty
  result are verified. Anonymous datasource queries return HTTP 401.
- The browser opens the search result, error attributes and waterfall, collapses/restores descendants, reloads
  the deep link and traverses history. Containment at 390/768/1280 px passes without retries.
- Existing deterministic tests retain malformed/oversized response, cancellation and 10,000-span-bound coverage.

The daemon is nonroot/read-only, has no capabilities, uses no-new-privileges, and has 1 CPU/512 MiB plus bounded
ephemeral storage. Only random loopback HTTP/OTLP ports are published; the monitor also binds loopback. Cleanup
checks the exact created container ID and ownership label. No shared images or production configuration are removed.
This is an interoperability workload, not a throughput or production sizing claim.

Optional `METRICSPANEL_TEMPO_QA_READY_FILE` names a new file below the resolved OS temp directory. After API/browser
checks, the test writes only its local URL/dashboard/trace ID, then waits up to five minutes for `<file>.done`.
This allows Edge to inspect the same actual data without storing credentials. Windows short/long path aliases
resolve before checking that boundary. Close the temporary page before signalling completion; include the hold
in the main test timeout.

## Evidence — 2026-10-26

Windows Go 1.27 plus actual Linux Tempo 3.1.1 passed two API/zero-retry browser rounds in 20.512s (2.6s browser per
round). Edge separately displayed three spans, the database error, +10ms/25ms waterfall and error attributes, with
no captured console warnings/errors. That held run passed in 90.109s, including inspection wait. Integration vet,
TypeScript and the original `TestTempoTrace*` regressions passed (6.038s). Owned containers were removed.

An initial fixture omitted the dashboard aggregation field; a later receipt draft rejected Windows's short temp
alias. Both failed attempts remain fixture failures and were corrected before the final repeated checks.
Only tests/CI/docs change. Production remains v0.2.30; this is not a production Tempo deployment or a new game/server
binary rollout. Hosted results for the new verification source are recorded in [status](status.md).
