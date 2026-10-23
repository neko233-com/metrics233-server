import { useEffect, useState } from "react";
import { ArrowLeft, ArrowRight, Info, Network, RefreshCw, Search, Server } from "lucide-react";
import { api, message } from "../api";
import { formatTimestamp } from "../display-format";
import { getLocale } from "../i18n";
import { Alert, Button, Input } from "../ui";
import { groupBusinessInstances } from "./business-instance-groups";
import { instanceTitle, reportFresh, sortDirectory, type DirectorySort, type InstancePage } from "./business-instance-directory-model";

const text = (en: string, zh: string) => getLocale() === "zh" ? zh : en;
export function environmentName(value: string, short = false): string {
  const names: Record<string, [string, string]> = { release: ["Production", "正式"], test: ["Test", "测试"], intranet: ["Intranet", "内网"], shared: ["Shared", "共享"] };
  const name = names[value];
  return name ? text(name[0], name[1] + (short ? "" : "环境")) : value || text("Unspecified", "未标明环境");
}
const channelName = (value: string) => value === "wechat" ? text("WeChat", "微信") : value === "taptap" ? "TapTap" : value || "—";

export default function BusinessInstanceDirectory({ active, select, reload, tick = 0 }: {
  active: boolean; select: (id: string) => void; reload: () => Promise<void>; tick?: number;
}) {
  const [cursor, setCursor] = useState("");
  const [previous, setPrevious] = useState<string[]>([]);
  const [revision, setRevision] = useState(0);
  const [page, setPage] = useState<InstancePage | null>(null);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);
  const [search, setSearch] = useState("");
  const [query, setQuery] = useState("");
  const [environment, setEnvironment] = useState<string | null>(null);
  const [sort, setSort] = useState<DirectorySort>("server");
  useEffect(() => {
    if (!active) return;
    const controller = new AbortController(); setLoading(true);
    void api<InstancePage>(`/business/instances?limit=50&cursor=${encodeURIComponent(cursor)}&q=${encodeURIComponent(query)}`, { signal: controller.signal })
      .then(value => { if (!controller.signal.aborted) { setPage(value); setError(""); } })
      .catch(cause => { if (!controller.signal.aborted) setError(message(cause)); })
      .finally(() => { if (!controller.signal.aborted) setLoading(false); });
    return () => controller.abort();
  }, [active, cursor, revision, query, tick]);
  const groups = groupBusinessInstances(sortDirectory(page?.items || [], sort));
  const visible = groups.filter(group => environment === null || group.environment === environment);
  const counts = new Map(groups.map(group => [group.environment, group.instances.length]));
  const environments = [...new Set(["release", "test", "intranet", "shared", ...groups.map(group => group.environment)])];
  const showPaging = previous.length > 0 || !!page?.next_cursor;
  function move(next: string, history: string[]) { setCursor(next); setPrevious(history); setEnvironment(null); }
  return <section className="business-instance-directory" aria-label={text("Business instances", "业务实例")} aria-busy={loading}>
    <header className="business-directory-heading">
      <div><div className="business-title-line"><h1>{text("Dashboards", "仪表盘")}</h1>{page && <span>{text(`${page.items.length} instances on this page`, `本页 ${page.items.length} 个实例`)}</span>}</div><p>{text("Game services and gateways", "游戏服务与网关")}</p></div>
      <Button className="business-refresh" disabled={loading} onClick={() => { setRevision(value => value + 1); void reload().catch(cause => setError(message(cause))); }}><RefreshCw size={16} />{text("Refresh", "刷新")}</Button>
    </header>
    {error && <Alert severity="error" title={text("Refresh failed; previous results retained", "更新失败，保留上次结果")}>{error}</Alert>}
    {page?.at_capacity && <Alert severity="warning" title={text("Instance observation capacity reached", "实例观测容量已满")}>{text(`Existing ${page.capacity} identities continue to update.`, `现有 ${page.capacity} 个身份继续更新，暂不登记新身份。`)}</Alert>}
    <div className="business-directory-tools">
      <form className="business-directory-search" onSubmit={event => { event.preventDefault(); move("", []); setQuery(search.trim()); }}>
        <div className="business-search-input"><Search size={18} aria-hidden="true" /><Input aria-label={text("Search all instances", "搜索全部实例")} type="search" maxLength={128} value={search} onChange={event => setSearch(event.target.value)} placeholder={text("Search service, serverId or IP", "搜索服务、serverId 或 IP")} /></div>
        <Button type="submit" disabled={loading}>{text("Search", "搜索")}</Button>
      </form>
      <select aria-label={text("Sort this page", "本页排序")} value={sort} onChange={event => setSort(event.target.value as DirectorySort)}><option value="server">{text("Server ID", "服务编号")}</option><option value="recent">{text("Latest report", "最近上报")}</option><option value="host">{text("Host IP", "主机 IP")}</option></select>
      {query && <Button fill="text" onClick={() => { setSearch(""); setQuery(""); move("", []); }}>{text("Clear search", "清除搜索")}</Button>}
    </div>
    <div className="business-environment-filter" role="group" aria-label={text("Environments on this page", "本页环境")}>
      <button aria-pressed={environment === null} onClick={() => setEnvironment(null)}>{text("All", "全部")}<span>{page?.items.length ?? "—"}</span></button>
      {environments.map(value => <button key={value} aria-pressed={environment === value} onClick={() => setEnvironment(value)}>{environmentName(value, true)}<span>{counts.get(value) || 0}</span></button>)}
    </div>
    <div className="business-directory-columns" aria-hidden="true"><span>{text("Service instance", "服务实例")}</span><span>{text("Environment / channel", "环境与渠道")}</span><span>{text("Host", "宿主机")}</span><span>{text("Last report", "最近上报")}</span><span /></div>
    {!page && loading && <p className="business-directory-empty" role="status">{text("Loading instances…", "正在加载实例…")}</p>}
    {visible.map(group => <section className="business-service-group" key={group.environment} aria-label={environmentName(group.environment)}>
      <h2>{environmentName(group.environment)}<span>{group.environment || "—"} · {group.instances.length}</span></h2>
      <ul>{group.instances.map(item => {
        const title = instanceTitle(item.identity, getLocale() === "zh"), fresh = reportFresh(item.last_seen);
        return <li key={item.id}><button className="business-service-row" onClick={() => select(item.id)} aria-label={text(`Open ${title} ${group.environment} ${item.identity.channel}`, `打开 ${title} ${environmentName(group.environment, true)} ${channelName(item.identity.channel)}`)}>
          <span className="business-service-identity"><span className="business-service-icon">{item.identity.role === "gateway" ? <Network size={21} /> : <Server size={21} />}</span><span><strong>{title}</strong><small>{item.identity.service} · {item.identity.instance}</small></span></span>
          <span className="business-service-scope"><span>{channelName(item.identity.channel)}</span><small>{group.environment || "—"} · {item.identity.implementation || "—"}</small></span>
          <span className="business-service-host"><span>{item.reported_ip || item.observed_ip || "—"}</span>{item.observed_ip && item.observed_ip !== item.reported_ip && <small>{text("Egress", "出口")} {item.observed_ip}</small>}</span>
          <span className={`business-service-report ${fresh ? "report-fresh" : "report-old"}`}><i aria-hidden="true" /><time dateTime={item.last_seen ? new Date(item.last_seen).toISOString() : undefined}>{formatTimestamp(item.last_seen || undefined)}</time></span>
          <ArrowRight className="business-service-arrow" size={18} aria-hidden="true" />
        </button></li>;
      })}</ul>
    </section>)}
    {page && !visible.some(group => group.instances.length) && <div className="business-directory-empty" role="status"><Server size={26} /><p>{query ? text("No instances match this search.", "没有匹配的实例。") : environment !== null ? text("No instances in this environment on this page.", "本页没有此环境的实例。") : text("No reporting services connected yet.", "尚未接入上报服务。")}</p>{(query || environment !== null) && <Button onClick={() => { setSearch(""); setQuery(""); setEnvironment(null); move("", []); }}>{text("Show all", "查看全部实例")}</Button>}</div>}
    <footer className="business-directory-footer"><p><Info size={15} />{text("Last report indicates received data, not service health.", "最近上报仅表示收到数据，不代表服务健康。")}</p>{showPaging ? <div className="business-directory-paging"><Button size="sm" disabled={!previous.length || loading} onClick={() => { const before = [...previous]; move(before.pop() || "", before); }}><ArrowLeft size={15} />{text("Previous", "上一页")}</Button><span>{text(`Page ${previous.length + 1} · up to 50`, `第 ${previous.length + 1} 页 · 每页最多 50 个`)}</span><Button size="sm" disabled={!page?.next_cursor || loading} onClick={() => move(page!.next_cursor, [...previous, cursor])}>{text("Next", "下一页")}<ArrowRight size={15} /></Button></div> : <span>{text("All search results", "当前搜索结果已显示完毕")}</span>}</footer>
  </section>;
}
