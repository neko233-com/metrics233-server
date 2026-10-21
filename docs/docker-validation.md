# Docker production runtime validation

Run the release image checks from the repository root:

```powershell
pwsh -NoProfile -File scripts/dockerfile_test.ps1 -Repeat 2
```

The entry point builds the real `Dockerfile --target runtime`, including Vite's production bundle and the Linux
Go binary. `scripts/docker-verify.ps1` is a compatibility entry point to the same checks. No second runtime
Dockerfile is maintained: release and test image definitions must stay identical.

The default `compose.yml` also selects the single-service SQLite runtime, with the same read-only/capability and
resource constraints. `compose.clickhouse.yml` is an explicit optional overlay. Existing ClickHouse deployments
must include that overlay on upgrade; switching to default SQLite does not migrate historical metric samples.
The default `docker build .` also selects this production runtime; database fixtures require `--target integration`.

To add the existing database, exporter, plugin, alert and dashboard integration suite:

```powershell
pwsh -NoProfile -File scripts/dockerfile_test.ps1 -Repeat 2 -IncludeDatabaseIntegration
```

Add `-Browser` to run the existing Playwright setup/login/roles browser smoke against another isolated release
container. It requires Node, `npm ci` in `web` and the Playwright Chromium installation on the host.
The compatibility `docker-verify.ps1` entry point includes this browser check by default.
Combining `-Browser -IncludeDatabaseIntegration` additionally executes `sql-explore.spec.ts --retries=0` against
the still-running real MariaDB/PostgreSQL fixture. It sets the external SQL URL/token only in the child environment,
redacts fixture credentials from diagnostics and removes temporary browser outputs through Go test cleanup.

For Linux static analysis and the full Go race suite, the optional builder target reuses the same module/build
caches and provides GCC without requiring a compiler on a Windows host:

```powershell
docker build --target validation --progress plain -t metricspanel-validation:local .
docker image rm metricspanel-validation:local
```

This target is a development image, separate from the runtime size budget. Remove only the tag created for the
verification; keep shared caches and other images intact.
The local validation target and hosted CI explicitly allow 45 minutes per race-tested Go package; the earlier
completed local result below used the previous 20-minute budget. CI allows 60 minutes for the test job, including frontend
and browser validation. The dense SQLite boundary regression retains its real 5,000,001-row fixture and retention
deletion. Race instrumentation exceeded Go's default ten-minute package budget locally, and the hosted runner
later exceeded the explicit 20-minute budget. Both timeouts are failed attempts, not successful race checks.
The quantitative fixture, counts and assertions are not reduced or skipped; the larger CI budget still requires
a completed immutable-source run before claiming success.

For a previously built release image, pass `-Image metricspanel233:local`. Supplied images are never removed by
the test. To run the database suite alone, use `scripts/test-docker.ps1`; its fixture image runs several real
services in one container and is intentionally larger and less restricted than the production image.

## Checks and budgets

`TestDockerProductionRuntime` verifies both `--auth=token` and `--auth=local`:

- The release image content (`docker image inspect .Size`) is at most 256 MiB and contains an actual health check,
  trusted CA bundle and non-root user. The test also reports `docker image ls` local footprint: Docker's containerd
  store may report a larger unpacked disk size, which must not be confused with the content-byte measurement.
- The process runs as UID 10001 with a read-only root filesystem, no Linux capabilities, no privilege escalation,
  one CPU, 256 MiB memory, 128 PIDs and a 16 MiB `/tmp`. Only `/data` and `/tmp` are writable. Host ports bind to
  ephemeral `127.0.0.1` ports; restarts resolve the current mapping again.
- A non-loopback listener without a workspace token fails to start. The public health response remains minimal;
  metrics, detailed health, stats, Prometheus queries and ingest require authentication.
- First-run local setup, one-time setup reuse rejection, login, HttpOnly/SameSite cookies, Viewer read-only
  enforcement, Editor ingest, Admin-only user management, cross-origin mutation rejection and logout revocation.
- Namespace `/apis/` endpoints reject anonymous access in both modes; token mode does not open the local identity
  catalog even when an unusable identity-db option is configured. A separate HTTP-upstream case uses the public
  HTTPS root-url with preserved Host, covering setup/login/Secure cookies, APIs, foreign-origin refusal and logout.
  This models the documented reverse-proxy contract without claiming a deployed public TLS endpoint.
- Duplicate native JSON writes replace the same timestamp; a 10,000-sample business batch is durably acknowledged.
  Both OTLP/HTTP JSON paths and both snappy/protobuf Prometheus remote-write paths store queryable metrics.
  Native and label-filtered PromQL queries return their expected values. Named IANA timezone previews work inside
  Alpine through the binary's embedded timezone data.
