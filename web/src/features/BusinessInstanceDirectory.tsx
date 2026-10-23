import { useEffect, useState } from "react";
import { ArrowDown, ArrowLeft, ArrowRight, ArrowUp, Box, Gamepad2, Info, Network, Pencil, RefreshCw, Search, Server } from "lucide-react";
import { api, formatValue, message } from "../api";
import { formatTimestamp } from "../display-format";
import { getLocale } from "../i18n";
import { Alert, Button, Input } from "../ui";
import { useSession } from "../ui/session";
import BusinessNoteEditor from "./BusinessNoteEditor";
import { instanceTitle, reportFresh, type BusinessInstance, type DirectorySort, type InstancePage, type InstanceSummary } from "./business-instance-directory-model";
import "./business-instance-cards.css";

const text = (en: string, zh: string) => getLocale() === "zh" ? zh : en;
export function environmentName(value: string, short = false): string {
  const names: Record<string, [string, string]> = { release: ["Production", "正式"], test: ["Test", "测试"], intranet: ["Intranet", "内网"], shared: ["Shared", "共享"] };
  const name = names[value]; return name ? text(name[0], name[1] + (short ? "" : "环境")) : value || text("Unspecified", "未标明环境");
}
const channelName = (value: string) => value === "wechat" ? text("WeChat", "微信") : value === "taptap" ? "TapTap" : value || "—";
const percent = (value: number | null | undefined) => value == null ? "—" : value.toFixed(1) + "%";

function ResourceMeter({ label, value, warning, title }: { label: string; value?: number | null; warning: number; title?: string }) {
  const risk = value == null ? "unknown" : value >= 95 ? "critical" : value >= warning ? "warning" : "safe";
  return <div className={"business-card-meter risk-" + risk} title={title}><div><span>{label}</span><strong>{percent(value)}</strong></div><progress aria-label={label} max={100} value={value == null ? 0 : Math.min(100, Math.max(0, value))} />{value == null && <small>{text("Awaiting sample", "等待采样")}</small>}</div>;
}

function InstanceCard({ item, summary, select, edit, canEdit }: { item: BusinessInstance; summary?: InstanceSummary; select: (id: string) => void; edit: (item: BusinessInstance) => void; canEdit: boolean }) {
  const title = instanceTitle(item.identity, getLocale() === "zh");
  const Icon = item.identity.role === "gateway" ? Network : item.identity.role === "sdk" ? Box : Gamepad2;
  const fresh = reportFresh(item.last_seen);
  const host = summary?.host_expires_at && summary.host_expires_at >= Date.now() ? summary : undefined;
  const memoryDetail = summary?.memory_total ? text("Available", "可用") + " " + formatValue(summary.memory_available ?? 0, "bytes") + " / " + formatValue(summary.memory_total, "bytes") : undefined;
  const trafficValue = (value: number | null | undefined) => value == null ? "—" : formatValue(value, "Bps");
  return <article className="business-instance-card" aria-label={title}>
    <button className="business-card-open" onClick={() => select(item.id)} aria-label={text("Open ", "打开 ") + title + " " + environmentName(item.identity.environment, true) + " " + channelName(item.identity.channel)}>
      <Icon className="business-card-icon" size={28} strokeWidth={1.7} /><span className="business-card-identity"><strong>{title}</strong><span title={item.identity.service + " / " + item.identity.instance + " / " + (item.reported_ip || item.observed_ip || "—")}>{item.identity.instance} <span className="business-card-address"> | {item.reported_ip || item.observed_ip || "—"}</span></span></span><ArrowRight size={17} className="business-card-arrow" />
    </button>
    <div className="business-card-tags"><span className={"business-env env-" + (["release", "test", "intranet", "shared"].includes(item.identity.environment) ? item.identity.environment : "custom")}><i />{environmentName(item.identity.environment, true)}</span><span>{channelName(item.identity.channel)}</span><span>{item.identity.implementation || "—"}</span></div>
    {canEdit ? <button className="business-card-note" onClick={() => edit(item)} aria-label={text("Edit note for ", "编辑 ") + title + text("", " 备注")}><Pencil size={14} /><span>{item.note?.description || text("Add note", "添加备注")}</span></button> : <p className="business-card-note"><span>{item.note?.description || text("No note", "暂无备注")}</span></p>}
    <div className="business-card-resource-title"><span>{text("Host resources", "主机资源")}</span>{summary?.host_name && <span title={summary.host_name}>{summary.host_name}</span>}</div>
    <div className="business-card-meters"><ResourceMeter label="CPU" value={host?.cpu_percent} warning={85} /><ResourceMeter label={text("Memory", "内存")} value={host?.memory_percent} warning={90} title={memoryDetail} /><ResourceMeter label={text("Disk", "磁盘")} value={host?.disk_percent} warning={85} title={text("Busiest writable partition", "最满可写分区")} /></div>
    <div className="business-card-traffic"><span>{summary?.traffic_scope === "http" ? text("HTTP body traffic", "HTTP 请求体流量") : text("Service traffic", "业务流量")}</span><div><span><ArrowDown size={17} /><small>{text("Receive", "下行")}</small><strong>{trafficValue(fresh ? summary?.receive_per_second : null)}</strong></span><span><ArrowUp size={17} /><small>{text("Send", "上行")}</small><strong>{trafficValue(fresh ? summary?.transmit_per_second : null)}</strong></span></div></div>
    <footer className={fresh ? "report-fresh" : "report-old"}><i /><span>{text("Last report", "最近上报")}</span><time dateTime={item.last_seen ? new Date(item.last_seen).toISOString() : undefined}>{formatTimestamp(item.last_seen || undefined)}</time></footer>
  </article>;
}

