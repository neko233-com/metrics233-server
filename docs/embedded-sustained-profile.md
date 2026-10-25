# Sustained embedded-runtime profile

This opt-in contract uses the formal runtime with paced HTTP writes and concurrent PromQL. It complements the
[million-point compression profile](embedded-storage.md#reproducible-profile), which excludes HTTP/concurrent
queries. Neither measurement establishes a universal throughput, latency or sizing SLO.

## Reproduce

From the server module with Docker available:

```sh
METRICS233_EMBEDDED_SOAK=1 go test -tags=integration ./tests/integration \
  -run '^TestDockerEmbeddedSustainedProfile$' -count=1 -v -timeout=15m
```

PowerShell: set `$env:METRICS233_EMBEDDED_SOAK='1'` before `go test`. The default is ten minutes;
`METRICS233_EMBEDDED_SOAK_SECONDS` permits an explicit 30..3600 seconds. Include startup/restart in the external
timeout. Ordinary CI skips this machine-dependent profile. By default it builds the formal `runtime` image.
`METRICSPANEL_RUNTIME_TEST_IMAGE` reuses an operator-verified image, which the test does not delete; record its
actual binary version/hash.

## Workload and boundaries

- 4000 series of one gauge, labelled by series/environment. A 4000-sample seed precedes 500-sample batches paced
  at one per second. The ten-minute run acknowledges 304000 total observations.
- One concurrent instant-sum PromQL query per second. Bounded requests, full write ACKs, valid finite query results,
  and exact latest-value sum/sample count are checked. A normal restart repeats both exact checks; crash/backup
  behavior is separately verified by the existing runtime suite.
- p50/p95/p99/max are reported without speed assertions. Write timing includes eight seed batches. `memory.peak`
  is the kernel cgroup peak including charged cache/kernel memory, not a sampled Go heap. `cpu.stat` includes startup.
  `du -sk /data` reports complete active allocated space in KiB, including control/key/TSDB/vector/WAL files; it is
  not a compressed-sample-only ratio.

The existing runtime profile supplies UID 10001, read-only root, no capabilities, no-new-privileges, 1 CPU,
Docker `256m` hard limit (268435456 bytes) and `GOMEMLIMIT=128MiB`. A new labelled volume stores all data. A random
token is passed only through inherited environment/HTTP headers; only a random loopback port is published.
Cleanup checks container/volume ownership. No production or pre-existing data is used.

## Evidence — 2026-10-26

Reference: Windows AMD Ryzen 9 9950X; Docker 29.7.2 Linux x86_64, WSL2 kernel
`6.6.114.1-microsoft-standard-WSL2`. Actual verified v0.2.30 Linux amd64 release binary:
`42ad38eb8b65b362ec3e2fccffaf81aaafc338355f1c40a3f29c1941de3cbb93`.
Only the binary COPY was replaced in the formal runtime definition; storage/profile limits were preserved.
This is distinct from the private production binary's build parameters/hash.

The 30-second smoke passes in 46.210s including startup/restart: 19000 ACKed observations, 30 concurrent queries,
exact restart readback, kernel peak 77332480 bytes (77.3 MB), full active directory 1728 KiB. Write
p50/p95/p99/max: 11.889/109.730/109.844/109.844 ms; query: 6.013/10.484/11.020/11.020 ms.
Cumulative cgroup CPU: 1.452s, no throttled periods. These are preliminary short-run observations. The separate
ten-minute result is recorded in [status](status.md); it must not be inferred from the smoke.

The independent ten-minute run passes in 616.148s including startup/restart. Its 600.023s paced phase acknowledges
304000 observations and completes 600 concurrent queries. Both exact checks pass again after restart. Kernel
cgroup peak: 87609344 bytes (87.6 MB). Complete active data before restart: 5320 KiB (5447680 bytes, 5.45 MB).
Cumulative cgroup CPU: 19.903s; no throttled periods or OOM.

| HTTP observation | p50 ms | p95 ms | p99 ms | max ms |
| --- | ---: | ---: | ---: | ---: |
| Synced 500-sample write, including seed | 10.476 | 17.534 | 20.673 | 110.290 |
| Concurrent instant PromQL sum | 8.078 | 13.223 | 17.737 | 19.661 |

This covers the declared single-gauge/4000-series/500-samples-per-second shape; it does not establish random-float
compression, cardinality churn, multi-user queries, maximum ingest capacity or a weeks-long soak. Backups and
Docker image/cache space are excluded from the active `/data` directory.
