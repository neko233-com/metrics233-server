# Storage query planning

This contract covers native metric queries and the raw-sample boundary used by the upstream PromQL engine.
It does not change the sample schema, ingestion format, dashboard query language or retention configuration.
Embedded compressed storage is the default; SQLite remains an explicit legacy backend. Neither needs a
separate database process. See [embedded storage](embedded-storage.md) for direct indexed PromQL and migration.

## Two query paths with different semantics

| Path | Temporal reduction | Limits |
| --- | --- | --- |
| Native `Embedded.Query` | Streaming compressed iterators, bucket-last and reset-aware rate semantics below | Same 200-series / 5M non-rate / 250000 rate budgets; no raw intermediate copy |
| Embedded PromQL | Direct indexed TSDB querier with full raw observations | Upstream engine sample/time budgets; 100000 active head-series ceiling |
| Native `Store.Query`, SQLite | SQL selects the last observation in each series/bucket; Go applies the requested cross-series aggregation | At most 31 days, step at least 1 second, duration/step at most 2,000; 200 selected series; 5,000,000 source observations for non-rate queries |
| Native `rate`, SQLite | Raw consecutive observations, reset-aware delta divided by elapsed seconds; last calculated rate per bucket | Same range/step/series limits; 250,000 observations including one step of lookback |
| Native `ClickHouse.Query` | Existing server-side `argMax(value,timestamp)` buckets over `FINAL` reads, followed by cross-series aggregation | Same range/step and at most 200 returned series; server execution/memory limits remain authoritative |
| PromQL `LoadSeries`, both backends | Full raw observations; no display decimation or native bucketing | 1,000,000 selected observations and 10,000 selected series; excess fails explicitly |
| Series/label discovery | Metadata only, with an observation-existence check in the requested range | 10,000 matching series |

Ranges include both endpoints. A point exactly at `end` can occupy the final bucket, so a duration/step of 2,000
can produce 2,001 points per series. Empty buckets stay empty. Native `sum`, `avg`, `min` and `max` aggregate the
**last value of each series in a bucket**. They are not temporal extrema over every raw observation in that bucket.
For example, observations `1,999,2` in one series and `3,-999,4` in another produce native `min=2`, `max=4`.
Raw PromQL selectors still receive all six observations, including both peaks. Window functions, counter resets,
lookback and staleness must not be evaluated from a display-decimated source.

The ClickHouse 200-series guard applies to returned native series (`last`/`rate`); its cross-series aggregations
can reduce more source series. SQLite explicitly checks the selected source-series limit before scanning values.
This pre-existing backend distinction remains visible rather than claiming identical capacity limits.

## SQLite plan

1. Resolve metric name and exact native label filters against the series catalog. An indexed `EXISTS` probe on
   `(series_id,timestamp)` excludes series with no observation in range; at most 201 candidate IDs are returned
   so the 200-series limit fails before scanning their values.
2. For non-rate queries, count at most 5,000,001 selected observations using the existing covering primary key.
   A larger selection fails before bucket aggregation. This count and the subsequent read share one read
   transaction, so concurrent ingestion/overwrites cannot change the selection between the budget check and read.
3. Group selected samples by `series_id,(timestamp-start)/step`, take `max(timestamp)`, and retrieve its exact
   value through the primary key. Only bucket rows cross the database driver into the Go aggregation path.
4. `rate` retains its raw consecutive-observation path and lower budget. It never substitutes bucket endpoints for
   an intermediate reset. Query cancellation propagates through the database context and closes the transaction.

The plan needs no new table or index. SQLite still reads selected source observations to form buckets, and the
bounded count adds another covering-index pass. This reduces driver traffic and Go allocation; it does not turn
arbitrary historical aggregation into a constant-time lookup. Grouping/order may use temporary B-trees, and the
existing single SQLite connection still serializes work with other operations.

The measured `EXPLAIN QUERY PLAN` includes:

```text
CO-ROUTINE buckets
SEARCH samples USING PRIMARY KEY (series_id=? AND timestamp>? AND timestamp<?)
USE TEMP B-TREE FOR GROUP BY
SCAN buckets
SEARCH samples USING PRIMARY KEY (series_id=? AND timestamp=?)
SEARCH series USING INTEGER PRIMARY KEY (rowid=?)
USE TEMP B-TREE FOR ORDER BY
```

Raw PromQL selection first matches series metadata, compiling regular expressions once per selector. It then
loads only the selected IDs and time interval, using the primary key, within the same read snapshot. Unrelated
series no longer consume the million-sample budget. Metadata regex/negative matching may still scan the catalog;
it does not load all those series' retained samples. Missing labels keep Prometheus empty-string matcher behavior.
SQLite and shared Go matching use the upstream `FastRegexMatcher`, including whole-string anchors, newline/DotNL
and explicit inline flags; ClickHouse separately applies its server-side anchored `match` semantics.

## ClickHouse plan and block min/max

