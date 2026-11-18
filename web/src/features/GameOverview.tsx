import { useEffect, useMemo, useState } from "react";
import { Download, RefreshCw } from "lucide-react";
import { api, formatValue, message, type Panel } from "../api";
import { Chart } from "../components/Chart";
import { getLocale } from "../i18n";
import { Alert, Button } from "../ui";
import {
  detailCharts, gameSignals, headlineSignals, makeGrafanaGameDashboard, primaryCharts, readSnapshot,
  sampleFreshnessSeconds, scopesExpression, servicePath, signalExpression, snapshotExpression,
  type GameScope, type InstantSample, type SignalID, type Snapshot,
} from "./game-overview-model";

type PrometheusVector = { data: { result: InstantSample[] } };
type Measurement = Snapshot[SignalID];
const emptyScope = { channel: "", environment: "" };
const timeRanges = [
  { id: "15m", title: ["Last 15 minutes", "最近 15 分钟"] },
  { id: "1h", title: ["Last hour", "最近 1 小时"] },
  { id: "6h", title: ["Last 6 hours", "最近 6 小时"] },
  { id: "24h", title: ["Last 24 hours", "最近 24 小时"] },
] as const;
const latencySignals = ["loginLatency", "centerLatency"] as const;
const operationSignals = ["sql", "sqlFailures", "saveAge", "publicLinks", "collectDropped"] as const;

async function querySnapshot(query: string, signal: AbortSignal): Promise<InstantSample[]> {
  const response = await api<PrometheusVector>("/prometheus/api/v1/query", {
    method: "POST",
    headers: { "Content-Type": "application/x-www-form-urlencoded" },
    body: new URLSearchParams({ query }),
    signal,
  });
  return response.data.result;
}

const text = (labels: readonly [string, string]): string => labels[getLocale() === "zh" ? 1 : 0];

function display(value: number | undefined, unit = ""): string {
  if (value === undefined) return "—";
  if (unit === "ms") return formatValue(value / 1000, "seconds");
  if (unit === "count") return new Intl.NumberFormat(getLocale(), { maximumFractionDigits: 2 }).format(value);
  if (unit === "per-minute") return formatValue(value) + text([" /min", " /分钟"]);
  return formatValue(value, unit);
}

function Health({ measurement, loading }: { measurement: Measurement; loading: boolean }) {
  const age = measurement?.age;
  const stale = age !== undefined && age > sampleFreshnessSeconds;
  const available = measurement?.value !== undefined;
  const label = loading ? text(["Loading…", "加载中…"]) :
    stale ? text(["Stale", "已过期"]) : available ? text(["Reporting", "正在上报"]) : text(["No data", "暂无数据"]);
  return <span className={"game-health " + (stale ? "stale" : available ? "reporting" : "")}
    title={age === undefined ? undefined : Math.round(age) + text([" seconds since latest sample", " 秒前收到最新采样"])}>
    <span aria-hidden="true" />{label}
  </span>;
}

function downloadDashboard() {
  const dashboard = makeGrafanaGameDashboard(getLocale() === "zh" ? "zh" : "en");
  const anchor = document.createElement("a");
  const url = URL.createObjectURL(new Blob([JSON.stringify(dashboard, null, 2)], { type: "application/json" }));
  anchor.href = url;
  anchor.download = "metrics233-game-overview.json";
  anchor.click();
  window.setTimeout(() => URL.revokeObjectURL(url), 1000);
}

function SignalCharts({ definitions, scope, range, tick }: {
  definitions: typeof primaryCharts;
  scope: GameScope;
  range: string;
  tick: number;
}) {
  const locale = getLocale();
  const panels = useMemo(() => definitions.map((definition): Panel => {
    const signal = gameSignals[definition.series[0]];
    const expressions = definition.series.map((id) =>
      'label_replace((' + signalExpression(id, scope) + '), "signal", ' +
      JSON.stringify(gameSignals[id].caption[locale === "zh" ? 1 : 0]) + ', "", "")');
    return {
      id: definition.id,
      title: definition.title[locale === "zh" ? 1 : 0],
      metric: signal.metric,
      unit: signal.unit,
      aggregation: "last",
      expr: expressions.join(" or "),
    };
  }), [definitions, locale, scope]);
  return <div className="game-chart-grid">{panels.map((panel) =>
    <Chart key={panel.id} panel={panel} range={range} tick={tick} formatter={display} />)}</div>;
}

function SignalList({ ids, samples }: { ids: readonly SignalID[]; samples: Snapshot }) {
  return <dl className="game-detail-list">{ids.map((id) => <div key={id}>
    <dt>{text(gameSignals[id].caption)}</dt>
    <dd>{display(samples[id]?.value, gameSignals[id].unit)}</dd>
  </div>)}</dl>;
}