- Graceful SIGTERM, Docker health after restart, persisted metrics, masked encrypted collector settings, local
  identities and existing sessions survive a restart.
- With the service stopped, a non-root helper archives the full data volume without network access. A SHA-256
  check precedes restoration into a fresh volume. The restored release process must query the same data and sign
  in with the saved identities; a separately opened copy must decrypt the original collector credential and refuse
  to open when its encryption key is removed.
- Image size, ingest duration, query duration, cgroup memory, CPU and PIDs are reported. After the modest batch
  and restart, cgroup memory must stay below 230 MiB within the configured 256 MiB limit and OOMKilled must be false.

Tokens, passwords and collector secrets are randomly generated per run. They are passed in a child environment
or HTTP body, never Docker command arguments or test diagnostics. Only first-run setup writes its documented
one-time token to the private container log; failures redact that token before printing diagnostics.

## Cleanup and acceptance boundary

Every run uses uniquely named and labeled runtime containers and volumes, and registers cleanup before starting
the application. The image is removed only when this test created it. Go removes its temporary offline database
copy. No `docker system prune`, `docker builder prune` or global image cleanup is performed, and unrelated browser
reports and local databases are preserved. Docker's normal shared build cache and downloaded base layers remain
available for later builds; their disk size is separate from the production runtime image budget.

These checks require a Linux Docker Engine and Go matching `go.mod`; the memory probe uses Linux cgroup v2.
Builds need access to their configured Node/Go/base-image sources unless those dependencies are cached.
The database suite's fixed Compose project uses a local lock and must not run concurrently with itself.

Passing this suite proves bounded single-node Linux container behavior and recovery for the exercised workload.
The timings describe this local Docker workload, not a capacity guarantee. Broader browser workflows, HTTPS proxy
configuration, real production exporters, long-duration retention and host-level disaster recovery require their
own acceptance evidence.

## Recorded local runtime verification before the A1 auth changes (2026-10-21)

`pwsh -NoProfile -File scripts/dockerfile_test.ps1 -Repeat 2 -Browser` passed against the then-current working-tree
source in 162.837 seconds. Both runs built the production image and passed the token, local-account and real
Chromium authentication/role checks. The Viewer browser session opened Business monitoring, had no create
dashboard button, and displayed the required Editor/Admin role explanation. No unexpected browser errors occurred.

The Linux amd64 engine used Docker 29.7.2. Each application container was limited to one CPU and 256 MiB:

| Resource snapshot | Run 1 memory.current | Run 2 memory.current |
| --- | ---: | ---: |
| Token mode after ingest/restart | 20,512,768 bytes | 19,435,520 bytes |
| Local accounts after ingest/restart | 85,110,784 bytes | 86,343,680 bytes |
| After browser setup and repeated password sign-ins | 225,488,896 bytes | 225,214,464 bytes |

The runtime image reported 44,712,781 content bytes (42.6 MiB) through `image inspect.Size`; Docker's local
unpacked footprint was 162 MB. Native HTTP ingestion acknowledged 10,000 samples in 49.7–55.2 ms, and the sum
query took 9.4–10.9 ms. These are individual bounded local measurements. The browser authentication snapshot
includes recent password hashing and must not be represented by the lower token-mode memory value.

The default `docker build .` was separately built and inspected: it selected the non-root `metricspanel` runtime
entrypoint. The temporary default-build image was removed after inspection.

The optional Linux `validation` target passed `GOMAXPROCS=4 go vet -p=2 ./...` and
`CGO_ENABLED=1 GOMAXPROCS=4 go test -p=2 -race ./... -count=1` in its 230.8-second validation step. Its temporary
image tag was removed. Two full real-database API/storage cycles passed, followed by a clean third run with
`METRICSPANEL_DOCKER_SQL_BROWSER_TEST=1 go test -tags=integration ./tests/integration -run '^TestDockerEndToEnd$'
-count=1 -v -timeout=20m` (185.277 seconds). Its real MariaDB/PostgreSQL browser checks reported **3 passed, no
skips, no retries** in 25.6 seconds, covering SQL annotation rendering/editing, credential masking, configured
datasources, Explore, variables and native dashboard persistence. The fixture containers/network/volumes and
temporary browser outputs were removed after the check.

## Final pre-migration source baseline (2026-10-21)

After the A1 authentication corrections, the unified command passed without test retries:

```powershell
pwsh -NoProfile -File scripts/dockerfile_test.ps1 -Repeat 2 -Browser -IncludeDatabaseIntegration
```

The integration package completed in **535.733 seconds**. The two real-database cycles passed in 204.78 s and
173.95 s; each also ran all three SQL Chromium checks against the live fixture (26.2 s / 24.8 s), giving **six
passed, zero skipped, zero retries**. The two production-runtime cycles passed in 81.02 s and 75.93 s.

