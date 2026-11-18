import type { Labels } from "../api";

type Copy = readonly [en: string, zh: string];
type GameRole = "logic" | "gateway" | "sdk" | "center";
type Reducer = "sum" | "rate" | "max" | "p95";
type Signal = {
  metric: string;
  role: GameRole;
  caption: Copy;
  unit: string;
  reducer?: Reducer;
  matchers?: Labels;
  scale?: number;
  daily?: boolean;
};

export type GameScope = { channel: string; environment: string };
export type InstantSample = { metric: Labels; value: [seconds: number, value: string] };
export const sampleFreshnessSeconds = 90;

const logicScopes = 'game_players_online{role="logic",channel!="",environment!=""}';
const sdkScopes = 'sdk_today_population_observed_at_seconds{role="sdk",target_channel!="",target_environment!=""}';
const dailyScopeLabels = 'label_replace(label_replace(' + sdkScopes +
  ', "channel", "$1", "target_channel", "(.+)"), "environment", "$1", "target_environment", "(.+)")';
export const scopesExpression = "count by (channel, environment) (" + logicScopes + " or " + dailyScopeLabels + ")";

export const gameSignals = {
  todayLogins: { metric: "sdk_today_login_unique_players", role: "sdk", caption: ["Unique logins today", "今日去重登录"], unit: "count", daily: true },
  todayRegistrations: { metric: "sdk_today_registered_unique_players", role: "sdk", caption: ["New registrations today", "今日新增注册"], unit: "count", daily: true },
  players: { metric: "game_players_online", role: "logic", caption: ["Online players", "在线玩家"], unit: "count" },
  connections: { metric: "gateway_connections_active", role: "gateway", caption: ["Client connections", "客户端连接"], unit: "count" },
  logins: { metric: "game_player_sessions_started_total", role: "logic", caption: ["Sessions started", "新建玩家会话"], unit: "per-minute", reducer: "rate", scale: 60 },
  links: { metric: "game_gateway_link_active", role: "logic", caption: ["Backend links", "后端 Link"], unit: "count" },
  publicLinks: { metric: "game_gateway_link_active", role: "logic", caption: ["Public backend links", "公网 Link"], unit: "count", matchers: { peer_scope: "public" } },
  sdk: { metric: "http_requests_total", role: "sdk", caption: ["HTTP requests", "HTTP 请求"], unit: "ops", reducer: "rate" },
  sdkFailures: { metric: "http_requests_failed_total", role: "sdk", caption: ["SDK HTTP failures", "SDK HTTP 失败"], unit: "ops", reducer: "rate" },
  center: { metric: "center_rpc_requests", role: "center", caption: ["Center RPC requests", "中心服 RPC 请求"], unit: "ops", reducer: "rate" },
  collect: { metric: "game_data_collect_events_total", role: "logic", caption: ["Events sent", "已发送事件"], unit: "ops", reducer: "rate", matchers: { outcome: "sent" } },
  collectDropped: { metric: "game_data_collect_events_total", role: "logic", caption: ["Events dropped", "丢弃事件"], unit: "ops", reducer: "rate", matchers: { outcome: "dropped" } },
  ingress: { metric: "gateway_bytes_received_total", role: "gateway", caption: ["Received", "接收"], unit: "Bps", reducer: "rate" },
  egress: { metric: "gateway_bytes_sent_total", role: "gateway", caption: ["Sent", "发送"], unit: "Bps", reducer: "rate" },
  loginAttempts: { metric: "game_login_attempts", role: "logic", caption: ["Login attempts", "登录请求"], unit: "ops", reducer: "rate" },
  gatewayRPC: { metric: "game_gateway_rpc_calls", role: "logic", caption: ["Gateway RPC calls", "网关 RPC 调用"], unit: "ops", reducer: "rate" },
  centerRPC: { metric: "game_center_rpc_calls", role: "logic", caption: ["Center RPC calls", "中心 RPC 调用"], unit: "ops", reducer: "rate" },
  loginLatency: { metric: "game_login_duration_bucket", role: "logic", caption: ["Login latency P95", "登录耗时 P95"], unit: "seconds", reducer: "p95" },
  centerLatency: { metric: "game_center_rpc_duration_bucket", role: "logic", caption: ["Center RPC latency P95", "中心 RPC 耗时 P95"], unit: "ms", reducer: "p95" },
  sql: { metric: "game_sql_calls_total", role: "logic", caption: ["SQL operations", "SQL 操作"], unit: "ops", reducer: "rate" },
  sqlFailures: { metric: "game_sql_failures_total", role: "logic", caption: ["SQL failures", "SQL 失败"], unit: "ops", reducer: "rate" },
  saveAge: { metric: "game_entitysave_oldest_pending_age_ms", role: "logic", caption: ["Oldest pending save", "最久待保存时间"], unit: "ms", reducer: "max" },
  kvKeys: { metric: "center_kv_keys", role: "center", caption: ["Center KV keys", "中心服 KV 键数"], unit: "count" },
  gatewayCPU: { metric: "process_cpu_seconds_total", role: "gateway", caption: ["Gateway CPU cores", "网关 CPU 核数"], unit: "count", reducer: "rate" },
  gatewayRSS: { metric: "process_resident_memory_bytes", role: "gateway", caption: ["Gateway RSS", "网关常驻内存"], unit: "bytes" },
} as const satisfies Record<string, Signal>;

