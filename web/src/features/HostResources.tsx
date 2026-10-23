import { useEffect, useMemo, useState } from "react";
import { api, formatValue, timeAgo } from "../api";
import { getLocale } from "../i18n";
import { RiskGauge, type GaugeStep } from "../ui/RiskGauge";
import "./ManagedHosts.css";
import HostNetworkResources from "./HostNetworkResources";

export type HostResourcesData = {
  observed_at: number; cpu_percent: number | null; cores: number; memory_percent: number | null;
  memory_total: number; memory_available: number; uptime: number;
  disks: { device: string; mountpoint: string; fstype: string; total: number; available: number; used_percent: number }[];
  network?: { device: string; receive_bytes: number; transmit_bytes: number; receive_per_second: number | null; transmit_per_second: number | null }[];
};
const text = (en: string, zh: string) => getLocale() === "zh" ? zh : en;
const specs = [
  { name: "CPU", metric: "host_cpu_usage_percent", warning: 85, color: "#39c98a" },
  { name: "Memory", metric: "host_memory_used_percent", warning: 90, color: "#59c5b9" },
  { name: "Disk", metric: "host_disk_used_percent", warning: 85, color: "#72a8ef" },
];
const resourceName = (name: string) => name === "CPU" ? "CPU" : name === "Memory" ? text("Memory", "内存") : text("Disk", "磁盘");
const bands = (warning: number): GaugeStep[] => [{ value: null, color: "green" }, { value: warning, color: "yellow" }, { value: 95, color: "red" }];

export type HostAlertRule = {
  uid: string; labels: Record<string, string>; annotations: Record<string, string>; paused: boolean;
  for_seconds: number; keep_firing_for_seconds: number; notification_cooldown_seconds?: number;
  grafana?: { condition?: string; data?: { refId?: string; model?: { type?: string; expression?: string; expr?: string; conditions?: { evaluator?: { type?: string; params?: number[] }; unloadEvaluator?: { type?: string; params?: number[] } }[] } }[] };
};
export function hostAlertPolicy(rules: HostAlertRule[], id: string, resource: string, severity = "warning") {
  const metric = specs.find(s => s.name.toLowerCase() === resource)?.metric;
  for (const rule of rules) {
    if (!rule) continue;
    if (rule.paused || rule.labels?.managed_host !== id || rule.labels?.resource !== resource || rule.labels?.severity !== severity || rule.labels?.scope === "acceptance") continue;
    const graph = rule.grafana, nodes = Array.isArray(graph?.data) ? graph.data : [];
    const source = nodes.find(node => node?.refId === "A")?.model;
    const condition = nodes.find(node => node?.refId === graph?.condition)?.model;
    if (!metric || !source?.expr?.includes(metric) || condition?.type !== "threshold" || condition.expression !== "A" || !Array.isArray(condition.conditions) || condition.conditions.length !== 1 || !condition.conditions[0]) continue;
    const check = condition.conditions[0], threshold = check.evaluator?.params?.[0], recovery = check.unloadEvaluator?.params?.[0];
    if (check.evaluator?.type !== "gt" || check.unloadEvaluator?.type !== "lt" || typeof threshold !== "number" || typeof recovery !== "number" || !Number.isFinite(threshold) || !Number.isFinite(recovery) || threshold < 0 || threshold > 100 || recovery < 0 || recovery >= threshold) continue;
    return { threshold, recovery, pending: rule.for_seconds, recovering: rule.keep_firing_for_seconds, cooldown: rule.notification_cooldown_seconds || 0 };
  }
  return null;
}
const duration = (seconds: number) => seconds >= 3600 && seconds % 3600 === 0 ? `${seconds / 3600}${text(" h", "小时")}` : seconds >= 60 && seconds % 60 === 0 ? `${seconds / 60}${text(" min", "分钟")}` : `${seconds}${text(" s", "秒")}`;
export function hostDiskName(mount: string) {
  if (mount === "/System/Volumes/Data") return text("System data disk", "系统数据盘");
  if (mount === "/") return text("System disk", "系统盘");
  if (mount === "/data") return text("Data disk", "数据盘");
  return mount;
}
type Point = [number, number];