export default function BusinessInstanceDirectory({ active, select, reload, tick = 0 }: { active: boolean; select: (id: string) => void; reload: () => Promise<void>; tick?: number }) {
  const { canEdit } = useSession();
  const [cursor, setCursor] = useState(""); const [previous, setPrevious] = useState<string[]>([]);
  const [revision, setRevision] = useState(0); const [page, setPage] = useState<InstancePage | null>(null);
  const [error, setError] = useState(""); const [loading, setLoading] = useState(false);
  const [search, setSearch] = useState(""); const [query, setQuery] = useState("");
  const [environment, setEnvironment] = useState(""); const [role, setRole] = useState("all");
  const [sort, setSort] = useState<DirectorySort>("server"); const [editing, setEditing] = useState<BusinessInstance | null>(null);
  const scope = JSON.stringify([cursor, query, environment, role, sort]);
  const [pageScope, setPageScope] = useState("");
  const displayed = pageScope === scope ? page : null;
  const resetPage = () => { setCursor(""); setPrevious([]); };
  useEffect(() => {
    if (!active) return;
    const controller = new AbortController(); setLoading(true);
    const params = new URLSearchParams({ limit: "24", cursor, q: query, environment, role, sort });
    void api<InstancePage>("/business/directory?" + params, { signal: controller.signal })
      .then(value => { if (!controller.signal.aborted) { setPage(value); setPageScope(scope); setError(""); } })
      .catch(cause => { if (!controller.signal.aborted) setError(message(cause)); })
      .finally(() => { if (!controller.signal.aborted) setLoading(false); });
    return () => controller.abort();
  }, [active, cursor, revision, query, environment, role, sort, tick]);
  const environments = [...new Set(["release", "test", "intranet", "shared", ...(page?.environments || [])])];
  return <section className="business-instance-directory" aria-label={text("Business instances", "业务实例")} aria-busy={loading}>
    <header className="business-directory-heading"><div className="business-title-line"><h1>{text("Dashboards", "仪表盘")}</h1><span>{text("Instances", "实例")} {displayed?.total ?? "—"}</span></div><Button className="business-refresh" disabled={loading} onClick={() => { setRevision(value => value + 1); void reload().catch(cause => setError(message(cause))); }}><RefreshCw size={16} />{text("Refresh", "刷新")}</Button></header>
    {error && <Alert severity="error" title={text("Refresh failed", "更新失败")}>{error}<Button fill="text" onClick={() => { resetPage(); setRevision(value => value + 1); }}>{text("Reload results", "重新读取结果")}</Button></Alert>}
    {displayed?.at_capacity && <Alert severity="warning" title={text("Instance capacity reached", "实例观测容量已满")}>{text("Existing identities continue to update.", "现有身份继续更新，暂不登记新身份。")}</Alert>}
    <div className="business-directory-tools"><form className="business-directory-search" onSubmit={event => { event.preventDefault(); resetPage(); setQuery(search.trim()); }}><div className="business-search-input"><Search size={18} aria-hidden="true" /><Input aria-label={text("Search all instances", "搜索全部实例")} type="search" maxLength={128} value={search} onChange={event => { setSearch(event.target.value); if (!event.target.value) { resetPage(); setQuery(""); } }} placeholder={text("Service, serverId, IP or note", "搜索服务、serverId、IP 或备注")} /></div><Button type="submit" variant="primary">{text("Search", "搜索")}</Button></form>
      <label className="business-directory-sort"><span>{text("Sort", "排序")}</span><select aria-label={text("Sort all results", "全部结果排序")} value={sort} onChange={event => { resetPage(); setSort(event.target.value as DirectorySort); }}><option value="server">{text("Server ID", "服务编号")}</option><option value="recent">{text("Latest report", "最近上报")}</option><option value="host">{text("Host IP", "主机 IP")}</option></select></label>
      <label className="business-directory-environment"><span>{text("Environment", "环境")}</span><select aria-label={text("Environment", "环境")} value={environment} onChange={event => { resetPage(); setEnvironment(event.target.value); }}><option value="">{text("All environments", "全部环境")}</option>{environments.filter(Boolean).map(env => <option key={env} value={env}>{environmentName(env)}</option>)}</select></label>
    </div>
    <div className="business-environment-filter business-role-tabs" role="group" aria-label={text("Service categories", "服务分类")}>{[["all", text("All", "全部")], ["game", text("Game servers", "游戏服")], ["gateway", text("Gateways", "网关")], ["sdk", "SDK"]].map(([value, label]) => <button key={value} aria-pressed={role === value} onClick={() => { resetPage(); setRole(value); }}>{label}<span>{displayed?.counts?.[value] ?? "—"}</span></button>)}</div>
    {!displayed && loading && <p className="business-directory-empty" role="status">{text("Loading instances…", "正在加载实例…")}</p>}
    <div className="business-card-grid">{displayed?.items.map(item => <InstanceCard key={item.id} item={item} summary={displayed.summaries?.[item.id]} select={select} edit={setEditing} canEdit={canEdit} />)}</div>
    {displayed && !displayed.items.length && <div className="business-directory-empty" role="status"><Server size={26} /><p>{text("No matching instances.", "没有匹配的实例。")}</p><Button onClick={() => { setSearch(""); setQuery(""); setEnvironment(""); setRole("all"); resetPage(); }}>{text("Show all", "查看全部实例")}</Button></div>}
    <footer className="business-directory-footer"><p><Info size={15} />{text("Last report indicates received data, not service health.", "最近上报表示收到数据，不代表服务健康。")}</p>{previous.length > 0 || displayed?.next_cursor ? <div className="business-directory-paging"><Button size="sm" disabled={!previous.length || loading} onClick={() => { const before = [...previous]; setCursor(before.pop() || ""); setPrevious(before); }}><ArrowLeft size={15} />{text("Previous", "上一页")}</Button><span>{text("Page ", "第 ") + (previous.length + 1)} · {displayed?.matched ?? "—"} {text("results", "个结果")}</span><Button size="sm" disabled={!displayed?.next_cursor || loading} onClick={() => { setCursor(displayed!.next_cursor); setPrevious([...previous, cursor]); }}>{text("Next", "下一页")}<ArrowRight size={15} /></Button></div> : <span>{displayed?.matched ?? 0} {text("results", "个结果")}</span>}</footer>
    {editing && <BusinessNoteEditor item={editing} close={() => setEditing(null)} saved={note => { setPage(value => value ? { ...value, items: value.items.map(item => item.id === editing.id ? { ...item, note } : item) } : value); setRevision(value => value + 1); }} />}
  </section>;
}