export type SignalID = keyof typeof gameSignals;
export const headlineSignals: readonly SignalID[] = ["todayLogins", "todayRegistrations", "players", "connections", "logins", "links"];
export const servicePath = [
  { title: ["Gateway", "网关"], signal: "connections", scope: "shared" },
  { title: ["SDK", "SDK"], signal: "sdk", scope: "shared" },
  { title: ["Game logic", "逻辑服"], signal: "players", scope: "scope" },
  { title: ["Center", "中心服"], signal: "center", scope: "shared" },
  { title: ["Data collection", "数据采集"], signal: "collect", scope: "scope" },
] as const;

type ChartSpec = { id: string; title: Copy; series: readonly SignalID[] };

export const primaryCharts: readonly ChartSpec[] = [
  { id: "gateway-traffic", title: ["Gateway traffic", "网关流量"], series: ["ingress", "egress"] },
  { id: "login-rpc", title: ["Login & RPC", "登录与 RPC"], series: ["loginAttempts", "gatewayRPC", "centerRPC"] },
];
export const detailCharts: readonly ChartSpec[] = [
  { id: "population", title: ["Today’s unique login and registrations", "今日去重登录与新增注册"], series: ["todayLogins", "todayRegistrations"] },
  { id: "links", title: ["Backend link paths", "后端 Link 路径"], series: ["links", "publicLinks"] },
  { id: "sql", title: ["Database operations", "数据库操作"], series: ["sql", "sqlFailures"] },
  { id: "save", title: ["Save queue age", "存档队列等待时间"], series: ["saveAge"] },
  { id: "events", title: ["Data collection outcomes", "数据采集结果"], series: ["collect", "collectDropped"] },
  { id: "sdk", title: ["SDK HTTP requests", "SDK HTTP 请求"], series: ["sdk", "sdkFailures"] },
  { id: "center", title: ["Center RPC requests", "中心服 RPC 请求"], series: ["center"] },
  { id: "kv", title: ["Center KV keys", "中心服 KV 键数"], series: ["kvKeys"] },
  { id: "login-latency", title: ["Login latency P95", "登录耗时 P95"], series: ["loginLatency"] },
  { id: "center-latency", title: ["Center RPC latency P95", "中心 RPC 耗时 P95"], series: ["centerLatency"] },
  { id: "gateway-cpu", title: ["Gateway CPU", "网关 CPU"], series: ["gatewayCPU"] },
  { id: "gateway-memory", title: ["Gateway memory", "网关内存"], series: ["gatewayRSS"] },
];

function selector(signal: Signal, scope: GameScope, grafana = false): string {
  const matchers = Object.entries({ role: signal.role, ...signal.matchers })
    .map(([name, value]) => name + "=" + JSON.stringify(value));
  if (signal.role !== "gateway" && (signal.reducer === "rate" || signal.reducer === "p95")) {
    matchers.push('otel_temporality="cumulative"');
  }
  if (signal.role === "logic" || signal.daily) {
    for (const key of ["channel", "environment"] as const) {
      const label = signal.daily ? "target_" + key : key;
      if (grafana) matchers.push(label + '=~"${' + key + ':regex}"');
      else if (scope[key]) matchers.push(label + "=" + JSON.stringify(scope[key]));
    }
  }
  return signal.metric + "{" + matchers.join(",") + "}";
}

