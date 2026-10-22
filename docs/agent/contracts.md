# Contracts

## Product boundary

One binary, one workspace, one SQLite control database. The product is collection + storage + dashboard, not a
distributed multi-tenant platform. Grafana import/export is optional migration support.

## Authentication

- `--auth=token` (default): single workspace Bearer token; non-loopback listeners require at least 16 characters.
- `METRICSPANEL_INGEST_TOKEN` optionally separates reporting from management in token or local mode: environment-only,
  32–8192 printable non-whitespace ASCII characters. Token mode requires a distinct 16+ character management token.
  It permits only POST to the five ingest paths, never query/management/source-policy/Live/session operations;
  source IP policy still applies. Local mode assigns only a scoped Editor system principal, rejects custom-authorizer
  bypass, and never authenticates reads or account operations. See [reporting access](../reporting-access.md).
- `--auth=local`: users, passwords and browser sessions; first run prints a one-time setup token. Production uses
  preinitialized accounts and clears `METRICSPANEL_TOKEN`, which otherwise remains a legacy Admin credential.
  Linux root can run offline `bootstrap-admin --identity-db /absolute/protected/identity.db --login admin` with
  hidden password confirmation before serving; existing accounts are never overwritten. See [local accounts](../local-auth.md).
- Roles are workspace-wide: Viewer reads, Editor mutates monitoring resources, Admin controls users, datasources,
  plugins and notification configuration. Unknown mutations fail closed to Admin.
- Session cookie: HttpOnly, SameSite=Lax, Secure under HTTPS, rotates every 24 hours.
- `--trusted-proxies` only trusts `X-Forwarded-For` from the listed CIDRs for rate limiting and reporting-source IP policy.
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
- Managed-host SSH and credentials are Admin-only. Independent outbound host credentials grant only own-configuration
  reads and own-metric/heartbeat writes. Installation, applied settings, heartbeat and source data are separate gates.
  See [managed hosts](../managed-hosts.md); private networks need a reachable LAN installation executor.
- `/#reporting-sources` observes actual accepted push batches, independently of scrape targets and internal self-metrics.
  All five ingest paths use canonical client-IP policy before parsing or writing; labels never authorize a source.
  Admin-only versioned policy supports block/allow/reset and explicit allowlist-only mode. Observations (1024 LRU)
  and policies (4096, never observation-evicted) persist in each organization's catalog. See [reporting sources](../reporting-sources.md).

## Notifications

- Native contact points support generic Webhook and Feishu custom-bot v2 interactive cards (default), with explicit `feishu_format: text` compatibility.
- Feishu URL and optional signing secret are encrypted and write-only; public metadata exposes presence flags.
- Sending requires HTTP 2xx plus an explicit JSON `code: 0`; redirects, invalid ACKs and timeouts fail delivery.
- Local-mode configuration/test mutations require Admin. Queue acceptance (202) is separate from delivered state.
- Admin-only contact-point `POST .../preview` renders fixed sample data without saving, evaluating, queuing or sending. Existing saved title/message templates are rendered once under their process-local step/output/time budgets; card data is plain text, with only a configured HTTPS workspace action link. Default cards omit raw PromQL/internal labels and distinguish condition-zero recovery from measured zero resource use.
- Rule `notification_cooldown_seconds` (0–604800; old API default 0, new UI default 3600) imposes a durable
  successful-firing-ACK minimum interval, including explicit reminders: repeat is at least max(CD, policy repeat).
  Pending/recovery periods and failed-delivery retries remain independent; repeat 0 still disables reminders.
- Unacknowledged cooldown periods cannot emit orphan recovery; restart, pause/edit and disable/enable preserve ACKs.
- See [Feishu notifications](../feishu-notifications.md) for API, secret-update and verification boundaries.
- See [notification cooldown](../alert-notification-cooldown.md) for exact timing and retention semantics.

## Storage and vectors

- Embedded compressed TSDB (XOR chunks/Zstd WAL) and a local Badger/HNSW vector database are the metric default;
  SQLite WAL stores control data separately. Explicit SQLite metrics remain a legacy compatibility backend.
- Existing metrics require offline backed-up, exact-fingerprint-verified migration before native cutover.
  See [embedded storage](../embedded-storage.md) for quotas, disk accounting, durability and rollback boundaries.
- ClickHouse is optional for high-volume metrics and HNSW vector search.
- SQLite vector search is exact with a 50,000 candidate cap; ClickHouse uses approximate HNSW.
- Metric retention defaults to 15 days. Embedded and SQLite Admins can save a versioned 1–3650 day setting under Collectors; startup/hourly cleanup uses that setting. Monitoring/account configuration remains. ClickHouse settings remain read-only; changing its TTL requires an explicit `ALTER TABLE`. See [data retention](../data-retention.md).
- Native SQLite non-rate queries reduce bucket-last values in SQL (5M source observations); rate retains raw points
  (250k). PromQL never consumes display-decimated observations. See [query planning](../storage-query-planning.md).

## Renderers

- First-party metric panels use readable names with explicit aliases preserved; original labels and query/reducer
  metadata are available through the keyboard-accessible Metric details dialog. Friendly names never become
  query or authorization identities. Unknown metrics and aggregate expression meanings are not guessed. See
  [panel readability](../panel-readability.md).
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
- Heatmap, histogram and vector remain first-party. The editor also offers native bar/pie charts and state
  timeline/status history, with bounded models, virtualized category/state rows, missing-data gaps and explicit
  availability mappings. Histogram distinguishes raw distributions from classic cumulative `le` buckets.
  See [native panel types](../native-panel-types.md) for the full catalog comparison and the dedicated financial,
  graph/profiling/map/layout/widget types that have no verified native implementation.
- The Grafana bridge is allowed only for imported resources and plugins under `web/src/grafana`.

## Deployment

Bind loopback behind an HTTPS reverse proxy. Set `--root-url` to the public HTTPS origin. Back up the control
database, `identity.db`, `secrets.key` and `plugins/` together. Do not expose the HTTP port directly to the internet.
See `docs/public-deployment.md`.

`Dockerfile` defaults to the small non-root `runtime`; database fixtures and Linux race validation are explicit
targets. Default Compose uses one hardened embedded service. `compose.clickhouse.yml` explicitly selects ClickHouse;
existing ClickHouse installations must retain that overlay, because backend switching does not migrate samples.
