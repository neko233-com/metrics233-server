# Native monitoring panels

The native editor offers eleven metric visualizations. This comparison follows the
[Grafana visualization catalog](https://grafana.com/docs/grafana/latest/visualizations/panels-visualizations/visualizations/)
as a reference for monitoring tasks; it does not claim complete Grafana option or plugin parity.

| Visualization | Native behavior and useful application |
| --- | --- |
| Time series | Lines/area/bars, axes, percent stacks, thresholds, comparisons, zoom and annotations; throughput, resource use and latency trends. |
| Stat | Reduced numeric value, units and timestamp; process uptime remains numeric. Multiple auto-oriented values wrap into readable cards. |
| Gauge / Bar gauge | Bounded numeric values and configured threshold colors; CPU, memory, disk and queue occupancy. |
| Table | Native virtualized result rows, frame selection, filtering/sorting and field display configuration. |
| Bar chart | Signed categorical values with a zero baseline and complete virtualized category list; per-server connections or per-Entity counts. |
| Pie chart | Non-negative proportions, explicit zero/missing behavior and a complete virtualized legend. At most 15 individual slices plus a labeled Other slice; combining slices never removes their individual legend rows. Negative input produces a visible warning instead of misleading proportions. |
| State timeline | Discrete observations merged only when their displayed state/color agree and time intervals touch. Missing/invalid observations and long gaps stay blank. |
| Status history | The same state/time contract with separate observation cells rather than merged intervals. |
| Histogram | Raw returned numeric observations produce a bounded value distribution. Classic Prometheus `le` buckets from all frames are grouped by their other labels and converted from cumulative counts to non-overlapping bucket counts. Duplicate bounds, decreasing/negative counts or mixed bucket/raw input are rejected visibly. String-category/count frames are pre-aggregated bins. |
| Heatmap | Native canvas visualization of time and numeric series/bucket magnitudes. This is not an inferred player location or a fabricated distribution. |

State panels do not guess whether a custom numeric value means healthy or unhealthy. The editor's explicit
**Availability: 0 unavailable, 1 available** preset sets value mappings to red/green. Original values and custom
mapping preservation are available. Threshold colors and existing field display mappings remain authoritative.
The row label says **Last observed**; an old observation is not a claim that the service is currently healthy.

New category/state renderers process at most 5,000 rows and 200,000 numeric/time cells. Exceeding the budget is
visible and requires a narrower query. Category/state rows are virtualized and preserve their mounted scroll
container during background refresh. Pie geometry is bounded to 16 slices while its legend retains all categories
within the row budget. Histogram geometry is bounded to 200 bins with a visible notice when truncated. Null,
nonfinite, boolean and numeric-looking string values are never coerced into a numeric zero. Pie proportions use
scaled weights so large finite inputs do not overflow the denominator. These are display budgets, not storage
or query limits, and do not restrict the separate all-player-regions business table.

## Other Grafana catalog entries

| Catalog entry | Current boundary |
| --- | --- |
| Logs / Traces | Native viewers for configured Loki/Tempo data sources; local metric storage does not invent log/trace ingestion. |
| Text | Existing sanitized text/Markdown panel host; no arbitrary script execution. |
| Vector | Existing native vector heat strips, separate from Grafana's catalog. |
| Candlestick / Trend / XY chart | No dedicated native financial/non-time-x editor or renderer. Optional imported-plugin compatibility is separate and is not a verified native implementation. |
| Node graph / Flame graph / Geomap / Canvas | No native graph/profiling/map/layout authoring claim. They require their own input and interaction contracts. |
| Dashboard list / Alert list / Annotations list / News | Existing product pages are separate workflows, not implementations of these embedded panel types. |

## HTTP configuration

`PATCH /api/v1/dashboards/{id}/panels` keeps the existing Editor permission and `If-Match` revision boundary.
New `visualization` values are `barchart`, `piechart`, `state-timeline` and `status-history`; `pie` is an import alias.
Optional `panel.state_mapping="availability"` is limited to the two state types. Omission retains opaque mappings;
explicit `""` clears mappings. Only `config.fieldConfig.defaults.mappings` changes. Queries, overrides, thresholds,
units, ownership metadata and unrelated configuration remain intact. Invalid mapping types/values fail before save.

The native metric editor also exposes the existing histogram and heatmap renderers. Logs, traces and other
data-source-specific panels continue to use their established query/import paths.
