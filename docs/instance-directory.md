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

Exporter hosts require `netdev` and use `node_network_*_bytes_total`. A visible NIC selector defaults to a conventional
physical NIC when one is available. Physical, bridge and tunnel interfaces remain separate and must not be summed.
Collectors and historical/custom host dashboards are upgraded separately; provisioning preserves customized boards.
