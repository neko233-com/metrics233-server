# Embedded compressed storage

The default `--storage embedded` is a single-process design with three separate
engines. No additional database container or network listener is required.

| Data | Engine | Durability and work bounds |
| --- | --- | --- |
| Metric float64 samples | Embedded Prometheus TSDB; lossless XOR chunks, Zstd WAL, indexed time blocks | WAL sync before ingestion acknowledgement; 100000 active head series; bounded reference cache |
| Explicit 64-dimensional metric windows | Embedded Badger v4, Zstd payloads and indexes; numeric-key HNSW graphs | Synchronous transactions; 100000 windows; persisted payloads rebuild graphs at startup |
| Accounts/configuration/alerts | Independent SQLite control and identity catalogs | Existing protected keys, transactions and versioned APIs |

HNSW is an approximate candidate search followed by exact distance reranking.
`exact` explicitly scans the filtered collection. Sparse filters fall back to a
filtered exact search when ANN cannot fill the requested result. It does not
embed every metric sample automatically. Missing samples remain missing and
float64 observations are never quantized or display-decimated for PromQL.

The application links pinned upstream Prometheus `v0.315.0` TSDB, not a second
Prometheus daemon. The small CC0 HNSW core is attributed under
`internal/vectorstore/hnsw`. Its persistence is owned entirely by Badger.
Native PromQL queries use the engine's indexed querier directly. Native bucket
queries retain the 200-series, 5M-point non-rate and 250000-point rate budgets;
raw materialization retains its 1M-point limit. Conflicting duplicate timestamps
are not an update API; producers should emit one value per series/timestamp.
Empty label values use Prometheus semantics and are normalized to absent labels;
the native reference cache and migration checksum use that same identity.

Retention defaults to 15 days and remains editable through the versioned HTTP
settings endpoint. Queries immediately hide expired data. Startup/hourly cleanup
removes expired observations and windows, then reclaims obsolete blocks; block
compaction can temporarily need extra disk space. Statistics cache counts and
file sizes for 30 seconds. Linux/macOS disk figures count allocated file blocks;
other platforms report logical file lengths, including reserved sparse space.
Single-node storage still requires backups and a local reliable filesystem.

## Existing SQLite migration

Stop **every writer** for the workspace before executing:

```sh
metricspanel storage migrate --db data/control.db --metrics-dir data/metrics \
  --backup-dir /protected/new-migration-backup --source-stopped --prune-legacy
metricspanel serve --db data/control.db --storage embedded --metrics-dir data/metrics
```

The destination and backup directory must be new. Migration creates a consistent
SQLite backup plus adjacent key/identity backups, streams only retained samples
and windows, flushes blocks, reopens both engines and compares exact float bits,
sample counts, an order-independent cryptographic fingerprint and vector payloads.
Legacy label JSON is decoded once per retained series (maximum 100000). Migration
records how many series had empty labels normalized. If two legacy identities
would become one canonical series, migration refuses before copying observations;
it never silently deduplicates potentially conflicting source rows. The protected
SQLite backup retains the original label representation.
Only then can `--prune-legacy` delete old metric rows and VACUUM the control DB.
Configuration and alert history are preserved. Failure leaves the original
observations until verified publication; partial directories are never silently
adopted or overwritten. Inspect and retain failed copies for operator recovery.

Back up the **complete stopped data directory**, including TSDB `wal`, `wbl`,
`chunks_head`, vector files, control/identity catalogs and `secrets.key`. Returning
to an older binary which only reads SQLite requires restoring the matching old
snapshot as well. Do not roll back code alone after legacy reclamation.

Explicit `--storage sqlite` remains a legacy compatibility backend. ClickHouse
remains an explicitly selected external high-volume option.

## Reproducible profile

```sh
METRICS233_STORAGE_PROFILE=1 go test ./internal/store \
  -run '^TestEmbeddedMillionPointProfile$' -count=1 -v
```

The deterministic workload has 1048576 points/128 series/68 hours, equally split
between constants, counters, smooth gauges and seeded random jitter. There is no
speed assertion or universal compression promise. A Linux Docker volume run
(2 CPUs, 768 MiB container limit) measured SQLite 68374528 bytes against native
time-series 5050368 bytes and complete native data 6336512 bytes. Synced ingestion
took 3.29s versus SQLite 8.09s; a 32-series one-hour native query took 1.19ms and
reopen 12.3ms. These are local fixture measurements, not production capacity.
Abrupt process exit recovery and migration/restart checksum tests also passed.
