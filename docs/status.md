# Project status and progress protocol

### 2026-10-27 — Native MyDB monitoring and Grafana interoperability candidate

The new `mydb` collector uses a dedicated authenticated HTTP `/metrics` endpoint and
rejects responses without a valid `mydb_up`. Quick connect creates only observed
panels, up to 24 performance/security panels and 11 optional sustained rules. Missing
data preserves alert state; counter rules can recover at a zero rate. Native MySQL
collection also exports its fixed security-switch variables as numeric ON/OFF values.
MyDB collection accepts only Bearer credentials, requires HTTPS outside loopback and
rejects query parameters so URL credentials cannot enter exported instance labels.
The UI clears database credentials when saving a MyDB target; actual Edge verifies
the Bearer-only form and saving with the configured token left blank.

Real isolated MyDB `0.1.37-dev.1` verifies anonymous/wrong-token rejection, read-only
token isolation from SQL/config/status/backup APIs, SQL/credential privacy, failed
authentication signals, ordinary scraping and Prometheus query readback. The final
candidate repeats that chain with 24 panels and 11 rules. The owning Go model,
collector, server and telemetry packages and vet pass. Infrastructure includes MyDB in
the database filter, and the frontend type check and Vite build pass. The pre-existing
BusinessInstances edit remains independently owned.

Actual Grafana `13.2.3` queries installed MyDB through MetricsPanel233 with a local
Viewer's HTTP Basic identity, imports and reads back all 14 supplied dashboard panels,
and verifies that the Viewer cannot manage monitoring connections. The fixture pins
the bundled Prometheus plugin and disables automatic plugin installation/update;
earlier attempts exposed a registration race during background updates on the read-only
image. The corrected fixture passes without those update errors. Actual Edge renders
the native MyDB database filter and 24 panels, and all 14 Grafana panels with
availability/audit-health values of 1 and no console errors. An idle WAL/latency window
remains missing data. Public DNS/HTTPS and external notification delivery are separate.
Source commit `f149269` and its aggregate gitlink were published. The first complete
candidate CI exposed the stale twelve-kind Docker catalog assertion after MyDB made
the catalog thirteen kinds; it now checks the full expected kind set, including MyDB.
Actual Windows archive validation found the same stale count in the browser form
case; it now checks all thirteen kinds and MyDB's Bearer-only credential form.
The same run's production runtime and real Tempo checks passed twice. The corrected
`040a0c9` Docker CI passes twice, including the full catalog and database integration.
Two clean five-platform `0.2.32` builds at that source produce seven identical delivery
files. The actual Linux ZIP passes two confined runtime/browser rounds in 159.094s,
including SHA-256-checked offline restore of metrics, encrypted settings and identities.
Eight memory.current observations top out at 134758400 bytes under 1 CPU / 256 MiB;
this is not a kernel peak. The Windows ZIP's first browser run lacked Go on PATH;
after fixing the invocation, 105 cases pass and the stale catalog-count case fails.
The corrected thirteen-kind/Bearer-only form case then passes independently without
retries against the same actual archive. Three default environment-dependent skips
remain separate from real Docker SQL/Tempo coverage. Final-source complete CI,
public release and production monitoring deployment remain pending; roadmap weighting
stays 96% under the existing scope.

### 2026-10-26 — 0.2.31 public release, complete CI and production Edge accepted

A deterministic regression against the previous UI keeps CPU 25%, memory 50% and disk 20% after the host
snapshot's expiry with refresh Off. Another regression against the old scheduler receives no catch-up request
when the clock advances beyond its deadline before a foreground event. The first isolated scheduler test lacked
its own identity and timed out in fixture setup; after adding that identity, the preserved failure is specifically
the missing second request. Neither failure was hidden by a retry or a longer assertion timeout.

The candidate adds boundary-driven snapshot expiry and foreground/pageshow catch-up. Directory host tooltips,
current service rates, gauges, host heartbeat and NIC rates become unknown/expired without network polling;
expired capacity is not presented as current. Automatic refresh catches up once when overdue and remains Off
when configured Off. Historical curves, mounted card/canvas nodes, unsaved notes/search and manual NIC choice
survive, including recovery on a new sample. No UI palette, Grafana classic series palette or gauge styling changes.

Five targeted native-server browser cases pass without retries in 9.6s; 174 frontend units and the production
bundle build pass. The complete candidate browser suite passes 106 cases without retries in 7.5 minutes; two
external-SQL cases and the separately owned live-Tempo case skip without their environments.
Immutable source/tag 6270a48/v0.2.31 builds twice from a fresh official checkout: all seven delivery files match,
ZIP CRC/architecture/provenance and the native Windows version pass. The actual Windows ZIP again passes all
106 default browser cases without retries in 7.5m. The actual Linux ZIP, verified in-container by SHA-256 and
version, passes two confined runtime/browser rounds in 158.247s; eight memory.current snapshots max at 134332416
bytes (not a kernel peak). Its separate four-database integration/three real SQL browser cases pass in 210.734s;
SQL browser alone takes 26.2s. Formal limits, permissions, restart and offline restore assertions stay intact.

