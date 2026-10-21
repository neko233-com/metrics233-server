import { getLocale, t as tr } from "../i18n";
import { lazy, Suspense, useEffect, useState } from "react";
import { Play, Search } from "lucide-react";
import {
  aggregations,
  api,
  message,
  parseLabels,
  ranges,
  type Metric,
  type Panel,
} from "../api";
import { Chart } from "../components/Chart";
import { PatternAnalysis } from "./PatternAnalysis";
import "./DatasourceExplore.css";
const DatasourceExplore = lazy(() =>
  import("./DatasourceExplore").then((module) => ({
    default: module.DatasourceExplore,
  })),
);

export function storedQueryUnit(metric: string, aggregation: string, expression: string, selected = "auto"): string {
  if (selected !== "auto") return selected;
  if (expression.trim()) return "";
  if (aggregation === "rate") return /_bytes(?:_total)?$/.test(metric) ? "binBps" : "ops";
  if (metric.endsWith("_bytes")) return "bytes";
  if (metric.endsWith("_seconds")) return "seconds";
  return "";
}

export function Explore({
  metrics,
  tick,
}: {
  metrics: Metric[];
  tick: number;
}) {
  const [metric, setMetric] = useState("metricspanel_memory_bytes"),
    [aggregation, setAggregation] = useState("last"),
    [range, setRange] = useState("30m"),
    [labels, setLabels] = useState("{}"),
    [error, setError] = useState("");
  const [query, setQuery] = useState<{
    panel: Panel;
    range: string;
  }>({
    panel: {
      id: "explore",
      title: "Query result",
      metric: "metricspanel_memory_bytes",
      aggregation: "last",
      unit: "bytes",
    },
    range: "30m",
  });
  const [filter, setFilter] = useState("");
  const [expr, setExpr] = useState("");
  const [unit, setUnit] = useState("auto");
  const [sourceUID, setSourceUID] = useState("stored");
  const [sources, setSources] = useState<
    { uid: string; name: string; type: string }[]
  >([]);
  const [sourceError, setSourceError] = useState("");
  useEffect(() => {
    const controller = new AbortController();
    void api<{ uid: string; name: string; type: string }[]>(
      "/api/datasources",
      { signal: controller.signal },
    )
      .then((sources) => {
        if (!controller.signal.aborted) setSources(sources);
      })
      .catch((reason) => {
        if (!controller.signal.aborted) setSourceError(message(reason));
      });
    return () => controller.abort();
  }, []);
  return (
    <>
      <div className="page-heading">
        <div>
          <h1>{tr("Explore")}</h1>
          <p>
            {getLocale() === "zh"
              ? "查询已存储的指标或已配置的数据源。"
              : "Query stored metrics or a configured datasource."}
          </p>
        </div>
      </div>
      <label className="explore-source-selector">
        {getLocale() === "zh" ? "数据源" : "Datasource"}
        <select
          aria-label={getLocale() === "zh" ? "数据源" : "Datasource"}
          value={sourceUID}
          onChange={(event) => setSourceUID(event.target.value)}
        >
          <option value="stored">
            {getLocale() === "zh" ? "本地指标存储" : "Stored metrics"}
          </option>
          {sources
            .filter((source) => source.uid !== "metricspanel")
            .map((source) => (
              <option key={source.uid} value={source.uid}>
                {source.name} · {source.type}
              </option>
            ))}
        </select>
      </label>
      {sourceError && (
        <p role="alert" className="form-error">
          {sourceError}
        </p>
      )}
      {sourceUID !== "stored" ? (
        <Suspense fallback={<p role="status">{tr("Loading datasource…")}</p>}>
          <DatasourceExplore
            key={sourceUID}
            uid={sourceUID}
            type={
              sources.find((source) => source.uid === sourceUID)?.type || ""
            }
            range={range}
            onRange={setRange}
          />
        </Suspense>
      ) : (
        <>
          <form
            className="query-builder"
            onSubmit={(e) => {
              e.preventDefault();
              try {
                setQuery({
                  panel: {
                    id: "explore",
                    title: "Query result",
                    expr,
                    metric,
                    aggregation,
                    unit: storedQueryUnit(metric, aggregation, expr, unit),
                    labels: parseLabels(labels),
                  },
                  range,
                });
                setError("");
              } catch (e) {
                setError(String(e));
              }
            }}
          >
            <div className="query-fields">
              <label>
                {tr("Metric")}
                <select
                  value={metric}
                  onChange={(e) => setMetric(e.target.value)}
                >
                  {metrics.map((m) => (
                    <option key={m.name}>{m.name}</option>
                  ))}
                </select>
              </label>
              <label>
                {tr("Aggregation")}
                <select
                  value={aggregation}
                  onChange={(e) => setAggregation(e.target.value)}
                >
                  {aggregations.map((a) => (
                    <option key={a} value={a}>
                      {tr(a)}
                    </option>
                  ))}
                </select>
              </label>
              <label>
                {tr("Time range")}
                <select
                  value={range}
                  onChange={(e) => setRange(e.target.value)}
                >
                  {ranges.map((r) => (
                    <option key={r.value} value={r.value}>
                      {tr(r.label)}
                    </option>
                  ))}
                </select>
              </label>
              <button className="primary">
                <Play size={17} />
                {tr("Run query")}
              </button>
            </div>
            <label>
              {tr("Unit")}
              <select aria-label={tr("Unit")} value={unit} onChange={event => setUnit(event.target.value)}>
                <option value="auto">{tr("Auto / number")}</option>
                <option value="">{getLocale() === "zh" ? "数字（无单位）" : "Number (no unit)"}</option>
                <option value="bytes">{getLocale() === "zh" ? "字节" : "Bytes"}</option>
                <option value="binBps">{getLocale() === "zh" ? "字节/秒" : "Bytes/second"}</option>
                <option value="seconds">{getLocale() === "zh" ? "耗时（秒）" : "Duration (seconds)"}</option>
                <option value="uptime">{tr("uptime")}</option>
                <option value="percent">{getLocale() === "zh" ? "百分比（0–100）" : "Percent (0–100)"}</option>
                <option value="percentunit">{getLocale() === "zh" ? "比例（0–1 → %）" : "Ratio (0–1 → %)"}</option>
                <option value="count">{tr("count")}</option>
                <option value="ops">{getLocale() === "zh" ? "每秒" : "Per second"}</option>
              </select>
            </label>
            <p className="subtle">{getLocale() === "zh" ? "自动单位只用于所选指标；PromQL 表达式默认显示数字，可在此指定结果单位。" : "Auto units use the selected metric only. PromQL results default to numbers; choose their unit explicitly."}</p>
            <label>
              {tr("PromQL expression")}
              <input
                className="mono"
                value={expr}
                onChange={(e) => setExpr(e.target.value)}
                placeholder="sum(rate(app_requests_total[5m]))"
              />
            </label>
            <label>
              {tr("Label filters")}
              <input
                className="mono"
                value={labels}
                onChange={(e) => setLabels(e.target.value)}
                placeholder={'{"service":"api"}'}
              />
            </label>
            {error && (
              <p className="form-error" role="alert">
                {error}
              </p>
            )}
          </form>
          <div className="explore-chart">
            <Chart panel={query.panel} range={query.range} tick={tick} />
          </div>
          <PatternAnalysis
            metric={query.panel.expr ? "" : query.panel.metric}
            range={query.range}
            labels={JSON.stringify(query.panel.labels || {})}
          />
          <section className="metric-browser">
            <div className="section-heading">
              <h2>
                {tr("Metric browser")}
                <span>{metrics.length}</span>
              </h2>
              <label className="search-input">
                <Search size={17} />
                <input
                  aria-label={tr("Search metrics")}
                  placeholder={tr("Search metrics")}
                  value={filter}
                  onChange={(e) => setFilter(e.target.value)}
                />
              </label>
            </div>
            <div className="table-scroll">
              <table>
                <thead>
                  <tr>
                    <th>{tr("Metric name")}</th>
                    <th>{tr("Series")}</th>
                    <th />
                  </tr>
                </thead>
                <tbody>
                  {metrics
                    .filter((m) =>
                      m.name.toLowerCase().includes(filter.toLowerCase()),
                    )
                    .map((m) => (
                      <tr key={m.name}>
                        <td className="mono">{m.name}</td>
                        <td>{m.series_count}</td>
                        <td>
                          <button
                            className="text-button"
                            onClick={() => {
                              setMetric(m.name);
                              setExpr("");
                              setLabels("{}");
                              setQuery({
                                panel: {
                                  id: "explore",
                                  title: "Query result",
                                  metric: m.name,
                                  aggregation,
                                  unit: storedQueryUnit(m.name, aggregation, "", unit),
                                  labels: {},
                                },
                                range,
                              });
                            }}
                          >
                            {tr("Explore")}
                          </button>
                        </td>
                      </tr>
                    ))}
                </tbody>
              </table>
            </div>
          </section>
        </>
      )}
    </>
  );
}