function HostTrends({ id, observedAt, enabled, rules }: { id: string; observedAt: number; enabled: boolean; rules: HostAlertRule[] }) {
  const [data, setData] = useState<{ points: Point[][]; end: number; error: boolean } | null>(null);
  useEffect(() => {
    if (!enabled || !observedAt) return;
    const abort = new AbortController(), end = Math.floor(Date.now() / 1000);
    const selector = `{managed_host=${JSON.stringify(id)}}`;
    void Promise.all(specs.map(spec => {
      const query = spec.name === "Disk" ? `max(${spec.metric}${selector})` : spec.metric + selector;
      return api<{ data: { result: { values: [number, string][] }[] } }>(
        `/prometheus/api/v1/query_range?${new URLSearchParams({ query, start: String(end - 3600), end: String(end), step: "30" })}`, { signal: abort.signal },
      ).then(result => (result.data.result[0]?.values || []).slice(-121).map(([at, value]) => [at, Number(value)] as Point).filter(([, value]) => Number.isFinite(value)));
    })).then(points => { if (!abort.signal.aborted) setData({ points, end, error: false }); })
      .catch(() => { if (!abort.signal.aborted) setData(previous => previous ? { ...previous, error: true } : { points: [], end, error: true }); });
    return () => abort.abort();
  }, [id, observedAt, enabled]);
  return <div className="host-trends">{specs.map((spec, index) => {
    const policy = hostAlertPolicy(rules, id, spec.name.toLowerCase()), threshold = policy?.threshold ?? spec.warning;
    const points = data?.points[index] || [];
    const path = points.map(([at, value], i) => `${i && at - points[i - 1][0] <= 90 ? "L" : "M"} ${34 + (at - ((data?.end || 0) - 3600)) / 3600 * 290} ${132 - Math.max(0, Math.min(100, value)) * 1.08}`).join(" ");
    return <section key={spec.name} className="host-trend" aria-label={`${resourceName(spec.name)} ${text("last 60 minutes", "最近60分钟趋势")}`}>
      <h3>{resourceName(spec.name)}<span>{text("Last 60 min", "最近60分钟")}</span></h3>
      {points.length ? <svg viewBox="0 0 340 162" role="img" aria-label={`${resourceName(spec.name)} ${text("usage trend", "使用率趋势")}`}>
        {[0, 25, 50, 75, 100].map(value => <g key={value}><line x1="34" x2="324" y1={132 - value * 1.08} y2={132 - value * 1.08} stroke="#2a3542" /><text x="28" y={136 - value * 1.08} textAnchor="end">{value}%</text></g>)}
        <line x1="34" x2="324" y1={132 - threshold * 1.08} y2={132 - threshold * 1.08} stroke="#edb84b" strokeDasharray="4 4" />
        <path d={path} fill="none" stroke={spec.color} strokeWidth="1.7" vectorEffect="non-scaling-stroke" />
        <text x="34" y="153">{text("60 min ago", "60分钟前")}</text><text x="324" y="153" textAnchor="end">{text("Now", "现在")}</text>
      </svg> : <p className="host-trend-empty">{data?.error ? text("Trend unavailable", "趋势查询暂不可用") : text("Waiting for observations", "等待采样数据")}</p>}
      <p className="host-trend-key">{policy ? text(`Dashed line: alert threshold ${threshold}%`, `黄色虚线：提醒线 ${threshold}%`) : text(`Dashed line: reference ${threshold}%`, `黄色虚线：参考线 ${threshold}%`)}</p>
      {data?.error ? <small role="status">{text("Refresh failed; showing previous data", "刷新失败，保留上次数据")}</small> : null}
    </section>;
  })}</div>;
}

