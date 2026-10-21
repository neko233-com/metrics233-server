# Contracts

## Product boundary

One binary, one workspace, one SQLite control database. The product is collection + storage + dashboard, not a
distributed multi-tenant platform. Grafana import/export is optional migration support.

## Authentication

- `--auth=token` (default): single workspace Bearer token; non-loopback listeners require at least 16 characters.
- `--auth=local`: users, passwords and browser sessions; first run prints a one-time setup token.
- Roles are workspace-wide: Viewer reads, Editor mutates monitoring resources, Admin controls users, datasources,
  plugins and notification configuration. Unknown mutations fail closed to Admin.
- Session cookie: HttpOnly, SameSite=Lax, Secure under HTTPS, rotates every 24 hours.
- `--trusted-proxies` only trusts `X-Forwarded-For` from the listed CIDRs for rate limiting.
- Password login and account Basic authentication share 8 failures / 5 minutes per client IP; successful
  authentication does not consume the quota. The attempt map is capped at 4096 active client entries.
- Explicit `--root-url` / `METRICSPANEL_ROOT_URL` establishes the local-account public origin behind HTTPS
  termination; preserve the public `Host`. Forwarded protocol headers never establish origin trust.
- `/apis/` namespace routes share the authenticated role boundary; unknown mutations require Admin.
- Public endpoints: `/api/health` (minimal), static SPA assets, login/setup. Everything else requires auth.

## Collection

- Pull: Prometheus/OpenMetrics scrape with 10,000 samples / 4 MiB per scrape, eight concurrent scrapes.
- Push: OTLP/HTTP `POST /v1/metrics` (alias `/otlp/v1/metrics`), protobuf or JSON, gzip, 16 MiB / 10,000 samples.
  Resource attributes become labels; histograms expand to `_bucket`/`_count`/`_sum`.
- Push: Prometheus remote write `POST /api/v1/write` (alias `/prometheus/api/v1/write`), snappy `prompb.WriteRequest`,
  32 MiB compressed / 64 MiB decoded / 100,000 samples; `__name__` selects the metric name.
- Push: native JSON `POST /api/v1/ingest` for scripts and the agent CLI.
- Pull and push share the metric store and the Editor/Admin ingest role policy; Viewer writes return 403.
- Business onboarding (`/#business`) creates native dashboards from received metrics with explicit Gauge/Counter
  semantics and exact label filters. It never injects samples or chooses business alert thresholds.
- Native MySQL/MariaDB, Redis, PostgreSQL and supported service/database metrics APIs.
- Collector secrets are encrypted; responses expose presence flags only.

## Storage and vectors

- SQLite WAL is the zero-configuration default for control data and metrics.
- ClickHouse is optional for high-volume metrics and HNSW vector search.
- SQLite vector search is exact with a 50,000 candidate cap; ClickHouse uses approximate HNSW.
- Retention is a configuration boundary; changing ClickHouse TTL requires an explicit `ALTER TABLE`.
- Native SQLite non-rate queries reduce bucket-last values in SQL (5M source observations); rate retains raw points
  (250k). PromQL never consumes display-decimated observations. See [query planning](../storage-query-planning.md).

## Renderers

- Product primitives live in `web/src/ui`; product renderers in `web/src/render`.
- Table and TimeSeries are the default first-party renderers.
- TimeSeries covers lines, comparison, legend, tooltip, zoom, log axes, percent stacking, multiple axes and
  annotations. Use `?renderer=compat` or localStorage `metricspanel-renderer=compat` to restore the compatibility
  bridge for imported-resource edge cases and troubleshooting.
- Stat, gauge and bar gauge are first-party registry entries sharing `StatRenderer`; text/Markdown remains a
  product-owned HTML path in the panel host.
- Logs are first-party: virtualized rows, search and label key/value filters over Loki frames.
- Traces are first-party for Tempo search and trace-ID lookup: hierarchy, timeline, error attributes, collapse,
  pagination, empty/error/retry and `?traceId=` deep links. Tempo/Loki use the existing authenticated HTTP backend;
  trace/log ingestion and local trace/log storage are outside the metric-store contract.
- Above 50k time/numeric cells, TimeSeries prepares transferable buffers in slices and uses a Worker to normalize,
  stack and preserve min/max/gap boundaries. All series share the soft pixel budget; zoom re-queries. JSON parsing
  and the unavailable-Worker fallback remain main-thread work. See the measured performance baseline.
- Heatmap, histogram and vector are first-party: canvas time-bucket heatmap, normalized bars and 64-dimension heat
  strips. Optional Grafana-only kinds (pie, candlestick, geomap, node graph, state timeline) are out of scope.
- The Grafana bridge is allowed only for imported resources and plugins under `web/src/grafana`.

## Deployment

Bind loopback behind an HTTPS reverse proxy. Set `--root-url` to the public HTTPS origin. Back up the control
database, `identity.db`, `secrets.key` and `plugins/` together. Do not expose the HTTP port directly to the internet.
See `docs/public-deployment.md`.

`Dockerfile` defaults to the small non-root `runtime`; database fixtures and Linux race validation are explicit
targets. Default Compose uses one hardened SQLite service. `compose.clickhouse.yml` explicitly selects ClickHouse;
existing ClickHouse installations must retain that overlay, because backend switching does not migrate samples.
