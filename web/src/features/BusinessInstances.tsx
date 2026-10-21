import { lazy, Suspense, useEffect, useRef, useState } from "react";
import { ArrowLeft, ArrowRight, Copy, RefreshCw } from "lucide-react";
import { api, ApiError, jsonBody, message, timeAgo, type Dashboard, type Panel } from "../api";
import { getLocale } from "../i18n";
import { Alert, Button, Field, Input } from "../ui";
import { useSession } from "../ui/session";
import { nativePanelForSDK } from "../grafana/panel-queries";
import type { TimeSelection } from "../grafana/time-range";
import { DashboardVariableBar, useDashboardVariables } from "../components/DashboardVariables";
import "./business-instances.css";

const NativePanel = lazy(() => import("../grafana/GrafanaPanel"));
const text = (en: string, zh: string) => getLocale() === "zh" ? zh : en;
function localizedBuiltin(panel: Panel): Panel {
  const titles: Record<string, string> = { online: text("Online players", "在线玩家"), sessions: text("Successful player sessions · cumulative", "累计成功玩家会话"), "players-trend": text("Online players over time", "在线玩家趋势") };
  return { ...panel, title: titles[panel.id] || panel.title };
}
function independentPanels(panels: Panel[]): Panel[] {
  const copy = structuredClone(panels);
  const pending: unknown[] = [copy];
  while (pending.length) {
    const value = pending.pop();
    if (!value || typeof value !== "object") continue;
    const object = value as Record<string, unknown>;
    delete object.libraryPanel;
    delete object._metricspanelLibrarySource;
    for (const child of Object.values(object)) if (child && typeof child === "object") pending.push(child);
  }
  return copy;
}
type Identity = { service: string; role: string; serverId: string; environment: string; channel: string; instance: string; implementation: string };
type Instance = { id: string; identity: Identity; reported_ip: string; observed_ip: string; first_seen: number; last_seen: number; binding: { version: number; mode: "default" | "inherit" | "independent"; dashboard_id: string } };
type InstancePage = { items: Instance[]; next_cursor: string; capacity: number; at_capacity: boolean };
type Population = { available: boolean; players: number | null; snapshot_at: number; stale: boolean; status: "ready" | "missing" | "ambiguous" | "invalid" | "query_error" };
type Resolved = { instance: Instance; dashboard: Dashboard; population?: Population };
type Props = { dashboards: Dashboard[]; open: (dashboard: Dashboard) => void; reload: () => Promise<void>; tick?: number };

