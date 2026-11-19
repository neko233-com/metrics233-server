import { useEffect, useLayoutEffect, useRef, useState, type ReactNode } from "react";
import { ArrowDown, ArrowLeft, ArrowRight, ArrowUp, Box, ChevronRight, Gamepad2, Info, Network, Pencil, RefreshCw, Search, Server } from "lucide-react";
import { api, formatValue, message } from "../api";
import { formatTimestamp } from "../display-format";
import { getLocale } from "../i18n";
import { useObservationClock } from "../observation-clock";
import { Alert, Button, Input } from "../ui";
import { useSession } from "../ui/session";
import BusinessNoteEditor from "./BusinessNoteEditor";
import { pinnedPlatforms, PlatformBadge, platformName } from "./BusinessPlatform";
import { instanceTitle, reportFresh, type BusinessInstance, type DirectorySort, type InstancePage, type InstanceSummary, type ReportState } from "./business-instance-directory-model";
import "./business-instance-cards.css";

const text = (en: string, zh: string) => getLocale() === "zh" ? zh : en;
const defaultEnvironments = ["release", "test", "audit", "intranet", "shared"];
const serviceRoles = [
  ["all", "All servers", "全部服务器"],
  ["game", "Game servers", "游戏服"],
  ["gateway", "Gateways", "网关"],
  ["center", "Control servers", "中心服"],
  ["sdk", "SDK services", "SDK 服务"],
] as const;

export function environmentName(value: string, short = false): string {
  const names: Record<string, [string, string]> = {
    release: ["Production", "正式"], production: ["Production", "正式"],
    test: ["Test", "测试"], audit: ["Review", "提审"],
    intranet: ["Intranet", "内网"], shared: ["Shared", "共享"],
  };
  const name = Object.hasOwn(names, value) ? names[value] : undefined;
  return name ? text(name[0], name[1] + (short ? "" : "环境")) : value || text("Unspecified", "未标明环境");
}

function ResourceMeter({ label, value, warning, title }: { label: string; value?: number | null; warning: number; title?: string }) {
  const risk = value == null ? "unknown" : value >= 95 ? "critical" : value >= warning ? "warning" : "safe";
  return <div className={"business-card-meter risk-" + risk} title={title}>
    <span>{label}</span>
    <strong title={value == null ? text("Awaiting sample", "等待采样") : undefined}>{value == null ? "—" : value.toFixed(1) + "%"}</strong>
    {value == null ? <span className="business-meter-track" aria-hidden="true" /> :
      <progress aria-label={label} max={100} value={Math.min(100, Math.max(0, value))} />}
  </div>;
}

function InstanceRow({ item, summary, select, edit, canEdit, now }: {
  item: BusinessInstance; summary?: InstanceSummary; select: (id: string) => void;
  edit: (item: BusinessInstance) => void; canEdit: boolean; now: number;
}) {
  const { identity } = item;
  const title = instanceTitle(identity, getLocale() === "zh");
  const Icon = identity.role === "gateway" ? Network : identity.role === "sdk" ? Box : identity.role === "center" ? Server : Gamepad2;
  const fresh = reportFresh(item.last_seen, now);
  const host = summary?.host_expires_at && summary.host_expires_at >= now ? summary : undefined;
  const memoryDetail = host?.memory_total
    ? text("Available", "可用") + " " + formatValue(host.memory_available ?? 0, "bytes") + " / " + formatValue(host.memory_total, "bytes") : undefined;
  const trafficValue = (value: number | null | undefined) => fresh && value != null ? formatValue(value, "Bps") : "—";
  const address = item.reported_ip || item.observed_ip;
  const note = item.note?.description;
  const environment = environmentName(identity.environment, true);
  const trafficScope = summary?.traffic_scope === "http" ? text("HTTP body traffic", "HTTP 请求体流量") : text("Service traffic", "业务流量");

  return <article className="business-instance-card" aria-label={title}>
    <div className="business-row-identity">
      <button className="business-card-open" onClick={() => select(item.id)}
        aria-label={text("Open ", "打开 ") + title + " " + environment + " " + platformName(item.platform)}>
        <span className="business-card-icon"><Icon size={19} strokeWidth={1.8} aria-hidden="true" /></span>
        <span className="business-card-identity">
          <strong>{title}</strong>
          <span title={[identity.service, identity.instance, address].filter(Boolean).join(" · ")}>
            {identity.instance}{address && <span className="business-card-address"> · {address}</span>}
          </span>
        </span>
        <ChevronRight size={14} className="business-card-arrow" aria-hidden="true" />
      </button>
      {canEdit ? <button className="business-card-note" onClick={() => edit(item)} aria-label={text("Edit note for ", "编辑 ") + title + text("", " 备注")}>
        <Pencil size={13} aria-hidden="true" /><span>{note || text("Add note", "添加备注")}</span>
      </button> : note && <p className="business-card-note"><span>{note}</span></p>}
    </div>
    <div className="business-card-tags">
      <PlatformBadge platform={item.platform} channel={identity.channel} />
      <span className="business-env" title={identity.environment || undefined}>{environment}</span>
      {identity.implementation && <span className="business-env">{identity.implementation}</span>}
    </div>
    <div className="business-card-meters" aria-label={text("Host resources", "主机资源")} title={summary?.host_name}>
      <ResourceMeter label="CPU" value={host?.cpu_percent} warning={85} />
      <ResourceMeter label={text("Memory", "内存")} value={host?.memory_percent} warning={90} title={memoryDetail} />
      <ResourceMeter label={text("Disk", "磁盘")} value={host?.disk_percent} warning={85} title={text("Busiest writable partition", "最满可写分区")} />
    </div>
    <div className="business-card-traffic" aria-label={trafficScope} title={trafficScope}>
      <span><ArrowDown size={14} aria-hidden="true" /><small>{text("Receive", "下行")}</small><strong>{trafficValue(summary?.receive_per_second)}</strong></span>
      <span><ArrowUp size={14} aria-hidden="true" /><small>{text("Send", "上行")}</small><strong>{trafficValue(summary?.transmit_per_second)}</strong></span>
    </div>
    <footer className={fresh ? "report-fresh" : "report-old"}>
      <span><i aria-hidden="true" />{fresh ? text("Last report", "最近上报") : text("Report overdue", "上报超时")}</span>
      <time dateTime={item.last_seen ? new Date(item.last_seen).toISOString() : undefined}>{formatTimestamp(item.last_seen || undefined)}</time>
    </footer>
  </article>;
}

