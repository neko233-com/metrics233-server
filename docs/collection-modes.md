# Collection modes: pull and push

MetricsPanel233 accepts both collection models at the same time, writing into the same metric store and sharing the
same authentication and role policy.

## Pull

Pull jobs fetch an endpoint on an interval.

```sh
metricspanel targets add --name node --url http://127.0.0.1:9100/metrics --interval 15s
metricspanel targets add --name mysql --url http://127.0.0.1:9104/metrics --interval 30s
metricspanel targets add --file examples/collectors/redis.json
```

- Prometheus text, OpenMetrics 1.0 and delimited protobuf, including classic histograms and summaries.
- Native MySQL/MariaDB, Redis, PostgreSQL, ClickHouse, Elasticsearch, Hadoop/YARN/HDFS, Hive/Kafka, Spark and Flink.
- Limits: 10,000 samples and 4 MiB per scrape, eight concurrent scrapes.
- The direct text parser enforces the same 4 MiB/10,000-sample limits as HTTP scraping. HTTP requests have a
  10-second deadline, support encrypted Basic/Bearer connection settings, and do not follow redirects.
- Target `job`/`instance` and configured labels have the same precedence for text, OpenMetrics and protobuf.
  Protocol `le`/`quantile` labels retain their bucket/quantile identity even if a target config contains those keys.
  Classic text histograms reject missing counts, decreasing bucket counts and an inconsistent `+Inf` bucket.
- A failed scrape does not ingest a valid prefix of its payload. It still records `up=0`, duration, sample count
  and the target error using a short independent context, including when the HTTP deadline is exceeded.
- Management: `POST /api/v1/targets`, `PUT /api/v1/targets/{id}`, `POST /api/v1/targets/{id}/scrape`.

## Push

In token mode, give reporting agents the dedicated `METRICSPANEL_INGEST_TOKEN`, configured separately from the
private management `METRICSPANEL_TOKEN`. The dedicated credential can write only the five ingest endpoints and
cannot query metrics or undo source-IP restrictions. Use one `Authorization: Bearer` header. See [reporting access](reporting-access.md)
for startup constraints, trusted proxies, source policies and failure semantics; local mode does not accept this setting.

### Prometheus remote write

`POST /api/v1/write` (alias `POST /prometheus/api/v1/write`).

```yaml
remote_write:
  - url: https://metrics.example.com/api/v1/write
    authorization:
      credentials: <ingest-only-token>
    queue_config:
      max_samples_per_send: 10000
```

- Body: snappy-compressed `prompb.WriteRequest` (`Content-Encoding: snappy`).
- Raw protobuf without `Content-Encoding` is accepted for simple senders.
- Limits: 32 MiB compressed, 64 MiB decoded, 100,000 samples per request, stored in 10,000-sample batches. The snappy
  decoded length is checked before allocating/decompressing its output.
- `__name__` becomes the metric name and is removed from labels. Any sample-bearing series with a missing or empty
  `__name__` rejects the complete request with `400` before storage; it is never silently discarded. Empty requests,
  metadata-only requests and series without samples remain accepted with zero stored samples.
- `X-Accepted-Samples` reports samples in completed storage batches, including on failure; success is `204 No Content`.
- Invalid snappy/protobuf returns `400`; unsupported encoding returns `415`; oversized bodies return `413`.
- This is a **finite classic-sample subset** of the v1 wire format, not full Remote Write 1.0 conformance. Native
  histogram points and the standard stale marker (`0x7ff0000000000002`) return explicit `400` errors: the metric store
  does not yet represent their semantics. Other NaN/Inf values also return `400`. Metadata and exemplars are not stored.
- Remote write v2 (`io.prometheus.write.v2.Request`) is not implemented. Its protobuf media-type parameter returns
  `415`; use the v1 `prometheus.WriteRequest` schema with finite classic bucket series.
- Every accepted series/sample is checked before the first storage call, including duplicate label names and the
  shared store's name/label/timestamp/value constraints. A malformed sample after position 10,000 therefore cannot
  commit the first batch and then return `400`.
- Storage failures return retryable `503`, not a permanent-input error. A request spanning multiple storage batches
  is **not atomic across batches**; its response reports the last completed batch count. Retry the whole request with
  stable timestamps. SQLite replaces the same series/timestamp on retry; another backend's failed-call outcome may
  be ambiguous. This endpoint does not claim a distributed or cross-backend transaction.

### OTLP/HTTP

