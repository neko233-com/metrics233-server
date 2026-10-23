import { lazy, Suspense, useEffect, useState } from "react";
import { api, message, type Dashboard, type Panel } from "../api";
import { nativePanelForSDK } from "../grafana/panel-queries";
import type { TimeSelection } from "../grafana/time-range";
import { getLocale } from "../i18n";
import { Alert } from "../ui";

const NativePanel = lazy(() => import("../grafana/GrafanaPanel"));
const text = (en: string, zh: string) => getLocale() === "zh" ? zh : en;
type NetworkView = { business: Dashboard; host?: Dashboard; devices: string[]; device: string; host_name: string; host_stale: boolean };
const networkValues = {};
function NetworkPanels({ panels, tick }: { panels: Panel[]; tick: number }) {
  const [range, setRange] = useState<TimeSelection>("1h");
  const render = (panel: Panel) => <NativePanel key={panel.id} panel={nativePanelForSDK(panel)} values={networkValues} range={panel.visualization === "stat" ? "1h" : range} onRange={setRange} tick={tick} />;
  return <Suspense fallback={<p role="status">{text("Loading charts…", "正在加载图表…")}</p>}>
    <div className="business-instance-stats">{panels.filter(panel => panel.visualization === "stat" && !panel.id.includes("volume")).map(render)}</div>
    <div className="business-instance-stats network-volume">{panels.filter(panel => panel.id.includes("volume")).map(render)}</div>
    <div className="business-network-history">{panels.filter(panel => panel.visualization === "timeseries").map(render)}</div>
  </Suspense>;
}
export default function BusinessNetwork({ id, tick }: { id: string; tick: number }) {
  const [view, setView] = useState<NetworkView | null>(null);
  const [device, setDevice] = useState(""); const [error, setError] = useState("");
  useEffect(() => {
    const controller = new AbortController();
    void api<NetworkView>(`/business/instances/${id}/network?language=${getLocale()}&device=${encodeURIComponent(device)}`, { signal: controller.signal })
      .then(value => { if (!controller.signal.aborted) { setView(value); setError(""); } })
      .catch(cause => { if (!controller.signal.aborted) setError(message(cause)); });
    return () => controller.abort();
  }, [id, tick, device]);
  return <div className="business-network-view">
    {error && <Alert severity="error" title={text("Network data unavailable", "网络数据暂时无法读取")}>{error}</Alert>}
    {view && <><section className="business-network-section" aria-label={text("Service traffic", "业务流量")}><header><div><h2>{text("Service traffic", "业务流量")}</h2><p>{text("Current service only; excludes other host processes. Missing data stays unknown.", "仅当前服务业务字节，不含整机其他进程；无采样时显示未知。")}</p></div><span className="subtle">{text("Last 60 minutes", "最近60分钟")}</span></header><NetworkPanels panels={view.business.panels} tick={tick} /></section>
      <section className="business-network-section" aria-label={text("Host interface traffic", "主机网卡流量")}><header><div><h2>{text("Host interface traffic", "所在主机 · 网卡流量")}{view.host_name ? ` · ${view.host_name}` : ""}</h2><p>{text("One interface at a time. Never add physical, bridge and tunnel counters together.", "单张网卡口径，不把物理网卡、网桥和隧道重复相加。")}</p></div>{view.devices.length > 0 && <label className="business-network-device">{text("Interface", "网卡")}<select aria-label={text("Network interface", "网络接口")} value={view.devices.includes(device) ? device : view.device} onChange={event => setDevice(event.target.value)}>{view.devices.map(name => <option key={name} value={name}>{name}</option>)}</select></label>}</header>
        {view.host_stale && <p className="host-sample-warning">{text("Host collection is paused or stale; current rates are hidden.", "主机采集暂停或数据已过期，隐藏当前速率，保留历史趋势。")}</p>}
        {view.host ? <NetworkPanels panels={view.host.panels} tick={tick} /> : <p className="subtle">{text("Associate a host and update its collector to enable interface traffic.", "关联实际主机并更新采集器后显示网卡流量；主机指标采集需要启用 netdev。")}</p>}
      </section></>}
  </div>;
}
