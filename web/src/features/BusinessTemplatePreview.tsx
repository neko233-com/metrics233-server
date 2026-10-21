import { lazy, Suspense, useEffect, useState, type ReactNode } from "react";
import { api, ApiError, message, timeAgo, type Dashboard } from "../api";
import { getLocale } from "../i18n";
import { Alert, Button, Field } from "../ui";
import { useSession } from "../ui/session";
import { TimeRangePicker } from "../components/TimeRangePicker";
import { RefreshPicker } from "../components/RefreshPicker";
import { DashboardVariableBar, useDashboardVariables } from "../components/DashboardVariables";
import { layoutPanels } from "../grafana/layout";
import { nativePanelForSDK } from "../grafana/panel-queries";
import { queryValues } from "../grafana/variables";
import type { TimeSelection } from "../grafana/time-range";
import { boundTemplateInstances, isResolvedTemplateInstance, templateSelection, type ResolvedTemplateInstance, type TemplateInstancePage } from "./business-template-model";
import BusinessDataTable from "./BusinessDataTable";
import { useBusinessPanelSize } from "./use-business-panel-size";
import "./business-instances.css";

const NativePanel = lazy(() => import("../grafana/GrafanaPanel"));
const text = (en: string, zh: string) => getLocale() === "zh" ? zh : en;

export default function BusinessTemplatePreview({ dashboard, range, onRange, tick, refreshChoice, onRefreshChoice, refresh, children }: {
  dashboard: Dashboard; range: TimeSelection; onRange: (range: TimeSelection) => void; tick: number;
  refreshChoice: string; onRefreshChoice: (value: string) => void; refresh: () => void; children: ReactNode;
}) {
  const { canEdit } = useSession();
  const [editing, setEditing] = useState(false);
  const [large, setLarge] = useBusinessPanelSize();
  const [tab, setTab] = useState<"overview" | "regions" | "sql">("overview");
  const [revision, setRevision] = useState(0);
  const [cursor, setCursor] = useState("");
  const [previous, setPrevious] = useState<string[]>([]);
  const [page, setPage] = useState<TemplateInstancePage | null>(null);
  const [pageCursor, setPageCursor] = useState("");
  const [selected, setSelected] = useState("");
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [resolved, setResolved] = useState<ResolvedTemplateInstance | null>(null);
  const [resolving, setResolving] = useState(false);
  const [resolveError, setResolveError] = useState("");
  useEffect(() => {
    const controller = new AbortController(); setLoading(true); setError("");
    api<TemplateInstancePage>(`/business/instances?template_id=${encodeURIComponent(dashboard.id)}&limit=100&cursor=${encodeURIComponent(cursor)}`, { signal: controller.signal })
      .then(value => {
        if (controller.signal.aborted) return;
        const items = boundTemplateInstances(value.items, dashboard.id);
        setPage({ ...value, items });
        setPageCursor(cursor);
        setSelected(before => templateSelection(items, value.next_cursor, cursor, before));
      })
      .catch(cause => { if (!controller.signal.aborted) { if (cause instanceof ApiError && [403, 404].includes(cause.status)) { setPage(null); setSelected(""); setResolved(null); } setError(message(cause)); } })
      .finally(() => { if (!controller.signal.aborted) setLoading(false); });
    return () => controller.abort();
  }, [dashboard.id, cursor, revision]);
  useEffect(() => {
    if (!selected || editing) return;
    const controller = new AbortController(); setResolving(true); setResolveError("");
    api<ResolvedTemplateInstance>(`/business/instances/${encodeURIComponent(selected)}/dashboard`, { signal: controller.signal })
      .then(value => {
        if (controller.signal.aborted) return;
        if (!isResolvedTemplateInstance(value, selected, dashboard.id)) { setResolved(null); throw new Error(text("This instance no longer inherits this template. Refresh the instance list.", "该实例已不再继承此模板，请刷新实例列表。")); }
        setResolved(value);
      })
      .catch(cause => { if (!controller.signal.aborted) { if (cause instanceof ApiError && [403, 404].includes(cause.status)) setResolved(null); setResolveError(message(cause)); } })
      .finally(() => { if (!controller.signal.aborted) setResolving(false); });
    return () => controller.abort();
  }, [selected, dashboard.id, dashboard.updated_at, revision, tick, editing]);
  const items = page?.items || [];
  const active = pageCursor === cursor && items.some(item => item.id === selected) && resolved &&
    isResolvedTemplateInstance(resolved, selected, dashboard.id) ? resolved : null;
  return <>
    <section className="business-template-preview" aria-label={text("Business template preview", "业务模板预览")}>
      {!editing && <div className="page-heading dashboard-heading"><div><h1>{dashboard.name}</h1><p>{text("Shared business template · instance preview", "共享业务模板 · 实例预览")}</p></div><div className="toolbar"><TimeRangePicker value={range} onChange={onRange} /><RefreshPicker value={refreshChoice} onChange={onRefreshChoice} /><Button onClick={() => { setRevision(value => value + 1); refresh(); }}>{text("Refresh metrics", "刷新指标")}</Button></div></div>}
      <Alert severity="info" title={text("Shared template", "共享模板")}>{text("Select an instance to preview. Template edits apply to all inheriting instances.", "选择实例预览；修改模板会同步到所有继承实例。")}</Alert>
      <div className="business-instance-toolbar">
        <Field label={text("Preview instance", "预览实例")}><select aria-label={text("Preview instance", "预览实例")} value={selected} disabled={loading || editing} onChange={event => { setSelected(event.target.value); setTab("overview"); }}><option value="">{text("Choose a bound instance", "选择已绑定实例")}</option>{items.map(item => <option key={item.id} value={item.id}>{[item.identity.service, item.identity.role, item.identity.serverId, item.identity.environment || "—", item.identity.channel || "—", item.identity.instance, item.identity.implementation || "—"].join(" · ")}</option>)}</select></Field>
        <Button disabled={loading} onClick={() => setRevision(value => value + 1)}>{text("Refresh bound instances", "刷新绑定实例")}</Button>
        {canEdit && <Button aria-pressed={editing} onClick={() => setEditing(value => !value)}>{editing ? text("Return to instance preview", "返回实例预览") : text("Edit shared template", "编辑共享模板")}</Button>}
      </div>
      {error && <Alert severity="error" title={text("Cannot load bound instances", "无法加载绑定实例")}>{error}</Alert>}
      {loading && !page && <p role="status">{text("Loading bound instances…", "正在加载绑定实例…")}</p>}
      {!loading && !error && !items.length && <p role="status">{text("No bound instances. Bind a reported instance from Dashboards.", "尚无绑定实例。请前往“仪表盘”为真实上报实例绑定模板。")}</p>}
      {!loading && items.length > 1 && !selected && !editing && <p role="status">{text("Select an instance to display its panels. Instances in different environments are kept separate.", "请选择一个实例查看面板；不同环境的实例保持独立。")}</p>}
      {(cursor || page?.next_cursor) && <div className="business-instance-toolbar"><Button disabled={loading || !previous.length || editing} onClick={() => { const before = [...previous]; setCursor(before.pop() || ""); setPrevious(before); }}>{text("Previous", "上一页")}</Button><Button disabled={loading || !page?.next_cursor || editing} onClick={() => { setPrevious(before => [...before, cursor]); setCursor(page!.next_cursor); }}>{text("Next", "下一页")}</Button></div>}
      {!editing && resolveError && <Alert severity="error" title={text("Cannot preview instance", "无法预览实例")}>{resolveError}</Alert>}
      {!editing && !active && selected && resolving && <p role="status">{text("Loading instance panels…", "正在加载实例面板…")}</p>}
      {!editing && active && <><h2>{text("Instance preview", "实例预览")} · {active.instance.identity.service} · {active.instance.identity.role} · {active.instance.identity.serverId}</h2><p className="subtle">{active.instance.identity.environment || "—"} / {active.instance.identity.channel || "—"} · {active.instance.identity.instance} · {active.instance.identity.implementation || "—"} · {text("Last report", "最近上报")}：{timeAgo(active.instance.last_seen)}</p><details><summary>{text("Full instance identity", "完整实例身份")}</summary><dl className="business-instance-scope">{Object.entries(active.instance.identity).map(([name, value]) => <div key={name}><dt>{name}</dt><dd>{value || text("Not reported", "未上报")}</dd></div>)}</dl></details><div className="business-instance-tabs" role="group" aria-label={text("Instance sections", "实例区块")}>{([["overview", text("Overview", "概览")], ["regions", text("Player regions · all", "玩家地区 · 全部")], ["sql", text("Slow SQL Top10", "慢 SQL Top10")]] as const).map(([key, label]) => <Button key={key} aria-pressed={tab === key} onClick={() => setTab(key)}>{label}</Button>)}{tab === "overview" && <Field label={text("Panel size", "面板大小")}><select aria-label={text("Panel size", "面板大小")} value={large ? "large" : "normal"} onChange={event => setLarge(event.target.value === "large")}><option value="normal">{text("Template layout", "模板布局")}</option><option value="large">{text("Large", "大面板")}</option></select></Field>}</div></>}
    </section>
    {editing ? children : active ? tab === "overview" ? <TemplatePanels key={active.instance.id} dashboard={active.dashboard} range={range} onRange={onRange} tick={tick} large={large} /> : <BusinessDataTable key={`${active.instance.id}:${tab}`} id={active.instance.id} kind={tab} tick={tick} refresh={revision} /> : null}
  </>;
}

