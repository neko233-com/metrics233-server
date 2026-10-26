# Quick connect

**Collectors → Quick connect** tests a real database/exporter, previews its observed-metric dashboard and
optional sustained alerts, then saves the collector, dashboard, rules and operation receipt in one transaction.
**Dashboards → Infrastructure** lists the resulting resource cards separately from business services and shared
templates. GitHub Dark console tokens stay independent of time-series and gauge palettes.

## User flow

1. Enter the actual service address, type and credentials; choose sustained alerts and an existing contact point,
   or keep the existing notification policy.
2. Test the actual connection. Review observed metric count, panels, pending/recovery periods and notification CD.
3. Apply once and open the dashboard. Ordinary collection supplies its data; preview observations are never ingested.
   Rate panels require at least two source observations.

Linux/Node, Windows, MyDB, MySQL, Redis and PostgreSQL receive curated panels only when their required metrics were
observed. Exporter runtime internals stay in Explore instead of filling a host-health dashboard. Other implemented
protocols receive a generic starter view bounded to 24 panels, without inferring arbitrary business semantics.
All collected metrics remain queryable beyond this starter-panel limit.
Host starters put CPU/memory instruments first, use a full-width filesystem view and larger paired network
panels, and place collection status after resource data. Native gauges size with panel height rather than a
90-pixel ceiling. Existing dashboards retain their authored positions; use the versioned layout editor to change them.

Existing target cards can generate a dashboard using protected saved settings. Their connection, labels and
credentials remain unchanged. Overlapping target identities are rejected. Retries never overwrite customized
dashboards or recreate removed resources.
The existing-card action creates panels only, avoiding duplicate alerts alongside existing rules. Agents may
explicitly include reviewed sustained rules using the same API; new Quick connect forms offer this choice.

Exporter discovery checks only fixed loopback ports on the **monitoring server**, after an explicit Admin action.
It does not scan a subnet, guess database passwords, install agents or identify the browser computer. Remote/LAN
machines need a reachable collector address or the separately authenticated [outbound host agent](managed-hosts.md).

## Sustained defaults

| Observed resource | Trigger | Pending | Recovery confirmation | Notification CD |
| --- | --- | --- | --- | --- |
| Collection | Failed collection | 2 min | 1 min | 1 h |
| CPU | >85% warning / >95% critical | 10 / 5 min | 1 min | 1 h |
| Memory | >90% warning / >95% critical | 10 / 5 min | 1 min | 1 h |
| Disk | >85% warning / >95% critical | 15 / 5 min | 1 min | 1 h |
| MySQL connection capacity | >85% warning / >95% critical | 5 / 2 min | 1 min | 1 h |

Resource rules use recovery hysteresis and fresh source observations. Missing data does not mean zero or recovery;
no-data and evaluation errors keep the previous state. Per-volume and per-interface identities are preserved.
A fresh failed scrape continues the availability timer even after an older backend-specific `_up` series expires;
missing backend observations with successful scraping still do not imply recovery.
Monitoring process self Overview is separate from these sources. Preview/apply sends no test notification;
without a contact point, events remain in Alerts. Provider delivery is a separate acceptance layer.

## Agent HTTP / CLI

Authenticated `GET /api/v1/agent/capabilities` and `metricspanel schema` advertise these operations:

| Operation | Role | Effect |
| --- | --- | --- |
| `GET /api/v1/connections` | Viewer | Bounded saved receipts; no credentials |
| `POST /api/v1/connections/discover` | Admin | Read-only fixed-loopback probes; empty body/query |
| `POST /api/v1/connections/preview` | Admin | Actual protocol probe; no configuration/sample writes |
| `POST /api/v1/connections/apply` | Admin | Reviewed atomic configuration plus receipt |

Preview accepts `{ "setup": { "id": "<canonical UUID>", "language": "en", "alerts": false,
"target": { "name": "local-node", "kind": "prometheus", "url": "http://127.0.0.1:9100/metrics",
"enabled": true, "interval_seconds": 15 } } }`. Existing collectors use `existing_target_id` and omit target
settings; saved credentials are read internally. Apply adds the exact preview `plan_hash`.

Plans are redacted. The server's keyed proof binds private connection settings without exposing a plain credential
fingerprint. Changed observed panel/rule contract or stored credentials returns 409 and requires review again.
Reuse identical setup/UUID/hash after an uncertain response: a completed operation returns its original receipt
even if the exporter is then unavailable. Changed input, deleted generated resources or a second bundle for the
same collector fails closed; retries cannot silently recreate or overwrite them.

CLI: `metricspanel connections list|discover`, `connections preview|apply --file FILE|-`.
Credential documents belong on protected stdin/files, never arguments or logs. Reporting-only tokens cannot use
management operations. Limits: 64 KiB request; two probes per server; 12 s request/10 s collector timeout; shared
eight-scrape budget; 10k samples/4 MiB per response; 1024 durable operation receipts per catalog. Preview lists the
first 256 metric names and reports the full count. Receipts remain as retry tombstones; this is bounded provisioning.

## Evidence boundaries

Unit/HTTP checks cover role/token isolation, real HTTP auth, observed-only panels, freshness, credential drift,
encrypted restart, concurrent retry, deleted resources and transaction rollback. Docker extends four real database
fixtures plus eight authenticated protocol fixtures through probe → apply → retry → dashboard → exact-scope
ordinary ingestion. Protocol fixtures are not live Hadoop/Kafka/etc. clusters. Browser and production results
are recorded separately in [the status ledger](status.md).