Both production images reported **44,728,886 content bytes (42.7 MiB)** and **162 MB local Docker footprint**.
All runtime cases used UID 10001, a read-only root filesystem, one CPU and a 256 MiB memory cap:

| cgroup memory.current snapshot | Round 1 | Round 2 |
| --- | ---: | ---: |
| Token mode after ingest/restart | 18,051,072 bytes | 18,980,864 bytes |
| Local-account mode after ingest/restart | 86,786,048 bytes | 85,172,224 bytes |
| HTTPS-origin setup/login/API/logout case | 221,011,968 bytes | 221,323,264 bytes |
| After Chromium setup/repeated password sign-ins | 220,884,992 bytes | 222,109,696 bytes |

The HTTPS-origin case passed twice in 9.06 s / 9.10 s using the real server over an HTTP upstream with preserved
public Host. Setup/login, Secure cookies, namespace/API access, foreign-Origin 403 and logout revocation all
passed. Token mode ignored the deliberately unusable local identity-catalog path. Native 10,000-sample ingest
took 41.5–47.9 ms and native sum queries 5.3–9.6 ms in these bounded local runs. The larger password-work snapshots
must not be replaced by the smaller token-mode number. All reported runtime OOMKilled checks were false; the
separate database fixture was also observed below its 2 GiB cap with OOMKilled=false.

The initial full Linux race attempt failed at the default ten-minute package deadline: the stage took 754.4 s,
`internal/store` reached 600.089 s and the 5,000,001-row boundary/retention test was deleting rows through
`Store.Prune` when interrupted. No race warning or assertion failure was reported before that timeout. The
validation target and CI were raised to an explicit 20-minute per-package deadline, preserving the fixture.
The complete rerun then **passed**: `GOMAXPROCS=4 go vet -p=2 ./...` followed by
`CGO_ENABLED=1 GOMAXPROCS=4 go test -p=2 -race ./... -count=1 -timeout=20m` completed the validation step in
**859.4 seconds**, with `internal/store` passing in **743.373 seconds**. No race warning was reported. The
temporary validation image tag was removed and its deletion was confirmed; shared build caches were retained.

These results use the frozen application before the requested `metrics233-all` / `metrics233-server` module and
repository migration. They establish that source baseline only; renamed modules, new release names and the
post-migration build require their own evidence. Temporary runtime/fixture containers, networks, volumes and
runtime image tags were removed, with filtered Docker readback confirming no remaining owned resources.

A separate read-only migration audit verified every backup SHA from the 649-file migration manifest and compared
the resulting server files after only the declared exact module-prefix and README-heading replacements. All
649 files matched, including 366 Go files, with no unexpected differences. The public `cmd/metricspanel` entry,
`METRICSPANEL_*` environment names, CLI flags and API route strings were preserved in that snapshot. This is
source-transformation evidence; it does not replace compilation or runtime verification after the rename.

## New server module runtime verification (2026-10-21)

The migrated `metrics233-server/` checkout was then used as the actual build context:

```powershell
cd metrics233-server
go list -m
pwsh -NoProfile -File scripts/dockerfile_test.ps1 -Repeat 1 -Browser
```

`go list -m` returned `github.com/neko233-com/metrics233-server`. No prebuilt image was supplied; the test built a
new uniquely tagged formal Dockerfile runtime from this directory. The new-module integration package passed in
**103.085 seconds**, including token mode, local accounts, HTTPS-origin HTTP upstream, Chromium role checks,
restart persistence and fresh-volume backup recovery. The token/local/HTTPS/browser subtests passed in
25.79 s / 26.69 s / 9.19 s / 11.72 s. Viewer Business monitoring navigation and mutation restrictions passed
in Chromium with no unexpected console or HTTP errors.

The new runtime reported **44,728,944 content bytes (42.7 MiB)** and **162 MB local Docker footprint**. Its
memory.current snapshots were 18,268,160 bytes (token), 86,056,960 (local), 220,053,504 (HTTPS password work),
and 219,918,336 (browser password work), under the same one-CPU / 256 MiB limits. Both modes restored encrypted
credentials and durable data successfully; all runtime OOMKilled checks were false. Owned containers, volumes
and temporary image tags were removed and filtered readback was empty.

This is direct post-migration Linux runtime and browser evidence from the working checkout. The database suite
and full Linux race were not repeated here: their actual completed pre-migration runs and the source-equivalence
audit remain separately recorded above. The committed/tagged CI run must supply its own immutable-source result;
this local run does not claim that a public release or deployment has occurred.

## Hosted immutable-source CI timeout (2026-10-21)

