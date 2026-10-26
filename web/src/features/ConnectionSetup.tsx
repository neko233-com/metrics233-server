import { useEffect, useState } from "react";
import { ArrowRight, Database, Search, Server, Wifi } from "lucide-react";
import { api, jsonBody, message, timeAgo, type Dashboard, type Target } from "../api";
import { getLocale, t } from "../i18n";
import { Alert, Button, Field, Input } from "../ui";
import { Dialog } from "../components/Dialog";
import { useSession } from "../ui/session";
import "./connection-setup.css";

const text = (en: string, zh: string) => getLocale() === "zh" ? zh : en;
export type ConnectionInput = { id: string; existing_target_id?: number; target: Partial<Target>; language: string; alerts: boolean; receiver: string };
type SetupRule = { title: string; for_seconds: number; keep_firing_for_seconds: number; notification_cooldown_seconds: number };
export type ConnectionPlan = { id: string; profile: string; samples: number; metric_count: number; metrics: string[]; target: Target; dashboard: Dashboard; rules: SetupRule[]; plan_hash: string };
export type ConnectionReceipt = { id: string; target_id: number; dashboard_id: string; rule_uids: string[]; profile: string; created_at: number };
export type ConnectionDraft = { input: ConnectionInput; plan: ConnectionPlan };

export async function previewConnection(target: Partial<Target>, options = { alerts: true, receiver: "" }, existingTargetID?: number): Promise<ConnectionDraft> {
  const input: ConnectionInput = { id: crypto.randomUUID(), ...(existingTargetID ? { existing_target_id: existingTargetID } : {}), target: existingTargetID ? {} : target, language: getLocale(), ...options };
  const plan = await api<ConnectionPlan>("/connections/preview", { method: "POST", body: jsonBody({ setup: input }) });
  return { input, plan };
}

export function useConnections(revision: unknown) {
  const [items, setItems] = useState<ConnectionReceipt[]>([]);
  const [error, setError] = useState("");
  const [ready,setReady]=useState(false);
  useEffect(() => {
    const controller = new AbortController();
    void api<ConnectionReceipt[]>("/connections", { signal: controller.signal }).then(value => { if (!controller.signal.aborted) { setItems(value); setError(""); setReady(true); } }).catch(cause => { if (!controller.signal.aborted) setError(message(cause)); });
    return () => controller.abort();
  }, [revision]);
  return { items, error, ready };
}

export function ConnectionReview({ draft, close, saved }: { draft: ConnectionDraft; close: () => void; saved: (receipt: ConnectionReceipt) => Promise<void> }) {
  const [plan, setPlan] = useState(draft.plan);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [receipt, setReceipt] = useState<ConnectionReceipt | null>(null);
  async function apply() {
    setBusy(true); setError("");
    try {
      const result = receipt || await api<ConnectionReceipt>("/connections/apply", { method: "POST", body: jsonBody({ setup: draft.input, plan_hash: plan.plan_hash }) });
      setReceipt(result);
      await saved(result);
    } catch (cause) { setError(t(message(cause))); }
    finally { setBusy(false); }
  }
  return <Dialog title={text("Connection preview", "接入预览")} onClose={busy ? () => {} : close}>
    <div className="connection-review">
      <p className="connection-reviewed-name"><Server size={20} /><strong>{plan.target.name}</strong><span>{plan.profile}</span></p>
      <p className="subtle">{plan.target.url}</p>
      <p>{receipt ? text("Configuration saved. Collection starts automatically; you can open the dashboard.","配置已保存，采集会自动开始，可以打开仪表盘。") : text("The actual connection test succeeded. Nothing has been saved yet.", "实际连接测试成功，尚未保存配置。")}</p>
      <div className="connection-review-counts"><span><b>{plan.samples}</b>{text("samples", "个样本")}</span><span><b>{plan.metric_count}</b>{text("metrics", "种指标")}</span><span><b>{plan.dashboard.panels.length}</b>{text("panels", "个面板")}</span><span><b>{plan.rules.length}</b>{text("alert rules", "条告警规则")}</span></div>
      <h3>{text("Ready-to-use dashboard", "默认仪表盘")}</h3>
      <ul className="connection-preview-panels">{plan.dashboard.panels.map(panel => <li key={panel.id}>{t(panel.title)}</li>)}</ul>
      {plan.rules.length > 0 && <><h3>{text("Sustained alerts", "持续告警")}</h3><ul className="connection-preview-rules">{plan.rules.map(rule => <li key={rule.title}><strong>{rule.title}</strong><span>{text("Wait", "持续")} {rule.for_seconds / 60} {text("min · recovery", "分钟 · 恢复确认")} {rule.keep_firing_for_seconds / 60} {text("min · cooldown", "分钟 · CD")} {rule.notification_cooldown_seconds / 3600}h</span></li>)}</ul><p className="subtle">{draft.input.receiver ? text("Notifications use the selected contact point.", "通知使用已选联系点。") : text("Notifications follow your existing routing policy. Without a contact point, events remain in the alert page.", "通知沿用已有通知策略；未配置联系点时，在告警页查看事件。")}</p></>}
      <p className="subtle">{draft.input.existing_target_id ? text("The existing collector, credentials and metric identities stay unchanged.", "保留已有采集器、凭据和指标身份，只创建面板及所选告警。") : text("The collector, dashboard and alert rules are saved together. Rates need at least two collection cycles.", "采集器、面板和告警一起保存；速率需要至少两个采集周期。")}</p>
      {error && <Alert severity="error" title={text("Connection not completed", "接入未完成")}>{error}</Alert>}
      <div className="form-actions"><Button disabled={busy} onClick={close}>{text("Cancel", "取消")}</Button>{!receipt && <Button disabled={busy} onClick={async () => { setBusy(true); setError(""); try { setPlan(await api<ConnectionPlan>("/connections/preview", { method: "POST", body: jsonBody({ setup: draft.input }) })); } catch (cause) { setError(t(message(cause))); } finally { setBusy(false); } }}>{text("Test again", "重新测试")}</Button>}<Button variant="primary" disabled={busy} onClick={() => void apply()}>{busy ? text("Connecting…", "正在接入…") : receipt ? text("Open dashboard", "打开仪表盘") : text("Connect and start collecting", "接入并开始采集")}</Button></div>
    </div>
  </Dialog>;
}

