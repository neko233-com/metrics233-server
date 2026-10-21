import { useState } from "react";
import { Info } from "lucide-react";
import { Dialog } from "../components/Dialog";
import { getLocale, t } from "../i18n";
import type { Panel } from "../api";
import "./MetricDetails.css";

export type MetricDetail = {
  id: string;
  name: string;
  metric?: string;
  description?: string;
  rawName?: string;
  labels?: Readonly<Record<string, string>>;
  unit?: string;
  timestamp?: number;
};

export function dataPointTime(values: ArrayLike<unknown>, times?: ArrayLike<unknown>): number | undefined {
  if (!times) return undefined;
  for (let index = Math.min(values.length, times.length) - 1; index >= 0; index--) {
    if (values[index] == null || !Number.isFinite(Number(values[index]))) continue;
    const value = times[index] instanceof Date ? (times[index] as Date).getTime() : Number(times[index]);
    if (Number.isFinite(value) && value > 0) return value;
  }
  return undefined;
}

export function pointTimeText(timestamp: number | undefined, timezone = "browser") {
  return timestamp === undefined ? "—" : new Date(timestamp).toLocaleString(getLocale() === "zh" ? "zh-CN" : "en-GB", {
    timeZone: timezone === "browser" ? undefined : timezone === "utc" ? "UTC" : timezone,
    hour12: false, timeZoneName: "short",
  });
}

const labelNames: Record<string, string> = {
  __name__: "Metric name", instance: "Instance identifier", job: "Collection job",
  serverId: "Server ID", server_id: "Server ID", role: "Service role", service: "Service",
  channel: "Channel", environment: "Environment", mountpoint: "Mount point", device: "Device",
  reported_ip: "Reported IP", source_ip: "Source IP label",
};
const reductions: Record<string, string> = {
  lastNotNull: "Latest available value", last: "Last value", mean: "Average in selected range",
  sum: "Sum in selected range", min: "Minimum in selected range", max: "Maximum in selected range",
  count: "Sample count", first: "First value", firstNotNull: "First available value",
  allValues: "Each returned value",
};
const units: Record<string, string> = { uptime: "Elapsed time", bytes: "Bytes (automatic scale)",
  percent: "Percentage (0–100)", percentunit: "Percentage (0–1)", count: "Count",
  short: "Number (compact)", none: "Number (no unit)",
  seconds: "Seconds", s: "Seconds", ms: "Milliseconds", ops: "Operations per second" };

/** Details are built only when opened; the default panel never dumps internal labels. */
export function MetricDetailsButton({ panel, items, reducer, timezone }: {
  panel: Panel; items: MetricDetail[]; reducer?: string; timezone?: string;
}) {
  const [open, setOpen] = useState(false);
  const [selected, setSelected] = useState("");
  const item = items.find((value) => value.id === selected) || items[0];
  const queries = panel.config?.targets?.filter(target => !target.hide).map(target => target.expr).filter(Boolean)
    || [];
  const query = queries.length ? queries.join("\n\n") : panel.expressions?.length ? panel.expressions.join("\n\n") : panel.expr || panel.metric;
  return <>
    <button type="button" className="icon-button metric-details-button"
      title={t("Metric details")} aria-label={`${t("Metric details")} · ${t(panel.title)}`}
      onClick={() => setOpen(true)}><Info size={16} /></button>
    {open ? <Dialog title={`${t("Metric details")} · ${t(panel.title)}`} onClose={() => setOpen(false)}>
      <div className="metric-details">
        {items.length > 1 ? <label>{t("Data series")}
          <select value={item?.id || ""} onChange={event => setSelected(event.target.value)}>
            {items.map(value => <option key={value.id} value={value.id}>{value.name}</option>)}
          </select>
        </label> : null}
        <h3>{item?.name || t(panel.title)}</h3>
        {item?.description ? <p>{item.description}</p> : null}
        <dl className="metric-details-summary">
          <dt>{t("Metric name")}</dt><dd className="mono">{item?.metric || t("Query result")}</dd>
          <dt>{t("Unit")}</dt><dd>{t(units[item?.unit || panel.unit] || item?.unit || panel.unit || "Not configured")}</dd>
          {reducer ? <><dt>{t("Displayed value")}</dt><dd>{t(reductions[reducer] || reducer)}</dd></> : null}
          <dt>{t("Data point time")}</dt><dd>{pointTimeText(item?.timestamp, timezone)}</dd>
        </dl>
        <p className="subtle">{t("Data point time is the query result timestamp, not the original collection time. Range statistics follow the selected time range.")}</p>
        {query ? <><h4>{t("Query")}</h4><pre>{query}</pre></> : null}
        <h4>{t("Raw labels")}</h4>
        <p className="subtle">{t("Labels identify the source and dimensions. They are diagnostic metadata, not an error message.")}</p>
        {Object.keys(item?.labels || {}).length ? <dl className="metric-details-labels">
          {Object.entries(item?.labels || {}).sort(([a], [b]) => a.localeCompare(b)).map(([key, value]) =>
            <div key={key}><dt>{labelNames[key] ? t(labelNames[key]) : key}<code>{key}</code></dt><dd>{value}</dd></div>)}
        </dl> : <p className="subtle">{t("This result has no source labels, for example after aggregation.")}</p>}
        {item?.rawName ? <details><summary>{t("Original series name")}</summary><pre>{item.rawName}</pre></details> : null}
        <div className="dialog-actions"><button type="button" className="button" onClick={() => setOpen(false)}>{t("Close")}</button></div>
      </div>
    </Dialog> : null}
  </>;
}
