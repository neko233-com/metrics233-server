# Instance directory and network observations

The first-party console uses GitHub Dark canvas/surface/control tokens. Visualization palettes stay independent:
native time-series retain Grafana classic colors, and gauges keep their numeric threshold bands. Frame-local
SDK series indexes are normalized across a plot on cloned metadata; authored fixed/threshold colors remain intact.

`GET /api/v1/business/directory` is a Viewer read over at most 4096 observed identities. `q` searches actual identity
values, reported/observed addresses and operator descriptions; `environment`, `role=all|game|gateway|sdk`, and
`sort=server|recent|host` apply before pagination. Limits are 1–100. Role counts cover the entire filtered catalog
before role selection, not just a page. Cursors are tied to filters and sort; a live recent-report sort may move
when new observations arrive. Default natural server sorting is stable. Template-scoped legacy pagination is unchanged.

Cards query bounded numeric summaries in a fixed number of engine queries, independently of card count. Host
resources require an explicit managed-host or exporter association; NAT/label addresses never infer that association.
Missing, stale or failed observations remain unknown. Refresh preserves mounted cards for the same filter scope;
a changed scope discards the old view and aborts old requests. Observation times are distinct from the browser clock.

`PUT /api/v1/business/instances/{id}/note` requires Editor and `{version,description}`. Descriptions allow multiline
UTF-8 up to 4096 bytes, rendered as plain text. Their own SQLite CAS revision is independent of reporting identity,
host binding and template changes. HTTP 409 preserves the draft and requires an explicit review of the latest text.
Ingest-only credentials cannot read or mutate notes. Discovery advertises these native HTTP contracts.

`GET /api/v1/business/instances/{id}/network?device=...&language=zh` returns scoped native business and optional
host-interface dashboards. Service receive/send use `service_payload_received_bytes_total` and
`service_payload_sent_bytes_total`, with legacy game/gateway counters as alternatives, never additive aliases.
SDK traffic means actually read/written HTTP body bytes. These are separate from whole-host NIC bytes and omit
other processes and protocol headers. Rate uses a 2-minute counter window; last-hour increases are estimates,
including reset handling. More than one fresh generation for an identity is ambiguous and hidden instead of summed.

Host agents emit bounded (64 interfaces) `host_network_receive_bytes_total`, `host_network_transmit_bytes_total`,
their `_bytes_per_second` gauges, and packet/error/drop counters, each labeled `device`. First observation, reset,
interface reappearance, read failure or nonpositive sampling interval establishes a new rate baseline. Genuine idle
rates remain zero. `host_resource_collection_success{resource="network"}` records availability independently.
Snapshots store only the latest bounded projection; history uses the compressed metric store.

Exporter hosts require `netdev` and use `node_network_*_bytes_total`. Automatic selection prefers a fresh interface
with positive observed receive/send traffic, then a conventional physical interface within that active set. With
no useful rate yet, it falls back to a conventional physical interface, a nonvirtual name, then the first name.
The first useful automatic choice stays pinned through idle/bursty refreshes while that interface remains present;
an explicit manual choice never silently changes. The selector offers Automatic to discard that manual choice.
Physical, bridge and tunnel interfaces remain separate and must not be summed. This name/rate heuristic does not
claim to discover the operating system's default route or to identify the Internet uplink in every topology.

The network response includes `recommended_device` and `recommendation_has_traffic` separately from the requested
`device`. Exporter discovery and ranking exclude counter observations older than max(90 seconds, 3 scrape intervals).
Stale/paused host rates cannot rank a recommendation. An explicit device is one UTF-8 name of at most 128 bytes,
with no surrounding whitespace or control characters; duplicate query parameters are rejected with HTTP 400.
If a manually selected interface disappears, its exact history remains selected and current rate panels are hidden.
The UI labels the missing observation rather than showing a different NIC under the old choice. On a new NIC scope,
the business view hides the prior NIC until the matching response arrives; service traffic and the chosen historical
window remain separate. Same-scope refreshes preserve the existing chart node and choice.

Host instruments use their content container's width: three columns when roomy, two below 900px (the third spans
the row), and one below 600px. The user's Large setting remains one column. This also covers a tablet with an open
sidebar; viewport width alone must not shrink three gauges into an unreadable narrow content area.
Collectors and historical/custom host dashboards are upgraded separately; provisioning preserves customized boards.

## Rendered acceptance and design comparison

The console palette uses a darker `#010409` navigation/header against the `#0d1117` page, `#161b22` cards,
`#21262d` controls and `#30363d` borders. Current locations use a neutral selection with a coral indicator;
links and keyboard focus are blue, primary actions green. These UI tokens do not set chart-series, gauge
needle or threshold-band colors. Semantic pending/warning states keep their own amber color.

The directory and network-view concepts were generated before implementation, then compared directly with the
final 0.2.24 production Edge render. The review covered five points:

1. Navigation and layout: retained grouped navigation and a 224px desktop sidebar, with three aligned service
   cards per row and a stacked single-column layout at 390px.
2. Palette: GitHub Dark canvas `#0d1117`, card `#161b22`, elevated control `#21262d`, border `#30363d`, green
   actions and blue links replace the concept's residual cyan accents. Chart classic colors and threshold bands
   remain independent, as requested.
3. Hierarchy and type: service identity, environment, editable note, physical-host meters, service traffic and
   absolute report time appear in that order. Stable server ordering and actual identity lengths replace
   illustrative names and concept ordering.
4. Containers and controls: search, sorting, environment and category selection share one toolbar flow. Card
   note slots preserve alignment. Network rate and estimated last-hour volume use separate native panels to
   preserve their different units and query semantics; this intentionally takes more height than the concept's
   combined tiles. Host NICs are selected explicitly rather than adding interface totals.
5. Responsive behavior and refresh: no horizontal overflow at 390px, real card/canvas node identity retained
   across same-scope refresh, and selected NIC/search drafts retained. New filter scopes still wait for their own
   result instead of presenting previous-scope cards.

Production acceptance used seven real identities. Chinese multiline notes were saved, searched and reopened,
then restored to their original value. Real service body traffic, Linux NIC throughput and LAN NIC switching were
observed; zero idle business rates were not replaced with invented traffic. These checks do not certify gameplay
Login/Ready/Ping, optional plugin parity or long-duration capacity.
