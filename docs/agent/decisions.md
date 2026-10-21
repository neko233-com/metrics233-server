# Decisions

## 2026-10-20 — final product target

The product is an out-of-the-box single-node monitoring appliance: **collection + time-series/vector storage +
dashboards**. It replaces the usual Grafana + separate scraper + separate vector database stack for small teams.
It is not a Grafana clone and does not require a Grafana server, Grafana UI or plugin ecosystem.

## 2026-10-20 — UI ownership

Product shell, forms, dialogs and navigation use `web/src/ui`. Grafana packages are confined to optional
import/plugin compatibility. The product navigation no longer advertises Grafana plugin management.

## 2026-10-20 — renderer migration is staged

The first-party renderer framework is in `web/src/render`:

- Own Table and TimeSeries are the defaults. The workspace time-range/date-math/zoom contract passed once the own
  legend exposed `data-testid="series-icon"` to the canvas assertion harness; the full browser suite is green with
  the own renderer (67 passed / 2 skipped).
- `?renderer=compat` or localStorage `metricspanel-renderer=compat` restores the compatibility bridge for
  imported-resource edge cases and troubleshooting. Do not re-flip the default without the full suite.
- `frame-budget.ts` and `incremental.ts` exist for cooperative slicing and model reuse; Worker decimation is next.

## 2026-10-20 — no push-triggered CI

The full validation job is expensive. It runs only for `v*` tags or manual dispatch. Local verification is the
normal loop; tag releases run the full race/browser/Docker matrix.

## 2026-10-20 — push and pull are both first-class

Scrape pull and push receivers share one metric store and one ingest role policy. Push covers Prometheus remote
write (`/api/v1/write`, snappy `prompb.WriteRequest`), OTLP/HTTP and native JSON; pull covers scrape jobs and native
SQL/Redis/service collectors. Neither mode is a compatibility afterthought, and remote write v2 is explicitly not
implemented until a sender requires it.

## 2026-10-20 — collector scope

Prioritize the game-backend set: MySQL/MariaDB, Redis, Linux/Prometheus exporters and business-service exports.
Loki/Tempo adapters exist but are not the primary product target until the core dashboard loop is complete.

## 2026-10-21 — bounded production verification

The default Docker build must produce the same non-root `runtime` used in production. Databases and test fixture
binaries require explicit `integration`/`test-backend` targets; `validation` supplies Linux GCC/race checks. Tests own
their containers, volumes and temporary tags and never prune shared Docker caches.

Business onboarding uses the existing ingest/query/dashboard roles and first-party renderer registry. Counter vs.
Gauge is explicit because catalog names alone cannot establish metric semantics. DAU, retention, currency conversion
and alert thresholds remain upstream business definitions.

The TimeSeries Worker shares a min/max row budget across series, preserves extrema and outage boundaries, and
re-queries on zoom. This does not remove JSON parsing cost or imply a universal frame rate. See the measured baseline.

Shared plugin managers canonicalize the root at file verification time as well as construction time. A manager
created before another manager initializes the shared directory must accept an equivalent Windows path alias while
still rejecting symlinks that resolve outside the package. The real Live isolation test and package escape tests
cover this boundary.