function TemplatePanels({ dashboard, range, onRange, tick, large }: { dashboard: Dashboard; range: TimeSelection; onRange: (range: TimeSelection) => void; tick: number; large: boolean }) {
  const state = useDashboardVariables(dashboard, range, tick);
  const options = Object.fromEntries(Object.entries(state.choices).map(([name, choices]) => [name, choices.map(choice => choice.value)]));
  const entries = layoutPanels(dashboard.panels, state.values, options);
  return <><DashboardVariableBar state={state} />{state.error && <Alert severity="error" title={text("Variable resolution failed", "变量解析失败")}>{state.error}</Alert>}<div className="chart-grid dashboard-panel-grid"><Suspense fallback={<p role="status">{text("Loading panels…", "正在加载图表…")}</p>}>{state.ready && entries.map(({ panel, values, x, y, w, h }) => <div className="dashboard-panel-cell" key={panel.id} data-panel-id={panel.id} style={{ gridColumn: large ? "1 / -1" : `${x + 1} / span ${w}`, gridRow: large ? "auto / span 14" : `${y + 1} / span ${h}` }}><NativePanel panel={nativePanelForSDK(panel)} values={queryValues(values, state.variables, state.choices)} range={range} onRange={onRange} tick={tick} dashboard={dashboard} /></div>)}</Suspense></div></>;
}