`POST /v1/metrics` (alias `/otlp/v1/metrics`), protobuf (`application/x-protobuf`) or JSON (`application/json`). Gzip,
identity and unencoded bodies are accepted; unknown encodings/media types return `415`. Malformed JSON/protobuf/gzip
returns `400`; both compressed and decoded bodies are bounded to 16 MiB and oversized bodies return `413`. Unknown
JSON fields are ignored for OTLP forward compatibility.

The receiver maps resource attributes to labels (`service.name` → `service_name`) and scope name/version to
`otel_scope_name`/`otel_scope_version`. Point attributes override matching resource labels. Empty, unsupported or
colliding normalized attribute names reject the original point; all derived samples also use the store's common
name, label-count/length, finite-value and timestamp validation. As a compatibility behavior, timestamp zero uses
server time; provided OTLP nanoseconds are converted to Unix milliseconds.

- Supported: gauge, **cumulative** sum, **cumulative explicit** histogram, and summary. Delta/unspecified sum or
  histogram temporality, exponential histograms and no-recorded-value/staleness markers are explicitly rejected
  through the OTLP partial-success response rather than being silently acknowledged or misread as cumulative counters.
- Histogram bucket counts must match the bounds shape and total count without uint64 overflow. Explicit bounds must
  be finite and strictly increasing. The receiver converts per-bucket counts to cumulative classic buckets and preserves
  the terminal `le="+Inf"` identity. A missing optional sum does **not** create a fabricated `_sum=0` series. A histogram
  with no distribution may contain only count and optional sum. Summary quantiles must be ordered within `[0,1]`.
- The request budget is **10,000 expanded storage samples in total**, including histogram buckets/count/sum and
  summary quantiles/count/sum. An omitted histogram sum consumes no budget: 9,999 buckets plus count is exactly
  10,000 samples and is accepted. This makes the previously stated 10,000-sample batch budget an enforced request limit;
  it is not raised to remote write's larger budget. An oversized request returns `413` before any storage call.
- Valid points can be accepted while invalid/unsupported points are rejected. HTTP `200` then includes
  `ExportMetricsServiceResponse.partial_success.rejected_data_points` and an English reason. The count is **original
  OTLP data points**, not expanded bucket series; a rejected histogram/summary writes none of its derived series.
  Fully rejected but decodable data also returns a populated partial-success response. Clients must not retry a
  partial-success response; correct the export schema/temporality instead.
- All conversions/validation finish before one bounded storage call. On SQLite that call commits one transaction;
  an input rejection cannot leave a valid prefix committed. Storage failure returns `503` with a standard OTLP
  `google.rpc.Status` body, and `X-Accepted-Samples` remains `0` until the storage call succeeds. Decode/limit/media
  errors also use OTLP Status JSON/protobuf, matching the request encoding. Authentication middleware retains the
  application's existing authentication-error response.

The conversion follows the [OTLP partial-success contract](https://opentelemetry.io/docs/specs/otlp/#partial-success)
and [metrics protobuf schema](https://github.com/open-telemetry/opentelemetry-proto/blob/main/opentelemetry/proto/metrics/v1/metrics.proto)
within these explicit supported-type and finite-store boundaries.

### Native JSON

`POST /api/v1/ingest`:

```json
{"samples":[{"name":"business_orders","labels":{"service":"api"},"value":233,"timestamp":1730000000000}]}
```

Timestamps are Unix milliseconds; `0` uses server time. This is the simplest path for scripts and the agent CLI.

## Roles

For application metrics, see the [business monitoring workflow](business-monitoring.md): discover received metrics,
choose Gauge/Counter semantics, scope labels and create a first-party dashboard without importing a template.

Remote write, OTLP and native JSON are Editor operations. Viewer credentials receive `403`; Admin is also allowed.
All three routes require the same authentication as the rest of the API, including the workspace token on non-loopback
listeners and local-account sessions/Basic auth in `--auth=local` mode.

## Verification

```sh
go test ./internal/collector ./internal/server ./internal/localauth -count=1
go test ./internal/localauth -count=1
```

Receiver tests cover both remote-write aliases, snappy/raw protobuf, decoded-size preflight, v2/native-histogram/stale
rejection, duplicate labels, missing/empty metric names after a valid first batch, empty/metadata-only requests,
a malformed point beyond the first batch and a simulated second-batch storage failure.
OTLP tests cover protobuf/JSON/gzip, unknown JSON fields, transport limits/errors, original-point partial counts,
histogram schema/overflow/optional sum, generated label identity, attribute collisions, request sample budgets and a
simulated storage outage. Collector tests use real HTTP fixtures for encrypted Bearer credentials, rejection,
redirect refusal and cancellation. Existing role tests cover Viewer denial and Editor access. These are protocol,
SQLite and local HTTP proofs; they are not a long-running external exporter or production-load acceptance.
