# Storage and renderer performance baselines

The zero-configuration metric backend is now embedded XOR/Zstd TSDB with a separate Badger vector index and
SQLite control catalog. Its reproducible million-point profile and actual disk/reopen measurements are in
[embedded storage](embedded-storage.md#reproducible-profile). Production migration evidence is recorded separately
in [status](status.md). The SQLite timings below describe the explicitly selected legacy metric backend; they are
not measurements or sizing limits of the default embedded engine. The browser baseline is a separate workload.
Every figure here is a bounded observation, not a production guarantee; rerun on the intended hardware/workload.

The [sustained formal-runtime profile](embedded-sustained-profile.md) separately measures actual HTTP/PromQL
concurrency: ten minutes, 4000 series, 500 samples/s, 304000 accepted observations and exact restart readback.
Its 87.6 MB cgroup peak and 5.45 MB active data are tied to that workload, not generic per-sample ratios.

## Reference machine

| Item | Value |
| --- | --- |
| CPU | AMD Ryzen 9 9950X (16 cores / 32 threads) |
| OS | Windows (amd64) |
| Go | 1.27 |
| Storage | SQLite WAL, default page settings |
| Build | `go test` benchmark, no race detector |

Record your own `goos`, `goarch`, CPU and Go version from the benchmark header. Disk type (NVMe vs. network share)
matters more than CPU for sustained ingestion.

## Commands

```sh
# 100,000 samples (20 services) plus a 24-hour, 60-second aggregation
go test ./internal/store -run TestBaselineIngestAndAggregate100k -count=1 -v

# Steady-state timings; -benchtime=1x keeps the run short
go test ./internal/store -run "^$" \
  -bench "BenchmarkSQLiteAggregate24Hours|BenchmarkSQLitePatternSearch10k" \
  -benchtime=1x -count=1
```

The benchmark code lives in `internal/store/sqlite_bench_test.go`. Each benchmark opens a temporary database, so it
does not touch `data/metrics.db`.

## Observed result

Run on the reference machine above:

| Workload | Result |
| --- | --- |
| Ingest 100,000 samples in 1,000-sample batches | 0.43 s (about 233k samples/s) |
| 24-hour aggregation, 60-second buckets (1,440 points) | 92 ms |
| Exact pattern search over 10,000 saved 64-dimension windows, top 10 | 6.8 ms |

These numbers cover a warm process and a local disk. They exclude HTTP overhead, PromQL evaluation, concurrent
writers, retention pruning and cold-start I/O.

A fresh 2026-10-21 Windows run during parallel browser/Docker validation measured 2.22 s for the 100,000-sample
ingestion test, 185 ms for the 24-hour aggregation benchmark and 12.4 ms for the 10k-pattern benchmark. These are
single iterations on the CPU above (`go1.27.0`, no race detector), not controlled throughput comparisons. Background
load materially affects this host; neither the older timings nor this rerun establish an operational SLO.

## Legacy SQLite vector search behavior

- SQLite always performs an exact scan: it loads candidate payloads, computes L2 distance and keeps a bounded top-K
  heap. There is no approximate index.
- Candidates are filtered by normalization, aggregation and optional metric/label equality before scanning.
- A single search is capped at 50,000 candidate patterns. Above that the request fails with an explicit error instead
  of silently returning partial results; add a metric or label filter, or use ClickHouse with its HNSW index.
- ClickHouse uses approximate HNSW search and supports `--exact`; SQLite has no approximate mode to disable.

## Legacy SQLite sizing guidance

Use the measured workload and its limitations when sizing the service:

| Deployment | Recommendation |
| --- | --- |
| SQLite single-node deployment | Use a local SSD; measure actual samples/s, active series, label cardinality, retention growth and concurrent queries. Collector count alone does not determine capacity |
| High-cardinality or bursty writes | The 100k-sample baseline does not establish a million-active-series capacity. Measure the workload and evaluate ClickHouse if storage or query latency is the bottleneck |
| More than 50,000 saved patterns | Use ClickHouse for HNSW, or filter pattern searches by metric/label |
| Multiple concurrent dashboard users | SQLite serializes writes; reads stay concurrent, but heavy ingest plus large queries compete for one disk |

For this legacy backend, back up the SQLite catalogs, adjacent `secrets.key` and plugin directory together.
The default embedded deployment additionally requires its complete stopped TSDB/vector directories; a control-only
SQLite copy is insufficient. Follow [the embedded backup contract](embedded-storage.md#existing-sqlite-migration)
and the [storage guide](../README.md#storage-and-performance).

## Browser TimeSeries baseline (2026-10-21)

This separate frontend measurement uses the production embedded UI and Chromium. A Playwright route supplies
a deterministic datasource response containing sine waves, one-point peaks/troughs and a null interval. It
measures dashboard navigation through two animation frames after the canvas is ready, including app startup,
response transfer/JSON parsing and drawing. Server storage, PromQL execution, concurrent users and production
network conditions are excluded. It is not an ingestion benchmark or a frontend latency SLO.

Reference: Windows amd64, AMD Ryzen 9 9950X, Node v24.19.0, Chromium 156.0.8078.4, viewport 1536 × 1024.
Final route-split embedded bundle measured at 06:56 Asia/Shanghai on 2026-10-21; tested binary SHA-256:
`6958d9c00fb4172bec395e3f7b88bd57c8551d17a2008d4306a1e7c69de36656`.
The first navigation and two subsequent navigations in the same browser context were measured; each creates a
new application document and Worker. Heap values use Chromium's approximate `performance.memory` counter,
which excludes Worker/process memory and is not a peak-memory measurement.

From `web`, after `npm run build` and a Go `-tags webui` build to `bin/metricspanel.exe` (or set
`METRICSPANEL_TEST_BINARY`):

```powershell
$env:RENDER_BENCH_POINTS = '100000'
$env:RENDER_BENCH_SERIES = '5'
$env:RENDER_BENCH_RUNS = '3'
$env:RENDER_BENCH_OUTPUT = Join-Path $env:TEMP 'metricspanel-render-100k.json'
node scripts/render-bench.mjs

$env:RENDER_BENCH_POINTS = '1000000'
$env:RENDER_BENCH_SERIES = '1'
$env:RENDER_BENCH_OUTPUT = Join-Path $env:TEMP 'metricspanel-render-1m.json'
node scripts/render-bench.mjs
```

| Input | API fixture size | Sampled rows | Navigation to presentation, 3 runs | Worker calculation, 3 runs | Main document JS heap |
| --- | --- | --- | --- | --- | --- |
| 100,000 timestamps × 5 series | 10,968,547 bytes | 2,527 | 1,043 / 877 / 861 ms | 10.5 / 11.8 / 10.7 ms | ~39.6 MB |
| 1,000,000 timestamps × 1 series | 33,488,564 bytes | 2,540 | 1,581 / 1,395 / 1,395 ms | 19.0 / 19.4 / 17.7 ms | ~86.4 MB |

The two workloads transferred 4,800,000 bytes in six buffers and 16,000,000 bytes in two buffers respectively;
all input buffers were confirmed detached after `postMessage`. Worker roundtrip was 17.0–17.9 ms and 23.4–27.8 ms.
The 100k workload recorded one 53 ms main-thread long task across three runs; the 1m workload recorded 53/73,
71 and 67 ms long tasks. Moving numeric work off-thread does not remove response parsing/startup costs.

The shared min/max target is approximately twice the CSS width, with all series sharing the row budget. It keeps
extrema after stacking, endpoints, boundary neighbours and both sides of every gap. Dense gap transitions can
exceed the target, deliberately preserving outages. Tooltips inspect sampled points; zoom requests a new
datasource query. Full-resolution legend last values and stored/exported source frames remain available.

`web/test/timeseries-numeric-model.test.cjs` verifies 100k spikes/gaps, a shared 20-series budget, irregular and
duplicate times, stacking, viewport boundaries, source immutability and transferable-buffer roundtrips.
`web/e2e/time-series.spec.ts` exercises the real Worker, preserved extrema/gaps, zoom requery, worker disposal
and the unavailable-Worker fallback. Browser evidence is separate from algorithm-unit evidence.

Known boundaries: Workers still require a sliced main-thread copy of numeric values; decoding large datasource
responses still runs on the main thread. Missing/failed Workers fall back to main-thread normalization without
a responsiveness guarantee. Optional Grafana threshold/continuous series-color modes may also compute field
statistics on the main thread; this baseline uses the default palette. Each changed streaming frame starts a new full normalization job; append-only
incremental processing, 200-series density and sustained 60 fps have not been established by this baseline.
