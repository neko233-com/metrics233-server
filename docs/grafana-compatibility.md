# Grafana interoperability and historical core audit

## Current supported workflows — 2026-10-25

The current implementation and executable contracts supersede the historical missing-feature rows below.
The first-party console uses independent GitHub Dark tokens; existing visualization palettes and saved query
identities remain intact. Current entry points are `web/src/render/index.ts`, `internal/server/builtin_sources.go`,
the native panel host and the focused contracts linked here.

- Native metric panels: time series, stat, gauge, bar gauge, table, bar chart, pie chart, state timeline, status
  history, histogram and heatmap. Native logs/traces viewers, sanitized text and metric-window vector views are
  separate paths. See [native panel types](native-panel-types.md) for exact inputs, budgets and unsupported types.
- Time series includes log/multiple axes, absolute/percent stacking, comparison alignment, thresholds, annotations,
  legends, tooltips and zoom. Tables use native virtualized rows and typed display fields. Current renderer tests
  exercise these paths; the old SDK-only warning below applies to the October 12 audit, not current acceptance.
- Query adapters include local/external Prometheus, MySQL/MariaDB, PostgreSQL, Loki log queries, and Tempo search
  plus trace-ID detail. Live SQL checks and local HTTP Loki/Tempo fixtures have distinct evidence layers; a fixture
  is not a live external daemon. Collection/storage support does not imply an additional Grafana datasource editor.
- Existing Classic/V1/V2 import/export, opaque integer/resource preservation, variables, query/expression editing,
  time windows, comparisons, annotations, snapshots, layout and alert graphs have executable browser coverage.
  The optional plugin host separately covers signed Clock, AppPlugin/SDK services, UI extensions and Go backend
  subprocesses. This verifies selected workflows rather than every official plugin option or package.

See [current status](status.md) for immutable source, test results and production acceptance, and
[renderer performance](performance-baseline.md) for measured large-frame limits. The historical manifest inventory
is retained for traceability; its implementation counts and priorities must not be used as the current roadmap.

## Historical manifest audit — 2026-10-12

Baseline: Grafana **v13.2.3** source tag. The remainder is the read-only October 12 inspection of MetricsPanel233's implementation at that time, not a current implementation inventory or a claim that all supported features were exercised. SQL real MariaDB/PostgreSQL checks and browser checks are tracked separately in docker logs.

This is an interoperability reference, not the product roadmap or an all-features acceptance target. Implement compatibility entries when they support a selected migration workflow; see the [project goals](project-goals.md).

