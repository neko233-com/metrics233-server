# Business instance dashboards

An accepted metric report discovers a business instance when its labels include
`service`, `role` (`gateway`, `logic`, or `game`), `serverId`, and `instance`.
`serverId="0"` is valid. The stable identity is the complete tuple of
`environment`, `channel`, `service`, `role`, `serverId`, `instance`, and
`implementation`; missing optional labels remain empty. Use stable instance
names, and set environment/channel explicitly rather than relying on defaults.

The catalog observes successful JSON, remote-write, OTLP and collector writes.
It does not seed example servers or infer availability from an old observation.
`last_seen` is the most recent accepted report, not a heartbeat SLA. Discovery is
auxiliary to committed samples: a catalog failure is logged and does not make
the caller retry an already accepted batch. Such an instance appears on a later
successful report. The control catalog holds at most 4096 identities; existing
identities keep updating at capacity, new ones are not registered, and the API/UI
report the capacity condition. Existing catalogs are not backfilled by scanning
all historical samples. They populate on subsequent real reports.

`reported_ip` is a literal host-IP label supplied by the caller. `observed_ip`
is the latest canonical socket/trusted-proxy client address recorded by the
server for an accepted HTTP report. Collector-only observations have no observed
client address. A later collector observation does not erase an existing
observed address. NAT/proxy addresses are possible. Labels do not prove ownership
of an instance and neither address changes ingest token/source-IP authorization.

## Templates and independent copies

Dashboards has a **Business virtual dashboards** tab, also available from the
Business page. The built-in view is virtual; discovering an instance does not
insert an ordinary saved dashboard. Logic/game views initially show actual
`game_players_online` (Gauge), `game_player_sessions_started_total` (Counter),
and an online-player trend. A gateway view starts without fabricated panels;
bind a native template or save a copy and select its actual metrics.

- `default`: built-in instance view, no saved dashboard reference.
- `inherit`: resolve the referenced native dashboard on each refresh. Template
  edits affect every inheriting instance. There are no implicit panel overrides.
- `independent`: reference an ordinary saved dashboard. Saving an independent
  copy captures the current resolved panels, then uses the existing dashboard
  editor. Later edits to the old template do not change that copy.

All three modes fix the full identity variables and `business_scope`. A template
PromQL selector should be `metric_name{${business_scope:raw}}`; this includes
empty optional label matchers, so a missing environment cannot accidentally
select a differently named environment. Individual fixed variables such as
`${serverId}` are available, but serverId alone is not a complete selector.
Non-scope template variables use the normal native variable resolver. These
variables are query conveniences, **not security isolation**; a panel author can
write another query. Template and independent dashboard reads retain their
existing dashboard/library ACL checks. A binding grants no dashboard permission.
Fixed values use the existing hidden, frozen single-choice variable path so
literal `$` characters in label values are not interpolated as dependencies.
Imported Grafana documents are not business templates; use native dashboards.
The ordinary 32-variable limit includes the eight fixed variables.

Binding mutations require Editor/Admin; Viewer can read the catalog and metric
tables. They require the current binding `version`; a stale save returns 409 and
the UI asks for a refresh rather than replaying the mutation. A deleted or
unreadable template fails explicitly and an Editor can reset the binding to the
built-in view. Saving a copy uses the existing dashboard API, then a binding CAS:
if the second operation fails, the saved dashboard remains in the normal list
and the UI identifies that copy instead of silently deleting it or retrying.

## Player regions and Entity SQL