export default function BusinessInstances({ dashboards, open, reload, tick = 0 }: Props) {
  const [selected, setSelected] = useState<string | null>(null);
  const [cursor, setCursor] = useState("");
  const [previous, setPrevious] = useState<string[]>([]);
  const [revision, setRevision] = useState(0);
  const [page, setPage] = useState<InstancePage | null>(null);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(true);
  const [search, setSearch] = useState("");
  const [query, setQuery] = useState("");
  useEffect(() => {
    if (selected) return;
    const controller = new AbortController(); setLoading(true); setError("");
    api<InstancePage>(`/business/instances?limit=50&cursor=${encodeURIComponent(cursor)}&q=${encodeURIComponent(query)}`, { signal: controller.signal })
      .then(value => { if (!controller.signal.aborted) setPage(value); })
      .catch(cause => { if (!controller.signal.aborted) setError(message(cause)); })
      .finally(() => { if (!controller.signal.aborted) setLoading(false); });
    return () => controller.abort();
  }, [cursor, revision, query, tick, selected]);
  if (selected) return <BusinessInstanceDetail key={selected} id={selected} dashboards={dashboards} open={open} reload={reload} tick={tick} back={() => { setSelected(null); setRevision(value => value + 1); }} />;
  return <section className="business-instance-directory" aria-label={text("Business instances", "业务实例")}>
    <div className="business-instance-toolbar"><div><h2>{text("Business virtual dashboards", "业务虚拟仪表盘")}</h2><p className="subtle">{text("One view per reported gateway or game server. Instances appear only after a real accepted report.", "每个真实上报的网关或游戏服各有一个视图；无上报时不会创建虚拟实例。")}</p></div><Button disabled={loading} onClick={() => setRevision(value => value + 1)}><RefreshCw size={16} />{text("Refresh instances", "刷新实例")}</Button></div>
    {error && <Alert severity="error" title={text("Cannot load instances", "无法加载实例")}>{error}</Alert>}
    {loading && <p role="status">{text("Loading instances…", "正在加载实例…")}</p>}
    {page?.at_capacity && <Alert severity="warning" title={text("Instance observation capacity reached", "实例观测容量已满")}>{text(`The ${page.capacity} existing identities continue to update; new identities cannot be observed.`, `现有 ${page.capacity} 个身份继续更新，暂不登记新身份。`)}</Alert>}
    <form className="business-instance-toolbar" onSubmit={event => { event.preventDefault(); setCursor(""); setPrevious([]); setQuery(search.trim()); }}><Field label={text("Search all instances", "搜索全部实例")}><Input aria-label={text("Search all instances", "搜索全部实例")} type="search" maxLength={128} value={search} onChange={event => setSearch(event.target.value)} placeholder="service / serverId / IP" /></Field><Button type="submit" disabled={loading}>{text("Search", "搜索")}</Button></form>
    <div className="table-scroll"><table className="business-instance-list"><thead><tr><th>{text("Instance dashboard", "实例仪表盘")}</th><th>{text("Scope", "范围")}</th><th>{text("Reported host IP", "上报宿主 IP")}</th><th>{text("Observed source IP", "服务端观测来源 IP")}</th><th>{text("Last report", "最近上报")}</th></tr></thead><tbody>{page?.items.map(item => <tr key={item.id}><td><Button onClick={() => setSelected(item.id)}>{item.identity.service} · {item.identity.role} · {item.identity.serverId}<ArrowRight size={15} /></Button><div className="subtle">{item.identity.instance} · {item.identity.implementation || "—"}</div></td><td>{item.identity.environment || text("Not reported", "未上报")} / {item.identity.channel || "—"}</td><td>{item.reported_ip || "—"}<small>{text("Advisory label", "仅为上报标签")}</small></td><td>{item.observed_ip || text("Collector / unavailable", "采集或尚未观测")}</td><td>{timeAgo(item.last_seen)}<small>{text("Observation, not availability", "上报记录，不代表在线状态")}</small></td></tr>)}</tbody></table></div>
    {!loading && !page?.items.length && <p role="status">{text("No reported business instances. Send service, role (gateway/logic/game), serverId and instance labels; environment and channel keep deployments separate.", "尚无业务实例。上报需包含 service、role（gateway/logic/game）、serverId 与 instance；environment、channel 用于区分环境。")}</p>}
    <div className="business-instance-toolbar"><Button disabled={!previous.length || loading} onClick={() => { const before = [...previous]; setCursor(before.pop() || ""); setPrevious(before); }}>{text("Previous", "上一页")}</Button><span>{text("Up to 50 instances per page", "每页最多 50 个实例")}</span><Button disabled={!page?.next_cursor || loading} onClick={() => { setPrevious(value => [...value, cursor]); setCursor(page!.next_cursor); }}>{text("Next", "下一页")}</Button></div>
  </section>;
}

