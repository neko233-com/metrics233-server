import { lazy, Suspense, useEffect, useRef, useState } from "react";
import { api, message, type Dashboard, type Panel } from "../api";
import { nativePanelForSDK } from "../grafana/panel-queries";
import type { TimeSelection } from "../grafana/time-range";
import { getLocale } from "../i18n";
import { Alert } from "../ui";
import { resolveNetworkChoice, type NetworkChoice } from "./host-network-selection";

const NativePanel = lazy(() => import("../grafana/GrafanaPanel"));
const text = (en: string, zh: string) => getLocale() === "zh" ? zh : en;
type NetworkView = { business: Dashboard; host?: Dashboard; devices: string[]; device: string; host_name: string; host_stale: boolean; recommended_device?: string; recommendation_has_traffic?: boolean };
const networkValues = {};
function NetworkPanels({ panels, tick, range, setRange }: { panels: Panel[]; tick: number; range: TimeSelection; setRange: (range: TimeSelection) => void }) {
  const render = (panel: Panel) => <NativePanel key={panel.id} panel={nativePanelForSDK(panel)} values={networkValues} range={panel.visualization === "stat" ? "1h" : range} onRange={setRange} tick={tick} />;
  return <Suspense fallback={<p role="status">{text("Loading charts…", "正在加载图表…")}</p>}>
    <div className="business-instance-stats">{panels.filter(panel => panel.visualization === "stat" && !panel.id.includes("volume")).map(render)}</div>
    <div className="business-instance-stats network-volume">{panels.filter(panel => panel.id.includes("volume")).map(render)}</div>
    <div className="business-network-history">{panels.filter(panel => panel.visualization === "timeseries").map(render)}</div>
  </Suspense>;
}
export default function BusinessNetwork({ id, tick }: { id: string; tick: number }) {
  const [result, setResult] = useState<{ id: string; view: NetworkView } | null>(null);
  const [choice, setChoice] = useState({ id, device: "" });
  const automatic = useRef<NetworkChoice | null>(null);
  const [retry, setRetry] = useState(0);
  const [serviceRange, setServiceRange] = useState<TimeSelection>("1h"), [hostRange, setHostRange] = useState<TimeSelection>("1h");
  const [failure, setFailure] = useState<{ id: string; message: string } | null>(null);
  const error = failure?.id === id ? failure.message : "";
  const locale = getLocale();
  const device = choice.id === id ? choice.device : "";
  const view = result?.id === id ? result.view : null;
  useEffect(() => {
    const controller = new AbortController();
    const pinned = automatic.current?.scope === id && automatic.current.pinned ? automatic.current.device : "";
    void api<NetworkView>(`/business/instances/${id}/network?language=${locale}&device=${encodeURIComponent(device || pinned)}`, { signal: controller.signal })
      .then(value => { if (!controller.signal.aborted) {
        if (!device) {
          automatic.current = resolveNetworkChoice(automatic.current, id, value.devices, value.recommended_device || value.device, value.recommendation_has_traffic !== false, !value.host_stale);
          if (automatic.current.device && automatic.current.device !== value.device) setRetry(count => count + 1);
        }
        setResult({ id, view: value }); setFailure(null);
      } })
      .catch(cause => { if (!controller.signal.aborted) setFailure({ id, message: message(cause) }); });
    return () => controller.abort();
  }, [id, tick, device, retry, locale]);
  const expectedDevice = device || (automatic.current?.scope === id ? automatic.current.device : "");
  const changing = !!view && !!expectedDevice && view.device !== expectedDevice;
  const missing = !!view?.device && !view.devices.includes(view.device);
  return <div className="business-network-view">
    {error && <Alert severity="error" title={text("Network data unavailable", "网络数据暂时无法读取")}>{error}</Alert>}
    {!view && !error && <p role="status">{text("Loading network observations…", "正在读取网络流量…")}</p>}
    {view && <><section className="business-network-section" aria-label={text("Service traffic", "业务流量")}><header><div><h2>{text("Service traffic", "业务流量")}</h2><p>{text("Current service only; excludes other host processes. Missing data stays unknown.", "仅当前服务业务字节，不含整机其他进程；无采样时显示未知。")}</p></div><span className="subtle">{text("Last 60 minutes", "最近60分钟")}</span></header><NetworkPanels panels={view.business.panels} tick={tick} range={serviceRange} setRange={setServiceRange} /></section>
      <section className="business-network-section" aria-label={text("Host interface traffic", "主机网卡流量")}><header><div><h2>{text("Host interface traffic", "所在主机 · 网卡流量")}{view.host_name ? ` · ${view.host_name}` : ""}</h2><p>{text("One interface at a time. Never add physical, bridge and tunnel counters together.", "单张网卡口径，不把物理网卡、网桥和隧道重复相加。")}</p></div>{(view.devices.length > 0 || view.device) && <label className="business-network-device">{text("Interface", "网卡")}<select aria-label={text("Network interface", "网络接口")} value={device} onChange={event => { automatic.current = null; setChoice({ id, device: event.target.value }); }}>
        <option value="">{text("Automatic", "自动")} · {(device ? view.recommended_device : expectedDevice) || view.device || text("waiting for observations", "等待采样")}</option>
        {missing && device && <option value={device}>{device} · {text("no current observation", "暂无当前采样")}</option>}
        {view.devices.map(name => <option key={name} value={name}>{name}</option>)}
      </select></label>}</header>
        {changing ? <p role="status">{text("Loading selected interface…", "正在加载所选网卡…")}</p> : <>{missing ? <p role="status" className="host-sample-warning">{text("The selected interface has no current observation. Its history is retained; choose another interface or Automatic.", "选中的网卡暂无当前采样，保留该网卡历史；可选择其他网卡或自动。")}</p> : view.host_stale && <p className="host-sample-warning">{text("Host collection is paused or stale; current rates are hidden.", "主机采集暂停或数据已过期，隐藏当前速率，保留历史趋势。")}</p>}
        {view.host ? <NetworkPanels panels={view.host.panels} tick={tick} range={hostRange} setRange={setHostRange} /> : <p className="subtle">{text("Associate a host and update its collector to enable interface traffic.", "关联实际主机并更新采集器后显示网卡流量；主机指标采集需要启用 netdev。")}</p>}</>}
      </section></>}
  </div>;
}
