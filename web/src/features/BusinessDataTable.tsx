import { useEffect, useRef, useState } from "react";
import { api, message } from "../api";
import { getLocale } from "../i18n";
import { Alert, Button, Field } from "../ui";
const text = (en: string, zh: string) => getLocale() === "zh" ? zh : en;
export type Population = { available: boolean; players: number | null; snapshot_at: number; stale: boolean; status: "ready" | "missing" | "ambiguous" | "invalid" | "query_error" };
type DataRow = { key: string; labels: Record<string, string>; values: Record<string, number | null> };
type DataPage = { items: DataRow[]; total: number; next_cursor: string; evaluated_at: number; window: string; population?: Population; regions_complete?: boolean; region_total?: number };
export function PopulationNotice({ population }: { population?: Population }) {
  if (!population) return null;
  if (!population.available) return <Alert severity="warning" title={text("Player snapshot unavailable", "玩家快照不可用")}>{population.status === "ambiguous" ? text("Multiple population series share the latest timestamp. Check the instance labels; they are not added together.", "最新时间戳存在多个人口序列，请检查实例标签；不会将它们相加。") : population.status === "invalid" ? text("The population sample is invalid.", "人口样本无效。") : text("No authoritative population sample is available in the last five minutes, or its query failed. Missing data is not zero and does not prove that players are offline.", "最近 5 分钟缺少权威人口样本或查询失败。缺失不等于 0，也不能据此认定玩家离线。")}</Alert>;
  const stamp = new Date(population.snapshot_at).toLocaleString();
  return population.stale ? <Alert severity="warning" title={text("Player data is stale (over 90 seconds)", "玩家数据陈旧（超过 90 秒）")}>{text(`The last snapshot at ${stamp} reported ${population.players} players. It does not establish the current population or server availability. Longer configured reporting intervals can also trigger this freshness warning.`, `最近快照 ${stamp} 上报 ${population.players} 名玩家，不代表当前人口或服务可用性；配置较长上报间隔也可能触发此新鲜度提示。`)}</Alert> : <p className="subtle">{text(`Player snapshot: ${stamp} · ${population.players} reported players`, `玩家快照：${stamp} · 上报 ${population.players} 人`)}</p>;
}
const ROW_HEIGHT = 44;
export default function BusinessDataTable({ id, kind, tick = 0, refresh = 0 }: { id: string; kind: "regions" | "sql"; tick?: number; refresh?: number }) {
  const [window, setWindow] = useState("1h");
  return <section><div className="business-instance-toolbar"><h3>{kind === "regions" ? text("All player regions", "全部玩家地区") : text("Slow SQL Top10", "慢 SQL Top10")}</h3>{kind === "sql" && <Field label={text("Recent window", "最近窗口")}><select aria-label={text("Recent window", "最近窗口")} value={window} onChange={event => setWindow(event.target.value)}><option value="5m">{text("Last 5 minutes", "最近 5 分钟")}</option><option value="15m">{text("Last 15 minutes", "最近 15 分钟")}</option><option value="1h">{text("Last 60 minutes", "最近 60 分钟")}</option></select></Field>}</div><PagedBusinessTable key={`${id}:${kind}:${window}`} id={id} kind={kind} window={window} tick={kind === "sql" ? tick : 0} refresh={refresh} />
    {kind === "regions" ? <p className="subtle">{text("Current online players grouped by country, province and city; all regions can be loaded. Unknown means the location was unavailable. IP location is approximate, not a person's exact location.", "按国家、省份、城市汇总当前在线玩家，可加载全部地区。Unknown 表示无法定位；IP 归属地为大致位置，并非玩家精确真实所在地。")}{" "}<a href="https://db-ip.com" target="_blank" rel="noreferrer">{text("IP location data: DB-IP Lite", "IP归属地数据：DB-IP Lite")}</a></p> : <details><summary>{text("How this is calculated", "统计口径")}</summary><p className="subtle">{text("Top 10 Entity / operation pairs by weighted average database execution time in this window. Calls count actual driver attempts after async batching; retries count separately. Counts are estimates from counter increases (≈). This is a ranking, not a slow-query threshold or raw SQL log.", "按窗口内加权平均耗时取最高 10 项（Entity + 操作）。执行次数统计异步合并后的实际数据库调用，重试单独计次；≈ 表示由 Counter 增量估算。此处为耗时排名，不表示每项都超过慢查询阈值，也不采集原始 SQL 文本。")}</p></details>}
  </section>;
}