function BusinessInstanceDetail({ id, dashboards, open, reload, back, tick = 0 }: Props & { id: string; back: () => void }) {
  const { canEdit } = useSession();
  const [resolved, setResolved] = useState<Resolved | null>(null);
  const [record, setRecord] = useState<Instance | null>(null);
  const [error, setError] = useState("");
  const [revision, setRevision] = useState(0);
  const [busy, setBusy] = useState(false);
  const [template, setTemplate] = useState("");
  const [range, setRange] = useState<TimeSelection>("1h");
  const [tab, setTab] = useState<"overview" | "regions" | "sql">("overview");
  const mutation = useRef<AbortController | null>(null);
  const templateVersion = useRef("");
  useEffect(() => () => mutation.current?.abort(), []);
  useEffect(() => {
    if (mutation.current) return;
    const controller = new AbortController(); setError("");
    api<Instance>(`/business/instances/${id}`, { signal: controller.signal }).then(value => { if (!controller.signal.aborted) setRecord(value); }).catch(cause => { if (!controller.signal.aborted) setError(message(cause)); });
    api<Resolved>(`/business/instances/${id}/dashboard`, { signal: controller.signal })
      .then(value => { if (!controller.signal.aborted) { setResolved(value); const version = `${value.instance.binding.version}:${value.instance.binding.mode}:${value.instance.binding.dashboard_id}`; if (version !== templateVersion.current) { templateVersion.current = version; setTemplate(value.instance.binding.mode === "inherit" ? value.instance.binding.dashboard_id : ""); } } })
      .catch(cause => { if (!controller.signal.aborted) { setResolved(null); setError(message(cause)); } });
    return () => controller.abort();
  }, [id, revision, tick]);
  async function bind(mode: Instance["binding"]["mode"], dashboardID: string) {
    const current = resolved?.instance || record;
    if (!current || !canEdit || mutation.current) return;
    const controller = new AbortController(); mutation.current = controller; setBusy(true); setError("");
    try {
      await api(`/business/instances/${id}/dashboard`, { method: "PUT", signal: controller.signal, body: jsonBody({ version: current.binding.version, mode, dashboard_id: dashboardID }) });
      if (!controller.signal.aborted) setRevision(value => value + 1);
    } catch (cause) {
      if (!controller.signal.aborted) setError(cause instanceof ApiError && cause.status === 409 ? text("Binding changed elsewhere. Refresh and review before saving again.", "绑定已在别处修改，请刷新核对后再保存。") : message(cause));
    } finally { mutation.current = null; if (!controller.signal.aborted) setBusy(false); }
  }
  async function copy() {
    if (!resolved || !canEdit || mutation.current) return;
    const controller = new AbortController(); mutation.current = controller; setBusy(true); setError(""); let created: Dashboard | undefined;
    try {
      const { id: _id, updated_at: _updated, ...source } = resolved.dashboard;
      created = await api<Dashboard>("/dashboards", { method: "POST", signal: controller.signal, body: jsonBody({ ...source, panels: independentPanels(resolved.instance.binding.mode === "default" ? source.panels.map(localizedBuiltin) : source.panels), name: `${resolved.instance.identity.service} · ${resolved.instance.identity.environment || "?"} · ${resolved.instance.identity.role} · ${resolved.instance.identity.serverId} · ${resolved.instance.identity.instance}` }) });
      await api(`/business/instances/${id}/dashboard`, { method: "PUT", signal: controller.signal, body: jsonBody({ version: resolved.instance.binding.version, mode: "independent", dashboard_id: created.id }) });
      await reload(); if (!controller.signal.aborted) open(created);
    } catch (cause) {
      if (!controller.signal.aborted) setError(created ? text(`Dashboard ${created.id} was saved, but binding failed. Find the saved copy in Dashboards; refresh before binding it. ${message(cause)}`, `副本 ${created.id} 已保存，但绑定失败。可在仪表盘列表找到它；请刷新后再绑定。${message(cause)}`) : message(cause));
    } finally { mutation.current = null; if (!controller.signal.aborted) setBusy(false); }
  }
  const item = resolved?.instance || record, dashboard = resolved?.dashboard;
  const mode = item?.binding.mode;
  return <section className="business-instance-detail" aria-label={text("Instance dashboard", "实例仪表盘")}>
    <div className="business-instance-toolbar"><Button onClick={back}><ArrowLeft size={15} />{text("All business instances", "全部业务实例")}</Button><Button disabled={busy} onClick={() => setRevision(value => value + 1)}><RefreshCw size={15} />{text("Refresh instance", "刷新实例")}</Button></div>
    {error && <Alert severity="error" title={text("Instance operation failed", "实例操作失败")}>{error}</Alert>}
    {error && item && !dashboard && canEdit && item.binding.mode !== "default" && <Button disabled={busy} onClick={() => void bind("default", "")}>{text("Reset built-in view", "恢复内置视图")}</Button>}
    {!resolved && !error && <p role="status">{text("Loading instance…", "正在加载实例…")}</p>}
    {item && dashboard && <><h2>{item.identity.service} · {item.identity.role} · {item.identity.serverId}</h2><p className="subtle">{item.identity.environment || text("Environment not reported", "环境未上报")} / {item.identity.channel || "—"} · {item.identity.instance}</p><details><summary>{text("Instance identity and source IPs", "实例身份与来源 IP")}</summary><dl className="business-instance-scope">{Object.entries(item.identity).map(([name, value]) => <div key={name}><dt>{name}</dt><dd>{value || text("Not reported", "未上报")}</dd></div>)}<div><dt>{text("Reported host IP (advisory)", "上报宿主 IP（标签）")}</dt><dd>{item.reported_ip || "—"}</dd></div><div><dt>{text("Canonical observed source IP", "服务端规范来源 IP")}</dt><dd>{item.observed_ip || "—"}</dd></div></dl>
      <p className="subtle">{text("The source IP may be a NAT/proxy address. Labels and fixed query variables are display/query scope, never authorization boundaries.", "来源 IP 可能为 NAT 或代理出口。标签和固定变量只限定展示查询，不是权限隔离。")}</p></details>
      <div className="business-instance-binding"><strong>{mode === "inherit" ? text("Live template inheritance", "实时继承模板") : mode === "independent" ? text("Independent dashboard", "独立自定义仪表盘") : text("Built-in instance view", "内置实例视图")}</strong>
      {canEdit && <div className="business-instance-actions"><Button disabled={busy} onClick={() => void copy()}><Copy size={15} />{text("Save independent copy", "另存独立副本")}</Button>{mode === "independent" && <Button disabled={busy} onClick={() => open(dashboard)}>{text("Edit independent dashboard", "编辑独立仪表盘")}</Button>}{mode === "inherit" && <Button disabled={busy} onClick={() => { const source = dashboards.find(value => value.id === item.binding.dashboard_id); if (source) open(source); }}>{text("Edit shared template", "编辑共享模板")}</Button>}<details><summary>{text("Template binding", "模板绑定")}</summary><p className="subtle">{text("Inheritance follows the latest template. Save a copy for independent panel edits. Use {${business_scope:raw}} in template metric selectors to include the full identity; serverId alone cannot distinguish environments. Fixed variables are query scope, not access control.", "继承会跟随模板最新版；另存副本可独立编辑图表。模板指标选择器请使用 {${business_scope:raw}} 传入完整身份，仅 serverId 无法区分环境。固定变量限定查询，不是权限控制。")}</p><Field label={text("Native dashboard template", "原生仪表盘模板")}><select value={template} aria-label={text("Native dashboard template", "原生仪表盘模板")} onChange={event => setTemplate(event.target.value)}><option value="">{text("Choose a dashboard", "选择仪表盘")}</option>{dashboards.filter(value => !value.grafana).map(value => <option key={value.id} value={value.id}>{value.name}</option>)}</select></Field><Button disabled={busy || !template} onClick={() => void bind("inherit", template)}>{text("Use live template", "使用实时模板")}</Button><Button disabled={busy || mode === "default"} onClick={() => void bind("default", "")}>{text("Reset built-in view", "恢复内置视图")}</Button></details></div>}</div>
      <div className="business-instance-tabs" role="group" aria-label={text("Instance sections", "实例区块")}>{([ ["overview", text("Overview", "概览")], ["regions", text("Player regions · all", "玩家地区 · 全部")], ["sql", text("Entity SQL", "Entity SQL")]] as const).map(([key, label]) => <Button key={key} aria-pressed={tab === key} onClick={() => setTab(key)}>{label}</Button>)}</div>
      {tab === "overview" && <PopulationNotice population={resolved?.population} />}
      {tab === "overview" && <><p className="subtle">{text("Online players are authenticated players (Gauge). Successful sessions are cumulative Counter events and may reset at process restart; they are not simultaneous players or gateway connections. Missing samples remain unavailable.", "在线玩家为已认证玩家 Gauge；累计成功会话为 Counter，进程重启可能重置，不等于同时在线人数或网关连接数。缺少样本不编造为 0。")}</p><InstanceNativePanels key={dashboard.id} dashboard={dashboard} builtin={mode === "default"} range={range} setRange={setRange} tick={revision + tick} />{!dashboard.panels.length && <p>{text("No panels configured for this gateway. Bind a native template or save an independent dashboard to add its reported metrics.", "该网关尚未配置图表。可绑定原生模板或另存独立仪表盘，添加它实际上报的指标。")}</p>}</>}
      {tab !== "overview" && <BusinessDataTable key={`${id}:${tab}:${revision}`} id={id} kind={tab} />}
    </>}
  </section>;
}

