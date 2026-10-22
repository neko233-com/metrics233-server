import { useEffect, useState } from "react";
import { api, formatValue, timeAgo } from "../api";
import { getLocale } from "../i18n";
import { RiskGauge, type GaugeStep } from "../ui/RiskGauge";

export type HostResourcesData = {
  observed_at: number; cpu_percent: number | null; cores: number; memory_percent: number | null;
  memory_total: number; memory_available: number; uptime: number;
  disks: { device: string; mountpoint: string; fstype: string; total: number; available: number; used_percent: number }[];
};
const text = (en: string, zh: string) => getLocale() === "zh" ? zh : en;
const specs = [
  { name: "CPU", metric: "host_cpu_usage_percent", warning: 85, color: "#39c98a" },
  { name: "Memory", metric: "host_memory_used_percent", warning: 90, color: "#59c5b9" },
  { name: "Disk", metric: "host_disk_used_percent", warning: 85, color: "#72a8ef" },
];
const resourceName = (name: string) => name === "CPU" ? "CPU" : name === "Memory" ? text("Memory", "内存") : text("Disk", "磁盘");
const bands = (warning: number): GaugeStep[] => [{ value: null, color: "green" }, { value: warning, color: "yellow" }, { value: 95, color: "red" }];
type Point = [number, number];

function HostTrends({ id, observedAt, enabled }: { id: string; observedAt: number; enabled: boolean }) {
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
    const points = data?.points[index] || [];
    const path = points.map(([at, value], i) => `${i && at - points[i - 1][0] <= 90 ? "L" : "M"} ${34 + (at - ((data?.end || 0) - 3600)) / 3600 * 290} ${132 - Math.max(0, Math.min(100, value)) * 1.08}`).join(" ");
    return <section key={spec.name} className="host-trend" aria-label={`${resourceName(spec.name)} ${text("last 60 minutes", "最近60分钟趋势")}`}>
      <h3>{resourceName(spec.name)}<span>{text("Last 60 min", "最近60分钟")}</span></h3>
      {points.length ? <svg viewBox="0 0 340 162" role="img" aria-label={`${resourceName(spec.name)} ${text("usage trend", "使用率趋势")}`}>
        {[0, 25, 50, 75, 100].map(value => <g key={value}><line x1="34" x2="324" y1={132 - value * 1.08} y2={132 - value * 1.08} stroke="#2a3542" /><text x="28" y={136 - value * 1.08} textAnchor="end">{value}%</text></g>)}
        <line x1="34" x2="324" y1={132 - spec.warning * 1.08} y2={132 - spec.warning * 1.08} stroke="#edb84b" strokeDasharray="4 4" />
        <path d={path} fill="none" stroke={spec.color} strokeWidth="1.7" vectorEffect="non-scaling-stroke" />
        <text x="34" y="153">{text("60 min ago", "60分钟前")}</text><text x="324" y="153" textAnchor="end">{text("Now", "现在")}</text>
      </svg> : <p className="host-trend-empty">{data?.error ? text("Trend unavailable", "趋势查询暂不可用") : text("Waiting for observations", "等待采样数据")}</p>}
      {data?.error ? <small role="status">{text("Refresh failed; showing previous data", "刷新失败，保留上次数据")}</small> : null}
    </section>;
  })}</div>;
}

export function HostResources({ id, resources, enabled, interval }: { id: string; resources?: HostResourcesData; enabled: boolean; interval: number }) {
  const fresh = !!resources && Date.now() - resources.observed_at <= Math.max(90000, interval * 3000);
  const active = enabled && fresh;
  const worst = resources?.disks.reduce<HostResourcesData["disks"][number] | null>((a, b) => !a || b.used_percent > a.used_percent ? b : a, null);
  const values = [resources?.cpu_percent ?? null, resources?.memory_percent ?? null, worst?.used_percent ?? null];
  return <div className="host-resources">
    {!resources ? <p className="host-resource-note">{text("Host resources appear after updating the collector. Database collection continues independently.", "更新采集器后显示主机资源，数据库采集独立运行。")}</p> : null}
    <div className="host-instruments">{specs.map((spec, i) => {
      const value = values[i];
      const state = value == null ? undefined : value >= 95 ? text("Critical", "严重") : value >= spec.warning ? text("Elevated", "偏高") : text("Normal", "正常");
      return <section className="host-instrument" key={spec.name}>
        <h3>{resourceName(spec.name)} {text("usage", "使用率")}</h3>
        <RiskGauge value={value} label={`${resourceName(spec.name)} ${text("usage", "使用率")}`} stale={!!resources && !active} steps={bands(spec.warning)} caption={state} />
        <p className="host-capacity">{i === 0 ? `${resources?.cores || "—"} ${text("logical cores", "逻辑核")}` : i === 1 && resources ? <>{text("Used", "已用")} {formatValue(resources.memory_total - resources.memory_available, "bytes")} / {formatValue(resources.memory_total, "bytes")}</> : worst ? <>{worst.mountpoint} · {text("Free", "可用")} {formatValue(worst.available, "bytes")}</> : "—"}</p>
      </section>;
    })}</div>
    <HostTrends id={id} observedAt={resources?.observed_at || 0} enabled={enabled} />
    <div className="host-resource-footer">
      <section className="host-disk-section"><h3>{text("Writable disks", "可写磁盘")}</h3>
        {resources?.disks.length ? <div className="host-disk-table"><table><thead><tr><th>{text("Mount", "挂载点")}</th><th>{text("Capacity", "总容量")}</th><th>{text("Available", "可用")}</th><th>{text("Used", "使用率")}</th></tr></thead>
          <tbody>{resources.disks.map(disk => <tr key={disk.mountpoint + disk.device}><td title={`${disk.device} · ${disk.fstype}`}>{disk.mountpoint}</td><td>{formatValue(disk.total, "bytes")}</td><td>{formatValue(disk.available, "bytes")}</td><td><span className={disk.used_percent >= 95 ? "host-risk-critical" : disk.used_percent >= 85 ? "host-risk-warning" : "host-risk-normal"}>{disk.used_percent.toFixed(1)}%</span><progress max="100" value={disk.used_percent} aria-label={disk.mountpoint} /></td></tr>)}</tbody></table></div> : <p className="subtle">{text("No valid disk sample", "暂无有效磁盘采样")}</p>}
      </section>
      <section className="host-resource-legend"><h3>{text("Reading this view", "查看口径")}</h3>
        <p>{text("Memory excludes reclaimable pages. Disk is the busiest writable local volume; APFS shared capacity is not summed.", "内存扣除可回收页面；磁盘取可写本地卷的最高占用，APFS 共享容量不相加。")}</p>
        <p>{text("Reference bands: CPU/disk 85%, memory 90%, critical 95%. Notification timing is configured in Alerts.", "参考线：CPU/磁盘85%、内存90%、严重95%。通知条件与持续时间以告警规则为准。")}</p>
        <span>{resources ? `${text("Observed", "最近采样")} ${timeAgo(resources.observed_at)} · ${text("Host uptime", "主机运行")} ${formatValue(resources.uptime, "uptime")}` : "—"}</span>
      </section>
    </div>
  </div>;
}
