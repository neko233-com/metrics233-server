# Project status and progress protocol

Updated: 2026-10-22. This file is the durable progress ledger. Update it whenever a progress unit completes.

### 2026-10-22 — Embedded storage (local acceptance; production cutover pending)

- Separate lossless XOR/Zstd TSDB, durable Badger/HNSW vectors and SQLite control metadata. Native indexed PromQL,
  versioned retention, per-organization engines and bounded memory/query work are implemented. Offline migration
  backs up catalogs/keys and checks counts, float bits and vector payloads after reopening before reclaiming legacy rows.
- Go full suite/vet, 157 frontend units/build, Linux abrupt-exit/migration and million-point profile passed. The
  profile measured 68.37 MB SQLite versus 5.05 MB TSDB / 6.34 MB complete native data. Production Docker runtime
  token/local auth, restart and fresh-volume backup recovery passed in 68.69s under 1 CPU/256 MiB. See embedded-storage.md.
- Next: fixed-source private worker release, stopped-data migration, live reporting/query continuity and final Edge UI
  acceptance. This entry does not claim that a source implementation alone changed the online storage backend.

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
| Renderers | 15% | 95% | Tempo detail and real Loki HTTP queries covered; no live Tempo daemon acceptance |
| Collection | 15% | 95% | C1 enforces decoded/expanded budgets, identity validation and fail-before-write behavior for supported protocol subsets; sustained external exporters remain target-specific |
| Storage and query | 15% | 95% | D1 SQL bucket-last planning, selected raw-sample budgets, real 1M ClickHouse corrections/EXPLAIN and 5M SQLite rejection/retention verified; sustained cardinality and disk growth remain target-specific |
| Auth and security | 15% | 95% | A1 namespace authentication, shared password throttling, trusted-proxy chain and canonical Origin regressions passed; scoped review is not a general security certification or public TLS acceptance |
| Ops and delivery | 10% | 90% | Repeated formal runtime/database recovery and new-module runtime passed; split-repository immutable CI/release and actual deployment remain separate gates |
| Docs and agent contract | 5% | 95% | Source contracts and measured boundaries synchronized |
| Performance | 10% | 90% | Worker/min-max and measured SQL/ClickHouse query plans reduce bounded workloads; main-thread JSON parsing and long-duration operational SLOs remain |

**Weighted overall: 94%** (weights total 100%, ±5 points). The estimate covers the supported single-node product
scope and completed local evidence. Delivery now includes the server/SDK repository split, so its outstanding
immutable-source CI/release gates remain visible. This is not availability, a release promise, a claim that every
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

1. **Immutable-source CI and release** — finish the split-repository/tag workflow when authorized, run its CI and
   validate published checksums, provenance and upgrade/rollback identities. Local artifacts and earlier CI runs
   are not evidence that the new repository's tag has passed.
2. **Target-environment acceptance** — real exporters, canonical HTTPS proxy, backup recovery and sustained
   cardinality/disk/retention for the chosen workload; requires a specified target. No public deployment is claimed.
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
