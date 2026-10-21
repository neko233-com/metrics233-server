# Project status and progress protocol

Updated: 2026-10-21. This file is the durable progress ledger. Update it whenever a progress unit completes.

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