function PlatformFilters({ counts, selected, select }: {
  counts: Record<string, number>; selected: string; select: (key: string) => void;
}) {
  const quick = pinnedPlatforms.filter(key => Object.hasOwn(counts, key));
  const more = Object.keys(counts).filter(key => !pinnedPlatforms.includes(key));
  if (selected && !quick.includes(selected) && !more.includes(selected)) more.push(selected);
  more.sort((a, b) => platformName(a).localeCompare(platformName(b), getLocale()));
  const total = Object.values(counts).reduce((sum, count) => sum + count, 0);

  return <div className="business-platform-filters" role="group" aria-label={text("Platforms", "平台")}>
    <button type="button" aria-pressed={!selected} onClick={() => select("")}>
      {text("All platforms", "全部平台")}<span className="business-filter-count" aria-hidden="true">{total}</span>
    </button>
    {quick.map(key => <button key={key} type="button" aria-pressed={selected === key} onClick={() => select(key)}>
      <PlatformBadge platform={key} /><span className="business-filter-count" aria-hidden="true">{counts[key]}</span>
    </button>)}
    {more.length > 0 && <select className="business-platform-more" aria-label={text("More platforms", "更多平台")}
      value={more.includes(selected) ? selected : ""} onChange={event => select(event.target.value)}>
      <option value="">{text("More platforms", "更多平台")}</option>
      {more.map(key => <option key={key} value={key}>{platformName(key)} ({counts[key] ?? 0})</option>)}
    </select>}
  </div>;
}

// The list owns its scroll position; same-scope refreshes reuse mounted rows.
function ServerList({ items, render }: { items: BusinessInstance[]; render: (item: BusinessInstance) => ReactNode }) {
  const list = useRef<HTMLDivElement>(null);
  const [viewport, setViewport] = useState({ height: 0, rowHeight: 0 });
  const [scrollTop, setScrollTop] = useState(0);
  const windowed = items.length > 10;

  useLayoutEffect(() => {
    const element = list.current;
    if (!element || !windowed) return;
    function measure() {
      if (!element) return;
      const rowHeight = parseFloat(getComputedStyle(element).getPropertyValue("--business-row-height"));
      const height = element.clientHeight;
      setViewport(previous => previous.height === height && previous.rowHeight === rowHeight ? previous : { height, rowHeight });
      setScrollTop(element.scrollTop);
    }
    measure();
    const observer = new ResizeObserver(measure);
    observer.observe(element);
    return () => observer.disconnect();
  }, [windowed]);

  const { height, rowHeight } = viewport;
  const overscan = 3;
  const start = windowed && rowHeight ? Math.min(items.length - 1, Math.max(0, Math.floor(scrollTop / rowHeight) - overscan)) : 0;
  const end = windowed && rowHeight ? Math.min(items.length, start + Math.ceil(height / rowHeight) + 2 * overscan) : items.length;

  return <div ref={list} className={"business-server-list" + (windowed ? " is-windowed" : "")}
    role={windowed ? "region" : undefined} aria-label={windowed ? text("Server results", "服务器结果") : undefined}
    tabIndex={windowed ? 0 : undefined} onScroll={event => setScrollTop(event.currentTarget.scrollTop)}>
    <div className="business-server-list-heading" aria-hidden="true">
      <span>{text("Server", "服务器")}</span><span>{text("Platform / environment", "平台 / 环境")}</span>
      <span>{text("Host resources", "主机资源")}</span><span>{text("Service traffic", "业务流量")}</span>
      <span>{text("Last report", "最近上报")}</span>
    </div>
    <div style={windowed ? { paddingTop: start * rowHeight, paddingBottom: (items.length - end) * rowHeight } : undefined}>
      {items.slice(start, end).map(render)}
    </div>
  </div>;
}

