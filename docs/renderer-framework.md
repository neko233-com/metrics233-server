# Renderer framework

This is the contract for MetricsPanel233's own visualization engine. Grafana packages are compatibility
adapters, not the renderer foundation.

## Goals

- One renderer registry for time series, tables, stats, gauges, logs, traces, heatmaps, vectors and future panels.
- Canvas-first hot paths (uPlot/Canvas2D now, optional WebGL later) with SVG/DOM for overlays and tables.
- Deterministic normalization before drawing: time alignment, stacking, comparison, null handling and field stats.
- Vector-native: 64-dimensional metric windows and embedding-like rows are first-class render inputs, not string blobs.
- Performance budgets are measured, not assumed.

## Data contract

The current query engine emits Grafana `DataFrame`/`Field` values. The renderer framework consumes that contract
during the transition. The renderer boundary must not import `@grafana/ui`.

- `frames[]`: one frame per query result; `fields[]` carry name, type, values, labels, display and config.
- Time series: one `time` field plus numeric fields; labels and `displayName` are preserved.
- Tables/logs/traces: string, number, time and labels; logs use `Line` + `Time`; traces use a table frame.
- Vectors: a frame with a 64-number field plus metadata; the vector renderer reads the same field values.

A future project-owned frame type can replace the Grafana data contract without changing renderers, because
renderers read only `frames`, `fields`, `values`, `labels`, `range` and `options`.

## Pipeline

1. **Deserialize** query frames into render inputs.
2. **Normalize**
   - sort and deduplicate timestamps;
   - align multiple frames by timestamp (nearest/union);
   - apply visibility (`hideFrom.viz`), null policy and unit/decimals;
   - compute stacking groups and comparison offsets;
   - compute min/max/current/first/last for legends and axes.
3. **Layout** panel chrome, viewport, legend, axes, annotations and overlays.
4. **Draw** Canvas for series; SVG for annotation markers/regions; DOM for tables and log rows.
5. **Interact** cursor/tooltip, legend toggle, drag zoom, keyboard and touch.
6. **Dispose** uPlot instances, observers, RAF and subscriptions on unmount or data signature change.

## Renderer registry

```ts
type RendererKind = "timeseries" | "table" | "stat" | "gauge" | "bargauge" | "logs" | "traces"
  | "heatmap" | "histogram" | "pie" | "candlestick" | "geomap" | "nodegraph"
  | "state-timeline" | "status-history" | "text" | "vector";

type RendererDefinition = {
  kind: RendererKind;
  label: string;
  capabilities: { compare?: boolean; stack?: boolean; streaming?: boolean; vector?: boolean };
  component: ComponentType<RendererProps>;
};
```

`web/src/render/registry.ts` maps kinds to definitions. `web/src/render/index.ts` resolves a panel's
`visualization` and falls back to the compatibility renderer only when an imported Grafana resource explicitly
requires it. Unknown kinds show a named error instead of silently rendering nothing.

## Visualization coverage matrix

| Kind | Status | Notes |
| --- | --- | --- |
| Time series | Implemented (default) | Own uPlot renderer; lines, comparison, legend, tooltip, zoom, annotations, log axes, percent stacking and multiple axes. `?renderer=compat` or localStorage restores the bridge |
| Table | Implemented (core) | Own HTML table; sorting, pagination, page numbers, footer reducers, display formatting |
| Stat / Gauge / Bar gauge | Implemented (default) | Shared `StatRenderer`; reducer selection, field filtering, threshold colors, gauge meter ARIA and bar fraction. Text/Markdown remains in the panel host |
| Text / Markdown | Implemented (panel host) | Sanitized HTML/Markdown rendered by the panel host |
| Logs | Implemented (default) | Own virtualized log view; search and label key/value filters; 10k-row browser contract |
| Traces | Implemented (search and detail) | Tempo search table plus trace lookup, span hierarchy/waterfall, duration/error status and attributes; `?traceId=` survives reload and browser history |
| Heatmap / Histogram | Implemented (default) | Canvas time-bucket heatmap with legend/hover; normalized HTML histogram bars |
| Pie / Candlestick / Geomap / Node graph | Out of scope | Not required for the SMB/game-backend monitoring target |
| State timeline / Status history | Out of scope | Alert state history is covered by alert views |
| Vector | Implemented (default) | 64-dimension heat strip with hover values, up to 200 rows |

Compatibility-only renderers for imported Grafana dashboards are tracked separately; they do not count as
first-party coverage.

### Tempo trace detail