function InstanceNativePanels({ dashboard, builtin, range, setRange, tick }: { dashboard: Dashboard; builtin: boolean; range: TimeSelection; setRange: (range: TimeSelection) => void; tick: number }) {
  const state = useDashboardVariables(dashboard, range, tick);
  return <><DashboardVariableBar state={state} />{state.error && <Alert severity="error" title={text("Variable resolution failed", "变量解析失败")}>{state.error}</Alert>}{state.ready ? <div className="business-instance-panels"><Suspense fallback={<p role="status">{text("Loading panels…", "正在加载图表…")}</p>}>{dashboard.panels.map(panel => <NativePanel key={panel.id} panel={nativePanelForSDK(builtin ? localizedBuiltin(panel) : panel)} values={state.effective} range={range} onRange={setRange} tick={tick} dashboard={builtin ? undefined : dashboard} />)}</Suspense></div> : <p role="status">{text("Resolving variables…", "正在解析变量…")}</p>}</>;
}

type DataRow = { key: string; labels: Record<string, string>; values: Record<string, number | null> };
type DataPage = { items: DataRow[]; total: number; next_cursor: string; evaluated_at: number; window: string; population?: Population; regions_complete?: boolean; region_total?: number };
function PopulationNotice({ population }: { population?: Population }) {
  if (!population) return null;
  if (!population.available) return <Alert severity="warning" title={text("Player snapshot unavailable", "玩家快照不可用")}>{population.status === "ambiguous" ? text("Multiple population series share the latest timestamp. Check the instance labels; they are not added together.", "最新时间戳存在多个人口序列，请检查实例标签；不会将它们相加。") : population.status === "invalid" ? text("The population sample is invalid.", "人口样本无效。") : text("No authoritative population sample is available in the last five minutes, or its query failed. Missing data is not zero and does not prove that players are offline.", "最近 5 分钟缺少权威人口样本或查询失败。缺失不等于 0，也不能据此认定玩家离线。")}</Alert>;
  const stamp = new Date(population.snapshot_at).toLocaleString();
  return population.stale ? <Alert severity="warning" title={text("Player data is stale (over 90 seconds)", "玩家数据陈旧（超过 90 秒）")}>{text(`The last snapshot at ${stamp} reported ${population.players} players. It does not establish the current population or server availability. Longer configured reporting intervals can also trigger this freshness warning.`, `最近快照 ${stamp} 上报 ${population.players} 名玩家，不代表当前人口或服务可用性；配置较长上报间隔也可能触发此新鲜度提示。`)}</Alert> : <p className="subtle">{text(`Player snapshot: ${stamp} · ${population.players} reported players`, `玩家快照：${stamp} · 上报 ${population.players} 人`)}</p>;
}
const ROW_HEIGHT = 44;
function BusinessDataTable({ id, kind }: { id: string; kind: "regions" | "sql" }) {
  const [window, setWindow] = useState("5m");
  return <section><div className="business-instance-toolbar"><h3>{kind === "regions" ? text("All player regions", "全部玩家地区") : text("Entity SQL · actual database attempts", "Entity SQL · 数据库实际执行")}</h3>{kind === "sql" && <Field label={text("Recent window", "最近窗口")}><select aria-label={text("Recent window", "最近窗口")} value={window} onChange={event => setWindow(event.target.value)}><option value="5m">5m</option><option value="15m">15m</option><option value="1h">1h</option></select></Field>}</div><PagedBusinessTable key={`${kind}:${window}`} id={id} kind={kind} window={window} />
    {kind === "regions" ? <p className="subtle">{text("Current online players grouped by country, province and city; all regions can be loaded. Unknown means the location was unavailable. IP location is approximate, not a person's exact location.", "按国家、省份、城市汇总当前在线玩家，可加载全部地区。Unknown 表示无法定位；IP 归属地为大致位置，并非玩家精确真实所在地。")}{" "}<a href="https://db-ip.com" target="_blank" rel="noreferrer">{text("IP location data: DB-IP Lite", "IP归属地数据：DB-IP Lite")}</a></p> : <details><summary>{text("Counter definitions", "统计口径")}</summary><p className="subtle">{text("Rows count successful driver RowsAffected, not queued entities or a durability guarantee. Calls and failures count real driver attempts (each retry counts). Recent counts use PromQL increase with reset handling and extrapolation; averages use duration sum/count, weighted across attempts. Cumulative values cover the current processes and may reset. Missing samples or insufficient window samples show —. Ordered by recent average latency, highest first; no top-k truncation.", "影响行数为驱动成功返回的 RowsAffected，不是入队 Entity 数或持久提交保证。执行/失败按实际 driver attempt 计数（重试每次计）。最近窗口使用可处理重置并外推的 PromQL increase，平均耗时按 duration sum/count 加权。累计值为当前进程累计，重启可能重置。无样本或窗口样本不足显示 —。默认按最近平均耗时从高到低排列，不截取 TopK。")}</p></details>}
  </section>;
}

