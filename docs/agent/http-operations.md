# Agent HTTP operations

[Quick connect](../quick-connect.md) adds Admin-only read-only `/api/v1/connections/preview` and reviewed atomic
`/api/v1/connections/apply`, using a stable operation UUID plus returned plan hash for safe uncertain retries.
Existing target settings are read and fenced internally; do not resend masked credentials. GET receipts is Viewer;
`discover` checks only fixed server-loopback exporter ports. CLI exposes `connections list|discover|preview|apply`.

`GET /api/v1/agent/capabilities` is an authenticated, versioned discovery document for automation. It includes a curated native operation catalog, role requirements, concurrency conditions, selected JSON request schemas and the actual protected route registrations. It is not a claim of complete OpenAPI coverage for compatibility/plugin APIs. The same HTTP operations power the browser; agents do not need browser automation to configure monitoring.

For local accounts, inspect `GET /api/auth/status`, then use `POST /login` with JSON `{"user":"admin","password":"<provided privately>"}` and a private cookie jar. Use HTTPS with the configured same origin. Never put credentials in shell history, source, logs or reports. Local account HTTP Basic is also supported over HTTPS and shares password failure limits. Choose the least account role needed. The ingest Bearer credential cannot read this discovery document or configure the server.

Read before changing. Preserve resource versions, use `If-Match` for native panel edits, and treat 409 as a request to re-read and decide again. Collector target writes currently have no CAS: read, back up privately, read again, write once and verify; an uncertain response is not permission to retry. Saved credentials are masked/write-only: omit secret changes when retaining a contact point or collector. Do not copy a masked response back as a secret.

Native panel `PATCH /api/v1/dashboards/{id}/panels` accepts optional `panel.display_unit`: omission retains a custom display unit, explicit `""` restores the base-unit mapping, and `"binBps"` requires `panel.unit="bytes"`; only `config.fieldConfig.defaults.unit` changes, preserving other configuration and the existing permission/revision boundary.

State panels (`state-timeline`, `status-history`) accept optional `panel.state_mapping`: `"availability"` maps 0/1 to red/green Unavailable/Available, omission preserves custom mappings, and `""` explicitly clears them. The change is limited to `fieldConfig.defaults.mappings` and retains the same permission/CAS checks. See [native panel types](../native-panel-types.md).

Preview alert queries and Feishu templates through their dedicated preview endpoints. Rule `evaluate`, scrape, contact-point test and notification delivery can have runtime/external effects and are not previews. A queued notification is not a delivered notification; inspect delivery state and the provider ACK separately.

Configure pending duration, recovery duration, no-data/error behavior and notification cooldown explicitly. A dashboard risk color is a display threshold, not an alert trigger. Missing metrics do not prove recovery or zero utilization. Business instance labels identify observations and fix template variables; they never grant authentication, source allowlisting or dashboard ACLs.

After saving, read the resource back, verify current measurements, then verify real notification delivery separately when authorized. Record the resource versions and non-sensitive result, not credential values. SQLite retention can be changed through its versioned endpoint; ClickHouse TTL changes remain an explicit database operation.
