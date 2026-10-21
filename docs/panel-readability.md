# Readable metric panels

The first-party stat/gauge/bar-gauge, time-series and simple Explore charts present metric identities as readable
names instead of dumping Prometheus selectors. This is a presentation layer only: original metric names, labels,
query expressions, source authorization, stored samples and business scope filters are unchanged.

- Explicit field display names and query legends take precedence. Known metric names have exact English/Chinese
  definitions; unknown metrics keep their original names. A ratio/rate expression that loses `__name__` uses its
  panel title, without inheriting a raw counter's meaning.
- The four built-in monitoring-service metrics have exact definitions: allocated Go heap bytes (`MemStats.Alloc`,
  not RSS), live goroutine count, cumulative handler request count and process uptime. Native `rate` aggregation
  has an explicit derived rate name/description, so the request-rate chart never claims to show a cumulative total.
- Business server, role, environment/channel and resource dimensions remain visible. Additional differing labels
  distinguish colliding names; a singleton's opaque instance/job labels stay in details. Native time-series IDs
  remain stable when locale or display names change, preserving visibility state.
- The keyboard-accessible **Metric details** dialog exposes the selected series, metric definition, unit, reducer,
  query and complete original labels. Labels are diagnostic metadata, not an error message. Long metadata wraps
  within the dialog, including narrow screens; detail rows are created only when the dialog is open.
- Uptime is numeric elapsed time. Its definition states that process restarts reset it. Custom units and colors
  remain authoritative. Numeric panels show the latest data-point timestamp and distinguish actual zero from
  missing/non-finite values; a missing gauge has no numeric `aria-valuenow`.
- A query result timestamp is not necessarily the original collection time. Range reducers follow the chosen
  dashboard range. Metric details state both limits rather than claiming a fresh scrape from query time alone.
- Time-series axes and legends use the same field-unit formatter; left/right axes remain independent. Simple
  charts provide a stable valid color and a readable legend for every returned series, with accurate counts.
- Stored Explore expressions default to a plain number instead of inheriting an unrelated metric-picker unit.
  Units can be selected explicitly, including separate 0–100 percentages and 0–1 ratios. Duration formatting
  preserves milliseconds, microseconds and nanoseconds rather than rounding short operations to zero seconds.

No production rules, dashboards, templates, credentials or notification receivers are changed by this UI behavior.
Browser, archive, Docker and live-deployment acceptance are recorded separately in `docs/status.md` and the
aggregate delivery ledger. A live zero-player snapshot does not demonstrate a nonzero player lifecycle. Per the
user's scope decision, acceptance does not require signing into a game with a test account.
