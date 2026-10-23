import { useEffect, useState } from "react";
import { api, message } from "../api";
import { Alert, Button, Field } from "../ui";
import { getLocale } from "../i18n";

type Variant = { template: string; calls: number; seconds: number; maximum_seconds: number };
type Group = Variant & { id: string; namespace: string; table: string; operation: string; variants: Variant[] };
type Summary = { groups: Group[]; tables: { namespace: string; table: string; calls: number; seconds: number; groups: number }[] };
const text = (en: string, zh: string) => getLocale() === "zh" ? zh : en;
const number = (value: number) => Number.isFinite(value) ? value.toLocaleString(undefined, { maximumFractionDigits: 2 }) : "—";

export default function SlowSQL({ id, tick = 0 }: { id: string; tick?: number }) {
  const [window, setWindow] = useState("1h");
  const [revision, setRevision] = useState(0);
  const [summary, setSummary] = useState<Summary | null>(null);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  useEffect(() => {
    const controller = new AbortController(); setBusy(true); setError(""); setSummary(null);
    api<Summary>(`/business/instances/${encodeURIComponent(id)}/slow-sql?window=${window}`, { signal: controller.signal })
      .then(value => { if (!controller.signal.aborted) setSummary(value); })
      .catch(cause => { if (!controller.signal.aborted) setError(message(cause)); })
      .finally(() => { if (!controller.signal.aborted) setBusy(false); });
    return () => controller.abort();
  }, [id, window, tick, revision]);
  return <section className="business-resource-section" aria-label={text("Slow SQL templates", "慢 SQL 模板归类")}>
    <div className="business-resource-heading"><h3>{text("Slow SQL · templates and tables", "慢 SQL · 模板与表归类")}</h3>
      <Field label={text("Window", "统计窗口")}><select value={window} onChange={event => setWindow(event.target.value)}><option value="5m">5 min</option><option value="15m">15 min</option><option value="1h">1 h</option></select></Field>
      <Button disabled={busy} onClick={() => setRevision(value => value + 1)}>{text("Refresh", "刷新")}</Button></div>
    <p className="subtle">{text("Driver calls ≥200 ms. SQL values are redacted. Similar templates (Levenshtein ≥90%) stay within the same table, operation and namespace. Counts and duration are counter increase estimates; the peak is the process lifetime maximum.", "驱动调用 ≥200 ms；SQL 参数已脱敏。同一命名空间、表及操作内，按莱温斯坦相似度 ≥90% 归类。窗口次数和总耗时为计数器增量估算；峰值为进程生命周期最大值。")}</p>
    {busy && <p role="status">{text("Loading…", "正在查询…")}</p>}
    {error && <Alert severity="error" title={text("Slow SQL unavailable", "慢 SQL 查询不可用")}>{error}</Alert>}
    {summary && summary.groups.length === 0 && <p>{text("No slow SQL templates received in this window. This does not prove the database has no slow queries.", "该窗口尚无慢 SQL 模板上报；这不代表数据库没有慢查询。")}</p>}
    {summary && summary.tables.length > 0 && <div className="table-scroll"><table><caption>{text("By physical table", "按物理表汇总")}</caption><thead><tr><th>{text("Namespace", "命名空间")}</th><th>Table</th><th>{text("Groups", "模板组")}</th><th>{text("Calls", "次数")}</th><th>{text("Total ms", "总耗时 ms")}</th></tr></thead><tbody>{summary.tables.map(row => <tr key={`${row.namespace}:${row.table}`}><td>{row.namespace}</td><td>{row.table}</td><td>{row.groups}</td><td>{number(row.calls)}</td><td>{number(row.seconds * 1000)}</td></tr>)}</tbody></table></div>}
    {summary?.groups.map(group => <details key={group.id}><summary>{group.namespace} · {group.table} · {group.operation} — {number(group.calls)} {text("calls", "次")} · {number(group.seconds * 1000)} ms · {group.variants.length} {text("templates", "个模板")}</summary>
      <div className="table-scroll"><table><thead><tr><th>SQL</th><th>{text("Calls", "次数")}</th><th>{text("Average ms", "平均 ms")}</th><th>{text("Process peak ms", "进程峰值 ms")}</th></tr></thead><tbody>{group.variants.map((variant, index) => <tr key={index}><td><code style={{ whiteSpace: "pre-wrap", overflowWrap: "anywhere" }}>{variant.template}</code></td><td>{number(variant.calls)}</td><td>{variant.calls > 0 ? number(variant.seconds / variant.calls * 1000) : "—"}</td><td>{number(variant.maximum_seconds * 1000)}</td></tr>)}</tbody></table></div>
    </details>)}
  </section>;
}