export default function BusinessInstanceDirectory({ active, select, reload, tick = 0 }: {
  active: boolean; select: (id: string) => void; reload: () => Promise<void>; tick?: number;
}) {
  const { canEdit } = useSession();
  const [cursor, setCursor] = useState("");
  const [previous, setPrevious] = useState<string[]>([]);
  const [revision, setRevision] = useState(0);
  const [result, setResult] = useState<{ scope: string; page: InstancePage } | null>(null);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);
  const [search, setSearch] = useState("");
  const [query, setQuery] = useState("");
  const [environment, setEnvironment] = useState("");
  const [platform, setPlatform] = useState("");
  const [role, setRole] = useState("all");
  const [report, setReport] = useState<ReportState | "">("");
  const [sort, setSort] = useState<DirectorySort>("server");
  const [editing, setEditing] = useState<BusinessInstance | null>(null);

  const scope = new URLSearchParams({ limit: "24", cursor, q: query, environment, platform, role, report, sort }).toString();
  const displayed = result?.scope === scope ? result.page : null;
  const now = useObservationClock(displayed?.items.flatMap(item => [
    item.last_seen - 30000, item.last_seen + 90001, (displayed.summaries?.[item.id]?.host_expires_at || 0) + 1,
  ]) || [], active);
  const environments = [...new Set([...defaultEnvironments, ...(result?.page.environments || [])])].filter(Boolean);

  function resetPage() {
    setCursor("");
    setPrevious([]);
  }

  function clearFilters() {
    setSearch("");
    setQuery("");
    setEnvironment("");
    setPlatform("");
    setRole("all");
    setReport("");
    resetPage();
  }

  useEffect(() => {
    if (!active) return;
    const controller = new AbortController();
    setLoading(true);
    setError("");
    void api<InstancePage>("/business/directory?" + scope, { signal: controller.signal })
      .then(page => { if (!controller.signal.aborted) setResult({ scope, page }); })
      .catch(cause => { if (!controller.signal.aborted) setError(message(cause)); })
      .finally(() => { if (!controller.signal.aborted) setLoading(false); });
    return () => controller.abort();
  }, [active, scope, revision, tick]);

  return <section className="business-instance-directory" aria-label={text("Business instances", "业务实例")} aria-busy={loading}>
    <header className="business-directory-heading">
      <div className="business-title-line">
        <h1>{text("Server directory", "服务器列表")}</h1>
        {result && <span>{result.page.total} {text("registered instances", "个实例")}</span>}
      </div>
      <Button className="business-refresh" disabled={loading} onClick={() => {
        setRevision(value => value + 1);
        void reload().catch(cause => setError(message(cause)));
      }}><RefreshCw size={16} />{text("Refresh", "刷新")}</Button>
    </header>
    {error && <Alert severity="error" title={text("Could not load servers", "无法读取服务器")}>
      {error}<Button fill="text" onClick={() => { resetPage(); setRevision(value => value + 1); }}>{text("Try again", "重试")}</Button>
    </Alert>}
    {displayed?.at_capacity && <Alert severity="warning" title={text("Instance capacity reached", "实例观测容量已满")}>
      {text("Existing identities continue to update.", "现有身份继续更新，暂不登记新身份。")}
    </Alert>}
    <PlatformFilters counts={result?.page.platforms ?? {}} selected={platform} select={key => { resetPage(); setPlatform(key); }} />
    <div className="business-directory-tools">
      <form className="business-directory-search" onSubmit={event => { event.preventDefault(); resetPage(); setQuery(search.trim()); }}>
        <div className="business-search-input">
          <Search size={17} aria-hidden="true" />
          <Input aria-label={text("Search all instances", "搜索全部实例")} type="search" maxLength={128} value={search}
            onChange={event => { setSearch(event.target.value); if (!event.target.value) { resetPage(); setQuery(""); } }}
            placeholder={text("Search service, server ID, IP or note", "搜索服务、服务器编号、IP 或备注")} />
        </div>
        <Button type="submit" variant="primary">{text("Search", "搜索")}</Button>
      </form>
      <label className="business-directory-environment">
        <span>{text("Environment", "环境")}</span>
        <select value={environment} onChange={event => { resetPage(); setEnvironment(event.target.value); }}>
          <option value="">{text("All environments", "全部环境")}</option>
          {environments.map(env => <option key={env} value={env}>{environmentName(env)}{env === "production" ? " (production)" : ""}</option>)}
        </select>
      </label>
      <label className="business-directory-sort">
        <span>{text("Sort", "排序")}</span>
        <select aria-label={text("Sort all results", "全部结果排序")} value={sort} onChange={event => { resetPage(); setSort(event.target.value as DirectorySort); }}>
          <option value="server">{text("Server ID", "服务器编号")}</option>
          <option value="recent">{text("Latest report", "最近上报")}</option>
          <option value="host">{text("Host IP", "主机 IP")}</option>
        </select>
      </label>
      <label className="business-directory-report" title={text("Overdue means no report within 90 seconds.", "超过 90 秒未收到数据即为上报超时。")}>
        <span>{text("Reporting", "上报状态")}</span>
        <select value={report} onChange={event => { resetPage(); setReport(event.target.value as ReportState | ""); }}>
          <option value="">{text("Any status", "全部状态")}</option>
          <option value="recent">{text("Recent reports", "近期上报")} ({result?.page.reports?.recent ?? "—"})</option>
          <option value="overdue">{text("Report overdue", "上报超时")} ({result?.page.reports?.overdue ?? "—"})</option>
        </select>
      </label>
    </div>
    <div className="business-environment-filter business-role-tabs" role="group" aria-label={text("Service categories", "服务类型")}>
      {serviceRoles.map(([key, en, zh]) => <button key={key} type="button" aria-pressed={role === key}
        onClick={() => { resetPage(); setRole(key); }}>
        {text(en, zh)}<span aria-hidden="true">{displayed?.counts[key] ?? "—"}</span>
      </button>)}
    </div>
    {!displayed && !error && <p className="business-directory-empty" role="status">{text("Loading instances…", "正在加载实例…")}</p>}
    {displayed && displayed.items.length > 0 && <ServerList key={scope} items={displayed.items} render={item =>
      <InstanceRow key={item.id} item={item} summary={displayed.summaries?.[item.id]}
        select={select} edit={setEditing} canEdit={canEdit} now={now} />
    } />}
    {displayed && !displayed.items.length && <div className="business-directory-empty" role="status">
      <Server size={26} aria-hidden="true" /><p>{text("No matching instances.", "没有匹配的实例。")}</p>
      {(query || platform || environment || role !== "all" || report) && <Button onClick={clearFilters}>{text("Clear filters", "清除筛选")}</Button>}
    </div>}
    {displayed && <footer className="business-directory-footer">
      <p><Info size={15} aria-hidden="true" />{text("Last report shows when data arrived. Check server details for health.", "最近上报显示收到数据的时间。运行情况请查看服务器详情。")}</p>
      {previous.length > 0 || displayed.next_cursor ? <div className="business-directory-paging">
        <Button size="sm" disabled={!previous.length || loading} onClick={() => {
          setCursor(previous.at(-1) || ""); setPrevious(previous.slice(0, -1));
        }}><ArrowLeft size={15} />{text("Previous", "上一页")}</Button>
        <span>{text("Page ", "第 ") + (previous.length + 1)} · {displayed.matched} {text("results", "个结果")}</span>
        <Button size="sm" disabled={!displayed.next_cursor || loading} onClick={() => {
          setPrevious([...previous, cursor]); setCursor(displayed.next_cursor);
        }}>{text("Next", "下一页")}<ArrowRight size={15} /></Button>
      </div> : <span>{displayed.matched} {text("results", "个结果")}</span>}
    </footer>}
    {editing && <BusinessNoteEditor item={editing} close={() => setEditing(null)} saved={note => {
      setResult(value => value && {
        ...value, page: { ...value.page, items: value.page.items.map(item => item.id === editing.id ? { ...item, note } : item) },
      });
      setRevision(value => value + 1);
    }} />}
  </section>;
}