Edge inspects an independent, loopback-only instance of that exact Linux release binary. With refresh Off, real
synthetic HTTP-body counters initially display nonzero receive/send; after the actual 90-second report deadline
both become unknown and the report reads overdue while the open note and search drafts stay unchanged. No
browser-clock replacement or response projection is used in this Edge check; CPU stays unknown because this
fixture deliberately has no associated host. Native protocol fixture data is separate from production data.
New ACKed samples and manual refresh recover nonzero rates while the search draft stays. A multiline note saves
through the actual API and matches a combined note search/SDK-category query. Edge captures no warnings/errors.
The temporary pages close and the ownership-checked Edge container, data volume and release verification image
are removed; formal runtime and SQL fixtures also report their own cleanup. Shared images/caches are preserved.
The [complete exact-tag workflow](https://github.com/neko233-com/metrics233-server/actions/runs/40222966206)
passes: test job 41m54s, complete Go vet/race 31m29s, 106 default browser cases/three environment skips/8.1m
with zero retries, two additional real Tempo/API/browser rounds, and Docker 7m53s. The dedicated Tempo step and
independent real-SQL browser run cover the owned environments skipped by the default suite.
[v0.2.31](https://github.com/neko233-com/metrics233-server/releases/tag/v0.2.31) was formally published at
2026-10-25 19:32:55 UTC. All seven anonymous official HTTPS downloads (215594380 bytes) match the independently
verified local deliveries. Original failed/intermediate workflows and immutable tags retain their attribution.

The existing single private native pipeline deploys the same fixed source as 0.2.31 after private tests/vet,
backup/target preflight, 174 frontend units/build, complete Go tests, collector cross-builds and immutable
archive/download checks. Official account/ingest readiness returns success; systemd, actual process image hash,
source/version/deployment metadata and unprivileged UID are read back. Native deployment and confined Docker
settings are distinct: no claim is made that the native unit has the container's Seccomp/NoNewPrivs settings.
Private control-plane identities and credentials remain outside this public repository.

Production Edge reloads the actual release and observes seven active reporting identities. With refresh Off and
an actual wait past 90 seconds, all 21 host CPU/memory/disk values and 14 service receive/send values expire to
unknown; each card reports overdue while open note/search drafts remain. Manual refresh restores all current
resources and freshness without dropping those drafts. No browser clock or response substitution, fake production
samples, threshold changes or extra provider notifications are used for this check.
Release filtering returns two instances, LAN-IP search one and Rust gateway category one. The gateway retains
three racing gauges, two actual traffic curves and separate idle service bytes versus nonzero physical-NIC traffic.
A temporary note is saved, found by complete-catalog search and reopened, then restored to the original empty value.
The 14 unpaused alert rules have zero firing/pending instances and current evaluations; the actual LAN CPU warning
again shows 30s evaluation, 600s sustained trigger, 300s recovery and 3600s cooldown. Final search is empty, seven
cards have numeric resources, default 5s refresh and the original business entry are restored, and no captured
browser warnings/errors remain. GitHub Dark chrome and Grafana classic/racing renderers stay independent.
F2 closes; the supported server roadmap remains 96%, without extending the claim to optional technologies or
unbounded capacity. R2c's independently accepted real-service evidence is retained below.

### 2026-10-26 — Production configuration readback and plan reconciliation

Edge still authenticates the actual root account and displays seven reporting identities with real host resources.
The overview explicitly identifies self-monitoring; the business directory remains separate. After navigation,
last-report timestamps advance under the visible 5s refresh setting. The alerts page shows 14 unpaused rules,
zero firing/pending instances and current evaluations. Expanding the actual LAN CPU warning rule confirms
30s evaluation / 600s sustained trigger / 300s recovery confirmation / 3600s cooldown. This read-only inspection
does not evaluate rules manually, change thresholds or send messages, and the business page is restored.

G2/A3/N1/N2 and their historical local-stage checklist entries now reference the later actual production, browser
and provider receipts rather than continuing to claim rollout is pending. Earlier failed/intermediate results and
the user-waived game-account/nonzero-player exercise keep their original boundaries. This is plan reconciliation,
not a new binary rollout or an additional provider ACK.

### 2026-10-26 — Embedded runtime sustained profile: accepted

The opt-in profile reuses formal 1 CPU/256m/GOMEMLIMIT=128MiB confinement and the actual verified 0.2.30 Linux
release binary. Its 30-second smoke passes in 46.210s: 4000 series, 19000 ACKed observations, 30 concurrent PromQL
queries, exact value/sample-count restart readback, kernel peak 77332480 bytes and full active directory 1728 KiB.
A separate ten-minute run passes in 616.148s including startup/restart: 304000 ACKed observations, 600 queries from
one reader concurrent with writes, 87609344-byte kernel cgroup peak, 5320 KiB complete active data before restart,
no OOM or throttled periods.
Latest-value sum and sample count remain exact after restart. The paced phase lasts 600.023s with 4000 series and
500 samples/s; write/query p95 are 17.534/13.223ms (write timings include seeds), cumulative cgroup CPU 19.903s.
This is not a maximum-capacity, arbitrary-cardinality/compression or weeks-long soak claim. See [the profile](embedded-sustained-profile.md)
for pacing, percentiles, engine/HTTP scope and cleanup. The performance index now separates default compressed
storage from legacy SQLite and requires complete embedded data in backups. No runtime code or production data changes.

### 2026-10-26 — Real Tempo daemon: final full workflow accepted

`TestTempoLiveDataSource` starts official Tempo 3.1.1 pinned by digest, posts actual OTLP spans, and queries
search/detail through an authenticated Metrics233 HTTP server. Two zero-retry API/browser rounds pass in 20.512s;
the actual daemon, hierarchy, service names, duration/error message, missing trace, anonymous-401, reload/history
and 390/768/1280 containment are verified. Edge separately inspects that same live fixture and captures three spans
and database error attributes, without console warnings/errors. The held run passes in 90.109s including inspection.
Existing deterministic trace boundaries pass in 6.038s; integration vet and TypeScript pass. Initial dashboard-field
and Windows temp-alias fixture failures remain attributed to the fixture. Checked cleanup removes owned containers.

The normal browser suite explicitly skips this case without its fixture; the new hosted test-job step runs it twice
against the real daemon. Docker integration separately covers its API path. The initial hosted Docker job fails
because cold-pull stderr progress was mixed into its stdout container ID; its second warm round passes the actual
API contract. That known-failed run (40215466866) retains its Docker failure and an explicitly cancelled test job.
The runner now separates streams and local API/browser passes again (9.831s). Full run
[40216847324](https://github.com/neko233-com/metrics233-server/actions/runs/40216847324) on 86f1081 passes complete
Go vet/race (33m28s), 103 browser cases/three environment skips (8.3m, no retries), and repeated Docker (11m25s).
Its final old-fixture Tempo step fails because random ID `01b67a925514db79c59669c69b986e1f` is correctly shortened
by upstream search; the second round passes. That workflow remains failed, not relabelled as green.

The final fixture deliberately supplies leading-zero OTLP IDs, verifies both full/short lookup and canonical span
IDs, and follows the actual short link in the browser. Two local rounds pass in 24.688s. Source 15b917a passes
[focused hosted vet/negative/API/browser verification](https://github.com/neko233-com/metrics233-server/actions/runs/40219239682)
in 2m38s: two real-service rounds 21.222s, zero retries, existing negative tests 1.178s. No runtime implementation
changed to hide the fixture mistake; these verification commits' cmd/internal/web/src/dependencies/Dockerfile
match v0.2.30. Final full workflow [40220691425](https://github.com/neko233-com/metrics233-server/actions/runs/40220691425)
on exact combined source b0c9045c880d682bae100cba6583656f9beccc9d passes: test job 42m07s, complete Go vet/race
31m40s (store race 1528.903s), 103 default browser cases/three environment skips in 8.1m without retries,
repeated Docker 8m52s and two actual Tempo/API/browser rounds in 20.272s (2.5/2.9s browser). The dedicated live
step runs the case that the unowned default suite skips. The focused and full results retain their own identities.
This evidence unit changes no product runtime or configured datasources; production remains v0.2.30/17791c5. It
closes a real-service evidence gap without implying a local trace store, service graphs, partial traces or production
Tempo installation. See [the reproducible contract](tempo-live-validation.md). R2c closes and roadmap becomes 96%.

### 2026-10-26 — 0.2.30 production, public release and full tag CI accepted

Immutable source `17791c531c8941b5df56abdb706f4e6435663227` fixes the actual mobile backdrop hit region and is
deployed as 0.2.30. The fixed native worker passed private tests/vet, backup/target preflight, 174 frontend units/build,
same-source account/token readiness, full Go tests, collector cross-builds, archive/download checks and zero-advisory
dependency audit. Completed deployment, actual process/source/binary identity, version/database health and the
official initialized-account/anonymous-401/ingest-query-403 readiness check pass; private identities live separately.
An early health read during the unfinished cutover saw a missing PID; checks after completed deployment all pass.

The actual Windows ZIP passes the complete **103-case zero-retry browser suite** (7.4 min), with two external-SQL
environment skips. The final Linux ZIP passes ten normal-click/keyboard/mobile-layout browser repetitions without
retries (41.9 s), in addition to the five Windows/ten Linux candidate rounds and the original-release geometric red
test. No forced clicks or weaker assertions were introduced. The actual Linux ZIP also passes two unchanged confined
runtime/browser rounds (156.531 s): nonroot/read-only, 1 CPU/256 MiB, existing memory assertions, five reporting paths,
roles, restart, HTTPS-origin and offline backup/restore. Eight reported memory snapshots peak at 131907584 bytes.
Two independent five-platform builds produce identical seven-file sets; independent ZIP CRC, architecture,
source/module/binary hashes and actual Windows version checks pass. All seven uploaded asset digests match;
after public publication, all seven files were anonymously downloaded from the official release and hash-matched
against the tested artifacts. Both README languages are included.
Native Darwin/Linux arm64 execution and a new real SQL-browser run are not inferred from these checks.

Production Edge again verifies seven instances, release filtering (two), gateway classification (one), LAN IP and
multiline-note search (one), save/reopen and exact original-note restoration. Same-scope refresh retains card node
116895, traffic canvas node 119476, search draft and explicit NIC choice. Real en0 receive/send and the independent
service-byte view work; Automatic selects the active NIC, and explicit en1 survives refresh. No synthetic production
samples, resource load or extra Feishu messages were used. Mobile normal-click and Enter closing pass; the backdrop
starts at the 250px drawer edge and has about 125px visible width at 390px. The 1512/1100/768/390 instrument grids use
three/two/one/one columns, explicit Large remains one, and no horizontal overflow or console warnings/errors were
captured. Console GitHub Dark and Grafana/racing visualization colors remain independent; browser dimensions and
the All instances view were restored. [Full v0.2.30 tag CI](https://github.com/neko233-com/metrics233-server/actions/runs/40210846886)
is green: the test job completes in 28m00s, including full Go vet/race and **103 passed / 2 external-SQL skipped**
hosted browser cases (7.3 min, no retries); repeated Docker integration/runtime completes in 10m50s.
[v0.2.30](https://github.com/neko233-com/metrics233-server/releases/tag/v0.2.30) was publicly published at
2026-10-25 16:46:06 UTC. The failed v0.2.29 tag/draft/history remain intact. The preceding candidate and failure
entries below are historical observations, superseded by this final acceptance. Weighted roadmap progress stays 95%.

### 2026-10-26 — Mobile navigation correction: earlier local candidate

The 0.2.29 exact-tag workflow passed full Go vet/race and repeated Docker integration/runtime checks, but its
final browser shell case timed out twice: 102 passed, two external-SQL skips and one failed case. Public assets
remain a draft and the immutable tag is unchanged. A native Linux browser run reproduced one failure in five
zero-retry repetitions: the 250px mobile drawer intercepted the close-backdrop button's centre. A geometric
regression assertion fails immediately on the original release (backdrop x=0, required x>=249).

The candidate shares the mobile drawer width and constrains the backdrop to the visible area outside that drawer.
GitHub Dark backdrop hover stays stable, without changing chart/gauge colors or using forced clicks. The original
normal mouse/keyboard/responsive assertions remain; the new check requires an uncovered touch target of at least
44px. Windows passes five consecutive zero-retry rounds (9.5s); the actual Linux candidate in the official native
browser image passes ten (33.4s). The production build passes. Fixed 0.2.30 artifacts, deployment, Edge and full tag
workflow remain pending. This corrective unit does not change the 95% roadmap scope.

### 2026-10-25 — 0.2.29 production accepted; public CI failed at mobile backdrop

Source `352c4c5492da3d344a175f3380ec03e5ca56f793` is fixed by immutable tag `v0.2.29` and is now running in
production. The existing reviewed native worker completed private tests/vet, target/backup preflight, 174 frontend
units/build, token/local readiness, full Go tests, collector cross-builds, archive verification and zero-advisory
dependency audit before publication. Post-deploy build identity and actual running-executable hash were checked;
minimal health reports version 0.2.29/database OK, and the official initialized-account/anonymous-401/ingest-query-403
readiness check passes. Deployment receipts and actual process identity remain in the private deployment repository.

The final Windows ZIP passes the complete zero-retry browser suite: **103 passed / 2 external-SQL skipped / 7.5 min**.
The initial full attempt lacked Go in PATH: 16 plugin-fixture startup failures (`spawn go ENOENT`), 87 passes and two
skips. That failed log is retained; supplying the proper toolchain and rerunning the entire frozen-source suite
produced the result above, without source changes or weaker assertions. External SQL-browser acceptance from
0.2.28 remains attributed to that release, not counted as new passes here.

Two independent five-platform builds produce identical seven-file sets. Independent ZIP CRC, architecture,
executable/provenance/hash checks pass; the actual Windows executable reports 0.2.29. The verified Linux amd64 ZIP
binary, copied into the otherwise unchanged formal runtime definition, passes **two confined runtime/browser
rounds in 159.852 seconds**. Existing nonroot, read-only, 1 CPU/256 MiB and memory assertions remain; the largest
reported memory observation is 135352320 bytes across eight snapshots. Restart, five reporting protocols, roles, HTTPS-origin and offline
backup/restore stay intact. Test-owned containers/volumes were removed. Darwin/Linux arm64 native execution is not
claimed. The seven uploaded draft assets match local hashes; publication was withheld after the mobile-backdrop
CI failure documented above.

Edge on production verifies all seven real instances; release filtering yields two, gateway classification one,
and LAN IP/Chinese-note search one. A multiline note was saved, reopened, searched and restored to its original
empty value. Card node 100057 and host-traffic canvas node 104944 remained mounted through same-scope refreshes;
the search draft and explicit NIC choice also remained. Automatic selection chose the genuinely active en0 rather
than idle anpi0, and real receive/send rates were visible independently of the zero-idle service rates. No synthetic
production samples, load or extra Feishu messages were introduced.

At 768px, the same 483px content area previously squeezed three approximately 154px instruments; it now displays
one full-width column. Production 1512/1100/768/390px checks show three/two/one/one columns, no horizontal overflow,
and a working explicit Large view. Desktop/tablet retain the current browser clock and GMT+8; narrow mobile
retains the timezone indicator. Console warnings/errors are
empty. Console tokens match GitHub Dark while Grafana classic line colors and racing threshold gauges stay
independent. [The compatibility reference](grafana-compatibility.md) now distinguishes current contracts from its
historical October 12 audit. Overall roadmap progress stays 95%; current capacity and optional-catalog boundaries
remain. [Exact-tag workflow](https://github.com/neko233-com/metrics233-server/actions/runs/40205605215) Docker passed
in 11m18s and full vet/race passed, while the 44m23s test job failed its last mobile-backdrop browser case.

### 2026-10-25 — Host default-view refinement: local candidate

The GitHub Dark console and independent Grafana/racing visualization palettes remain intact. Automatic NIC
selection now uses fresh positive observations, prefers active conventional physical NICs, and pins the first
useful choice while it remains present. Manual idle/missing selections retain their identity and history; missing
current rates stay hidden. Exporter freshness guards exclude old or foreign series. The business view separates
pending NIC changes from service traffic and preserves its historical zoom window across those changes.
Host instruments adapt to the content container (three/two/one columns); the explicit Large view stays one column.
The API and heuristic limitations are documented in [instance directory](instance-directory.md).

Local candidate validation: full `internal/server` tests (46.274 seconds) and vet pass; 174 frontend unit tests
and TypeScript/production build pass. Two zero-retry browser cases over the embedded native UI pass (6.0 seconds),
covering automatic/manual/missing NICs, same-scope canvas retention, delayed scope changes, actual historical
zoom queries, and 1512/1100/768/390px layouts without horizontal overflow or page errors. Their bounded heartbeat
and network projections are synthetic HTTP fixtures; chart queries use the real local metric store. Separate
Go HTTP tests exercise actual managed-host/exporter recommendation, stale/foreign series and invalid requests.
Initial browser attempts used an incorrect environment-label locator and an offscreen drag; those fixture issues
were corrected, with no reduced assertions or retries. Immutable public artifacts, hosted CI and production/Edge
acceptance for this candidate remain pending. Weighted progress stays 95%; no full Grafana/Netdata parity is claimed.

### 2026-10-25 — 0.2.28 GitHub Dark and public release accepted

Runtime and public `v0.2.28` fix source `ebfd82cd9919974dfd21d969834c3c1011e17395`. The console separates
inset navigation/header (`#010409`), page (`#0d1117`), panels (`#161b22`), controls (`#21262d`) and borders
(`#30363d`). Blue links/focus, coral current-location indicators and green primary actions have separate roles.
Forms, modals, tables, first-party theme consumers, connection cards and layout controls share those tokens.
Pending alerts stay amber. Chart series, racing gauges, thresholds and refresh logic remain independent.

The immutable native worker passed private tests/vet, reviewed target preflight, 169 frontend units/build,
same-source token/account readiness, full Go tests, collector cross-builds, archive/download verification and
dependency audit (zero known advisories), then deployed successfully. Actual running-binary/build identities,
the deployment receipt and the reviewed mirror transport repair live in the private deployment repository.
Post-deploy readiness again passed initialized-account, anonymous-query 401 and ingest-only-query 403 checks.

Edge inspected seven real instances and actual CPU/memory/filesystem/service/NIC data. Formal environment
selection returned two services; the gateway category returned one. Chinese multiline notes were saved,
searched and reopened, then restored to their original contents. A same-scope refresh retained the original card
node and search draft; the selected active NIC and canvas node also survived refresh. Desktop 1512px, tablet
768px and mobile 390px views had no horizontal overflow; the tablet retained the full current clock and timezone.
The browser captured no warnings/errors. No synthetic production samples, resource load or extra Feishu messages
were introduced. Existing game binaries and dashboard/template/query/plugin behavior remain intact.

Public delivery is now independently established:

- Two five-platform builds produce identical seven-file delivery sets, including both README languages.
  Independent ZIP CRC, executable architecture, source/provenance and binary-hash checks pass. The actual Windows
  ZIP runs version 0.2.28 and its complete zero-retry browser suite passes 101 cases, with two external-SQL skips.
- Real MariaDB/Redis/PostgreSQL/ClickHouse integration passes in 207.759 seconds. All three real MySQL/PostgreSQL
  browser cases pass without retries. Other service integrations use protocol fixtures, not live external clusters.
- The actual Linux amd64 ZIP binary passes two confined runtime/browser rounds in 152.120 seconds, preserving
  token/local roles, reporting, restart, offline backup/restore, HTTPS-origin and the unchanged resource assertions.
- [The exact-tag hosted workflow](https://github.com/neko233-com/metrics233-server/actions/runs/40198818834)
  passes full frontend, release-script, Go vet/race, complete browser and repeated Docker jobs.
- [Public v0.2.28](https://github.com/neko233-com/metrics233-server/releases/tag/v0.2.28) contains all seven
  verified assets. A fresh anonymous HTTPS download of every asset matches the independent local hashes.
  The preceding immutable v0.2.27 also passed its own hosted workflow, package checks and anonymous downloads.

Darwin and Linux arm64 executable identity/architecture is verified; native execution there is not claimed.
This acceptance preserves the previously supported Grafana workflows; the documented optional/unsupported
catalog and live external-daemon/long-duration capacity boundaries remain. O1 is complete; weighted roadmap
progress is now 95% under the supported single-node product scope, not full Grafana/Netdata parity.

### 2026-10-25 — Public release closure: intermediate failure record

The following paragraphs retain the intermediate state and failures; the final 0.2.28 receipt above supersedes
their pending statements.

Production 0.2.26 has the acceptance recorded below, but public downloadable releases and full hosted tag CI
remain at 0.2.6. The complete browser suite still contained nine obsolete MetricsPanel233 title assertions.
Those locators now use the actual Metrics233 title, preserving their original flows and safety assertions.
Full-suite, tag-CI, five-platform archive and same-input reproducibility results will be recorded only after they
complete. The optional toolchain/animation proposals do not block this delivery; overall historical weighting
remains 94% until the outstanding acceptance gates are established.
The first full Windows-archive pass found additional stale business-list/button, retention-section and timestamp
assertions, plus a real 768px topbar overflow (scrollWidth 773 with a 768px viewport). Edge confirmed the latter:
the self-hosted badge crowded the current clock. The follow-up retains clock/timezone/navigation, truncates long
breadcrumbs and removes the redundant narrow-screen badge. Responsive coverage expands to 701/760/900px.
Both five-platform builds of the 0.2.26 candidate produced identical seven-file delivery sets; independent ZIP CRC,
binary hashes, source/architecture metadata and the actual Linux archive's confined runtime checks passed.
That does not establish its complete-browser or hosted-CI gate. The immutable candidate tag is preserved while
the corrected release proceeds separately.
The first Windows archive's complete zero-retry suite finished at 87 passed / 14 failed / 2 external-SQL skips.
Fourteen failures cover the tablet overflow and outdated card, gauge-SVG, missing/time, decimal-unit, localized
Feishu and disjoint-histogram expectations. The corrected seven-file subset passed 33/34; its sole remaining
`KB /s` spacing assertion then passed independently. Original identity, environment, template, 235-row virtualization,
global SQL Top10, replay/staleness, unsafe-content, unit persistence and risk-color checks remain in place.
The original tag's Docker job passed; its still-running test job was explicitly cancelled after the real browser
failure evidence, rather than advertised as green. The next release also packages both README languages, hashes
the Chinese document as a build input, and gives download users account-based startup steps without build tools.

### 2026-10-25 — Quick connect, GitHub Dark and readable host instruments accepted

Runtime 0.2.26 fixes source `1da41fefa8ea9412d58756d94cfd002be983262b`. The immutable native worker passed
full Go tests, 169 frontend units/build, native token/account readiness, collector cross-builds, archive/download
checks and dependency audit (zero known advisories), then deployed successfully. Full source vet also passed
in the separate fixed checkout. Runtime health reports 0.2.26/database=ok and its actual build identity matches.
Private source identity, deployment and binary/archive receipts remain in the private deployment repository.

Edge verified the seven real instances, formal/gateway filters, IP search, Chinese multiline note
save/search/reopen/restoration and retained card DOM/search draft across refresh. Existing production Node
collector preview/apply created a six-panel scoped dashboard without changing collector identity or adding
duplicate alerts. The versioned layout editor saved its larger CPU/memory first row, full-width filesystem view
and paired NIC charts. After the 0.2.26 reload, actual CPU/memory, two writable filesystems and independent
per-NIC traffic rendered. LAN interfaces returned distinct real rates; the same canvas and chosen NIC survived
same-scope refresh. Idle business rates remain honest zero and the active SDK has actual HTTP-body traffic.

The production Edge render confirms GitHub Dark canvas `#0d1117`, surface `#161b22`, border `#30363d` and
text `#e6edf3`. Visualization palettes, threshold bands, needle and classic trend behavior remain independent.
Production instruments measure 200px high instead of the old 90px ceiling. Desktop 1513px and mobile 390px
were inspected; mobile instruments are 200px in a single 358px-wide column, with client/scroll width both 390px.
Final production screenshots were saved and the validation tab had no captured warnings/errors. Existing
custom boards are not automatically rearranged. Six formal layout-browser cases passed without retries,
including numeric/mouse/keyboard editing, Classic/V1/V2 fidelity, CAS conflict and preserved mobile/desktop data.

Exact-source runtime Docker checks passed twice (204.782 s total), retaining authentication, 10000-sample ingest,
restart, offline backup/restore, HTTPS-origin and browser-role gates. Browser cgroup memory was
131305472 / 131371008 bytes, below the unchanged 230 MiB gate in a 256 MiB container. The runtime defaults to
the operator-overridable Go soft limit `GOMEMLIMIT=128MiB`; native binaries retain the normal Go default.
Real database integration passed twice on `c787f3e` (209.69 / 202.08 s); later changes affect runtime headroom,
diagnostic reporting and starter-panel geometry, not those database transports. The earlier complete
`cf61d28` baseline passed separately (567.474 s), before the availability correction. No assertion was relaxed.

This release does not redeploy game/SDK binaries or inject synthetic production metrics, high load or extra
Feishu messages. Historical roadmap weighting remains 94%; this is not full Grafana/Netdata parity or a
long-duration capacity certification. See [the quick-connect contract](quick-connect.md) for bounded provisioning.

### 2026-10-25 — Implementation and intermediate verification record

The following records explain the intermediate failures; the final acceptance receipt above supersedes pending
statements in this historical snapshot.

Goal: actual connection → reviewed starter panels/sustained alerts → ordinary collection, with the same native
HTTP/CLI contract for agents. Implemented read-only real-protocol probes, private-snapshot plan proofs, atomic
collector/dashboard/rule/receipt transactions, concurrent idempotent retries, existing-secret reuse, fixed-loopback
exporter discovery and separate Infrastructure cards. Recognized host/database profiles use curated observed-only
panels; generic starters are bounded. GitHub Dark console tokens remain independent of chart/gauge palettes.

Model, collector, role, store, server and CLI checks passed; frontend 169 units and production build passed.
Full local Go tests and vet passed. Edge exercised real Node discovery and separate actual MariaDB connection,
observed-only preview, plan-drift 409/review-again, atomic creation, populated CPU/memory/NIC and 19 MySQL panels.
Final source, responsive and production checks remain separately gated below.
The actual query/alert engine regression covers native MySQL/Redis/PostgreSQL plus an HTTP exporter: fresh
`up=0` continues the two-minute pending timer after old backend `_up` expires, then fresh success must pass
the full one-minute recovery confirmation. The original rule failed at 90 seconds; the independent fresh-failure
branch fixes it and all four cases pass. Final deployment is pinned after this correction.
On that corrected source, both database suites passed; browser/account flows also passed, but one repeated
browser run reached 248086528 cgroup bytes, violating the unchanged 230 MiB headroom gate below its 256 MiB limit.
The small Docker runtime now defaults to the standard Go `GOMEMLIMIT=128MiB` soft limit (operator-overridable);
the explicit integration/database image is unchanged. Runtime revalidation retains the original memory assertion
and logs anonymous/file/kernel memory separately. Final results are recorded after the new runtime checks.
An initial Docker baseline passed actual database and persistence suites twice but found an obsolete product-name
locator in the browser authentication script. That locator is fixed with the role/security assertions retained,
and current embedded-build auth smoke passed. New Docker cases now require real probe/apply/retry/dashboard and
exact-scope ingestion for four databases plus eight protocol fixtures. Final immutable-source Docker/browser,
worker and production deployment are still pending; earlier-source results do not establish these gates.

Narrow dependency maintenance updates Zod 4.6.5 to 4.6.7 after
[GHSA-mrhx-c7xc-m3w5](https://github.com/advisories/GHSA-mrhx-c7xc-m3w5); npm audit reports zero known advisories.
This does not establish exploitation of the application's Zod call sites or a general security certification.

Next: finish rendered connection/resource interactions, run formal Docker twice with Browser/database coverage,
publish exact reviewed source, then verify production Edge. The historical weighted roadmap stays 94% (see the
existing eight workstream table); it is not a percentage for this broader continuing goal. Live vendor clusters,
automatic arbitrary network discovery, every Grafana/Netdata panel and npm/NuGet registry uploads are not inferred.
See [the quick-connect contract](quick-connect.md) for limits and per-layer evidence boundaries.

### 2026-10-23 — Final GitHub Dark cards and network acceptance

- Runtime 0.2.24 fixes source `87258d928e3e270fddfaf76d243bd5571bfb2efb`. The native worker passed full Go
  tests/vet, 169 frontend units/build, native account/token readiness, collector builds and archive/download
  checks. Deployment 439 succeeded; runtime health reports 0.2.24 and database=ok. Archive SHA256:
  `d9677e0a1a2f02f942efc7ef33ca46c757912535c3313192772bab3c77a1ab620`.
- The existing LAN collector was upgraded from the fixed 0.2.23 source. Its actual new counters and 17 NICs
  were observed through the production UI, beyond the initial installation acknowledgement. The existing Linux
  exporter was extended through a backed-up systemd override; netdev success and four receive-counter series
  were read back. Business and physical-NIC bytes stay separate and idle business counters honestly show zero.
- Edge verified all seven real identities and their explicit host resources; a newly observed SDK host association
  was completed only after confirming its actual running executable on that machine. Environment/category
  filters, global host-IP ordering, Chinese multiline note save/search/reopen/restore and actual per-NIC switching
  passed. Same-scope refresh preserved the actual card DOM node and chart canvas node after completion, the
  unsubmitted search draft and chosen NIC. This establishes no remount in these exercised refresh flows, rather
  than a claim about every browser/driver timing.
- Final desktop 1513×1039 and mobile 390×844 were inspected, with mobile client/scroll width both 375px
  (15px scrollbar). The final production validation tab had no captured warnings/errors. Both generated concepts
  and the latest actual screenshots were directly viewed; the five-point comparison and intentional functional
  differences are recorded in [the instance contract](instance-directory.md).
- Go reporting package tests and all six Rust reporter tests, including actual HTTPS/token/payload-alias proof,
  passed. Producer changes are pushed separately; this monitoring release does not assert that a new game
  binary was deployed. Current production legacy-counter fallback was exercised. No synthetic metrics, load
  spike or new Feishu notification was injected for this UI acceptance. Overall roadmap weighting remains 94%.

### 2026-10-23 — Card release and sanitizer patch

The immutable 0.2.23 worker for `6d0eae8fd08350e5cbba9a3cb77fa13be4291b97` passed all Go tests,
frontend 169 units/build, readiness and artifact checks, and deployment 438 succeeded. The existing LAN
collector upgrade was installed; installation acknowledgement and first network heartbeat are separate checks.
Final production Edge interaction and network acceptance are still being verified.

The publication audit reported DOMPurify 3.4.16 advisories. Our text-panel call sanitizes strings with the HTML
profile; it does not enable `SAFE_FOR_TEMPLATES`, DOM-return modes, hooks or XHTML parsing. No exploitation of
this call was established. The dependency is nevertheless upgraded narrowly to 3.4.17, the
[maintainer's patched release](https://github.com/cure53/DOMPurify/releases/tag/3.4.17). npm audit then reports
zero known vulnerabilities; this is a dependency advisory check, not a general security certification.
The patched immutable production release remains a separate gate.

### 2026-10-23 — Instance cards and independent console/visualization themes

Global directory filters and counts precede pagination; freeform notes use a separate Editor CAS revision.
Business payload/HTTP body rates and per-NIC host measurements are distinct. The GitHub Dark console palette
does not replace classic time-series, gauge or risk palettes. Plot metadata now gives separate query frames a
global series index while preserving authored fixed colors and source frames.

Targeted Go/store/auth/PromQL/host tests, frontend units/build and Rust actual HTTPS passed. Edge locally verified
seven populated cards, category and note search, multiline saved notes, separate business/NIC views, and a 390px
viewport without horizontal overflow. The initial full Windows Go run found the new Bps unit missing from native
validation; that contract and its host-dashboard regression were fixed and passed. Fixed-source worker, actual
collector rollout and final production Edge acceptance remain separate pending gates.

### 2026-10-23 — Fixed-source 0.2.22 production acceptance

- Immutable runtime source `28b6c727c298887659f1ee08d08d3ab53e2d7ac4` passed the complete fixed-source
  worker: frontend units/build, full Go tests/vet, account/token readiness and archive verification. Official
  build 1077 and deployment 430 succeeded; public health reports 0.2.22 and database=ok. Artifact SHA256 is
  `ee0e7925182957469ae133ffc65623b811bbdaf746ab664e78c6532d5da0e74d4`.
- Edge used the existing authorized administrator session to run native stored PromQL queries for actual
  SDK, gateway and logic HTTP/WebSocket, Go goroutine and Rust Tokio task series. Seven actual business
  identities include the new shared SDK; its process/HTTP/outbound Link/RPC dashboard works without player
  population cards. Navigation and lazy routes work after explicitly refreshing the previously stale client.
- The real SDK SQL page renders table statistics and the slow-template/table grouping view. The selected
  production window has no reported templates meeting the 200 ms threshold, with an explicit honest empty
  state. Populated variant grouping, redaction, authentication and exact-instance isolation are covered by real
  local HTTP tests; no artificial SQL or synthetic business metrics were injected into production.
- The existing 0.2.21 artifact belonged to another source and was not overwritten. A subsequent worker caught
  the original ANN recall defect before publication; the queued-bridge and true-maximum fixes retain the
  recall requirement. Runtime release evidence is distinct from future device, capacity and multi-node checks.

### 2026-10-23 — Publication resilience regressions

Edge reproduced a blank page when a stale client requested a removed lazy asset. A page recovery boundary now
keeps the application navigation/session controls visible and offers explicit refresh, without automatically
discarding edits or exposing the exception. Its fallback/recovery unit and TypeScript checks passed.
A full worker run also exposed an existing ANN recall failure. A failing queued-bridge regression identified
premature termination; min-heap maximum pruning was independently incorrect. Both are fixed without reducing
the recall assertion. Vector-store/HNSW suites and 30 consecutive original recall checks passed. The exact
fixed-source production worker and final Edge acceptance subsequently passed as recorded above.

### 2026-10-23 — Bounded slow SQL diagnostics and transport metrics

Native slow SQL counters now feed an exact-instance read API with redacted template variants, bounded Levenshtein groups and table summaries. SDK identities have process/HTTP/WebSocket/RPC panels and do not claim player populations. Targeted real HTTP/auth/instance-isolation/SQL grouping tests and TypeScript compilation passed. Producer lifecycle and TLS ACK tests passed separately. Final fixed-source worker, deployment and Edge evidence are recorded above; see docs/slow-sql.md for semantics.


Updated: 2026-10-23. This file is the durable progress ledger. Update it whenever a progress unit completes.

### 2026-10-23 — Console layout and current-value display

- One aligned, environment-grouped instance directory replaces repeated tables. Search, bounded paging, opt-in
  recent-report ordering and stable default service-number ordering preserve identity and freshness semantics.
  Instance navigation, a single panel-size control and expandable settings/history foreground physical-host,
  process and business resources without losing inherited templates, SQL sorting or player-region lists.
- Decimal MB/GB, complete browser-local timestamps and a separately rendered GMT clock apply across product
  views. Current stat cards omit per-point timestamps; source freshness is checked independently of PromQL
  evaluation time. Historical queries retain their meaning and valid zero samples are not treated as missing.
- An isolated candidate excluding unrelated work passed 167 frontend units, the production frontend build and
  targeted business/host Go tests and vet. Fixed-source private 0.2.21 subsequently passed the complete worker
  Go suite/vet, frontend units/build, native account/token readiness, collector builds and archive/download checks,
  then deployed successfully. Runtime source is 548ab439f37034e8c5e0f38e1a7f0816bbdd923d.
- Edge verified the actual six reporting identities, environment selection, retained filters, live LAN host
  instruments, decimal process memory, recent-hour resource history and SQL multi-sort. Native desktop 2514×1234,
  concept-sized 1586×992 and mobile 390×844 were inspected; mobile had no horizontal overflow and the production
  console had no relevant errors/warnings. Both generated concepts and latest production screenshots were viewed
  directly for the five-point comparison. Illustrative mockup controls/data are intentionally not copied.
  Overall roadmap weighting remains 94%; this does not certify every optional subsystem or new gameplay login.

### 2026-10-22 — Reporting-server resource and sorting production acceptance

- Fixed-source private 0.2.19 completed full worker tests/vet, frontend units/build, native account/token readiness,
  cross-platform collector builds, archive/provenance checks and the exact authorized deployment. All six existing
  identities now have explicit physical-host associations. Three process queries (CPU, RSS and uptime) per instance,
  the LAN host snapshot and the production node-exporter CPU/memory/disk queries returned real data.
- Edge displayed the actual LAN host gauges/capacities, last-hour curves, configured sustained/recovery/1h-CD
  rules and the game process resources. Existing five inherited templates remain. A formal game server's SQL view
  sorted the complete 84-row eligible set by calls descending and latency descending before returning ten rows.
  Header-click, Shift-click and the priority editor were verified on the real production page.
- The new signed Feishu card test received one successful provider HTTP 200 ACK; it was explicitly marked as a
  notification test rather than a fabricated resource fault. Prior native-engine firing/recovery/CD acceptance
  remains a separate completed gate. No new player-account Login/Ready/Ping acceptance is claimed.
- Follow-up discovery metadata describes resource reads, Admin-only host association schemas and multi-sort cursor
  semantics. The UI marks stale reporting and hides current node-exporter gauges while preserving historical
  trends when collection is paused/stale. Targeted discovery/business tests/vet and 163 frontend units/build
  passed. Fixed-source private 0.2.20 subsequently passed the full worker/archive/readiness gates and exact
  production deployment. Live discovery returned all four resource/host/ranking operations with the expected
  roles; all six process/physical-host associations and complete-set SQL sorting passed again. Source runtime
  is e5f93cb41065d4d88c5605006cdc64c8edff72dd; later acceptance documentation does not change its code.
  Overall roadmap weighting remains 94%.

### 2026-10-22 — Reporting-server dashboards and multi-column ranking

- Business instance pages now distinguish physical host resources, the actual service process and business panels.
  Administrator-confirmed, versioned associations support managed hosts and existing node-exporter collectors;
  neither reported IP nor shared NAT peers create associations. Infrastructure credentials stay outside the
  readable resource projection. Existing inherited/custom panels remain intact and receive a separate process view.
- CPU (one-core process percentage), RSS, numeric uptime, Go heap/goroutines and real gateway/traffic metrics
  are available in built-in instance views. The self Overview explicitly directs reporting-server inspection to
  Dashboards. Template controls are collapsed and the time controls no longer stretch across several rows.
- SQL and region tables support five ordered sort keys, numeric missing-last handling, deterministic ties and
  global ordering before pagination/Top10. Cursors reject changed sorting and eligibility; SQL defaults to 60 minutes.
- Targeted business/store/auth tests and vet passed, including host-binding CAS, Viewer/Editor restrictions,
  safe projections, environment/restart separation, complete-set SQL sorting and cursor boundaries. Frontend
  units passed 163/163 and the production build passed. Edge fixture rendering displayed all resource sections.
- Production 0.2.18 stopped at an SSH handshake reset before any target mutation; 0.2.17 remained active and
  healthy with all six actual reporting processes. A new immutable release and live resource associations are
  the next acceptance gates. Overall product roadmap weighting remains 94%; SDK work is separate.

### 2026-10-22 — Embedded storage production accepted; host-card clarity follow-up

- Fixed-source private version 0.2.17 completed the same full worker gates through the authorized SSH operator
  path and existing release control plane. The stopped 16661384-sample source passed reopened exact-float/count
  checks, normalized 40 empty-label series, reclaimed old metric rows and started with embedded TSDB/vector storage.
  Control and identity/key permissions remained protected. This was not a new BuildWorld job.
- Online readback confirmed new samples, indexed last-hour host history, all six existing business identities,
  two active collectors and editable 15-day retention. Initial native statistics were about 42.0 MB; the stopped
  SQLite control file was 588.1 MB. Retained safety backups are separate from active storage figures.
- Signed Feishu acceptance on the native engine passed 24 state transitions, one firing and one recovery ACK,
  second-cycle 1h-CD suppression and owned-rule cleanup. Ingest-only credentials passed 401/403 read/admin isolation.
- Follow-up host cards explain idle CPU, available memory/free disk, human disk names and the actual configured
  threshold graph/durations instead of relying on reference text. Custom/paused/unknown conditions do not invent a
  policy. Rules load once per page every 30s; same-scope refreshes retain panels. Technical details are collapsed.
  160 frontend units/build, owning alerting tests/vet and Edge fixture rendering passed. Online clarity rollout is
  still pending. Overall product roadmap weighting remains 94%; this does not certify every optional workstream.

### 2026-10-22 — Embedded storage (earlier local phase and failed-first-cutover investigation)

- Separate lossless XOR/Zstd TSDB, durable Badger/HNSW vectors and SQLite control metadata. Native indexed PromQL,
  versioned retention, per-organization engines and bounded memory/query work are implemented. Offline migration
  backs up catalogs/keys and checks counts, float bits and vector payloads after reopening before reclaiming legacy rows.
- Go full suite/vet, 157 frontend units/build, Linux abrupt-exit/migration and million-point profile passed. The
  profile measured 68.37 MB SQLite versus 5.05 MB TSDB / 6.34 MB complete native data. Production Docker runtime
  token/local auth, restart and fresh-volume backup recovery passed in 68.69s under 1 CPU/256 MiB. See embedded-storage.md.
- Next: fixed-source private worker release, stopped-data migration, live reporting/query continuity and final Edge UI
  acceptance. This entry does not claim that a source implementation alone changed the online storage backend.
- Linux race validation completed all other packages, including store (833.27s) and vectors. The sole server failure
  assumed a container always has a writable local disk; that fixture now accepts explicit collection_error rather
  than fabricating a healthy resource. The corrected server package race run passed in 110.89s. These are separate
  runs, not a claim that the first full Docker validation target was green. Existing overall roadmap weighting is 94%.
- The online baseline remained 0.2.15 / SQLite with about 15.5M samples. Edge could read and capture the host UI,
  but navigation/submit commands stalled after reconnects. Read-only build records still showed the prior successful
  release; operator submission has not yet produced a verified new build receipt. Production migration remains pending.
- A subsequent authorized fixed-source SSH worker completed the private release gates and attempted cutover.
  Exact migration verification rejected 40 empty-label series; the deployment restored stopped data and the old
  binary, leaving reporting healthy on 0.2.15. Offline comparison found all 16106623 samples present, with the
  difference limited to Prometheus omitting empty labels. A failing regression now passes with canonical identities,
  explicit collision refusal, normalized-series accounting and per-series decoding. Full retained-data replay then
  passed reopening/count/float-bit fingerprint checks for all 16106623 samples and 40 normalized series in 166s.
  The original offline source and protected backups remain. A new immutable online release is still pending.

### 2026-10-22 — Staggered recovery grouping (fixed-source rollout and provider retest accepted)

- The fixed-source host/UI release passed its private worker gates and was deployed; the updated dedicated Darwin
  collector reported actual CPU, memory and writable data-volume observations. Six sustained/recovery/CD rules and
  native host/MySQL dashboards were configured through the authenticated API. Exact private-host evidence remains
  in the private deployment ledger, separate from visual fixtures.
- Real-clock notification acceptance completed two full cycles for all three resources (24 transitions). It found
  one firing plus two recovery cards, caused by resource evaluations resolving two seconds apart. This was a split
  first-period recovery, not a second-period firing bypassing CD. The owned temporary rules were removed.
- Added a deterministic failing regression for staggered group recovery, then a quiet-window fix for CD-enabled
  groups. Existing zero-wait behavior is preserved and recovery waits are capped at 30 seconds. Targeted grouping,
  cooldown and Feishu tests passed. Fixed-source 0.2.15 rollout passed the full private worker. A new real-clock
  run produced exactly 24 transitions, one firing and one resolved signed Feishu ACK; second-cycle notifications
  were suppressed by the 1h CD. Owned temporary rules were removed. The earlier failed acceptance remains distinct.

### 2026-10-22 — Host resources and operations UI (source/browser accepted; production pending)

- Goal: add local CPU, reclaimable-memory and writable-disk observations to the existing bounded outbound agent;
  expose a scoped host dashboard, last-hour views, native threshold instruments and concise Feishu fields.
- Changed: `internal/hostagent`, `internal/telemetry`, host catalog/HTTP validation, native host dashboard/rule
  factories, shared first-party UI tokens, host resources page and gauge renderer. No extra exporter/listener.
  macOS mount enumeration uses cached local metadata so a stale network mount cannot block enumeration.
- Verification: full local Go package suite passed before the final bounded-observation validation; the final
  affected telemetry/server/alerting cases and owning-package vet passed separately. 157 frontend units passed;
  TypeScript/Vite and CGO-disabled Darwin arm64 build passed. Edge rendered the actual embedded host page with
  explicitly marked fixture observations, all three needles/last-hour trends, disk capacity and collapsed actions.
  Those fixtures are visual QA, not live host reporting or real notification-delivery evidence.
- Next acceptance: fixed-source private release, actual OS agent samples and dashboard, persisted sustained
  thresholds/recovery/cooldown, real Feishu provider ACK and second-cycle suppression. Storage compression work is
  separately in progress; no benchmark or data migration is claimed here. Overall roadmap weighting remains 94%.
- Risks: reference gauge rails are identified as reference values; notification policies use versioned rule settings.
  Foreign-platform build inspection is separate from execution. See [host resources](host-resources.md).

### 2026-10-22 — Native panels, managed hosts and MySQL (scoped acceptance complete)

- Common native panels add categorical bar/pie and state views, bounded virtual lists, honest gaps and corrected
  histogram semantics. Navigation groups monitoring, metadata and system settings. Counts omit thousands separators;
  explicit intranet identities remain separate from release/test/shared.
- Admin host control includes encrypted credentials, revision checks, pinned SSH keys, checksummed fixed installers,
  dedicated non-login service accounts, independent outbound credentials, pause and credential revocation. MySQL uses
  fixed read-only status/capacity queries and an 18-panel dashboard with fixed host scope and preserved customizations.
- `internal/telemetry` and `collector.TargetStore` decouple protocol readers from control persistence. The stripped
  macOS arm64 agent decreased from 76338898 to 11080050 bytes without linking the control store/query engine/web UI.
- The unified candidate passed 155 frontend units, TypeScript/Vite/webui builds and full owning Go packages/vet;
  the final dashboard-language/ID correction passed its targeted model/server tests and another frontend build.
  Edge verified all eleven native panel types, 240-category virtual scrolling, state gaps/mappings, grouped navigation,
  host creation/pause/revocation, and eighteen panels with actual read-only MySQL data. Browser warnings/errors were empty.
- Final runtime-source Docker end-to-end passed in 201.59 seconds for SQLite and ClickHouse, including real MariaDB,
  Redis/PostgreSQL/ClickHouse collection, native SQL and restart/isolation/persistence checks. It used one 2 CPU/2 GiB
  container and removed its containers/network/volumes. This does not claim a new race-detector or hosted-CI run.
- Immutable production releases 0.2.12 and 0.2.13, target service installation and independent Darwin heartbeat/sample
  acceptance passed and are recorded separately in the private ledger. Overall weighting stays 94%; no general shell orchestrator, automatic LAN relay,
  whole-cluster completion or universal production certification is claimed. See [managed hosts](managed-hosts.md).
- Production 0.2.12 and a real Darwin arm64 LaunchDaemon were accepted separately: protected per-host bootstrap,
  dedicated non-login identity, matching artifact digest and 467 actual MySQL samples per batch. Initial zsh installation
  revealed tail-exec changing sudo's no-TTY parent ticket. The corrected installer keeps the shell alive, passes the
  parent-identity/cached-credential regressions and the real Mac installation. Source `24d3805f8556ce66fd119ad45cfe70902a4a2dc9`
  was then published as 0.2.13 with full worker Go/frontend and native token/local readiness gates. The actual Mac agent
  was updated from the matching approved artifact. A real control round trip reached paused version 2/2 and then
  collecting version 3/3 with another 467-sample batch. Public HTTPS missing/wrong ingest credentials returned 401,
  ingest-only query/admin access returned 403 and an authenticated empty batch was rejected without storing samples.
  Dedicated process identities, protected files, exact runtime provenance, preserved stopped-data snapshots and
  authenticated Edge views were checked. Final documentation commits do not change that deployed runtime source.

### 2026-10-21 — Template center and instance dashboards

- Navigation now distinguishes reusable templates from real instance dashboards. Unbound business templates
  preview only authorized live inherit bindings; one binding is selected automatically, multiple bindings
  require an explicit choice. Editing the shared template never persists a preview instance's fixed scope.
- Instance lists group explicit `release`, `test` and `shared` environments; unknown values stay separate.
  The default time range is 60 minutes, with explicit URL/user choices preserved. Large panel sizing is
  selectable and remembered. Refresh keeps same-scope content; changing instance clears the previous scope.
  Unknown metric-description guidance stays in the details dialog; main stat cards show only known or
  explicitly configured descriptions. This last presentation-only correction passed its targeted browser
  regression and 14 presenter unit tests, with null/zero values and units unchanged.
- SQL defaults to the ten Entity/operation pairs with highest weighted average latency in the last hour,
  with milliseconds and estimated execution/failure counts. Player regions remain complete and virtualized.
  Background ticks skip an in-flight SQL query instead of repeatedly canceling a valid slow response.
- Native panel editing supports bytes/second through optional `display_unit=binBps` with base `unit=bytes`.
  Custom field settings, queries, thresholds, owner metadata and revision checks are retained.
- Feishu cards use Chinese states, Beijing time and notification cooldown wording. Contact-point tests and
  temporary acceptance runs are clearly marked as tests; recovery never presents a masked zero as actual
  resource usage. Custom text, plaintext escaping, signing, encoded size limits and provider ACK checks remain.
- Validation: 147 frontend unit tests, TypeScript/Vite/webui builds, targeted business binding/ACL and
  panel-edit HTTP tests, server vet, full alerting/notification-template package tests and targeted Feishu
  regressions passed. Browser checks cover seven real-query fixture panels including zero, scope switching,
  60-minute defaults, large sizing, five-second refresh without clearing content, a 6.2-second SQL response,
  all 235 fixture regions, SQL Top10, and bytes/second editing with persisted configuration readback.
- These are scoped source/build/browser checks. Immutable production deployment, real reports and real
  notification delivery are separate private-deployment acceptance layers; no new Docker/race or player
  test-account acceptance is claimed by this entry.

### 2026-10-21 — Readable panels and metric details

- Stat/gauge/bar-gauge, native time-series and simple Explore charts now use readable metric/series names.
  Explicit aliases stay authoritative; known metrics have exact bilingual definitions, unknown names remain
  inspectable, business/resource dimensions distinguish series, and raw selectors live in a keyboard-accessible
  details dialog. This is presentation only, without query/identity/permission changes. See [panel readability](panel-readability.md).
- Uptime stays numeric with restart semantics and query-point time. Missing gauges no longer pretend to be zero;
  all-values mode associates each displayed value with its own timestamp. Time-series left/right axes keep their
  own units and percent stacking; details metadata is memoized independently of cursor updates. Explore no longer
  borrows an unrelated metric-picker unit for PromQL, subsecond durations retain precision, and simple charts
  provide valid stable colors and legends for all series.
- Initial frontend verification passed 138 units and embedded builds. The first full browser sweep passed 93,
  skipped two SQL-environment cases and found three failures: a newly added fixture assumed its requested dashboard ID was retained,
  SQL comparison aliases retained the SDK's automatic English suffix in Chinese, and one navigation hit local
  `ERR_NO_BUFFER_SPACE`. These are retained as failed-run evidence; corrected-source acceptance is recorded below.
- Corrected source passed all **139 frontend units**, TypeScript/Vite and an embedded webui build. A final
  **nine-case browser run passed with no retries in 23.6 seconds**, covering all original failures, aliases,
  twelve simple series, Chinese uptime/details, 390/768/1280 containment, all-value timestamps, zero/missing gauges,
  independent percent/bytes axes, comparison localization and the full bilingual workspace flow. The old workspace
  assertion for a raw PromQL legend now checks the expression through Metric details. The complete sweep was not
  repeated after these scoped corrections; its earlier results are not reassigned to final source.
- Online release remains a separate gate in the private deployment ledger. Existing
  v0.2.6 full Go/CI/Docker evidence retains its original source attribution. The user explicitly excluded signing
  into a game with a test account; this UI acceptance does not claim a nonzero player lifecycle.
- Live v0.2.7 acceptance found three built-in self metrics still using their technical names. The follow-up
  explicitly defines Go allocated heap (not RSS), goroutines and cumulative handler requests, and distinguishes
  native request-rate aggregation from the source counter. This follow-up has its own source/verification gate;
  the earlier UI results are not silently reassigned to it.
- The self-metric follow-up passed **141/141 frontend units**, TypeScript/Vite, an embedded webui build and the
  same **nine browser cases without retries in 29.4 seconds**, now including the real default overview's three
  friendly self-metric legends. The final live upgrade is recorded separately by the release operator.
- Weighted overall remains 94% under the existing workstream basis; this scoped UI correction is not a new
  sustained-load or universal production-certification claim.

### 2026-10-21 — Transport timeout diagnostics

- A full Linux race run exposed a deadline ordering case: the HTTP client's timeout can finish before the
  request context's timer. Notification delivery now recognizes wrapped context and network deadline errors
  as the same fixed, sanitized timeout diagnostic. Cancellation and ordinary transport failures retain their
  generic diagnostic; receiver URLs, credentials and error bodies are never persisted. Retry and cooldown
  behavior are unchanged.
- Windows `go test ./internal/alerting -count=1 -timeout=3m` and `go vet ./internal/alerting` passed.
  Four deterministic transport cases supplement the existing real TLS timeout/cancellation test. The final
  Linux race rerun, immutable release and production acceptance remain separate gates. Overall weighting below
  is unchanged.
- The fresh full browser sweep of the preceding UI candidate passed 93 cases, with two SQL-environment skips
  covered by the separately executed six real SQL Docker browser cases; no failures or retries. This
  notification diagnostic change does not modify the UI or database contracts.

### 2026-10-21 — Feishu cards and safe message templates (local acceptance)

- Feishu now defaults to an interactive card, with explicit text compatibility. Cards show localized status/severity,
  host/resources, rule evaluation values, configured trigger/recovery periods, cooldown and event timestamps; raw
  PromQL and internal labels stay out of the default view. Zero-valued recovery conditions are distinguished from
  zero actual resource use. Formal rule timing/state evaluation is unchanged.
- Existing saved title/message templates and their durable render snapshots are reused under the existing
  process-local step/output/time limits. Card data remains plain text, mention/link injection is neutralized, and
  the only action URL derives from configured HTTPS root. Admin-only sample preview does not queue or send.
- Targeted Go Feishu/template/CD and role tests, vet, frontend typecheck/build and four real notifications browser
  cases passed locally. Linux Go 1.27.2 + GCC vet/race over four owning packages passed twice (80 top-level
  tests, no failures/skips/races). Actual TLS fixtures cover grouped firing/recovery, rejected partial ACK and cooldown;
  this is separate from a real Feishu group delivery. The unified final UI passed four notification and three
  business-instance browser cases without retries. Real database integration passed twice (including six SQL
  browser cases), followed by two newly built final-source runtime/browser rounds in 173.509 seconds. Image
  content was 42.8 MiB and Docker local footprint 163 MB; verification resources were removed and read back empty.
  See `docs/docker-validation.md` for source-layer distinctions, exact resource snapshots and commands. The full
  45-minute Docker race target was not rerun in this cycle. Publication/production card acceptance remain with
  the release operator. Overall weighting below remains unchanged (not universal certification).

- Final Windows browser sweep completed with 91 passed, two SQL environment skips (covered by the six real SQL
  Docker browser cases above), and two test-harness failures. The isolated reporting-source fixture and native
  stat renderer selector were corrected; four reporting/risk cases, the snapshot case and all five layout cases
  then passed without retries. A fresh whole-suite run and immutable release CI are separate pending gates.

## Reporting protocol (permanent)

After each completed progress unit, report in this order:

1. **目标 (Goal)** — the outcome this unit was meant to deliver.
2. **做了什么 (Done)** — commits, files, behavior changes and evidence.
3. **接下做什么 (Next)** — ordered next units, each with an acceptance check.
4. **总进度 (Overall %)** — weighted overall completion plus the per-workstream table below.
5. **风险 / 阻塞 (Risks / blockers)** — what could invalidate the claim.
6. **验证 (Verification)** — commands actually run and their results.

Rules:

- Update the ledger and percentage in the same change as the work.
- Never report a unit as done without executable evidence; distinguish local, browser, Docker and CI evidence.
- Percentages are progress estimates, not release promises. State the weighting basis.
- Keep the report in the user's language (Chinese in this project).

## Completion ledger

Weights reflect product risk and user-visible value.

| Workstream | Weight | Progress | Basis and remaining work |
| --- | --- | --- | --- |
| Product shell and first-party UI | 15% | 95% | Business onboarding, lazy features and deterministic modal focus; optional motion is not a production gate |
| Renderers | 15% | 100% | Native panel matrix, Loki fixed HTTP contract and actual Tempo ingestion/API/browser plus final full CI accepted; supported scope is explicit |
| Collection | 15% | 95% | C1 enforces decoded/expanded budgets, identity validation and fail-before-write behavior for supported protocol subsets; sustained external exporters remain target-specific |
| Storage and query | 15% | 95% | D1 SQL bucket-last planning, selected raw-sample budgets, real 1M ClickHouse corrections/EXPLAIN and 5M SQLite rejection/retention verified; sustained cardinality and disk growth remain target-specific |
| Auth and security | 15% | 95% | A1 namespace authentication, shared password throttling, trusted-proxy chain and canonical Origin regressions passed; scoped review is not a general security certification or public TLS acceptance |
| Ops and delivery | 10% | 100% | Exact v0.2.28 full tag CI, reproducible five-platform archives, anonymous downloads, backup/recovery and actual native deployment/Edge acceptance passed as separate gates |
| Docs and agent contract | 5% | 95% | Source contracts and measured boundaries synchronized |
| Performance | 10% | 95% | Worker/min-max, SQL/ClickHouse plans and ten-minute confined embedded HTTP/PromQL profile measured; main-thread JSON parsing and longer operational SLOs remain |

**Weighted overall: 96%** (weights total 100%, ±5 points). The estimate covers the supported single-node product
scope and evidence recorded above. Immutable-source CI/release, anonymous distribution and actual deployment
are separately established. This is not availability, a release promise, a claim that every
exporter or Grafana feature is supported, or universal production certification.

## Definition of done by workstream

### Renderers

- Each kind has a registry entry, a typed contract and a browser test that asserts real paint or DOM output.
- Product pages never import `@grafana/ui`; the compatibility bridge remains only for imported resources, plugins and
  the explicit `?renderer=compat` fallback.
- Responsive containment (390/768/1280), dynamic panel size and empty/error states are covered.

### Performance

- A reproducible benchmark reports first-frame time, draw time, decimation ratio and heap for a fixed large frame.
- Downsampling preserves peaks and gaps; zooming re-queries instead of silently discarding detail.
- Budgets are recorded in `docs/performance-baseline.md`; no universal latency claim without measurements.

### Product shell

- Collapsible left navigation, right content region, keyboard focus order and mobile collapse work at 390/768/1280.
- Routing, permissions and deep links are unchanged; the shell uses `web/src/ui` primitives.
- ReactBits, if introduced, is limited to decorative motion and respects reduced-motion preferences.

### Collection, storage and security

- New collectors and query paths ship with protocol tests and a failure-mode test (timeout, auth, malformed input).
- ClickHouse-only behavior states its SQLite fallback or limitation.
- Security-relevant changes update `docs/public-deployment.md` and the Admin/Editor/Viewer matrix.

## Recent delivered units

### G2 working candidate — 2026-10-21

Local-account administration and a distinct five-path ingest credential, offline protected bootstrap, persisted 15-day SQLite retention, native panel risk thresholds/numeric uptime, Chinese onboarding and authenticated `/api/v1/agent/capabilities` are implemented. Targeted Go/roles/CAS tests passed; browser risk/retention cases passed against an embedded candidate. Frontend TypeScript and 120 unit tests passed. The whole Windows Go run found one stale default-uptime expectation; it was updated to assert the intended stat panel and its targeted regression passed. Full final-source regression remains a separate gate.

Sustained alert tests use actual persisted evaluation, outbox and local HTTP delivery in grouped/ungrouped modes: a short spike does not notify, pending survives restart, recovery flapping does not repeat, recovery waits continuously and a second firing remains inside the successful-ACK 1h CD. Production defaults and external provider delivery are private target acceptance; the deterministic test does not pretend to wait one real hour.

Business virtual dashboards and interactive Feishu cards/templates have targeted browser/API evidence. Final candidate is being reviewed for old population series across restarts and safe imported threshold colors. No new public tag, final Docker suite or production deployment is claimed by this working-candidate entry.

| Date | Unit | Evidence |
| --- | --- | --- |
| 2026-10-20 | Agent contract, tag-only CI, external skill reference | `docs/agent/*`; CI run 39436800402 green for `a51c275` |
| 2026-10-20 | First-party TimeSeries default with compat fallback | Full browser suite 68 passed / 2 skipped; `?renderer=compat` fallback |
| 2026-10-20 | Responsive acceptance for charts and tables | 390/768/1280 width tests, dynamic panel height, narrow table containment |
| 2026-10-20 | Stat/Gauge/BarGauge renderer registry entries | `StatRenderer` shared reducer/empty states; browser test asserts stat text, meter ARIA values, bar fraction and no page overflow |
| 2026-10-20 | Shell left navigation and right content | Collapsible desktop rail with localStorage, keyboard toggle, mobile drawer, 390/768/1280 no-overflow test |
| 2026-10-20 | Logs renderer | Virtualized 10k rows, search, label key/value filters, empty/error states; browser contract asserts DOM row count and filter counts |
| 2026-10-20 | Traces renderer | Tempo search table, search, duration/start sorting, optional status and `?traceId=` deep link; browser contract covers all paths |
| 2026-10-20 | Push + pull collection modes | Prometheus remote write (`/api/v1/write`, snappy protobuf), OTLP/HTTP and native JSON push alongside scrape pull; receiver and role tests; `docs/collection-modes.md` |
| 2026-10-20 | Heatmap, histogram and vector renderers | Canvas time-bucket heatmap, normalized histogram bars and 64-dimension vector heat strips; three browser contracts cover counts, hover and overflow |
| 2026-10-21 | D1 storage/query planning | Native SQLite SQL bucket-last, 5,000,001-row rejection/retention, raw PromQL budgets and newline matching; real ClickHouse 26.8 1M rows, historical correction and EXPLAIN; [measured scope](storage-query-planning.md) |
| 2026-10-21 | C1 collection protocol boundaries | Remote-write decoded preflight and invalid-later-batch rejection; OTLP JSON/protobuf/gzip, expanded budgets, generated-label identity and original-point partial success; [supported subset](collection-modes.md) |
| 2026-10-21 | A1 authentication/security review | Real namespace API anonymous 401, session/role access, shared Basic/login failure quota, concurrent quota, trusted proxy chain and configured HTTPS Origin; [focused review](security-readiness-review.md) |
| 2026-10-21 | Unified frozen-source Docker baseline | Runtime + Chromium + real databases twice, 535.733 s; six SQL browser passes with zero skips/retries; complete Linux vet/race rerun passed in 859.4 s after preserving the initial ten-minute timeout failure; [layers and resources](docker-validation.md) |
| 2026-10-21 | Server module/repository migration | 649 files, including 366 Go files, matched the declared transformation; new `metrics233-server/` build context and formal Linux runtime/Chromium passed in 103.085 s; this is distinct from the earlier full race/database source baseline |
| 2026-10-21 | SDK minimum-family toolchain check | Separate standard-library SDK passed actual Linux Go 1.23.12 vet, tests (1.164 s) and race (2.506 s), with local toolchain/network disabled and source unchanged; its optional external-server binary test skipped once per run |
| 2026-10-21 | Fixed reporting-token rejection proof | Actual local HTTP tests use valid native business JSON, both remote-write paths and both OTLP paths: missing/wrong Bearer returns 401 and leaves metric storage empty; the correct fixed token writes successfully. Query/cookie credentials do not substitute for the header. SDK v0.1.0 already has `WithToken`; its package tests passed, with the optional external-server case skipped in this run |
| 2026-10-21 | Feishu notification implementation | Encrypted/write-only bot URL and signing secret; official v2 signing, bounded text, business ACK, timeout/redirect/retry; real local TLS firing/resolved and test-queue fixtures plus native API and local-account role checks passed. UI typecheck/build and 118 unit tests passed; this unit's browser, immutable release and actual robot configuration are tracked separately below |
| 2026-10-21 | Durable alert-notification cooldown | Per-receiver/rule/instance/severity successful ACK imposes the firing minimum interval, including explicit reminders; grouped/ungrouped restart, early recovery, retry, new identity/receiver, rule/contact changes, repeat floor and retained recovery receipt tests passed. API bounds/readback passed; UI browser and production rule configuration remain separate gates |
| 2026-10-21 | Ingest-only fixed credential and source review | Independent environment-only reporting token writes only five exact POST routes; query/management/unblock/Live/unknown access rejected. Real HTTP tests and independent policy-DB failure, metadata failure, partial-write, mapped-IP, organization and concurrent/capacity regressions passed; no production credentials read |

## Next units (ordered)

Execution order and acceptance criteria are maintained in [PLAN.md](../PLAN.md); this table only tracks progress.

1. **Operator-first host defaults** — select a fresh active NIC on first use, preserve an explicit choice and adapt
   instruments to their actual content width. Verify idle/missing interfaces and container/mobile layouts.
2. **Sustained target-environment acceptance** — established production exporters/HTTPS/recovery do not certify
   long-duration cardinality, disk growth or retention for every workload. Live external-daemon coverage remains scoped.
3. **Profile-led capacity work** — D1 range planning is delivered. Further persisted rollups, indexes or streaming
   parsing changes need measured benefit and correction/retention semantics; they are not automatic migrations.
4. **Optional toolchain/motion** — Vite+ and ReactBits remain optional; no migration until parity is established.

## 2026-10-21 production baseline unit

- **Goal / 目标：** harden the small single-node performance + business monitoring workflow and exercise the actual
  Linux runtime; keep test fixtures out of the default product image.
- **Done / 做了什么：** business push-to-native-dashboard flow, lazy routes, transferable Worker/min-max, Tempo
  detail/Loki HTTP adapters, deterministic modal focus, canonical plugin containment, hardened SQLite Compose and
  explicit ClickHouse overlay, production Docker and offline restore validation, followed by D1 query planning,
  C1 receiver limits and A1 authentication fixes. The public server and SDK now have separate module identities;
  Git publication and immutable release status belong to their own delivery checks. Unrelated local work was preserved.
- **Verification / 验证：** frontend 118 unit tests; Business 2/2; TimeSeries 6/6; variables 12/12 across three
  no-retry runs; trace/Loki HTTP browser 1/1; Go vet and full Windows tests; Linux vet and full race tests;
  runtime+browser two rounds and database integration two rounds. The full pre-migration Chromium run reported
  **81 passed / 2 SQL skipped / 0 flaky**; those missing real-SQL cases were separately exercised in the live
  database fixture, which reported three passes per cycle. These are separate runs and are not added together
  into an invented full-suite count. See the evidence layers below.
- **Risks / 边界：** no public deployment or live Tempo daemon claim; 256 MiB is a tested workload profile rather
  than a general capacity guarantee. Default Compose and overlay passed configuration validation; the production
  runtime itself ran in isolated containers. Large JSON parsing still causes main-thread long tasks.
- **Next / 后续：** see the ordered immutable-release, target-environment and measured-capacity checks above. See
  [Docker evidence](docker-validation.md), [performance evidence](performance-baseline.md) and
  [business semantics](business-monitoring.md).

## Evidence layers after D1 / C1 / A1 and repository split

| Layer | Actual result | Boundary |
| --- | --- | --- |
| Windows Go / frontend | Server owning/full vet and tests passed; frontend 118 unit tests and targeted Business/TimeSeries/variables/Tempo-Loki HTTP browser checks passed | HTTP fixtures are not live external Tempo/Loki/exporter installations |
| Full Linux Go vet/race, frozen pre-migration source | Explicit `-timeout=20m` passed; `internal/store` 743.373 s, validation step 859.4 s | Initial default ten-minute timeout was a failed attempt, retained in the record; the real 5M fixture was not reduced or skipped |
| Repeated real database + runtime + browser | `dockerfile_test.ps1 -Repeat 2 -Browser -IncludeDatabaseIntegration` passed in 535.733 s | This complete run predates module rename; SQL browser six passes have no skips or retries |
| New server module Linux runtime | Rebuilt the actual `metrics233-server/` context without an old image; token/local/HTTPS-origin/Chromium and fresh-volume recovery passed in 103.085 s | Full database/race results stay attributed to the earlier equivalent source baseline; committed/tagged CI is a separate check |
| Image and bounded resources | New runtime: 44,728,944 content bytes (42.7 MiB), 162 MB local Docker footprint, UID 10001, one CPU / 256 MiB; token memory 18,268,160 bytes, local 86,056,960, password-work snapshots about 220 MB | Content bytes are not local disk footprint; lower token memory does not represent password-heavy work or a capacity guarantee |
| SDK Linux compatibility/race | Actual `go1.23.12 linux/amd64`, `GOTOOLCHAIN=local`, `GOWORK=off`; `go vet -p=2 ./...`, `go test -p=2 ./... -count=1 -v`, `CGO_ENABLED=1 go test -p=2 -race ./... -count=1 -v` passed | `TestServerBinaryIntegration` explicitly skipped without its optional Linux server binary; local HTTP/TLS fixtures ran; no SDK source edits |

All owned Docker runtime/fixture containers, networks, volumes and temporary image tags were removed with readback
verification. SDK verification used the already-cached Go 1.23 image, a source-read-only/no-network container and
an isolated disposable build volume; the cached image and unrelated shared caches were preserved. No raw private
deployment configuration, addresses or credentials belong in this public ledger. No new tag/release CI result,
public DNS/TLS deployment, live production exporter, long-duration SLO or host disaster-recovery acceptance is
inferred from these local checks.

## 2026-10-21 fixed token and Feishu notification unit

- **目标：** 固定 Token 保护所有指标上报入口，并在既有通知策略/队列中支持飞书群机器人。
- **做了什么：** 确认固定 `METRICSPANEL_TOKEN` 已有完整 Bearer 链路，新增五入口有效载荷的真实 HTTP 拒写/落库回归；SDK 只补同值配置说明，API 不变。新增 `feishu` 联系点、地址和签名密文存储、脱敏读取、保留/清除/CAS 语义、官方 HMAC 签名、20 KiB JSON 限制、明确 `code: 0` 成功判定，以及 TLS、重试、超时、取消、恢复通知和 Admin 写权限回归。UI 提供 Feishu 类型和只写凭据字段。接口见 [Feishu notifications](feishu-notifications.md)。
- **验证：** `go test ./internal/alerting ./internal/server ./internal/localauth ./internal/model -count=1 -timeout=180s` 全部通过（10.263 / 25.482 / 3.007 / 0.051 秒）；随后新增的飞书 firing/resolved 和 local 账号权限用例通过针对性重跑（alerting 1.370 秒、localauth 0.559 秒）。`go test ./cmd/metricspanel -count=1 -timeout=180s` 通过（3.624 秒）；相关五包 `go vet`、`git diff --check` 通过。SDK `go test ./... -count=1` 通过（1.176 秒），可选独立二进制用例本轮未启用。UI `tsc`、正式 build、118 单测通过，新增浏览器配置流程待主任务用最终 webui 二进制验收。
- **后续：** 完成该浏览器文件，再由发布任务构建和发布不可变服务端产物；生产机器人私有配置与真实发群由操作者执行。当前后端测试只向本地合成 TLS 接收端发送。
- **总进度：** 保持加权 **94%**，权重仍合计 100%；本单元不把新增功能的局部验收扩大为通用容量、全量协议或公网可用性认证。
- **边界：** loopback 开发模式未配置 Token 时仍允许本地访问；将其公开到 HTTPS 代理前必须配置固定 Token。固定 token 具有 workspace 管理/读写权限，并非只写或每租户隔离凭据。飞书队列 202 仅代表接受，HTTP 200 也不等于投递成功；本轮尚未宣称真实飞书发送或新版本线上验收。

## 2026-10-21 alert notification cooldown unit

- **目标：** 避免资源告警恢复后短暂抖动反复通知，且不吞首次告警、严重级别升级、恢复或真正的失败重试。
- **做了什么：** 增加规则 `notification_cooldown_seconds`（0–604800；旧配置为 0、新建 UI 默认 3600），成功 firing ACK 与 sent 同事务持久化。分组按成员筛选，非分组持久化延迟并在发送前复核活跃周期；未送达周期提前恢复取消待发 firing 和孤立恢复。编辑/暂停/停启不清 ACK；最长七天锚点与独立活动周期回执保留，避免普通历史清理破坏恢复。新增 CLI schema 与 [时间契约](alert-notification-cooldown.md)。
- **验证：** `TestNotificationCooldown*` 调度测试覆盖实际本地 HTTP、分组/非分组、重启、失败、历史清理和静默恢复提醒；原生 API 验证 0/3600/604800 和越界拒绝。原版 same-period 豁免先由 repeat10s/CD300s 真实红测复现，再修复为 10、299 秒不发、300 秒发；CD0 仍按 10 秒，4h repeat 不提前，repeat0 不变成隐式提醒，503 重试保留原 ID/5 秒退避，真正恢复不被旧 CD next_at 延后。最新专属测试通过（2.522 秒）。最终 `go test ./internal/alerting ./internal/server ./internal/localauth ./internal/model ./internal/requestip ./cmd/metricspanel -count=1 -timeout=180s` 全通过（18.485/34.672/4.625/0.068 秒；requestip 无独立测试文件；CLI 5.093 秒），相关七包 `go vet`、`git diff --check` 通过。
- **后续：** 规则 UI 浏览器核对新建 3600、编辑旧规则 0 保留、字段提交；随后与飞书、固定 Token 同一新版本发布。生产资源规则的冷却值由操作者按实际规则配置，不自动改业务阈值。
- **总进度：** 保持加权 **94%**，权重合计 100%。这些局部新功能没有抹去容量、外部依赖和目标环境的验收边界。
- **边界：** 持续 Firing 默认就不 repeat；`repeat_interval: 0s` 是关闭提醒，不是零延迟循环。非零 CD 是所有成功 firing 的最低间隔，显式同周期提醒至少 max(CD,repeat)；CD0 保留旧时序。CD 不是跨实例的全局消息频率上限。确定性测试推进调度时间验证一小时边界，不冒充已在线等待一小时或已向真实群发送。

## 2026-10-21 ingest-only credential and reporting-source review unit

- **目标：** 可以安全地把固定上报凭据交给业务程序，而不授予查询、修改来源策略或通知配置的权限。
- **做了什么：** 新增仅环境输入的 `METRICSPANEL_INGEST_TOKEN`，要求 token 模式、独立管理 Token、32–8192 非空白 ASCII。精确五 POST ingest 路由允许，匹配该凭据的其余地址/方法统一 403；错误、重复/混合 Authorization 和 query/cookie 替代均拒绝。管理 Token 保留原功能；local 配置专用变量启动前拒绝，且不创建存储。同步 SDK 配置说明，SDK API 不变。见 [reporting access](reporting-access.md)。
- **独立复查：** 未在新增来源策略中确认额外绕过。五路策略表失效返回 503 且未读体/调用后端，错 Token 先 401；辅助观测 trigger 失败不改变实际落库 ACK；remote-write 第二批失败只累计完成的 10000 样本。IPv4-mapped/多字段 XFF/非可信伪头、物理组织隔离/伪 org 头、32 并发观测计数、16 同版策略 CAS 单赢家、满 4096 条策略后失败不能顺带关闭 allowlist，均有可执行回归。
- **验证：** 专属 server/CLI 凭据与来源测试通过（1.731/0.053 秒）；store `go test ./internal/store -run '^TestReporting' -count=1 -timeout=90s` 通过（0.336 秒）。最终完整相关包及 vet 结果见上一单元，保留这些局部证据与旧 Docker/race 基线的区别。
- **后续：** 最终浏览器分别使用随机管理/专用凭据核对来源列表、上报和防自解封；不可变版本发布和线上私有第二 Token 由主任务执行。本地没有读取线上 Token，也没有把真实地址/凭据写入公开仓库。
- **总进度/边界：** 加权仍 **94%**。来源观测是有界辅助目录，不是完整访问审计；已入场请求不承诺因后来封禁而撤回。共享上报 Token 也不是每应用可独立吊销账号，不能把权限收窄测试称为通用安全认证。

## 2026-10-21 final feature browser acceptance

- **目标/结果：** 最终合并的飞书、CD、来源控制和仅上报凭据通过完整 Windows Go 全包测试，以及最终嵌入式 UI 的 Chromium 套件：**86 passed / 2 SQL-environment skipped / 0 failed**，单 worker、无重试，6.5 分钟。SQL 集成先前独立 Docker 证据保持原归属，不把跳过算作通过。
- **实际交互：** 飞书加密字段保留/清除与队列、CD 新默认/旧零值/范围、来源拉黑/解除/允许名单/409 刷新、独立凭据不能解封，以及 390/768/1280 布局已验收。源 IP 的 NAT 和地址变化边界保持明确。
- **后续/边界：** 不可变 v0.2.2 产物、tag CI、生产升级及真实飞书回执分别记录；本条不把本地机器人夹具称为真实群送达。加权仍 **94%**，通用容量与外部服务长期运行未由这些功能测试认证。
