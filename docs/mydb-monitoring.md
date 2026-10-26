# MyDB monitoring

Choose **Collectors → Quick connect → MyDB**, enter the reachable `/metrics` URL and
the dedicated read-only metrics token. Review the actual observations, generated
dashboard and optional sustained rules, then apply. Connection credentials remain
encrypted and write-only. This uses the native HTTP collector, with no exporter
sidecar or Grafana dependency. MyDB targets require HTTPS with a trusted certificate
outside loopback. The MyDB collector and form accept only Bearer credentials and
never forward database usernames/passwords to the HTTP metrics endpoint.
The URL must omit query parameters, keeping credentials out of exported instance labels.

MyDB config:

```yaml
observability:
  metrics_token_file: /run/secrets/mydb-metrics-token
```

The file contains 32–8192 printable non-whitespace ASCII bytes. Restrict file access
to the service identity and operators. Restart MyDB after changing its path or token.
`MYDB_METRICS_TOKEN_FILE` overrides the path. This credential authorizes only
`GET /metrics`; it cannot execute SQL, read admin APIs, modify settings or back up data.
Older session-based management credentials continue to work independently.

The starter dashboard includes only observed series: connection capacity, query/error/
slow-query rates, failed authentication, audit health/rejections, storage/checkpoint
errors and security switches. Fixed metric names avoid SQL text and username labels.
Older MyDB versions do not receive imaginary security panels or alerts.

Optional defaults include two-minute availability, audit/storage errors, persistent
authentication failures, disabled strong-password checks, unenforced TLS and enabled
LOCAL INFILE. Connection capacity warns above 85% for five minutes and is critical
above 95% for two minutes. Recovery requires one minute; notifications have a one-hour
cooldown. Missing/stale data keeps the previous state. No contact point means local
alert events only; generating rules does not prove external notification delivery.
Security-switch rules describe configuration, not evidence of compromise. An enabled
strong-password policy does not prove every historical account satisfies that policy.

MySQL remains a separate `mysql://` SQL collector for MySQL/MariaDB and MyDB's wire
interface. `SHOW GLOBAL STATUS` captures numeric status names without a version
allowlist. The fixed variable query additionally
exports `require_secure_transport`, `local_infile`, `read_only` and `super_read_only`
as 0/1; absent variables are omitted. No application tables or credential catalogs are
read. Use `tls_mode: verify-full` and a least-privilege monitoring account.

MyDB can also push OTLP/HTTP metrics to `/v1/metrics` with the reporting-only token.
That path has different metric names/resource labels and does not automatically create
this pull dashboard. Trace export requires a separate OTLP trace receiver.

Grafana can query the same stored data through a Prometheus datasource with URL
`https://metrics.example.com/prometheus` and a local **Viewer** account over HTTP Basic.
Keep datasource credentials in Grafana secure settings. A reporting-only ingest token
cannot query metrics; a workspace management token grants broader permissions and is
not the recommended Grafana credential. The MyDB scrape token cannot authenticate to
MetricsPanel233. Alternatively, have a standard Prometheus scrape MyDB and use it as
Grafana's datasource. MyDB's `/metrics` is an exposition endpoint, not a Grafana query API.
