import { lazy, Suspense, useEffect, useRef, useState } from "react";
import { ArrowLeft, Copy, RefreshCw, Settings2, Server } from "lucide-react";
import { api, ApiError, jsonBody, message, timeAgo, type Dashboard, type Panel } from "../api";
import { getLocale } from "../i18n";
import { Alert, Button, Field } from "../ui";
import { useSession } from "../ui/session";
import { nativePanelForSDK } from "../grafana/panel-queries";
import type { TimeSelection } from "../grafana/time-range";
import { DashboardVariableBar, useDashboardVariables } from "../components/DashboardVariables";
import { TimeRangePicker } from "../components/TimeRangePicker";
import { rangeFromURL, writeRangeURL } from "../grafana/time-range";
import BusinessInstanceDirectory, { environmentName } from "./BusinessInstanceDirectory";
import { instanceTitle, type BusinessInstance as Instance } from "./business-instance-directory-model";
import BusinessDataTable, { PopulationNotice, type Population } from "./BusinessDataTable";
import SlowSQL from "./SlowSQL";

import { useBusinessPanelSize } from "./use-business-panel-size";
import { localizeInstancePanel, instancePanelGroups } from "./business-instance-panels";
import { HostResources, type HostResourcesData, type HostAlertRule } from "./HostResources";
import "./business-instances.css";