export function signalExpression(id: SignalID, scope: GameScope, current = false, grafana = false): string {
  const signal: Signal = gameSignals[id];
  const source = selector(signal, scope, grafana);
  const freshness = " and (time() - timestamp(" + source + ") <= " + sampleFreshnessSeconds + ")";
  if (signal.daily) {
    const period = selector({ ...signal, metric: "sdk_today_population_period_start_seconds" }, scope, grafana);
    const observed = selector({ ...signal, metric: "sdk_today_population_observed_at_seconds" }, scope, grafana);
    const dayStart = "scalar(floor(vector((time() + 28800) / 86400))) * 86400 - 28800";
    return "sum(max by (target_app_id, target_channel, target_environment) (" + source +
      " and (" + period + " == (" + dayStart + ")) and (time() - " + observed + " <= " + sampleFreshnessSeconds + ")))";
  }
  switch (signal.reducer) {
    case "p95":
      return "histogram_quantile(0.95, sum by (le) (rate(" + source + "[5m])" + (current ? freshness : "") + "))";
    case "rate":
      return "sum(rate(" + source + "[5m])" + (current ? freshness : "") + ")" + (signal.scale ? " * " + signal.scale : "");
    default:
      return (signal.reducer || "sum") + "(" + source + (current ? freshness : "") + ")";
  }
}

const snapshotSignals = [...new Set<SignalID>([
  ...headlineSignals, ...servicePath.map((stage) => stage.signal),
  "sql", "sqlFailures", "saveAge", "collectDropped", "loginLatency", "centerLatency", "publicLinks",
])];

export function snapshotExpression(scope: GameScope): string {
  return snapshotSignals.flatMap((id) => {
    const source = selector(gameSignals[id], scope);
    const age = "clamp_min(time() - max(timestamp(" + source + ")), 0)";
    return [signalExpression(id, scope, true), age].map((expr, index) =>
      'label_replace((' + expr + '), "mp_signal", "' + id + (index ? '_age' : '') + '", "", "")');
  }).join(" or ");
}

export type Snapshot = Partial<Record<SignalID, { value?: number; age?: number }>>;

export function readSnapshot(samples: readonly InstantSample[]): Snapshot {
  const bySignal: Snapshot = {};
  for (const sample of samples) {
    const label = sample.metric.mp_signal;
    if (!label) continue;
    const age = label.endsWith("_age");
    const id = (age ? label.slice(0, -4) : label) as SignalID;
    const raw = sample.value[1];
    const value = Number(raw);
    if (!Object.hasOwn(gameSignals, id) || !raw || !Number.isFinite(value)) continue;
    const signal = bySignal[id] ||= {};
    if (age) signal.age = value;
    else signal.value = value;
  }
  return bySignal;
}

export function makeGrafanaGameDashboard(locale: "en" | "zh") {
  const language = locale === "zh" ? 1 : 0;
  const source = { type: "prometheus", uid: "${DS_METRICS233}" };
  const scope = { channel: "", environment: "" };
  const unitID = (unit: string) => ({ seconds: "s", "per-minute": "cpm", Bps: "Bps" }[unit] || (unit === "count" ? "short" : unit));
  const panels = [
    ...headlineSignals.map((id, index) => ({
      id: index + 1,
      type: "stat",
      title: gameSignals[id].caption[language],
      gridPos: { x: index * 4, y: 0, w: 4, h: 4 },
      datasource: source,
      fieldConfig: { defaults: { unit: unitID(gameSignals[id].unit) }, overrides: [] },
      targets: [{ refId: "A", expr: signalExpression(id, scope, true, true), instant: true, range: false, datasource: source }],
    })),
    ...[...primaryCharts, ...detailCharts].map((chart, index) => ({
      id: headlineSignals.length + 1 + index,
      type: "timeseries",
      title: chart.title[language],
      gridPos: { x: index % 2 * 12, y: 4 + Math.floor(index / 2) * 7, w: 12, h: 7 },
      datasource: source,
      fieldConfig: { defaults: { unit: unitID(gameSignals[chart.series[0]].unit) }, overrides: [] },
      targets: chart.series.map((id, i) => ({
        refId: String.fromCharCode(65 + i),
        expr: signalExpression(id, scope, false, true),
        legendFormat: gameSignals[id].caption[language],
        datasource: source,
      })),
    })),
  ];
  return {
    __inputs: [{ name: "DS_METRICS233", label: "Metrics233", type: "datasource", pluginId: "prometheus", pluginName: "Prometheus" }],
    title: language ? "游戏监控总览" : "Game overview",
    schemaVersion: 39,
    tags: ["metrics233", "games"],
    time: { from: "now-1h", to: "now" },
    refresh: "30s",
    templating: { list: (["channel", "environment"] as const).map((name) => ({
      name,
      type: "query",
      label: name === "channel" ? (language ? "渠道" : "Channel") : (language ? "环境" : "Environment"),
      datasource: source,
      query: { query: "label_values(game_players_online{role=\"logic\"}, " + name + ")", refId: "PrometheusVariableQueryEditor-VariableQuery" },
      multi: true,
      includeAll: true,
      allValue: ".*",
      current: { selected: true, text: "All", value: "$__all" },
      refresh: 1,
    })) },
    panels,
  };
}