Tempo search remains a table frame. Selecting a trace or opening `?traceId=<hex-id>` sends a separate
`POST /api/ds/query` query with `queryType: "traceId"`, `traceId` and the panel target's Tempo datasource UID.
This uses the existing authenticated datasource-query policy; it does not add a new public endpoint or expose
datasource credentials. The [Tempo v1 trace API](https://grafana.com/docs/tempo/latest/api_docs/#query)
is queried without a search-window restriction so persisted deep links can resolve outside the dashboard range.

- The span frame has `traceID`, `spanID`, `parentSpanID`, `serviceName`, `operationName`, `startTime` (Unix ms),
  `durationMs`, `status` (`unset`/`ok`/`error`), `statusMessage`, `kind` and `tags` (JSON attributes).
- Both `batches`/`instrumentationLibrarySpans` and `resourceSpans`/`scopeSpans` JSON are accepted. Hex and protobuf
  JSON base64 IDs normalize to lowercase hexadecimal. Duration is computed from integer nanoseconds before conversion.
- Lookup accepts 1–32 hexadecimal trace-ID characters. HTTP 404 produces an empty span frame; malformed responses,
  invalid IDs/times, upstream errors and traces exceeding 10,000 unique spans fail explicitly. The existing proxy
  applies its 20-second timeout, cancellation, encrypted connection secrets and 32 MiB response limit.
- The first-party view orders children under their parent, draws time relative to the trace start, supports subtree
  collapse and span attributes, and paginates at 100 visible rows. Missing parents and cycles remain inspectable.
  A direct span query renders the same detail view. Superseded requests are aborted and Retry starts a new lookup.
- Browser contract coverage uses a protocol fixture served through a real HTTP upstream, the Go query API and the
  rendered UI. It covers hierarchy, error attributes, timeline, deep links/history, empty/error/retry and 390/768/1280
  containment. A separate [real Tempo 3.1.1 contract](tempo-live-validation.md) now verifies actual OTLP ingestion,
  TraceQL, detail and browser/Edge rendering against an immutable upstream daemon. Its local fixture is distinct
  from production deployment. Tempo v2 partial traces and a locally owned trace store are not implied.
- The browser datasource runtime routes the existing Prometheus, Loki and Tempo types through its built-in backend
  adapter; these queries do not require installing external datasource plugin packages. HTTP upstream fixture tests
  exercise normal Tempo search and Loki log queries in addition to trace detail. An attributed excerpt from the
  upstream Tempo trace fixture validates base64 IDs, error enums and nanosecond durations in Go tests; see
  `internal/server/testdata/tempo-fixtures.md` for the pinned source and scope.

## Performance budget

Current bounded behavior:

- TimeSeries uses a shared min/max row target of approximately 2 × CSS pixel width, rounded to 64-pixel steps,
  with a minimum target of 256 rows. Every bucket keeps each series' extrema; the union shares one target across
  series. Endpoints, viewport neighbours and both sides of every null gap survive. This is a soft target: numerous
  gaps, or more series than fit the target's extrema, deliberately increase the result instead of losing outages/peaks.
- Normalization above 50,000 time/numeric cells uses a module Worker; small frames use the same numeric algorithm
  synchronously. The threshold counts time columns as well as all numeric/enum columns, rather than just rows.
- Table pages render at most 100 rows; footer reducers scan all rows without creating per-cell React nodes.
- TimeSeries exposes input/aligned/rendered row counts and preparation mode as DOM data attributes. The benchmark
  records Worker calculation/roundtrip time, presentation, transfer detachment and main-thread long tasks.

200-series density, sustained 60 fps interactions and every-renderer timing instrumentation remain measurement
targets, not production guarantees. Native-query bucket limits and SQLite vector limits are separate server contracts.

### Frame slicing and incremental work

- `web/src/render/frame-budget.ts` provides `runInFrameSlices` and `nextFrame`: each slice processes at most
  `perFrame` items or `sliceMs` milliseconds, then yields to animation frames. AbortSignal cancels stale work.
- TimeSeries copies numeric inputs into owned Float64 buffers in approximately 8 ms slices, transfers ownership to
  its Worker, and receives only sampled columns back. Source DataFrames, field display functions and credentials
  never cross the Worker boundary. The Worker sorts/deduplicates/aligns times, applies stacking, then samples.
- New query frames, viewport buckets or zoom ranges replace the current job; cleanup terminates obsolete Workers,
  and aborted/stale jobs cannot commit results. Completed jobs also terminate. Large-to-small query transitions
  release the previous job's source-frame reference. A missing/failed Worker uses the same algorithm on the main
  thread after a sliced copy; if that fallback fails, busy state ends and a chart error is displayed.
- `useDeferredValue` and memoized frame metadata avoid normalization on cursor/legend changes. Relative ranges
  resolve on a range/query-frame change. Existing uPlot instances use `setData` for data changes; plot structure,
  axes, timezone and locale changes recreate the instance. Legend last values use the complete aligned model.
  Optional threshold/continuous series-color modes may still compute SDK field statistics on the main thread.
- Drag zoom still invokes the range callback and performs a new datasource query. Sampling affects display only;
  the saved query, source frames, server data and exported dashboard remain intact. Tooltips inspect sampled points.
- `web/src/render/incremental.ts` supplies cache/append helpers, but TimeSeries does not use them to append partial
  models: each changed frame normalizes a new job. Sustained append-only streaming optimization remains open.

Measurements go in `docs/performance-baseline.md` or a dedicated renderer benchmark; do not claim universal
performance from a single workstation result. `web/scripts/render-bench.mjs` starts a temporary server, generates a
configurable panel (`RENDER_BENCH_POINTS`, `RENDER_BENCH_SERIES`, `RENDER_BENCH_RUNS`) and reports raw JSON.
Its synthetic API responses isolate frontend processing; they do not validate server ingestion/query throughput.

## Migration

1. Own Table and TimeSeries are the product defaults; TimeSeries covers log axes, percent stacking and multiple axes.
   `?renderer=compat` or localStorage `metricspanel-renderer=compat` restores the Grafana bridge for compatibility
   resources, plugins and troubleshooting.
2. Heatmap, histogram, vector and Tempo trace detail are first-party registry paths.
3. Remove `@grafana/ui` from the default bundle and leave it only in the optional import/plugin chunk.