const NativePanel = lazy(() => import("../grafana/GrafanaPanel"));
const text = (en: string, zh: string) => getLocale() === "zh" ? zh : en;
function localizedBuiltin(panel: Panel): Panel {
  return localizeInstancePanel(panel, getLocale());
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
type HostBinding = { version: number; kind: "" | "managed_host" | "collector"; id: string };
type InstanceHost = { binding: HostBinding; name: string; enabled: boolean; available: boolean; last_seen: number; interval_seconds: number; resources?: HostResourcesData; dashboard?: Dashboard; rules?: HostAlertRule[] };
type HostOption = { kind: "managed_host" | "collector"; id: string; name: string };
type Resolved = { instance: Instance; dashboard: Dashboard; process_dashboard?: Dashboard; population?: Population; host?: InstanceHost; host_error?: boolean };
type Props = { dashboards: Dashboard[]; open: (dashboard: Dashboard) => void; reload: () => Promise<void>; tick?: number };
export default function BusinessInstances({ dashboards, open, reload, tick = 0 }: Props) {
  const [selected, setSelected] = useState<string | null>(null);
  return <>
    <div hidden={selected !== null}><BusinessInstanceDirectory active={selected === null} select={setSelected} reload={reload} tick={tick} /></div>
    {selected && <BusinessInstanceDetail key={selected} id={selected} dashboards={dashboards} open={open} reload={reload} tick={tick} back={() => setSelected(null)} />}
  </>;
}

function BusinessInstanceDetail({ id, dashboards, open, reload, back, tick = 0 }: Props & { id: string; back: () => void }) {
  const { canEdit, canAdmin } = useSession();
  const [resolved, setResolved] = useState<Resolved | null>(null);
  const [record, setRecord] = useState<Instance | null>(null);
  const [error, setError] = useState("");
  const [revision, setRevision] = useState(0);
  const [busy, setBusy] = useState(false);
  const [template, setTemplate] = useState("");
  const [hostOptions, setHostOptions] = useState<HostOption[]>([]);
  const [hostChoice, setHostChoice] = useState("");
  const [settingsOpen, setSettingsOpen] = useState(false);
  const [large, setLarge] = useBusinessPanelSize();
  const [range, setRange] = useState<TimeSelection>(() => { try { return rangeFromURL() || "1h"; } catch { return "1h"; } });
  const changeRange = (value: TimeSelection) => { setRange(value); writeRangeURL(value); };
  const [tab, setTab] = useState<"overview" | "regions" | "sql">("overview");
  const mutation = useRef<AbortController | null>(null);
  const templateVersion = useRef("");
  useEffect(() => () => mutation.current?.abort(), []);
  useEffect(() => {
    if (mutation.current) return;
    const controller = new AbortController(); setError("");
    api<Instance>(`/business/instances/${id}`, { signal: controller.signal }).then(value => { if (!controller.signal.aborted) setRecord(value); }).catch(cause => { if (!controller.signal.aborted) setError(message(cause)); });
    api<Resolved>(`/business/instances/${id}/dashboard?language=${getLocale()}`, { signal: controller.signal })
      .then(value => { if (!controller.signal.aborted) { setResolved(value); const version = `${value.instance.binding.version}:${value.instance.binding.mode}:${value.instance.binding.dashboard_id}`; if (version !== templateVersion.current) { templateVersion.current = version; setTemplate(value.instance.binding.mode === "inherit" ? value.instance.binding.dashboard_id : ""); } } })
      .catch(cause => { if (!controller.signal.aborted) setError(message(cause)); });
    return () => controller.abort();
  }, [id, revision, tick]);
  async function bindHost() {
    if (!resolved?.host || !canAdmin || mutation.current) return;
    const controller = new AbortController(); mutation.current = controller; setBusy(true); setError("");
    const selected = hostOptions.find(option => `${option.kind}:${option.id}` === hostChoice);
    try {
      await api(`/business/instances/${id}/host`, { method: "PUT", signal: controller.signal, body: jsonBody({ version: resolved.host.binding.version, kind: selected?.kind || "", id: selected?.id || "" }) });
      if (!controller.signal.aborted) setRevision(value => value + 1);
    } catch (cause) { if (!controller.signal.aborted) setError(message(cause)); }
    finally { mutation.current = null; if (!controller.signal.aborted) setBusy(false); }
  }
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
    <div className="business-detail-topline"><Button fill="text" onClick={back}><ArrowLeft size={15} />{text("All instances", "全部实例")}</Button><div><Button disabled={busy} onClick={() => setRevision(value => value + 1)}><RefreshCw size={15} />{text("Refresh", "刷新")}</Button><Button aria-expanded={settingsOpen} aria-controls="business-instance-settings" onClick={() => {
      if (!settingsOpen && canAdmin) { const binding = resolved?.host?.binding; setHostChoice(binding?.kind ? `${binding.kind}:${binding.id}` : ""); void api<HostOption[]>("/business/host-options").then(setHostOptions).catch(cause => setError(message(cause))); }
      setSettingsOpen(value => !value);
    }}><Settings2 size={15} />{text("Dashboard settings", "仪表盘设置")}</Button></div></div>
    {error && <Alert severity="error" title={text("Instance operation failed", "实例操作失败")}>{error}</Alert>}
    {error && item && !dashboard && canEdit && item.binding.mode !== "default" && <Button disabled={busy} onClick={() => void bind("default", "")}>{text("Reset built-in view", "恢复内置视图")}</Button>}
    {!resolved && !error && <p role="status">{text("Loading instance…", "正在加载实例…")}</p>}
    {resolved && item && dashboard && <><header className="business-detail-heading"><div><h1>{instanceTitle(item.identity, getLocale() === "zh")}</h1><p>{item.identity.service} · {item.identity.instance}</p></div><div className="business-detail-meta"><span>{environmentName(item.identity.environment, true)} · {item.identity.channel || "—"}</span><span><Server size={15} />{item.reported_ip || item.observed_ip || "—"}</span></div></header><details className="business-identity-details"><summary>{text("Instance identity and source IPs", "实例身份与来源 IP")}</summary><dl className="business-instance-scope">{Object.entries(item.identity).map(([name, value]) => <div key={name}><dt>{name}</dt><dd>{value || text("Not reported", "未上报")}</dd></div>)}<div><dt>{text("Reported host IP (advisory)", "上报宿主 IP（标签）")}</dt><dd>{item.reported_ip || "—"}</dd></div><div><dt>{text("Canonical observed source IP", "服务端规范来源 IP")}</dt><dd>{item.observed_ip || "—"}</dd></div><div><dt>{text("Last report", "最近上报")}</dt><dd>{timeAgo(item.last_seen)}</dd></div></dl>
      <p className="subtle">{text("The source IP may be a NAT/proxy address. Labels and fixed query variables are display/query scope, never authorization boundaries.", "来源 IP 可能为 NAT 或代理出口。标签和固定变量只限定展示查询，不是权限隔离。")}</p></details>
      {settingsOpen && <section className="business-instance-binding" id="business-instance-settings" aria-label={text("Dashboard settings", "仪表盘设置")}><h2>{text("Dashboard settings", "仪表盘设置")}<span>{mode === "inherit" ? text("Live template", "继承模板") : mode === "independent" ? text("Custom panels", "自定义面板") : text("Built-in panels", "内置面板")}</span></h2>
      {canAdmin && resolved.host && <div className="business-host-association"><Field label={text("Physical host", "所属物理主机")}><select aria-label={text("Physical host", "所属物理主机")} value={hostChoice} onChange={event => setHostChoice(event.target.value)}><option value="">{text("Not associated", "暂不关联")}</option>{hostOptions.map(option => <option key={`${option.kind}:${option.id}`} value={`${option.kind}:${option.id}`}>{option.name} · {option.kind === "managed_host" ? text("Host agent", "主机采集器") : text("Node exporter", "主机指标采集")}</option>)}</select></Field><Button disabled={busy || !hostOptions.length} onClick={() => void bindHost()}>{text("Save host association", "保存主机关联")}</Button><p className="subtle">{text("Choose the actual machine running this instance. Shared egress IPs do not identify a host.", "请选择实际运行此实例的机器，共用公网出口不代表同一台主机。")}</p></div>}
      {canEdit && <div className="business-instance-actions"><Button disabled={busy} onClick={() => void copy()}><Copy size={15} />{text("Save independent copy", "另存独立副本")}</Button>{mode === "independent" && <Button disabled={busy} onClick={() => open(dashboard)}>{text("Edit independent dashboard", "编辑独立仪表盘")}</Button>}{mode === "inherit" && <Button disabled={busy} onClick={() => { const source = dashboards.find(value => value.id === item.binding.dashboard_id); if (source) open(source); }}>{text("Edit shared template", "编辑共享模板")}</Button>}<details><summary>{text("Template binding", "模板绑定")}</summary><p className="subtle">{text("Inheritance follows the latest template. Save a copy for independent panel edits.", "继承会跟随模板最新版，另存副本后可独立编辑图表。")}</p><Field label={text("Native dashboard template", "原生仪表盘模板")}><select value={template} aria-label={text("Native dashboard template", "原生仪表盘模板")} onChange={event => setTemplate(event.target.value)}><option value="">{text("Choose a dashboard", "选择仪表盘")}</option>{dashboards.filter(value => !value.grafana).map(value => <option key={value.id} value={value.id}>{value.name}</option>)}</select></Field><Button disabled={busy || !template} onClick={() => void bind("inherit", template)}>{text("Use live template", "使用实时模板")}</Button><Button disabled={busy || mode === "default"} onClick={() => void bind("default", "")}>{text("Reset built-in view", "恢复内置视图")}</Button></details></div>}</section>}
      <div className="business-detail-navigation"><div className="business-instance-tabs" role="group" aria-label={text("Instance sections", "实例区块")}>{([ ["overview", text("Overview", "总览")], ["regions", text("Player regions · all", "玩家地区 · 全部")], ["sql", text("SQL diagnostics", "SQL 诊断")]] as const).filter(([key]) => (item.identity.role !== "gateway" || key === "overview") && (item.identity.role !== "sdk" || key !== "regions")).map(([key, label]) => <Button key={key} aria-pressed={tab === key} onClick={() => setTab(key)}>{label}</Button>)}</div>{tab === "overview" && <div className="business-range-controls"><TimeRangePicker value={range} onChange={changeRange} /><label className="business-panel-size"><span>{text("Panels", "面板大小")}</span><select aria-label={text("Panel size", "面板大小")} value={large ? "large" : "normal"} onChange={event => setLarge(event.target.value === "large")}><option value="normal">{text("Comfortable", "舒适")}</option><option value="large">{text("Large", "大面板")}</option></select></label></div>}</div>
      {Date.now() - item.last_seen > 90000 && <Alert severity="warning" title={text("Instance reporting is stale", "实例上报已超过90秒")}>
        {text("The last samples may belong to an earlier process. Historical curves do not establish current server availability.", `最近上报：${timeAgo(item.last_seen)}。当前值等待新采样，历史曲线不代表服务仍在线，请检查上报链路。`)}
      </Alert>}
      {tab === "overview" && <PopulationNotice population={resolved?.population} />}
      {tab === "overview" && <><section className="business-resource-section"><div className="business-resource-heading"><h3>{text("Physical host", "整机资源")}{resolved.host?.name ? ` · ${resolved.host.name}` : ""}</h3><span className="subtle">{text("Shared by processes on this machine", "同一主机上的服务共享这些资源")}</span></div>
        {resolved.host?.available && resolved.host.binding.kind === "collector" && <p className="subtle">{text("Host collector's last observation", "整机采集最近观测")}：{timeAgo(resolved.host.last_seen)}{!resolved.host.enabled || Date.now() - resolved.host.last_seen > Math.max(90000, resolved.host.interval_seconds * 3000) ? text(" · paused or stale; current gauges are hidden, historical curves remain", " · 采集暂停或数据过期，隐藏当前仪表，只保留历史趋势") : ""}</p>}
        {resolved.host_error ? <p role="status">{text("Host data is temporarily unavailable. Process metrics remain below.", "主机资源暂时读取失败，下方保留进程指标。")}</p> : resolved.host?.available ? resolved.host.binding.kind === "managed_host" ? <HostResources compact large={large} id={resolved.host.binding.id} resources={resolved.host.resources} enabled={resolved.host.enabled} interval={resolved.host.interval_seconds} rules={resolved.host.rules || []} rulesState="ready" /> : resolved.host.dashboard ? <InstanceNativePanels dashboard={!resolved.host.enabled || Date.now() - resolved.host.last_seen > Math.max(90000, resolved.host.interval_seconds * 3000) ? { ...resolved.host.dashboard, panels: resolved.host.dashboard.panels.filter(panel => panel.visualization === "timeseries") } : resolved.host.dashboard} builtin={false} hostView large={large} range={range} setRange={changeRange} tick={revision + tick} /> : null : <p className="business-host-empty">{text("Associate the machine in Dashboard settings to see its CPU, memory and disk. Process resource usage is shown below.", "在“仪表盘设置”关联实际主机后，这里显示整机 CPU、内存和磁盘。下方是当前服务进程的资源占用。")}</p>}</section>
        {resolved.process_dashboard && <InstanceNativePanels dashboard={resolved.process_dashboard} builtin={true} large={large} range={range} setRange={changeRange} tick={revision + tick} />}
        <InstanceNativePanels key={dashboard.id} dashboard={dashboard} builtin={mode === "default"} large={large} range={range} setRange={changeRange} tick={revision + tick} /></>}
      {tab !== "overview" && <BusinessDataTable key={`${id}:${tab}`} id={id} kind={tab} tick={tick} refresh={revision} />}
      {tab === "sql" && <SlowSQL id={id} tick={tick + revision} />}

    </>}
  </section>;
}

function InstanceNativePanels({ dashboard, builtin, range, setRange, tick, large, hostView = false }: { dashboard: Dashboard; builtin: boolean; range: TimeSelection; setRange: (range: TimeSelection) => void; tick: number; large: boolean; hostView?: boolean }) {
  const state = useDashboardVariables(dashboard, range, tick);
  const renderPanel = (panel: Panel) => <NativePanel key={panel.id} panel={nativePanelForSDK(builtin ? localizedBuiltin(panel) : panel)} values={state.effective} range={range} onRange={setRange} tick={tick} dashboard={builtin ? undefined : dashboard} />;
  const hasVariables = state.variables.some(variable => variable.type !== "constant" && variable.config?.hide !== 2 && variable.config?.hide !== "hideVariable");
  return <>{hasVariables && <div className="business-panel-controls"><DashboardVariableBar state={state} /></div>}{state.error && <Alert severity="error" title={text("Variable resolution failed", "变量解析失败")}>{state.error}</Alert>}{state.ready ? <Suspense fallback={<p role="status">{text("Loading panels…", "正在加载图表…")}</p>}>{instancePanelGroups(dashboard.panels, builtin).map(group => <section className={`business-resource-section${hostView ? " business-host-panels" : ""}`} key={group.id}>
    {group.id !== "custom" && <div className="business-resource-heading"><h3>{group.id === "process" ? text("Server process", "服务进程") : text("Business metrics", "业务指标")}</h3><span className="subtle">{group.id === "process" ? text("This instance only", "仅当前游戏服或网关实例") : text("Actual reported values", "实际上报的业务数据")}</span></div>}
    <div className={`business-instance-stats${large ? " business-instance-panels-large" : ""}`}>{group.panels.filter(panel => panel.visualization === "stat").map(renderPanel)}</div>
    <div className={`business-instance-gauges${large ? " business-instance-panels-large" : ""}`}>{group.panels.filter(panel => panel.visualization === "gauge").map(renderPanel)}</div><div className={`business-instance-panels${large ? " business-instance-panels-large" : ""}`}>{group.panels.filter(panel => panel.visualization !== "stat" && panel.visualization !== "gauge").map(renderPanel)}</div>
  </section>)}</Suspense> : <p role="status">{text("Resolving variables…", "正在解析变量…")}</p>}</>;
}
