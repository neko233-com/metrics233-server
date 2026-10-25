import { lazy, Suspense, useState } from "react";
import { formatValue, type Panel } from "../api";
import { nativePanelForSDK } from "../grafana/panel-queries";
import type { TimeSelection } from "../grafana/time-range";
import { getLocale } from "../i18n";
import type { HostResourcesData } from "./HostResources";
import { recommendHostInterface, resolveNetworkChoice, type NetworkChoice } from "./host-network-selection";

const NativePanel = lazy(() => import("../grafana/GrafanaPanel"));
const values = {};
const text = (en: string, zh: string) => getLocale() === "zh" ? zh : en;
export default function HostNetworkResources({ id, resources, active }: { id: string; resources?: HostResourcesData; active: boolean }) {
  const [open, setOpen] = useState(false);
  const [selected, setSelected] = useState<NetworkChoice | null>(null);
  const [range, setRange] = useState<TimeSelection>("1h");
  const devices = resources?.network || [];
  const recommended = recommendHostInterface(devices, active);
  const selection = resolveNetworkChoice(selected, id, devices.map(item => item.device), recommended.device, recommended.hasTraffic, active);
  if (selection !== selected) setSelected(selection);
  const device = selection.device;
  const missing = !!device && !devices.some(item => item.device === device);
  const expr = (direction: string) => `host_network_${direction}_bytes_per_second{managed_host=${JSON.stringify(id)},device=${JSON.stringify(device || "")}}`;
  const title = text("Interface traffic · last hour", "网卡流量 · 最近1小时");
  const panel: Panel = { id: "host-interface-history", metric: "", title, visualization: "timeseries", expr: expr("receive"), unit: "Bps", aggregation: "last", config: { type: "timeseries", targets: ["receive", "transmit"].map((direction, i) => ({ refId: i ? "B" : "A", expr: expr(direction), legendFormat: i ? text("Send", "发送") : text("Receive", "接收"), range: true, datasource: { uid: "metricspanel", type: "prometheus" } })), fieldConfig: { defaults: { unit: "Bps", color: { mode: "palette-classic" } }, overrides: [] } } };
  const rate = (v: number | null | undefined) => !active || v == null ? "—" : formatValue(v, "Bps");
  return <details className="host-history host-network-resources" onToggle={event => setOpen(event.currentTarget.open)}><summary>{text("Network interfaces · receive and send", "网卡流量 · 接收与发送")}</summary>
    <p className="subtle">{text("Counters are per interface. Do not add physical NICs, bridges and tunnels. The first observation establishes a baseline.", "逐张网卡统计，不把物理网卡、网桥和隧道重复相加；首次采样只建立基线。")}</p>
    {devices.length > 0 && <div className="host-disk-table"><table><thead><tr><th>{text("Interface", "网卡")}</th><th>{text("Receive", "接收")}</th><th>{text("Send", "发送")}</th></tr></thead><tbody>{devices.map(n => <tr key={n.device}><td>{n.device}</td><td>{rate(n.receive_per_second)}</td><td>{rate(n.transmit_per_second)}</td></tr>)}</tbody></table></div>}
    {(devices.length > 0 || device) ? <><label className="business-network-device">{text("History interface", "趋势网卡")} <select aria-label={text("History interface", "趋势网卡")} value={selection.manual ? device : ""} onChange={event => setSelected({ scope: id, device: event.target.value, manual: !!event.target.value, pinned: !!event.target.value })}>
      <option value="">{text("Automatic", "自动")} · {(selection.manual ? recommended.device : device) || text("waiting for observations", "等待采样")}</option>
      {missing && selection.manual && <option value={device}>{device} · {text("no current observation", "暂无当前采样")}</option>}
      {devices.map(n => <option key={n.device} value={n.device}>{n.device}</option>)}
    </select></label>{missing && <p role="status" className="host-sample-warning">{text("The selected interface has no current observation. Its history is retained; choose another interface or Automatic.", "选中的网卡暂无当前采样，保留该网卡历史；可选择其他网卡或自动。")}</p>}
    {open && device && <Suspense fallback={<p role="status">{text("Loading chart…", "正在加载图表…")}</p>}><NativePanel panel={nativePanelForSDK(panel)} values={values} range={range} onRange={setRange} tick={resources?.observed_at || 0} /></Suspense>}</> : <p className="subtle">{text("Update the host collector to receive interface observations.", "更新主机采集器后显示网卡观测。")}</p>}
  </details>;
}