`game_players_online_by_region{<identity>,country,province,city}` contains current
online players, aggregated upstream. Departed regions must publish zero.
The authoritative population is the unique latest accepted
`game_players_online` sample within the full instance scope (IP remains advisory).
Built-in population/trend panels exclude older IP-labelled population series.
Regions must have that exact population snapshot timestamp; an old region left
behind by a process restart cannot become part of a new population. A zero
population has an empty region result. Missing/ambiguous population is unavailable,
not zero. Region totals differing from the population are marked incomplete,
including partially accepted multi-batch snapshots; older regions never fill the
gap. `snapshot_at`, `stale` (>90 seconds), population value/status and region
completeness accompany the API data. Staleness describes sample freshness, not
server/player availability; configured intervals above 90s may trigger it too.
The normal PromQL five-minute lookback still bounds population discovery.
The UI retains zero rows, displays all returned regions, and never uses `topk`.
No player UID or raw player IP is needed or displayed. Unknown regions remain
explicit. IP location is approximate. Pages using DB-IP Lite results include
the attribution link [IP location data: DB-IP Lite](https://db-ip.com).

Entity SQL uses these cumulative Counters with the full identity plus `table`
and bounded `operation` labels:

| Metric | Meaning |
| --- | --- |
| `game_sql_calls_total` | Actual driver attempts; retries count separately |
| `game_sql_failures_total` | Failed actual attempts |
| `game_sql_rows_affected_total` | Successful driver RowsAffected, with the driver's affected-row semantics |
| `game_sql_duration_seconds_sum` | Sum of actual-attempt durations, seconds |
| `game_sql_duration_seconds_count` | Number of measured actual attempts |

These are not queue/enqueue counts or a durability guarantee. An unresolved table
is `unknown`; SQL text/parameters are not labels. The table offers 5m/15m/1h
windows, `increase` counts with counter-reset handling and Prometheus
extrapolation, plus process cumulative values (which can reset on restart).
Average latency is `1000 * sum(rate(duration_sum)) / sum(rate(duration_count))`,
weighted across attempts, in milliseconds. It is sorted descending by recent
average; insufficient samples/zero denominators stay unavailable (`—`), not zero.
Current cumulative sessions and SQL totals (including cumulative duration
sum/count averages) require the latest unique authoritative population snapshot
timestamp too, so an old IP-labelled process is not added to its replacement.
An Entity absent from the new snapshot has no current cumulative value; its
historical window can still be present. Window `rate`/`increase` deliberately
keep valid historical series across process/IP changes and counter resets.

Player regions use 100-row fetches and a fixed-height virtual window with at
most 20 data rows in the DOM. Scrolling near the end or **Load more** fetches the
next page; all regions remain available. The SQL UI instead requests the first
10 rows with `window=1h&limit=10`: the ten Entity/operation pairs with highest
weighted average latency in the last 60 minutes. It shows average milliseconds,
estimated driver execution and failure counts, not raw SQL text or an assertion
that every entry is slow. Entries without a calculable window average are omitted.
The complete paged SQL API remains available to HTTP clients. Cursors bind the
instance, table kind, window and evaluation timestamp, expire after 15 minutes,
and keep all pages at the same PromQL evaluation time. This is a time anchor,
not a database snapshot of late/backfilled writes. Queries use the existing
engine/raw-scan budgets, a 15s request deadline, and a maximum 10000-series
business result; exceeding a budget returns an error rather than partial top-k
data. Each cursor fetch re-evaluates the fixed instant; no unbounded result cache
or new background worker is introduced.
Overview panels, SQL Top10 and template reads follow workspace refresh and retain
previous content while refreshing the same scope. Region tables keep their
evaluation time and loaded pages during background refresh; **Refresh instance**
starts a fresh table snapshot. Switching instance or window mounts a separate
result view so that previous-scope data is never shown under the new identity.

## API

- `GET /api/v1/business/instances?limit=50&cursor=...&q=...&template_id=...` — stable-ID keyset
  pages, optional bounded search, capacity metadata. Optional `template_id` first checks
  native-dashboard and referenced-library read permission, then pages only exact
  live `inherit` bindings to that dashboard. Default, independent and copied-dashboard
  bindings are excluded. Unknown templates return 404; forbidden templates return 403.
  It never removes the instance resolver's fixed identity scope or grants catalog access.
- `GET /api/v1/business/instances/{id}` — observed identity and current binding.
- `GET /api/v1/business/instances/{id}/dashboard` — latest authorized native
  dashboard plus fixed variables; never writes a dashboard.
- `PUT /api/v1/business/instances/{id}/dashboard` —
  `{version,mode,dashboard_id}` (`dashboard_id=""` only for default).
- `GET /api/v1/business/instances/{id}/data?kind=regions|sql&window=5m&limit=100&cursor=...`
  — complete paged metric rows, total, evaluation time and continuation cursor.

These remain authenticated workspace APIs. Fixed ingest-only credentials cannot
read or mutate any of them. The catalog and bindings live in each physical
organization control database; they do not create a cross-organization index.