type Discovery = { preset: string; name: string; url: string; profile: string; samples: number; configured: boolean };
export function LocalConnectionDiscovery({ select }: { select: (target: Target) => void }) {
  const [busy, setBusy] = useState(false);
  const [items, setItems] = useState<Discovery[] | null>(null);
  const [error, setError] = useState("");
  return <section className="connection-discovery" aria-label={text("Discover local collectors", "发现本机采集源")}>
    <div><h2>{text("Find local exporters", "发现本机 Exporter")}</h2><p className="subtle">{text("Checks common loopback metrics ports on the monitoring server, not this browser's computer. Databases with credentials use Quick connect.", "检查监控服务器本机的常见指标端口，不是浏览器所在电脑。需要账号的数据库请使用快速接入。")}</p></div>
    <Button disabled={busy} onClick={async () => { setBusy(true); setError(""); try { const result = await api<{ connections: Discovery[] }>("/connections/discover", { method: "POST" }); setItems(result.connections); } catch (cause) { setError(t(message(cause))); } finally { setBusy(false); } }}><Search size={16} />{busy ? text("Checking…", "正在检查…") : text("Discover", "发现采集源")}</Button>
    {error && <p role="alert" className="form-error">{error}</p>}
    {items && (items.length ? <ul>{items.map(item => <li key={item.url}><div><strong>{item.name}</strong><span className="subtle">{item.url} · {item.samples} {text("samples", "个样本")}</span></div><Button disabled={item.configured} onClick={() => select({ id: 0, name: item.preset, kind: "prometheus", url: item.url, interval_seconds: 15, enabled: true, labels: {}, last_scrape: 0, last_error: "", samples: 0, duration_ms: 0 })}>{item.configured ? text("Already connected", "已接入") : text("Connect", "接入")}</Button></li>)}</ul> : <p role="status">{text("No local exporter found. Use Quick connect to enter a service address.", "未发现本机 Exporter。可用快速接入填写服务地址。")}</p>)}
  </section>;
}