Official baseline lists: [core data sources](https://github.com/grafana/grafana/blob/v13.2.3/docs/sources/datasources/_index.md#built-in-core-data-sources), [core visualizations](https://github.com/grafana/grafana/blob/v13.2.3/docs/sources/visualizations/panels-visualizations/visualizations/_index.md).

### Historical data sources

The tagged documentation lists 19 core sources. MetricsPanel233 has local query implementations for three. The other 16 can potentially use the installed-plugin host, but their official packages, query editors, resource APIs, authentication, platform executables, and real-service behavior are not bundled or individually established. Collector health/metrics for a product is a different capability from its Grafana datasource query implementation.

The SQL plugins moved to separate official repositories before this Grafana tag; the old monorepo SQL paths return 404 at v13.2.3. Separate-repository links below identify inspected official main branches as of this audit, not an invented package revision bundled by the Grafana tag.

| Official source | Official type ID | Local implementation status | Direct official source |
|---|---|---|---|
| Prometheus | prometheus | Builtin local/external query adapter; not full official editor parity | [plugin manifest](https://github.com/grafana/grafana-prometheus-datasource/blob/main/src/plugin.json) |
| MySQL | mysql | Builtin raw SQL, typed frames, time macros, variables; live MariaDB checks | [manifest](https://github.com/grafana/grafana-mysql-datasource/blob/main/src/plugin.json), [macros](https://github.com/grafana/grafana-mysql-datasource/blob/main/pkg/mysql/macros.go) |
| PostgreSQL | grafana-postgresql-datasource; alias postgres | Builtin raw SQL, typed frames, time macros, variables; live PostgreSQL checks | [manifest](https://github.com/grafana/grafana-postgresql-datasource/blob/main/src/plugin.json), [macros](https://github.com/grafana/grafana-postgresql-datasource/blob/main/pkg/postgresql/macros.go) |
| AWS CloudWatch | cloudwatch | Missing builtin datasource | [manifest](https://github.com/grafana/grafana/blob/v13.2.3/public/app/plugins/datasource/cloudwatch/plugin.json) |
| Azure Monitor | grafana-azure-monitor-datasource | Missing builtin datasource | [manifest](https://github.com/grafana/grafana/blob/v13.2.3/public/app/plugins/datasource/azuremonitor/plugin.json) |
| Google Cloud Monitoring | stackdriver | Missing builtin datasource | [manifest](https://github.com/grafana/grafana-cloudmonitoring-datasource/blob/main/src/plugin.json) |
| Graphite | graphite | Missing builtin datasource | [manifest](https://github.com/grafana/grafana/blob/v13.2.3/public/app/plugins/datasource/graphite/plugin.json) |
| InfluxDB | influxdb | Missing builtin datasource | [manifest](https://github.com/grafana/grafana-influxdb-datasource/blob/main/src/plugin.json) |
| OpenTSDB | opentsdb | Missing builtin datasource | [manifest](https://github.com/grafana/grafana-opentsdb-datasource/blob/main/src/plugin.json) |
| Elasticsearch | elasticsearch | Missing builtin datasource; existing collector is not search/log query parity | [manifest](https://github.com/grafana/grafana-elasticsearch-datasource/blob/main/src/plugin.json) |
| Loki | loki | Builtin LogQL range/instant search with logs frames and `/ready` health; dedicated editor and full logs Explore pending | [manifest](https://github.com/grafana/grafana-loki-datasource/blob/main/src/plugin.json) |
| Jaeger | jaeger | Missing builtin tracing datasource | [manifest](https://github.com/grafana/grafana-jaeger-datasource/blob/main/src/plugin.json) |
| Tempo | tempo | Builtin TraceQL search to table frames and `/ready` health; trace-ID detail and full trace views pending | [manifest](https://github.com/grafana/grafana-tempo-datasource/blob/main/src/plugin.json) |
| Zipkin | zipkin | Missing builtin tracing datasource | [manifest](https://github.com/grafana/grafana-zipkin-datasource/blob/main/src/plugin.json) |
| Parca | parca | Missing builtin profiling datasource | [manifest](https://github.com/grafana/grafana-parca-datasource/blob/main/src/plugin.json) |
| Pyroscope | grafana-pyroscope-datasource; alias phlare | Missing builtin profiling datasource | [manifest](https://github.com/grafana/grafana-pyroscope-datasource/blob/main/src/plugin.json) |
| Microsoft SQL Server | mssql | Missing builtin SQL datasource | [manifest](https://github.com/grafana/grafana-mssql-datasource/blob/main/src/plugin.json) |
| Alertmanager | alertmanager | Local alertmanager-compatible routes exist; arbitrary remote Alertmanager datasource/editor is not builtin | [manifest](https://github.com/grafana/grafana/blob/v13.2.3/public/app/plugins/datasource/alertmanager/plugin.json) |
| TestData | grafana-testdata-datasource | Grafana randomWalk is implemented; full TestData scenarios are missing | [manifest](https://github.com/grafana/grafana/blob/v13.2.3/public/app/plugins/datasource/grafana-testdata-datasource/plugin.json) |

Special sources, outside that list:

| Source | Existing behavior | Gap / source |
|---|---|---|
| Grafana | Builtin randomWalk/list; SDK snapshots, annotations, timeRegions and live measurements | Covered subset is real; [official module](https://github.com/grafana/grafana/blob/v13.2.3/public/app/plugins/datasource/grafana/module.ts) |
| Mixed | engine.ts groups each target by datasource UID and combines responses | No discoverable builtin Mixed instance or dedicated official editor; [official module](https://github.com/grafana/grafana/blob/v13.2.3/public/app/plugins/datasource/mixed/module.ts) |
| Dashboard | No sourcePanel/panel-output reuse implementation found | Needs lifecycle/cache/results sharing and query editor; [official datasource](https://github.com/grafana/grafana/blob/v13.2.3/public/app/plugins/datasource/dashboard/datasource.ts) |
| Expressions | Builtin __expr__ server graph and SDK editor are present | This audit does not certify every expression operation or SQL-on-frames rule |

SQL annotation mapping and editing are implemented in the builtin SDK datasource, and backend metadata advertises annotations=true. The fresh single-container browser suite passed all three desktop cases: both real SQL engines, dynamic variable labels/multiple selections, native SQL query editing, and English/Chinese annotation point/range, variable, field mapping, preview/save/reload and opaque-field preservation. No page/console warnings or errors were recorded. SQL-specific remaining gaps include the official visual SQL builder, schema/table/column completion, and a real certificate/TLS endpoint integration check. Query execution is bounded and runs read-only transactions; explicit limits can reject a very large imported query. Read-only prevents updates but is not a sandbox against resource-intensive SELECT functions; server timeout handles cancellation.

### Historical visualizations

The tagged documented list has 25 visualizations. Six have local renderers: time series and table now use first-party implementations, and stat/gauge/bar gauge/text retain their local implementations. The other 19 go through PluginPanel -> loadPlugin, which requires an installed package; no bundled official core renderer/import map was found. A community panel with a different ID does not make the original core ID compatible automatically.

| ID | Local renderer | Direct official manifest |
|---|---|---|
| timeseries | First-party uPlot lines, colors, visibility, current-value legend, hover tooltip, time-range selection and annotation overlay. Advanced Grafana log/multiple axes, stacking, bars, thresholds and option editing are not implemented by this renderer. The former SDK-only browser evidence does not verify the native renderer. | [manifest](https://github.com/grafana/grafana/blob/v13.2.3/public/app/plugins/panel/timeseries/plugin.json), [SDK component](https://github.com/grafana/grafana/blob/v13.2.3/packages/grafana-ui/src/graveyard/TimeSeries/TimeSeries.tsx) |
| stat | Present, numeric reduction only | [source](https://github.com/grafana/grafana/blob/v13.2.3/public/app/plugins/panel/stat/plugin.json) |
| gauge | Present, simplified SVG | [source](https://github.com/grafana/grafana/blob/v13.2.3/public/app/plugins/panel/gauge/plugin.json) |
| bargauge | Present, simplified progress element | [source](https://github.com/grafana/grafana/blob/v13.2.3/public/app/plugins/panel/bargauge/plugin.json) |
| table | First-party sortable HTML table with pages capped at 100 rows when pagination is enabled or a result exceeds 100 rows. SDK footer calculations and the full option editor are not implemented. Former SDK-only browser evidence does not verify the native table. | [source](https://github.com/grafana/grafana/blob/v13.2.3/public/app/plugins/panel/table/plugin.json) |
| text | Present, sanitized Markdown/HTML/code | [source](https://github.com/grafana/grafana/blob/v13.2.3/public/app/plugins/panel/text/plugin.json) |
| state-timeline | Missing builtin renderer | [source](https://github.com/grafana/grafana/blob/v13.2.3/public/app/plugins/panel/state-timeline/plugin.json) |
| status-history | Missing builtin renderer | [source](https://github.com/grafana/grafana/blob/v13.2.3/public/app/plugins/panel/status-history/plugin.json) |
| barchart | Missing builtin renderer | [source](https://github.com/grafana/grafana/blob/v13.2.3/public/app/plugins/panel/barchart/plugin.json) |
| histogram | Missing builtin renderer | [source](https://github.com/grafana/grafana/blob/v13.2.3/public/app/plugins/panel/histogram/plugin.json) |
| heatmap | Missing builtin renderer | [source](https://github.com/grafana/grafana/blob/v13.2.3/public/app/plugins/panel/heatmap/plugin.json) |
| piechart | Missing builtin renderer | [source](https://github.com/grafana/grafana/blob/v13.2.3/public/app/plugins/panel/piechart/plugin.json) |
| candlestick | Missing builtin renderer | [source](https://github.com/grafana/grafana/blob/v13.2.3/public/app/plugins/panel/candlestick/plugin.json) |
| trend | Missing builtin renderer | [source](https://github.com/grafana/grafana/blob/v13.2.3/public/app/plugins/panel/trend/plugin.json) |
| xychart | Missing builtin renderer | [source](https://github.com/grafana/grafana/blob/v13.2.3/public/app/plugins/panel/xychart/plugin.json) |
| logs | Missing builtin renderer | [source](https://github.com/grafana/grafana/blob/v13.2.3/public/app/plugins/panel/logs/plugin.json) |
| nodeGraph | Missing builtin renderer | [source](https://github.com/grafana/grafana/blob/v13.2.3/public/app/plugins/panel/nodeGraph/plugin.json) |
| traces | Missing builtin renderer | [source](https://github.com/grafana/grafana/blob/v13.2.3/public/app/plugins/panel/traces/plugin.json) |
| flamegraph | Missing builtin renderer | [source](https://github.com/grafana/grafana/blob/v13.2.3/public/app/plugins/panel/flamegraph/plugin.json) |
| canvas | Missing builtin renderer/editor | [source](https://github.com/grafana/grafana/blob/v13.2.3/public/app/plugins/panel/canvas/plugin.json) |
| geomap | Missing builtin renderer | [source](https://github.com/grafana/grafana/blob/v13.2.3/public/app/plugins/panel/geomap/plugin.json) |
| dashlist | Missing builtin renderer | [source](https://github.com/grafana/grafana/blob/v13.2.3/public/app/plugins/panel/dashlist/plugin.json) |
| alertlist | Missing builtin renderer | [source](https://github.com/grafana/grafana/blob/v13.2.3/public/app/plugins/panel/alertlist/plugin.json) |
| annolist | Missing builtin renderer | [source](https://github.com/grafana/grafana/blob/v13.2.3/public/app/plugins/panel/annolist/plugin.json) |
| news | Missing builtin renderer | [source](https://github.com/grafana/grafana/blob/v13.2.3/public/app/plugins/panel/news/plugin.json) |

Additional tagged manifests outside the documented 25: debug, gettingstarted, live, logstable, welcome. A row is rendered as a heading/layout element; full row collapse/section editing is a separate dashboard behavior.

The current native renderer scope is:

- Time series: first-party uPlot rendering for temporal numeric/enum fields, default colors, visibility toggles, last-value legend, hover tooltip, x-range selection, time comparison alignment and annotation points/regions. It does not currently implement Grafana stacking, logarithmic or multiple axes, bars, thresholds, or the complete Grafana option editor.
- Table: first-party HTML table with sorting, display formatting, frame selection and pages capped at 100 rows for large results. Grafana footer calculations, the complete cell/field option editor and SDK table behaviors are not implemented.
- Stat: numeric fields only; no full string/boolean stat behavior or all sparkline/background/display modes.
- Gauge/bar gauge: simple SVG/progress and thresholds; lacks complete official display modes, tick layout, sizing and orientation configuration.
- Text: sanitization is deliberate; imported interactive actions/embedded iframe content does not execute.
- The browser spec in web/e2e/time-series.spec.ts still describes the former SDK renderer and checks Grafana-only log/stack behavior. It must be reconciled before it can serve as native-renderer acceptance evidence. Existing SDK-specific results are historical, not current verification.

These builtin visualization paths lack the complete official PanelPlugin option schemas/editors because they bypass loadPanelPlugin. The installed third-party PluginPanel host has a separate SDK option editing path.

Audit evidence in repository: internal/server/plugins.go, internal/server/expressions.go, internal/server/sql_datasources.go, internal/server/grafana_source.go, internal/server/native_plugin_routes.go, web/src/grafana/plugin-runtime.ts, web/src/grafana/engine.ts, web/src/grafana/GrafanaPanel.tsx, web/src/grafana/GrafanaTable.tsx, web/src/grafana/GrafanaTimeSeries.tsx, web/test/time-series.test.cjs, web/e2e/time-series.spec.ts, web/src/components/Chart.tsx.

Priority suggested by dependencies: core renderer registry and faithful typed options; Missing core query adapters/editors for major logs/traces/storage sources; Dashboard datasource result reuse; then remaining editor/advanced options. This report only covers sources and visualizations, not an exhaustive Grafana product audit.
