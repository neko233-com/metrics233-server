import { lazy, Suspense, useState, type FormEvent } from "react";
import { Activity, ArrowRight, Copy } from "lucide-react";
import { api, jsonBody, message, parseLabels, type Dashboard, type Metric, type Panel, type Target } from "../api";
import { getLocale } from "../i18n";
import { Alert, Button, Field, Input } from "../ui";
import { useSession } from "../ui/session";
import { nativePanelForSDK } from "../grafana/panel-queries";
import "./business-monitoring.css";
import "./connection-setup.css";

const text = (en: string, zh: string) => getLocale() === "zh" ? zh : en;
const BusinessInstances = lazy(() => import("./BusinessInstances"));
const ConnectionResources = lazy(() => import("./ConnectionSetup"));

export default function BusinessMonitoring({ metrics, dashboards, targets, reload, open, navigate, tick = 0 }: {
  metrics: Metric[];
  dashboards: Dashboard[];
  targets: Target[];
  tick?: number;
  reload: () => Promise<void>;
  open: (dashboard: Dashboard) => void;
  navigate: (view: "collectors" | "explore" | "alerts") => void;
}) {
  const { canEdit } = useSession();
  const [surface,setSurface]=useState<"services"|"resources">(() => {
    try { return sessionStorage.getItem("metrics233-dashboard-category") === "resources" ? "resources" : "services"; } catch { return "services"; }
  });
  function selectSurface(value: "services" | "resources") {
    setSurface(value);
    try { sessionStorage.setItem("metrics233-dashboard-category", value); } catch { /* Session preference is optional. */ }
  }
  const [selected, setSelected] = useState("");
  const [kind, setKind] = useState<"gauge" | "counter">("gauge");
  const [name, setName] = useState("");
  const [labels, setLabels] = useState("{}");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [copied, setCopied] = useState(false);
  const metric = selected;
  const exists = metrics.some((item) => item.name === metric);
  const payload = JSON.stringify({ samples: [
    { name: "business_orders_total", labels: { service: "shop", environment: "production" }, value: 233, timestamp: 0 },
    { name: "business_active_users", labels: { service: "shop", environment: "production" }, value: 42, timestamp: 0 },
  ] }, null, 2);

  async function create(event: FormEvent) {
    event.preventDefault();
    if (!canEdit || busy || !exists) return;
    setError("");
    setBusy(true);
    try {
      const matchers = parseLabels(labels);
      if (Object.keys(matchers).length > 32 || Object.entries(matchers).some(([key, value]) =>
        !/^[a-zA-Z_][a-zA-Z0-9_]{0,99}$/.test(key) || key === "__name__" || new TextEncoder().encode(value).length > 512))
        throw new Error(text("Use at most 32 valid labels; values must be at most 512 bytes.", "最多使用 32 个合法标签，标签值不超过 512 字节。"));
      if (!/^[a-zA-Z_:][a-zA-Z0-9_:]{0,199}$/.test(metric)) throw new Error(text("Invalid metric name", "指标名无效"));
      const selector = metric + (Object.keys(matchers).length ? `{${Object.entries(matchers).map(([key, value]) => `${key}=${JSON.stringify(value)}`).join(",")}}` : "");
      const panel = (id: string, title: string, expr: string, visualization: string, unit = "count"): Panel => nativePanelForSDK({
        id, title, expr, metric, aggregation: "last", visualization, unit,
      });
      const dashboard = await api<Dashboard>("/dashboards", { method: "POST", body: jsonBody({
        name: name.trim() || `${text("Business", "业务")} · ${metric}`,
        panels: [
          panel("current", text("Current total", "当前总量"), `sum(${selector})`, "stat"),
          panel("trend", kind === "counter" ? text("Events per second · 5m rate", "每秒事件数 · 5 分钟速率") : text("Value over time", "数值趋势"), kind === "counter" ? `sum(rate(${selector}[5m]))` : `sum(${selector})`, "timeseries", kind === "counter" ? "ops" : "count"),
          panel("series", text("By label set", "按标签明细"), selector, "table"),
        ],
      }) });
      await reload();
      open(dashboard);
    } catch (cause) {
      setError(message(cause));
    } finally {
      setBusy(false);
    }
  }

  return <>
    <div className="connection-surfaces" role="group" aria-label={text("Dashboard categories","仪表盘分类")}><Button aria-pressed={surface==="services"} onClick={()=>selectSurface("services")}>{text("Business services","业务服务")}</Button><Button aria-pressed={surface==="resources"} onClick={()=>selectSurface("resources")}>{text("Infrastructure","基础设施")}</Button></div>
    {surface==="resources" ? <Suspense fallback={<p role="status">{text("Loading resources…","正在加载资源…")}</p>}><ConnectionResources targets={targets} dashboards={dashboards} open={open} reload={reload} navigate={()=>navigate("collectors")} tick={tick}/></Suspense> : <>
    {error && <Alert severity="error" title={text("Request failed", "操作失败")}>{error}</Alert>}
    <Suspense fallback={<p role="status">{text("Loading instances…", "正在加载实例…")}</p>}><BusinessInstances dashboards={dashboards} open={open} reload={reload} tick={tick} /></Suspense>
    <details className="business-setup"><summary>{text("Connect metrics or create a template", "接入指标或创建模板")}</summary><div className="business-grid">
      <section className="business-card" aria-labelledby="business-dashboard-title">
        <h2 id="business-dashboard-title"><Activity size={19} />{text("Build from your metrics", "使用真实指标建图")}</h2>
        <p className="subtle">{text("Choose a metric already received by this workspace. Counter rates need at least two samples in five minutes.", "选择工作区已收到的指标。计数器速率需要 5 分钟内至少两个样本。")}</p>
        {!metrics.length && <p role="status">{text("No metrics received yet. Connect an exporter or send a push batch first.", "尚未收到指标，请先连接 exporter 或推送指标。")}</p>}
        <form onSubmit={create}>
          <Field label={text("Metric", "指标")}><select aria-label={text("Metric", "指标")} required value={metric} onChange={(event) => { setSelected(event.target.value); setKind(event.target.value.endsWith("_total") ? "counter" : "gauge"); }}>
            <option value="">{text("Choose a metric", "选择指标")}</option>
            {metrics.map((item) => <option key={item.name} value={item.name}>{item.name} ({item.series_count})</option>)}
          </select></Field>
          <Field label={text("Metric type", "指标类型")}><select aria-label={text("Metric type", "指标类型")} value={kind} onChange={(event) => setKind(event.target.value as "gauge" | "counter")}>
            <option value="gauge">{text("Gauge · current value (online users, queue depth)", "Gauge · 当前值（在线人数、队列长度）")}</option>
            <option value="counter">{text("Counter · cumulative events (orders, errors)", "Counter · 累计事件（订单、错误次数）")}</option>
          </select></Field>
          <Field label={text("Dashboard name", "仪表盘名称")}><Input aria-label={text("Dashboard name", "仪表盘名称")} value={name} maxLength={200} placeholder={text("Business overview", "业务总览")} onChange={(event) => setName(event.target.value)} /></Field>
          <details><summary>{text("Label filters", "标签过滤")}</summary><Field label={text("Labels (JSON)", "标签（JSON）")}><Input aria-label={text("Labels (JSON)", "标签（JSON）")} value={labels} maxLength={18000} onChange={(event) => setLabels(event.target.value)} /></Field><p className="subtle">{text("Use service, environment or region to select a scope.", "可用 service、environment 或 region 限定范围。")}</p></details>
          {canEdit ? <Button type="submit" variant="primary" disabled={busy || !exists}>{busy ? text("Creating…", "正在创建…") : text("Create business dashboard", "创建业务仪表盘")}<ArrowRight size={16} /></Button> : <p className="subtle">{text("Editor or Admin role is required to create dashboards or push metrics.", "创建仪表盘或推送指标需要 Editor 或 Admin 角色。")}</p>}
        </form>
        <div className="business-actions"><Button onClick={() => navigate("explore")}>{text("Explore metrics", "查询指标")}</Button><Button onClick={() => navigate("alerts")}>{text("Configure alerts", "配置告警")}</Button></div>
      </section>
      <section className="business-card" aria-labelledby="business-connect-title">
        <h2 id="business-connect-title">{text("Connect your application", "接入你的应用")}</h2>
        <p>{text("Use an existing Prometheus/OpenMetrics exporter, OTLP SDK, or native JSON push. All paths share the same storage and queries.", "可使用 Prometheus/OpenMetrics exporter、OTLP SDK 或原生 JSON 推送，各接入方式共用存储与查询。")}</p>
        <dl className="business-endpoints">
          <dt>JSON</dt><dd><code>POST /api/v1/ingest</code></dd>
          <dt>OTLP/HTTP</dt><dd><code>POST /v1/metrics</code></dd>
          <dt>Remote write v1</dt><dd><code>POST /api/v1/write</code></dd>
        </dl>
        <Button onClick={() => navigate("collectors")}>{text("Connect an exporter", "连接 exporter")}</Button>
        <details className="business-example"><summary>{text("JSON payload example", "JSON 数据示例")}</summary>
          <p className="subtle">{text("Example values only. Send your actual cumulative totals and current values; timestamp 0 uses server time. Use Content-Type: application/json and your workspace authentication.", "以下仅为格式示例。请发送真实累计值和当前值；timestamp 为 0 使用服务器时间。使用 Content-Type: application/json 及工作区认证。")}</p>
          <pre><code>{payload}</code></pre>
          <Button onClick={async () => { try { await navigator.clipboard.writeText(payload); setCopied(true); } catch { setError(text("Copy unavailable; select the example text to copy.", "无法自动复制，请选择示例文本复制。")); } }}><Copy size={16} />{copied ? text("Copied", "已复制") : text("Copy payload", "复制数据示例")}</Button>
        </details>
        <p className="subtle">{text("Keep labels bounded: service, environment, region. Avoid user IDs, order IDs or secrets. Revenue metrics should state currency and units. DAU and retention require upstream deduplication; gauges alone do not calculate them.", "标签使用有界的服务、环境、区域；避免用户 ID、订单 ID 和凭据。收入指标应注明币种与单位。DAU 与留存需上游去重计算，不能直接由在线人数推算。")}</p>
      </section>
    </div></details>
    </>}
  </>;
}