The manual [server CI run 39538086462](https://github.com/neko233-com/metrics233-server/actions/runs/39538086462)
at source `6d8b8ea4242ef9181798d63a804fd2f98811db41` failed when `internal/store` reached its 20-minute package
deadline. `TestSQLiteNativeBucketPlanRejectsOversizedScanBeforeReduction` had run for 16 minutes 52 seconds and
was still deleting the real 5,000,001-row fixture through `Store.Prune`. No `DATA RACE` warning or other assertion
failure appeared before the timeout; that does not make the interrupted package pass. The Docker job's two
successful integration rounds are separate evidence and do not make the failed overall run green.

The earlier local full-race result above passed `internal/store` in 743.373 seconds on its own machine; it cannot
establish the same runtime on a hosted runner. CI now grants 45 minutes per Go package and 60 minutes for the
test job, preserving every fixture, count and assertion. This is a time-budget correction; the updated committed
source still needs a fresh completed CI result. The explicit local Docker `validation` target now also uses
`-timeout=45m`, keeping its fixture sizes, counts and assertions unchanged. That full target has not been
rerun in the current Feishu-card verification cycle. The focused notification/authentication Linux race
checks and the formal runtime/database/browser verification have separate scopes and results.

## Feishu, business snapshots and final runtime verification (2026-10-21)

The current `metrics233-server/` working checkout was verified in two layers:

```powershell
pwsh -NoProfile -File scripts/dockerfile_test.ps1 -Repeat 2 -Browser -IncludeDatabaseIntegration
pwsh -NoProfile -File scripts/dockerfile_test.ps1 -Repeat 2 -Browser
```

The combined command passed in **620.300 seconds**. Its real database rounds passed in **224.30 / 192.50
seconds**, including MariaDB, PostgreSQL, Redis and ClickHouse collection, SQLite 100,000-sample and ClickHouse
1,000,000-sample concurrent durable ingestion, 10,000 x 64 vector HNSW search with recall@10=10/10, physical
organization isolation, full-container restart, encrypted SQL configuration and dashboard/plugin/alert regressions.
Each round also ran all three real SQL browser cases with `--retries=0`: **3/3 + 3/3 passed**, without skips.
The fixture images contained 415,735,970 / 415,738,064 bytes and ran under two CPUs / 2 GiB.

During that command, the last business API correction restricted current cumulative counters to the latest
accepted population snapshot. It changed `internal/server/business_instances.go`; storage, collector, database
schema and the database regression components were unchanged. The first combined runtime is therefore recorded
as earlier-input evidence. The final API's eight owning cases passed separately, and the second command rebuilt
the formal runtime twice from the fully frozen source. It supplied no prebuilt `-Image`. A SHA-256 manifest of
571 build-related files was unchanged before and after this final command
(`0de93860f8fa9ba20ca2d86efbefcc744e9f7627075d653691ec074301ac8ebb`). The database layer was not repeated a
third time for this server-only query correction.

The final runtime command passed in **173.509 seconds**, with rounds of **77.43 / 96.03 seconds**. Both passed
token/local authentication, namespace and role refusal, minimal public health, UID 10001, read-only root,
capability drop, no-new-privileges, one CPU / 256 MiB limits, business/OTLP/remote-write ingestion and query,
graceful restart, local identity persistence, canonical HTTPS Origin over an HTTP upstream, Chromium setup/login/
Viewer restrictions, and offline SHA-256-checked recovery into a fresh volume. All OOM checks were false and both
browser smokes reported no unexpected console or HTTP failures.

Docker **29.7.2** reported **44,862,017 content bytes (42.8 MiB)** in each final round and **163 MB local
footprint**. These are different image-store measurements. The following `memory.current` snapshots include
cgroup page cache; they are not process RSS or peak-memory guarantees:

| Final runtime case | Round 1 bytes | Round 2 bytes |
| --- | ---: | ---: |
| Token after ingestion/restart | 19,382,272 | 17,743,872 |
| Local accounts after ingestion/restart | 86,589,440 | 85,508,096 |
| HTTPS-origin password work | 221,179,904 | 221,577,216 |
| Browser password work | 222,298,112 | 221,970,432 |

Filtered Docker readback confirmed zero verification containers, runtime volumes/image tags, and database-test
containers/volumes/networks after cleanup. Existing shared caches and the standard database-fixture build image
were retained; no global prune was used. Redacted logs were retained as
`dockerfile-test-repeat2-browser-db.log`, `dockerfile-test-final-runtime-repeat2-browser.log` and
`docker-final-cleanup.json` in the operator's temporary acceptance directory.

The final unified UI additionally passed four notifications browser cases and three business-instance cases
without retries. The focused notification/template/authentication Linux vet/race result is 80 top-level passes
across two repetitions, not a full-repository race result. The explicit 45-minute `validation` target was not
fully rerun in this cycle. These are local source/runtime/browser acceptance results; immutable release/CI and
actual Feishu-group card delivery remain separate gates.
