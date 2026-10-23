# Slow SQL templates and connection metrics

Game and SDK processes report `game_sql_slow_calls_total`, `game_sql_slow_duration_seconds_sum` and
`game_sql_slow_duration_seconds_max` through the existing authenticated metric ingest endpoint. Each series has
`table`, `operation` and a redacted `sql_template` of at most 480 bytes. SDK targets additionally carry
`target_app_id`, `target_channel` and `target_environment`. No SQL is executed by this feature.

The producer observes actual database-driver calls taking at least 200 ms. It excludes pool waiting and application
row consumption. It never reports bound arguments, string/numeric literals, comments, database credentials or
raw errors. Each producer retains at most 128 slow templates; overflow increments
`game_sql_slow_templates_dropped_total`. Unknown/oversized grammar is represented explicitly.

Authenticated Viewer, Editor and Admin users may read
`GET /api/v1/business/instances/{id}/slow-sql?window=5m|15m|1h`. Ingest-only credentials cannot read it.
The instance ID selects the exact organization store and seven-label identity; callers cannot supply arbitrary
PromQL. Queries have a five-second deadline and at most 256 template results. An exceeded limit returns 422.

The receiver redacts templates again, sorts deterministically and groups only within the same target namespace,
physical table and operation. Similarity uses bounded, banded Levenshtein distance with a 90% minimum and
an additional 24-edit maximum. Each candidate is compared to the representative, preventing transitive chains
from merging unrelated statements. This is a diagnostic grouping, not SQL semantic equivalence. Every original
redacted variant remains inspectable. Groups and table summaries sort by cumulative window duration.

Window counts and total duration use Prometheus `increase` and are estimates with counter-reset handling.
At least two observations are needed. The maximum is explicitly the reported process lifetime peak,
not an invented window percentile. An empty response means no usable reports, not proof of no slow queries.
The existing table-level SQL ranking remains available alongside the new template groups.

`http_connections_active` counts accepted connections still owned by the HTTP server; upgraded sockets are
counted by `websocket_connections_active`. HTTP/2 streams contribute requests, not TCP connections.
`http_requests_inflight`, `http_requests_total`, `http_requests_failed_total` (5xx) and
`http_request_duration_seconds_sum` describe handler activity. Inbound WebSockets include client, logic and SDK
links. The SDK's outbound gateway connections remain `sdk_gateway_links_connected`. All counters reset at
process restart. Go reports `go_goroutines`; Rust reports actual `tokio_tasks_alive` and `tokio_workers`.