function PagedBusinessTable({ id, kind, window, tick, refresh }: { id: string; kind: "regions" | "sql"; window: string; tick: number; refresh: number }) {
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
      const value = await api<DataPage>(`/business/instances/${id}/data?kind=${kind}&window=${window}&limit=${kind === "sql" ? 10 : 100}&cursor=${encodeURIComponent(cursor)}`, { signal: controller.signal });
      if (alive.current && !controller.signal.aborted) { setPage(value); setRows(previous => { const items = kind === "sql" ? value.items.filter(row => Number.isFinite(row.values.avg_ms_window) && (row.values.calls_window ?? 0) > 0).slice(0, 10) : value.items; if (!cursor) return items; const keys = new Set(previous.map(row => row.key)); return [...previous, ...items.filter(item => !keys.has(item.key))]; }); }
    } catch (cause) { if (alive.current && !controller.signal.aborted) setError(message(cause)); }
    finally { if (request.current === controller) request.current = null; if (alive.current && !controller.signal.aborted) setLoading(false); }
  }
  useEffect(() => { alive.current = true; void load(""); return () => { alive.current = false; request.current?.abort(); request.current = null; }; }, [id, kind, window, refresh]);
  // A refresh tick must not repeatedly cancel a valid slow query. Scope changes
  // and an explicit refresh still abort; background ticks skip an in-flight read.
  useEffect(() => { if (kind === "sql") void load(""); }, [tick]);
  const columns = kind === "regions" ? [ ["country", text("Country", "国家")], ["province", text("Province", "省份")], ["city", text("City", "城市")], ["value", text("Online players", "在线玩家")]] : [ ["table", "Entity"], ["operation", text("Operation", "操作")], ["avg_ms_window", text("Average ms ↓", "平均耗时 ms ↓")], ["calls_window", text("Executions ≈", "执行次数 ≈")], ["failures_window", text("Failures ≈", "失败次数 ≈")]];
  const start = Math.max(0, Math.floor(scrollTop / ROW_HEIGHT) - 4), end = Math.min(rows.length, start + 20);
  const label = kind === "regions" ? text("Player regions", "玩家地区") : text("Slow SQL Top10", "慢 SQL Top10");
  const isSQL = kind === "sql";
  const visibleRows = isSQL ? rows : rows.slice(start, end);
  const cell = (row: DataRow, key: string) => {
    if (key in row.labels) return row.labels[key];
    const value = row.values[key];
    if (value == null || !Number.isFinite(value)) return "—";
    if (isSQL && key.endsWith("_window") && key !== "avg_ms_window") {
      return value === 0 ? "0" : value > 0 && value < 0.1 ? "≈ < 0.1" : `≈ ${value.toLocaleString(getLocale(), { maximumFractionDigits: 1 })}`;
    }
    if (key === "avg_ms_window" && value > 0 && value < 0.001) return "< 0.001";
    return value.toLocaleString(getLocale(), { maximumFractionDigits: key === "avg_ms_window" ? 3 : 2 });
  };
  return <>
    {error && <Alert severity="error" title={text("Metric query failed", "指标查询失败")}>{error}</Alert>}
    {!isSQL && <><PopulationNotice population={page?.population} />{page?.regions_complete === false && page.population?.available && <Alert severity="warning" title={text("Region snapshot is incomplete", "地区快照尚不完整")}>{text(`Regional total: ${page.region_total} of ${page.population.players} players. Waiting for the complete batch.`, `同轮地区合计 ${page.region_total} / ${page.population.players} 人，等待完整批次。`)}</Alert>}</>}
    <div className={isSQL ? "table-scroll" : "business-virtual-table"} tabIndex={0} aria-label={text("Scrollable results", "可滚动结果")} onScroll={event => {
      if (isSQL) return;
      const element = event.currentTarget;
      setScrollTop(element.scrollTop);
      if (element.scrollHeight - element.scrollTop - element.clientHeight < 180 && page?.next_cursor && !error) void load(page.next_cursor);
    }}>
      <table aria-label={label} aria-rowcount={page ? (isSQL ? rows.length : page.total) + 1 : undefined}>
        <thead><tr>{columns.map(([key, name]) => <th key={key} scope="col">{name}</th>)}</tr></thead>
        <tbody>
          {!isSQL && start > 0 && <tr aria-hidden="true"><td colSpan={columns.length} style={{ height: start * ROW_HEIGHT, padding: 0 }} /></tr>}
          {visibleRows.map((row, index) => <tr key={row.key} aria-rowindex={(isSQL ? 0 : start) + index + 2}>{columns.map(([key]) => <td key={key} title={row.labels[key]}>{cell(row, key)}</td>)}</tr>)}
          {!isSQL && end < rows.length && <tr aria-hidden="true"><td colSpan={columns.length} style={{ height: (rows.length - end) * ROW_HEIGHT, padding: 0 }} /></tr>}
        </tbody>
      </table>
    </div>
    <div className="business-instance-toolbar">
      <span role="status">{loading ? text(rows.length ? "Refreshing…" : "Loading…", rows.length ? "正在更新…" : "正在加载…") : isSQL ? text(`${rows.length} results`, `${rows.length} 项`) : `${rows.length} / ${page?.total ?? "—"}`}{page ? ` · ${new Date(page.evaluated_at).toLocaleTimeString()}` : ""}</span>
      {!isSQL && page?.next_cursor && <Button disabled={loading} onClick={() => void load(page.next_cursor)}>{text("Load more", "加载更多")}</Button>}
      {error && <Button disabled={loading} onClick={() => void load("")}>{text("Retry", "重试")}</Button>}
    </div>
    {!loading && !error && page && rows.length === 0 && <p>{!isSQL && page.population?.available && page.population.players === 0 ? text("No players in this snapshot.", "该快照在线玩家为 0。") : isSQL ? text("No SQL latency can be calculated in this window yet.", "最近窗口暂无可计算的 SQL 耗时，等待更多上报。") : text("No samples available for this instance. Waiting for real reports.", "该实例暂无样本，等待真实上报。")}</p>}
  </>;
}
