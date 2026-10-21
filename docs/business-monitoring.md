# Business monitoring

For discovered gateway/game instances, live template inheritance, independent
dashboards, complete player-region tables and actual Entity SQL counters, see
[Business instance dashboards](business-instances.md).

Business metrics use the same durable store, PromQL engine, dashboard and alert paths as infrastructure metrics.
Open **Business monitoring** in the left navigation (`/#business`) to connect an application and build a dashboard
from metrics actually received by the workspace. The page never inserts demonstration samples.

## Connect

Choose the protocol already supported by the application:

| Application integration | Receiver |
| --- | --- |
| Prometheus/OpenMetrics exporter | Add its `/metrics` URL under Collectors |
| OpenTelemetry metrics SDK | OTLP/HTTP `POST /v1/metrics` (alias `/otlp/v1/metrics`) |
| Prometheus remote write v1 | `POST /api/v1/write` |
| Scripts or JSON-first clients | `POST /api/v1/ingest` |

For Go applications, [metrics233-lib-go](https://github.com/neko233-com/metrics233-lib-go) provides a standalone,
standard-library-only client: `client.WithLabels(labels).Counter(name).Inc()` and `Gauge(name).Set(value)` record
locally; `Push(ctx)` sends a snapshot. `Batch().Counter(name, delta).Gauge(name, value).Commit()` atomically records
a group without network I/O. Start `Run` explicitly in an application-owned goroutine and call `Close` with a fresh
bounded shutdown context. The SDK documents queue/series limits, retries and cumulative Counter semantics; it
does not add automatic initialization or require importing server packages.

Use workspace authentication and `Content-Type: application/json` for native JSON. A payload example:

```json
{"samples":[{"name":"business_active_users","labels":{"service":"shop","environment":"production"},"value":42,"timestamp":0}]}
```

`timestamp` is Unix milliseconds; `0` uses server time. Send actual values periodically, such as every 15–30 seconds.
The UI provides a copyable format example without credentials. See [collection modes](collection-modes.md) for limits
and encoding. Ingest requires Editor or Admin; a Viewer can inspect metrics and existing dashboards.

## Choose semantics

- **Gauge** records the current value: online users, queue depth, inventory or concurrent jobs. The generated dashboard
  shows `sum(metric)` plus the underlying series. Only sum disjoint populations; use the panel editor if summing is
  not meaningful for the chosen metric.
- **Counter** records a cumulative monotonic total: completed orders, errors or payment events. The dashboard shows
  the current accumulated total, `sum(rate(metric[5m]))` in events per second and the underlying series. PromQL handles
  counter resets; a useful rate requires at least two samples inside the range. `_total` suggests Counter, but the
  user can explicitly change the type because the metric catalog does not store type metadata.
- Scope the dashboard with exact label equality, for example `{"environment":"production","service":"shop"}`.
  The UI validates label names and JSON string values and safely quotes PromQL matcher values.
- Keep cardinality bounded. Service, region, environment and a small outcome set are suitable labels; user IDs, order
  IDs, request IDs and secrets are unsuitable. A time-series database is not an event or transaction ledger.
- Name revenue metrics with their currency and base units (for example `business_revenue_usd_cents_total`). Configure
  appropriate display units in the panel editor. Do not mix currencies in a sum. Financial reporting remains upstream.
- DAU, retention and conversion need explicit upstream deduplication, windows and denominators. An online-user gauge
  cannot establish those metrics on its own.

## Dashboard and alert

Select an existing metric, confirm Gauge/Counter and optionally set a dashboard name and label filters. **Create
business dashboard** writes a normal native dashboard with current total, trend and per-series table. Creation uses
the existing `/api/v1/dashboards` role policy; refreshing or reopening the deep link loads the saved dashboard.

Use **Configure alerts** for thresholds, pending periods and contact routing. Thresholds are business decisions and
are not created automatically. Missing data is not automatically converted to zero; configure NoData and Error
behavior in the alert editor according to the workload.

## Verification boundary

`web/e2e/business-monitoring.spec.ts` exercises real JSON ingestion, discovery, gauge/counter dashboards, label-scoped
values, persistence after a browser reload, malformed filters and 390/768/1280 containment against the Go webui binary.
The Docker runtime tests separately cover ingestion and durable recovery on Linux. Test values are synthetic; these
checks do not verify a particular application's instrumentation or its production business definitions.