`samples` remains a daily-partitioned `ReplacingMergeTree(version)` ordered by `(name,labels_json,timestamp)`.
`FINAL` is retained before aggregation so a newer correction at an existing timestamp wins even before background
merges. This follows the [ReplacingMergeTree correctness contract](https://clickhouse.com/docs/concepts/features/operations/update/replacing-merge-tree).
Native `argMax` is a query-time rollup; persisted asynchronous aggregate materialized views are not introduced.
Changing a historical sample must not leave a stale aggregate, and generic PromQL still needs the raw source.

On the measured 30-day fixture, an unqualified five-minute timestamp predicate already used partition Min-Max
and the existing primary index: **1 of 32 parts and 1 of 122 granules** were read. The selected timestamp granule
still needs its row predicate and `FINAL`; block boundaries are pruning metadata, not sampled metric values.
Adding and materializing an independent `timestamp_minmax` index in the disposable fixture produced **1 of 1**
granules after the existing pruning, with no further reduction. That redundant index is not installed in production.
The [official skip-index guidance](https://clickhouse.com/blog/index-based-pruning) explains why correlation and
measured selectivity matter. This result does not establish selectivity for every cardinality or series layout.

## Retention and compatibility

- SQLite `Prune(before)` deletes observations strictly before the cutoff and removes empty series in one
  transaction. Plans read the original table; there is no derived rollup to retain expired values or invalidate.
- ClickHouse retains the table's existing TTL. `CREATE TABLE IF NOT EXISTS` does not change a deployed TTL when
  `--retention-days` changes, and `Prune` does not issue mutations. Inspect `SHOW CREATE TABLE metricspanel.samples`
  and `system.parts` when checking the effective policy.
- An operator-approved change to an existing table uses, for example,
  `ALTER TABLE metricspanel.samples MODIFY TTL fromUnixTimestamp64Milli(timestamp) + INTERVAL 30 DAY`.
  Expiration runs during merges, not immediately on a wall-clock boundary. Shortening a TTL can remove retained
  observations; changing a startup flag alone is not evidence that the deployed table has changed.
- No migration, ingestion dual-write, automatic `MATERIALIZE INDEX`, `OPTIMIZE FINAL` or TTL mutation runs at
  application startup. Existing SQLite databases and ClickHouse samples stay readable without conversion.

See the [official TTL operations contract](https://clickhouse.com/docs/concepts/features/operations/delete/ttl)
for merge scheduling and the separate schema requirements of TTL rollups.

## Reproducible measurements (2026-10-21)

SQLite: Windows amd64, AMD Ryzen 9 9950X, Go 1.27.0, local temporary database, no race detector. A SQL recursive
fixture persists 1,000,000 observations in one series over 30 days. Fixture creation is outside the timed region;
ingestion performance is not measured. Each result contains 1,440 half-hour buckets.

```powershell
go test ./internal/store -run '^TestSQLite(Native|Raw)' -count=1 -v
go test ./internal/store -run '^$' -bench '^BenchmarkSQLiteQueryPlan30Days1M$' -benchtime=1x -count=3 -benchmem
```

| Path | Three measured queries | Allocated bytes/query | Allocations/query |
| --- | --- | --- | --- |
| SQL bucket-last plan | 473 / 468 / 465 ms | 1.06 MB | ~27,800 |
| Uncapped raw scan + Go bucket reference | 1,024 / 1,261 / 1,152 ms | 98.0 MB | ~7,039,000 |

The reference follows the old row-transfer/Go-bucketing pattern with its sample cap removed only for comparison.
The previous production path rejected selections above 250,000 observations; it did not support this million-row
workload. Go allocation counts are not total process RSS, peak memory, SQLite cache size or disk/temp-file usage.
The 5,000,001-sample rejection/retention test also persists a real oversized table and verifies that it fails before
reduction, then returns only the surviving observation after prune.

ClickHouse: Linux container on the same host, version **26.8.32.14**, **2 CPU / 1 GiB** container limit, official
`mirror.gcr.io/clickhouse/clickhouse-server:26.8-alpine` image digest
`sha256:3e65ca7a356f76994c539b25584b25531826f39575d231411be989506d193dff`.
The dedicated test creates/drops only its randomly named database on a disposable HTTP endpoint:

```powershell
$env:METRICSPANEL_QUERY_CLICKHOUSE_URL = 'http://127.0.0.1:58806' # use the disposable fixture's actual port
go test ./internal/store -run '^TestClickHouseQueryPlanningMillionSamples$' -count=1 -v
```

| Workload | Three observed HTTP wall times | Result |
| --- | --- | --- |
| Native `FINAL` + `argMax`, 30 days / 1M observations | 81 / 61 / 79 ms | 1,440 points |
| Five-minute `FINAL` range, extra skip index disabled | 3.14 / 2.77 / 3.16 ms | 115 observations, 1 granule |
| Same range, extra skip index enabled | 2.66 / 2.12 / 3.17 ms | 115 observations, still 1 granule |

The test also verifies a version-2 historical correction through the actual raw loader, records before/after
`EXPLAIN indexes=1`, and checks that query planning does not alter the 365-day fixture TTL. The small timing
difference for the redundant index is not evidence of a useful optimization. These are warm, single-client,
bounded fixtures; concurrent ingest, million-series cardinality, sustained disk growth and operational SLOs remain
workload-specific measurements. No blanket capacity or performance guarantee follows from these results.