export default function ConnectionResources({ targets, dashboards, open, reload, navigate, tick }: { targets: Target[]; dashboards: Dashboard[]; open: (dashboard: Dashboard) => void; reload: () => Promise<void>; navigate: () => void; tick: number }) {
  const { canAdmin } = useSession();
  const { items, error: catalogError, ready } = useConnections(tick);
  const [search, setSearch] = useState("");
  const [category, setCategory] = useState("all");
  const [page, setPage] = useState(0);
  const [busy, setBusy] = useState<number | null>(null);
  const [error, setError] = useState("");
  const [draft, setDraft] = useState<ConnectionDraft | null>(null);
  const byTarget = new Map(items.map(item => [item.target_id, item]));
  const categoryOf = (target: Target) => {
    const profile = byTarget.get(target.id)?.profile || target.kind || "prometheus";
    return ["node", "windows"].includes(profile) ? "host" : ["mydb", "mysql", "redis", "postgresql", "clickhouse", "elasticsearch"].includes(profile) ? "database" : "service";
  };
  const matches = targets.filter(target => `${target.name} ${target.url} ${target.kind}`.toLowerCase().includes(search.trim().toLowerCase()));
  const filtered = matches.filter(target => category === "all" || categoryOf(target) === category);
  const pages = Math.max(1, Math.ceil(filtered.length / 24));
  const currentPage = Math.min(page, pages - 1);
  const hasFilters = !!search.trim() || category !== "all";
  return <section className="connection-resources" aria-label={text("Infrastructure dashboards", "基础设施仪表盘")}>
    <header className="page-heading"><div><h1>{text("Infrastructure", "基础设施")}</h1><p>{text("Databases, exporters and their actual collection state.", "数据库、Exporter 及实际采集状态。")}</p></div>{canAdmin && <Button variant="primary" onClick={navigate}>{text("Connect resource", "接入资源")}</Button>}</header>
    <Field label={text("Search resources", "搜索采集资源")}><Input type="search" aria-label={text("Search resources", "搜索采集资源")} value={search} onChange={event => { setSearch(event.target.value); setPage(0); }} placeholder={text("Name / address / type", "名称 / 地址 / 类型")} /></Field>
    <div className="connection-resource-tabs" role="group" aria-label={text("Resource categories", "资源分类")}>{[["all",text("All","全部")],["host",text("Hosts","主机")],["database",text("Databases","数据库")],["service",text("Services","服务")]].map(([id,label]) => <Button key={id} aria-pressed={category === id} onClick={() => { setCategory(id); setPage(0); }}>{label} <span>{id === "all" ? matches.length : matches.filter(target => categoryOf(target) === id).length}</span></Button>)}</div>
    {(error || catalogError) && <Alert severity="error" title={text("Request failed", "操作失败")}>{t(error || catalogError)}</Alert>}
    {!filtered.length && <div className="empty-workspace"><h2>{text("No matching resources", "没有匹配的采集资源")}</h2><p>{hasFilters ? text("Try another name, address, type or category.", "请尝试其他名称、地址、类型或分类。") : text("Connect a database or exporter to get a dashboard from its real metrics.", "接入数据库或 Exporter，即可按真实指标生成仪表盘。")}</p><Button onClick={hasFilters ? () => { setSearch(""); setCategory("all"); } : navigate}>{hasFilters ? text("Clear filters", "清除筛选") : text("Open collectors", "前往采集器")}</Button></div>}
    <div className="connection-resource-grid">{filtered.slice(currentPage * 24, currentPage * 24 + 24).map(target => {
      const receipt = byTarget.get(target.id), dashboard = dashboards.find(item => item.id === receipt?.dashboard_id);
      const stale = !target.last_scrape || Date.now() - target.last_scrape > Math.max(90000, target.interval_seconds * 3000);
      const state = !target.enabled ? text("Paused", "已暂停") : target.last_error ? text("Collection failed", "采集失败") : stale ? text("Waiting for fresh samples", "等待新样本") : text("Collecting", "正在采集");
      return <article key={target.id} className="connection-resource-card" aria-label={target.name}>
        <header>{categoryOf(target) === "database" ? <Database size={21} /> : categoryOf(target) === "host" ? <Server size={21} /> : <Wifi size={21} />}<div><h2>{target.name}</h2><span>{receipt?.profile || target.kind || "prometheus"}</span></div></header>
        <p className="connection-resource-endpoint">{target.url}</p>
        <p className={`status ${!target.enabled || stale ? "muted" : target.last_error ? "unhealthy" : "healthy"}`}><i />{state}</p>
        <dl><div><dt>{text("Last collection", "最近采集")}</dt><dd>{timeAgo(target.last_scrape)}</dd></div><div><dt>{text("Samples / cycle", "每次样本")}</dt><dd>{target.last_scrape ? target.samples : "—"}</dd></div><div><dt>{text("Collection interval", "采集周期")}</dt><dd>{target.interval_seconds}s</dd></div></dl>
        {target.last_error && <p className="collector-error">{target.last_error}</p>}
        {!ready ? <p role="status" className="subtle">{text("Reading dashboard state…","正在读取仪表盘状态…")}</p> : dashboard ? <Button onClick={() => open(dashboard)}>{text("Open dashboard", "打开仪表盘")}<ArrowRight size={15} /></Button> : canAdmin && !receipt ? <Button disabled={busy !== null || !target.enabled || !!catalogError} onClick={async () => { setBusy(target.id); setError(""); try { setDraft(await previewConnection({}, { alerts: false, receiver: "" }, target.id)); } catch (cause) { setError(t(message(cause))); } finally { setBusy(null); } }}>{busy === target.id ? text("Testing…", "正在测试…") : text("Generate default dashboard", "生成默认仪表盘")}</Button> : <p className="subtle">{receipt ? text("The saved dashboard was removed. Manage this resource from Collectors.", "已保存的仪表盘已移除，请到采集器管理此资源。") : text("Ask an administrator to generate a resource dashboard.", "请管理员生成此资源的默认仪表盘。")}</p>}
      </article>;
    })}</div>
    {pages > 1 && <div className="connection-resource-pagination"><Button disabled={currentPage === 0} onClick={() => setPage(currentPage - 1)}>{text("Previous", "上一页")}</Button><span>{currentPage + 1} / {pages}</span><Button disabled={currentPage === pages - 1} onClick={() => setPage(currentPage + 1)}>{text("Next", "下一页")}</Button></div>}
    {draft && <ConnectionReview draft={draft} close={() => setDraft(null)} saved={async receipt => { await reload(); const boards = await api<Dashboard[]>("/dashboards"); const board = boards.find(item => item.id === receipt.dashboard_id); if (!board) throw new Error(text("Saved dashboard unavailable; refresh the resource list.", "已保存的仪表盘暂不可用，请刷新资源列表。")); setDraft(null); open(board); }} />}
  </section>;
}
