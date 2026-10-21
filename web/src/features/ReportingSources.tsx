import { useEffect, useRef, useState, type FormEvent } from "react";
import { Ban, Check, ChevronLeft, ChevronRight, RefreshCw, ShieldCheck, Trash2 } from "lucide-react";
import { api, ApiError, jsonBody, message, timeAgo } from "../api";
import { getLocale } from "../i18n";
import { Alert, Button, Field, Input } from "../ui";
import { useSession } from "../ui/session";
import "./reporting-sources.css";

type Source = {
  address: string;
  disposition: "" | "allow" | "block";
  allowed: boolean;
  first_seen: number;
  last_seen: number;
  accepted_batches: number;
  accepted_samples: number;
  protocols: string[];
  last_service: string;
};
type Snapshot = {
  version: number;
  allowlist_only: boolean;
  observation_limit: number;
  policy_limit: number;
  sources: Source[];
};
type Change = { address?: string; action?: "allow" | "block" | "reset"; allowlist_only?: boolean };
const text = (en: string, zh: string) => getLocale() === "zh" ? zh : en;
const pageSize = 50;

export default function ReportingSources({ notify }: { notify: (text: string, error?: boolean) => void }) {
  const { canAdmin } = useSession();
  const [snapshot, setSnapshot] = useState<Snapshot | null>(null);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState<string | null>(null);
  const [pendingAllowlist, setPendingAllowlist] = useState<boolean | null>(null);
  const [conflicted, setConflicted] = useState(false);
  const [revision, setRevision] = useState(0);
  const [query, setQuery] = useState("");
  const [page, setPage] = useState(0);
  const [address, setAddress] = useState("");
  const generation = useRef(0);
  const mutation = useRef<AbortController | null>(null);
  useEffect(() => () => mutation.current?.abort(), []);
  useEffect(() => {
    const controller = new AbortController();
    let timer: ReturnType<typeof setTimeout>;
    async function load() {
      if (!mutation.current) {
        const ticket = ++generation.current;
        setLoading(true);
        try {
          const value = await api<Snapshot>("/reporting-sources", { signal: controller.signal });
          if (!controller.signal.aborted && ticket === generation.current) {
            setSnapshot(value);
            setError("");
            setConflicted(false);
          }
        } catch (cause) {
          if (!controller.signal.aborted && ticket === generation.current) setError(message(cause));
        } finally {
          if (!controller.signal.aborted) setLoading(false);
        }
      }
      if (!controller.signal.aborted) timer = setTimeout(() => { void load(); }, 60000);
    }
    void load();
    return () => { controller.abort(); clearTimeout(timer); generation.current++; };
  }, [revision]);

  async function update(change: Change) {
    if (!canAdmin || !snapshot || conflicted || mutation.current) return false;
    const controller = new AbortController();
    mutation.current = controller;
    generation.current++;
    setBusy(change.address || "policy");
    setPendingAllowlist(change.allowlist_only ?? null);
    setError("");
    try {
      const value = await api<Snapshot>("/reporting-sources/policy", {
        method: "PUT", signal: controller.signal,
        body: jsonBody({ version: snapshot.version, allowlist_only: snapshot.allowlist_only, ...change }),
      });
      if (!controller.signal.aborted) {
        setSnapshot(value);
        notify(text("Reporting source policy updated", "上报来源策略已更新"));
      }
      return !controller.signal.aborted;
    } catch (cause) {
      if (!controller.signal.aborted) {
        if (cause instanceof ApiError && cause.status === 409) {
          setConflicted(true);
          setError(text("The policy changed elsewhere. Refresh, review the latest policy, then retry your action.", "策略已被其他操作更新。请刷新并核对最新策略，再重新操作。"));
        } else setError(message(cause));
      }
      return false;
    } finally {
      mutation.current = null;
      if (!controller.signal.aborted) {
        setPendingAllowlist(null);
        setBusy(null);
      }
    }
  }
  async function add(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const action = (event.nativeEvent as SubmitEvent).submitter?.getAttribute("value");
    if (action !== "allow" && action !== "block") return;
    if (await update({ address: address.trim(), action })) setAddress("");
  }
  const needle = query.trim().toLowerCase();
  const sources = snapshot?.sources || [];
  const filtered = needle ? sources.filter((source) => `${source.address} ${source.last_service || ""}`.toLowerCase().includes(needle)) : sources;
  const pages = Math.max(1, Math.ceil(filtered.length / pageSize));
  const currentPage = Math.min(page, pages - 1);
  const visible = filtered.slice(currentPage * pageSize, (currentPage + 1) * pageSize);
  const disabled = busy !== null || conflicted || !snapshot;
  const policyCount = sources.reduce((count, source) => count + Number(!!source.disposition), 0);

  return <>
    <div className="page-heading">
      <div><h1>{text("Reporting sources", "上报来源")}</h1><p>{text("Review observed source IPs and control who can push metrics.", "查看已观察到的来源 IP，控制谁可以推送指标。")}</p></div>
      <Button onClick={() => setRevision((value) => value + 1)} disabled={loading || busy !== null}><RefreshCw size={16} />{text("Refresh sources", "刷新来源")}</Button>
    </div>
    {error && <Alert severity="error" title={text("Request failed", "操作失败")}>{error}</Alert>}
    <section className="list-panel reporting-policy" aria-label={text("Source access policy", "来源访问策略")}>
      <div className="section-heading"><h2><ShieldCheck size={18} />{text("Source access policy", "来源访问策略")}</h2></div>
      <p className="subtle">{text("Policy applies to the server-verified client IP across JSON, remote write and OTLP. IPs behind the same NAT share a policy. A valid ingest token is still required; service labels are display hints. Pull collectors are managed separately.", "策略针对服务端确认的客户端 IP，同时作用于 JSON、remote write 和 OTLP。同一 NAT 出口的服务共用 IP 策略；仍须使用有效上报 Token，service 标签仅用于展示。主动采集目标独立管理。")}</p>
      {snapshot && <label className="reporting-only-toggle"><input type="checkbox" checked={pendingAllowlist ?? snapshot.allowlist_only} disabled={!canAdmin || disabled} onChange={(event) => { void update({ allowlist_only: event.target.checked }); }} />{text("Only allow listed IPs to report", "仅允许白名单 IP 上报")}</label>}
      <p className="subtle">{text("Blocked IPs are always denied. With allowlist-only enabled, removing an allow entry denies that IP again. Allow necessary egress IPs before enabling this mode.", "黑名单始终优先拒绝。开启仅白名单后，移出白名单的 IP 会再次被拒绝；请先允许必要的出口 IP。")}</p>
      {canAdmin ? <form className="reporting-add" onSubmit={add}>
        <Field label={text("IP address", "IP 地址")}><Input aria-label={text("IP address", "IP 地址")} required maxLength={64} autoComplete="off" placeholder="203.0.113.10 / 2001:db8::10" value={address} onChange={(event) => setAddress(event.target.value)} disabled={disabled} /></Field>
        <Button type="submit" value="allow" disabled={disabled || !address.trim()}><Check size={16} />{text("Add to allowlist", "加入白名单")}</Button>
        <Button type="submit" value="block" variant="destructive" disabled={disabled || !address.trim()}><Ban size={16} />{text("Block IP", "拉黑 IP")}</Button>
      </form> : <p className="subtle">{text("Only an Admin can change source access policies.", "只有管理员可以更改来源访问策略。")}</p>}
      {snapshot && <p className="subtle">{text("Observed sources", "观测来源")} ≤ {snapshot.observation_limit} · {text("Explicit IP policies", "显式 IP 策略")} {policyCount}/{snapshot.policy_limit} · {text("Old observations may be evicted; access policies persist independently.", "旧观测可被替换，访问策略独立保留。")}</p>}
    </section>
    <section className="list-panel" aria-label={text("Source IPs", "来源 IP 列表")}>
      <div className="section-heading reporting-list-heading"><h2>{text("Source IPs", "来源 IP 列表")}<span>{filtered.length}</span></h2><Input aria-label={text("Filter by IP or service", "按 IP 或服务筛选")} type="search" maxLength={256} placeholder={text("Filter IP / service", "筛选 IP / 服务")} value={query} onChange={(event) => { setQuery(event.target.value); setPage(0); }} /></div>
      <p className="subtle reporting-note">{text("Recent reports are observations, not online status. Only accepted batches are recorded. This page refreshes every minute; manual refresh is also available.", "最近上报时间代表已观察到的请求，并非在线状态。仅记录实际接受的批次；本页打开时每分钟刷新，也可手动刷新。")}</p>
      {!snapshot ? <p role="status" className="reporting-note">{loading ? text("Loading sources…", "正在加载来源…") : text("Refresh to load reporting sources.", "请刷新以加载上报来源。")}</p> : !visible.length ? <div className="empty-workspace"><h2>{needle ? text("No matching sources", "没有匹配来源") : text("No reporting sources yet", "尚无上报来源")}</h2><p>{text("Send a valid push batch or add an IP policy above.", "发送有效推送批次，或在上方手动添加 IP 策略。")}</p></div> : <div className="table-scroll"><table><thead><tr>
        <th>{text("Client IP / service", "客户端 IP / 服务")}</th><th>{text("Access", "访问策略")}</th><th>{text("Last report", "最近上报")}</th><th>{text("Accepted batches / samples", "已接受批次 / 样本")}</th>{canAdmin && <th>{text("Actions", "操作")}</th>}
      </tr></thead><tbody>{visible.map((source) => <tr key={source.address}>
        <td><strong className="mono">{source.address}</strong>{source.last_service && <div className="subtle reporting-service">{source.last_service}</div>}<div className="subtle">{source.protocols?.join(" · ") || "—"}</div></td>
        <td><span className={`status ${source.allowed ? "healthy" : "unhealthy"}`}><i />{source.disposition === "block" ? text("Blocked", "已拉黑") : source.disposition === "allow" ? text("Allowlisted", "白名单") : source.allowed ? text("Allowed by default", "默认允许") : text("Not allowlisted", "未列入白名单")}</span></td>
        <td>{source.last_seen ? <time dateTime={new Date(source.last_seen).toISOString()} title={new Date(source.last_seen).toLocaleString(getLocale())}>{timeAgo(source.last_seen)}</time> : text("No reports yet", "尚未上报")}</td>
        <td className="mono">{source.accepted_batches.toLocaleString()} / {source.accepted_samples.toLocaleString()}</td>
        {canAdmin && <td><div className="row-actions reporting-row-actions">
          {source.disposition !== "allow" && <Button size="sm" disabled={disabled} onClick={() => { void update({ address: source.address, action: "allow" }); }}>{source.disposition === "block" ? text("Unblock and allow", "解除并加入白名单") : text("Allow", "加入白名单")}</Button>}
          {source.disposition !== "block" && <Button size="sm" variant="destructive" disabled={disabled} onClick={() => { void update({ address: source.address, action: "block" }); }}>{text("Block", "拉黑")}</Button>}
          {source.disposition === "allow" && <Button size="sm" disabled={disabled} onClick={() => { void update({ address: source.address, action: "reset" }); }}><Trash2 size={14} />{text("Remove from allowlist", "移出白名单")}</Button>}
        </div></td>}
      </tr>)}</tbody></table></div>}
      {pages > 1 && <div className="reporting-pagination"><Button aria-label={text("Previous sources page", "上一页来源")} disabled={currentPage === 0} onClick={() => setPage(currentPage - 1)}><ChevronLeft size={16} /></Button><span>{currentPage + 1} / {pages}</span><Button aria-label={text("Next sources page", "下一页来源")} disabled={currentPage + 1 === pages} onClick={() => setPage(currentPage + 1)}><ChevronRight size={16} /></Button></div>}
    </section>
  </>;
}