export default function GameOverview({ tick, openExplore }: { tick: number; openExplore: () => void }) {
  const [scope, setScope] = useState<GameScope>(emptyScope);
  const [scopes, setScopes] = useState<GameScope[]>([]);
  const [scopeError, setScopeError] = useState("");
  const [range, setRange] = useState<string>("1h");
  const [refresh, setRefresh] = useState(0);
  const [readout, setReadout] = useState<{ scope: GameScope; samples: Snapshot; error: string }>();
  const [expanded, setExpanded] = useState(false);
  const query = useMemo(() => snapshotExpression(scope), [scope]);
  const activeTick = tick + refresh;

  useEffect(() => {
    const controller = new AbortController();
    querySnapshot(scopesExpression, controller.signal).then((items) => {
      if (controller.signal.aborted) return;
      setScopes(items.map((item) => ({ channel: item.metric.channel, environment: item.metric.environment })));
      setScopeError("");
    }).catch((error) => {
      if (!controller.signal.aborted) setScopeError(message(error));
    });
    return () => controller.abort();
  }, [refresh]);

  useEffect(() => {
    const controller = new AbortController();
    querySnapshot(query, controller.signal).then((result) => {
      if (!controller.signal.aborted) setReadout({ scope, samples: readSnapshot(result), error: "" });
    }).catch((error) => {
      if (!controller.signal.aborted) setReadout({ scope, samples: {}, error: message(error) });
    });
    return () => controller.abort();
  }, [activeTick, query, scope]);

  const ready = readout?.scope === scope;
  const samples = ready ? readout.samples : {};
  const error = (ready && readout.error) || scopeError;
  const channelOptions = [...new Set([scope.channel, ...scopes.map((item) => item.channel)].filter(Boolean))].sort();
  const environmentOptions = [...new Set([scope.environment, ...scopes.filter((item) => !scope.channel || item.channel === scope.channel)
    .map((item) => item.environment)].filter(Boolean))].sort();
  const renderSignal = (id: SignalID) => display(samples[id]?.value, gameSignals[id].unit);

  return <section className="game-overview" aria-labelledby="game-overview-heading" aria-busy={!ready}>
    <div className="game-overview-toolbar">
      <h2 id="game-overview-heading">{text(["Game overview", "游戏监控总览"])}</h2>
      <div className="game-overview-filters">
        <label>{text(["Channel", "渠道"])}
          <select value={scope.channel} onChange={(event) => setScope({ channel: event.target.value, environment: "" })}>
            <option value="">{text(["All", "全部"])}</option>
            {channelOptions.map((channel) => <option key={channel}>{channel}</option>)}
          </select>
        </label>
        <label>{text(["Environment", "环境"])}
          <select value={scope.environment} onChange={(event) => setScope({ ...scope, environment: event.target.value })}>
            <option value="">{text(["All", "全部"])}</option>
            {environmentOptions.map((environment) => <option key={environment}>{environment}</option>)}
          </select>
        </label>
        <select aria-label={text(["Time range", "时间范围"])} value={range} onChange={(event) => setRange(event.target.value)}>
          {timeRanges.map((option) => <option key={option.id} value={option.id}>{text(option.title)}</option>)}
        </select>
        <Button className="game-icon-button" aria-label={text(["Refresh", "刷新"])} title={text(["Refresh", "刷新"])} onClick={() => setRefresh((value) => value + 1)}>
          <RefreshCw size={16} aria-hidden="true" />
        </Button>
        <Button className="game-icon-button" aria-label={text(["Download Grafana dashboard", "下载 Grafana 仪表盘"])}
          title={text(["Download Grafana dashboard", "下载 Grafana 仪表盘"])} onClick={downloadDashboard}>
          <Download size={16} aria-hidden="true" />
        </Button>
      </div>
    </div>
    {error && <Alert severity="error" title={text(["Unable to load metrics", "指标查询失败"])}>{error}</Alert>}
    <dl className="game-headline">{headlineSignals.map((id) => <div key={id}>
      <dt>{text(gameSignals[id].caption)}</dt>
      <dd>{id === "logins" && samples[id]?.value !== undefined
        ? <>{display(samples[id]?.value)} <small>{text(["/min", "/分钟"])}</small></>
        : renderSignal(id)}</dd>
    </div>)}</dl>
    <p className="game-day-definition">{text(["Today starts at 00:00 China Standard Time. Accounts are deduplicated per channel and environment.", "今日从北京时间 00:00 起算。各渠道、环境按账号去重。"])}</p>
    <div className="game-path-table"><table>
      <thead><tr>
        <th scope="col">{text(["Service path", "服务链路"])}</th>
        <th scope="col">{text(["Scope", "范围"])}</th>
        <th scope="col">{text(["Signal", "观测指标"])}</th>
        <th scope="col">{text(["Current", "当前值"])}</th>
        <th scope="col">{text(["Status", "上报状态"])}</th>
      </tr></thead>
      <tbody>{servicePath.map((stage) => <tr key={stage.signal}>
        <th scope="row">{text(stage.title)}</th>
        <td>{stage.scope === "shared" ? text(["Shared service", "共享服务"]) : text(["Selected scope", "所选范围"])}</td>
        <td>{text(gameSignals[stage.signal].caption)}</td>
        <td className="game-value">{renderSignal(stage.signal)}</td>
        <td><Health measurement={samples[stage.signal]} loading={!ready} /></td>
      </tr>)}</tbody>
    </table></div>
    <h3>{text(["Operational signals", "运行指标"])}</h3>
    <SignalCharts definitions={primaryCharts} scope={scope} range={range} tick={activeTick} />
    <div className="game-details-grid">
      <section className="game-detail-card">
        <div className="game-detail-heading">
          <h3>{text(["Traces & latency", "链路追踪与延迟"])}</h3>
          <Button onClick={openExplore}>{text(["Open Explore", "打开查询探索"])}</Button>
        </div>
        <SignalList ids={latencySignals} samples={samples} />
        <p>{text(["Search traces in Explore using a configured Tempo datasource.", "在查询探索中选择已配置的 Tempo 数据源检索链路。"])}</p>
      </section>
      <section className="game-detail-card">
        <h3>{text(["Operations", "运行详情"])}</h3>
        <SignalList ids={operationSignals} samples={samples} />
      </section>
    </div>
    <details className="game-more-metrics" onToggle={(event) => setExpanded(event.currentTarget.open)}>
      <summary>{text(["Service metrics", "查看各服务趋势"])}</summary>
      {expanded && <SignalCharts definitions={detailCharts} scope={scope} range={range} tick={activeTick} />}
    </details>
  </section>;
}