export function HostResources({ id, resources, enabled, interval, rules = [], rulesState = "loading", compact = false, large = false }: { id: string; resources?: HostResourcesData; enabled: boolean; interval: number; rules?: HostAlertRule[]; rulesState?: "loading" | "ready" | "error"; compact?: boolean; large?: boolean }) {
  const [historyOpen, setHistoryOpen] = useState(!compact);
  const hostRules = useMemo(() => rules.filter(rule => rule?.labels?.managed_host === id), [rules, id]);
  const fresh = !!resources && Date.now() - resources.observed_at <= Math.max(90000, interval * 3000);
  const active = enabled && fresh;
  const worst = resources?.disks.reduce<HostResourcesData["disks"][number] | null>((a, b) => !a || b.used_percent > a.used_percent ? b : a, null);
  const values = [resources?.cpu_percent ?? null, resources?.memory_percent ?? null, worst?.used_percent ?? null];
  return <div className={`host-resources${large ? " host-resources-large" : ""}`}>
    {!resources ? <p className="host-resource-note">{text("Host resources appear after updating the collector. Database collection continues independently.", "更新采集器后显示主机资源，数据库采集独立运行。")}</p> : null}
    <div className="host-instruments">{specs.map((spec, i) => {
      const value = values[i];
      const policy = hostAlertPolicy(hostRules, id, spec.name.toLowerCase()), critical = hostAlertPolicy(hostRules, id, spec.name.toLowerCase(), "critical");
      const warning = policy?.threshold ?? spec.warning, serious = critical?.threshold ?? 95;
      const state = value == null ? undefined : value >= serious ? text("Very high now", "当前占用很高") : value >= warning ? text("Elevated now", "当前占用偏高") : i === 2 ? text("Plenty of space", "空间充足") : text("Normal now", "当前占用正常");
      return <section className="host-instrument" key={spec.name}>
        <h3>{i === 0 ? text("CPU · processor load", "CPU 使用率") : i === 1 ? text("Memory · used capacity", "内存使用率") : text("Disk · used space", "磁盘容量占用 · 最满分区")}</h3>
        <p className="host-instrument-description">{i === 0 ? text("Lower means more processing capacity is available", "整台主机的 CPU 占用") : i === 1 ? text("How much memory programs are using", "已用 = 总内存 − 当前可用内存") : text("Shows the local disk closest to being full", "总容量减可用空间，包含保留空间")}</p>
        <RiskGauge value={value} label={`${resourceName(spec.name)} ${text("usage", "使用率")}`} stale={!!resources && !active} steps={[...bands(warning).slice(0, 2), { value: serious, color: "red" }]} caption={state} />
        <div className="host-capacity">{value == null ? <span>{text("No valid capacity observation", "暂无有效容量采样")}</span> : i === 0 ? <><strong>{resources?.cores || "—"} {text("logical cores", "逻辑核")}</strong><span>{active && value != null ? text(`${(100 - value).toFixed(1)}% processing capacity idle`, `当前还有 ${(100 - value).toFixed(1)}% 算力空闲`) : text("Waiting for a fresh observation", "等待新采样后判断")}</span></> : i === 1 && resources ? <><strong>{text("Available", "还可用")} {formatValue(resources.memory_available, "bytes")}</strong><span>{text("Used", "已用")} {formatValue(resources.memory_total - resources.memory_available, "bytes")} / {text("Total", "总共")} {formatValue(resources.memory_total, "bytes")}</span></> : worst ? <><strong>{text("Free", "还剩")} {formatValue(worst.available, "bytes")}</strong><span>{hostDiskName(worst.mountpoint)} · {text("Total", "总共")} {formatValue(worst.total, "bytes")}</span></> : "—"}</div>
        {!active && resources ? <p className="host-sample-warning">{text("Previous capacity observation; refresh before making changes", "容量为上次采样，请恢复采集后再判断")}</p> : null}
      </section>;
    })}</div>
    <details className="host-history" open={historyOpen} onToggle={event => setHistoryOpen(event.currentTarget.open)}><summary>{text("Resource trends · last 60 minutes", "资源趋势 · 最近60分钟")}</summary>{historyOpen && <HostTrends id={id} observedAt={resources?.observed_at || 0} enabled={enabled} rules={hostRules} />}</details>
    <HostNetworkResources id={id} resources={resources} active={active} />
    <div className="host-resource-footer">
      <details className="host-disk-section"><summary>{text("All writable disks", "全部可写磁盘")} · {resources?.disks.length || 0}</summary>
        {resources?.disks.length ? <div className="host-disk-table"><table><thead><tr><th>{text("Disk", "磁盘")}</th><th>{text("Capacity", "总容量")}</th><th>{text("Available", "还剩")}</th><th>{text("Used", "已用比例")}</th></tr></thead>
          <tbody>{resources.disks.map(disk => <tr key={disk.mountpoint + disk.device}><td title={`${disk.mountpoint} · ${disk.device} · ${disk.fstype}`}>{hostDiskName(disk.mountpoint)}<small className="host-disk-mount">{disk.mountpoint}</small></td><td>{formatValue(disk.total, "bytes")}</td><td>{formatValue(disk.available, "bytes")}</td><td><span className={disk.used_percent >= 95 ? "host-risk-critical" : disk.used_percent >= 85 ? "host-risk-warning" : "host-risk-normal"}>{disk.used_percent.toFixed(1)}%</span><progress max="100" value={disk.used_percent} aria-label={hostDiskName(disk.mountpoint)} /></td></tr>)}</tbody></table></div> : <p className="subtle">{text("No valid disk sample", "暂无有效磁盘采样")}</p>}
        {resources ? <div className="host-sampling-summary"><div><span>{text("Host uptime", "主机已运行")}</span><strong>{formatValue(resources.uptime, "uptime")}</strong></div><div><span>{text("Latest observation", "最近采样")}</span><strong>{timeAgo(resources.observed_at)}</strong></div></div> : null}
      </details>
      <details className="host-resource-legend host-alert-explanation"><summary>{text("Resource alerts", "资源告警")}</summary>
        <p>{text("Crossing a line alone does not send an alert; the configured duration must also be met.", "超过提醒线不等于马上报警，还要满足下面的持续时间。")}</p>
        {rulesState !== "ready" ? <p role="status">{rulesState === "error" ? text("Rules unavailable. Open Alerts to verify the current policy.", "暂时无法读取规则，请到告警页核对。") : text("Reading configured alert rules…", "正在读取已配置的告警规则…")}</p> : specs.map(spec => {
          const policy = hostAlertPolicy(hostRules, id, spec.name.toLowerCase());
          const critical = hostAlertPolicy(hostRules, id, spec.name.toLowerCase(), "critical");
          return <div key={spec.name} className="host-alert-rule"><strong>{resourceName(spec.name)}</strong>{policy ? <div><span>{text(`Above ${policy.threshold}% for ${duration(policy.pending)} before notifying`, `超过 ${policy.threshold}%，连续 ${duration(policy.pending)} 才提醒`)}</span><small>{text(`Recovery: below ${policy.recovery}% for ${duration(policy.recovering)}; cooldown ${duration(policy.cooldown)}`, `恢复：低于 ${policy.recovery}%，连续 ${duration(policy.recovering)}；同类提醒冷却 ${duration(policy.cooldown)}`)}</small>{critical ? <small>{text(`Critical: above ${critical.threshold}% for ${duration(critical.pending)}`, `严重：超过 ${critical.threshold}%，连续 ${duration(critical.pending)}`)}</small> : null}</div> : <span>{text("No standard sustained rule; check Alerts for custom or paused rules", "未启用标准持续规则，自定义或暂停规则请到告警页查看")}</span>}</div>;
        })}
        <a className="host-alert-link" href="#alerts">{text("View or change alert rules →", "查看或调整告警规则 →")}</a>
      </details>
    </div>
    <details className="host-reading-details"><summary>{text("Sampling details and technical information", "采样口径与技术信息")}</summary><p>{text("Memory excludes reclaimable pages. Shared APFS capacity is counted once. Green means low current usage, amber elevated, red critical; these colors alone do not mean an alert was sent.", "内存按总量减可用量计算，可回收缓存计入可用。磁盘按已挂载文件系统计算，仪表显示最满分区；云盘扩大后仍需扩展分区和文件系统才会增加这里的容量，另挂磁盘也不会降低原分区比例。APFS 共享空间不重复相加。颜色表示当前用量，不代表已经发送告警。")}</p><p>{resources ? `${text("Observed", "最近采样")} ${timeAgo(resources.observed_at)} · ${text("Host uptime", "主机已运行")} ${formatValue(resources.uptime, "uptime")}` : "—"}</p>{resources?.disks.map(disk => <p key={disk.mountpoint + disk.device}>{hostDiskName(disk.mountpoint)}：{disk.mountpoint} · {disk.device} · {disk.fstype}</p>)}</details>
  </div>;
}