function PagedBusinessTable({ id, kind, window }: { id: string; kind: "regions" | "sql"; window: string }) {
  const [page, setPage] = useState<DataPage | null>(null);
  const [rows, setRows] = useState<DataRow[]>([]);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);
  const [scrollTop, setScrollTop] = useState(0);
  const request = useRef<AbortController | null>(null);
  const alive = useRef(true);
  async function load(cursor: string) {
    if (request.current) return;
    const controller = new AbortController(); request.current = controller; setLoading(true); setError("");
    try {
      const value = await api<DataPage>(`/business/instances/${id}/data?kind=${kind}&window=${window}&limit=100&cursor=${encodeURIComponent(cursor)}`, { signal: controller.signal });
      if (alive.current && !controller.signal.aborted) { setPage(value); setRows(previous => { if (!cursor) return value.items; const keys = new Set(previous.map(row => row.key)); return [...previous, ...value.items.filter(item => !keys.has(item.key))]; }); }
    } catch (cause) { if (alive.current && !controller.signal.aborted) setError(message(cause)); }
    finally { if (request.current === controller) request.current = null; if (alive.current && !controller.signal.aborted) setLoading(false); }
  }
  useEffect(() => { alive.current = true; void load(""); return () => { alive.current = false; request.current?.abort(); request.current = null; }; }, [id, kind, window]);
  const columns = kind === "regions" ? [ ["country", text("Country", "国家")], ["province", text("Province", "省份")], ["city", text("City", "城市")], ["value", text("Online players", "在线玩家")]] : [ ["table", text("Entity / table", "Entity / 数据表")], ["operation", text("Operation", "操作")], ["avg_ms_window", text("Recent avg ms ↓", "窗口均耗时 ms ↓")], ["calls_window", text("Recent calls", "窗口执行次数")], ["rows_window", text("Recent rows", "窗口影响行数")], ["failures_window", text("Recent failures", "窗口失败次数")], ["calls_total", text("Total calls", "累计执行")], ["rows_total", text("Total rows", "累计影响行数")], ["failures_total", text("Total failures", "累计失败")], ["avg_ms_total", text("Total avg ms", "累计均耗时 ms")]];
  const start = Math.max(0, Math.floor(scrollTop / ROW_HEIGHT) - 4), end = Math.min(rows.length, start + 20);
  const label = kind === "regions" ? text("Player regions", "玩家地区") : text("Entity SQL statistics", "Entity SQL 统计");
  return <>{error && <Alert severity="error" title={text("Metric query failed", "指标查询失败")}>{error}</Alert>}{kind === "regions" && <><PopulationNotice population={page?.population} />{page?.regions_complete === false && page.population?.available && <Alert severity="warning" title={text("Region snapshot is incomplete", "地区快照尚不完整")}>{text(`The same snapshot contains ${page.region_total} regional players out of ${page.population.players} reported players. Wait for the complete batch; older region values are not used to fill the gap.`, `同轮地区合计 ${page.region_total} 人，人口上报 ${page.population.players} 人。请等待完整批次；不会用旧地区值补数。`)}</Alert>}</>}<div className="business-virtual-table" tabIndex={0} aria-label={text("Scrollable results", "可滚动结果")} onScroll={event => { const element = event.currentTarget; setScrollTop(element.scrollTop); if (element.scrollHeight - element.scrollTop - element.clientHeight < 180 && page?.next_cursor && !error) void load(page.next_cursor); }}><table aria-label={label} aria-rowcount={page ? page.total + 1 : undefined}><thead><tr>{columns.map(([key, name]) => <th key={key} scope="col">{name}</th>)}</tr></thead><tbody>{start > 0 && <tr aria-hidden="true"><td colSpan={columns.length} style={{ height: start * ROW_HEIGHT, padding: 0 }} /></tr>}{rows.slice(start, end).map((row, index) => <tr key={row.key} aria-rowindex={start + index + 2}>{columns.map(([key]) => <td key={key} title={row.labels[key]}>{key in row.labels ? row.labels[key] : ["country", "province", "city", "table", "operation"].includes(key) ? "Unknown" : row.values[key] == null ? "—" : row.values[key].toLocaleString(getLocale(), { maximumFractionDigits: 2 })}</td>)}</tr>)}{end < rows.length && <tr aria-hidden="true"><td colSpan={columns.length} style={{ height: (rows.length - end) * ROW_HEIGHT, padding: 0 }} /></tr>}</tbody></table></div>
    <div className="business-instance-toolbar"><span role="status">{loading ? text("Loading…", "正在加载…") : `${rows.length} / ${page?.total ?? "—"}`} {text("rows loaded", "行已加载")}{page ? ` · ${new Date(page.evaluated_at).toLocaleTimeString()}` : ""}</span>{page?.next_cursor && <Button disabled={loading} onClick={() => void load(page.next_cursor)}>{text("Load more", "加载更多")}</Button>}{error && <Button disabled={loading} onClick={() => void load(page?.next_cursor || "")}>{text("Retry", "重试")}</Button>}</div>{!loading && !error && page?.total === 0 && <p>{kind === "regions" && page.population?.available && page.population.players === 0 ? text("No players in this snapshot.", "该快照在线玩家为 0。") : text("No samples available for this instance. Waiting for real reports.", "该实例暂无样本，等待真实上报。")}</p>}</>;
}
