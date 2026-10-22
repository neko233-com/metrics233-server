import { memo } from "react";
import { getLocale } from "../i18n";
import "./RiskGauge.css";

export type GaugeStep = { value: number | null; color: string };
const colors: Record<string, string> = {
  green: "#39c98a", yellow: "#edb84b", orange: "#ed9b48", red: "#f46b73",
  blue: "#72a8ef", "semi-dark-green": "#39c98a", "dark-green": "#39c98a",
};
const colorOf = (color: string) => colors[color] || color;
export function gaugeBands(steps: GaugeStep[], min: number, max: number, percentage = false) {
  const scale = max - min;
  if (!Number.isFinite(scale) || scale <= 0) return [];
  const points = steps.map(step => ({
    start: step.value == null ? 0 : Math.max(0, Math.min(1, percentage ? step.value / 100 : (step.value - min) / scale)),
    color: colorOf(step.color),
  })).filter(step => Number.isFinite(step.start)).sort((a, b) => a.start - b.start);
  if (!points.length || points[0].start > 0) points.unshift({ start: 0, color: "#627487" });
  return points.map((point, i) => ({ ...point, end: points[i + 1]?.start ?? 1 })).filter(band => band.end > band.start);
}
const point = (fraction: number, radius: number) => {
  const angle = (135 + fraction * 270) * Math.PI / 180;
  return [140 + Math.cos(angle) * radius, 132 + Math.sin(angle) * radius];
};
const arc = (start: number, end: number) => {
  const [x, y] = point(start, 110), [ex, ey] = point(end, 110);
  return `M ${x} ${y} A 110 110 0 ${end - start > 2 / 3 ? 1 : 0} 1 ${ex} ${ey}`;
};
const defaultSteps: GaugeStep[] = [{ value: null, color: "green" }, { value: 85, color: "yellow" }, { value: 95, color: "red" }];

/** Native SVG instrument: configured threshold rails, bounded needle and explicit
 * missing/stale state. Same-scope refreshes update the needle without remounting. */
export const RiskGauge = memo(function RiskGauge({ value, label, formatted, min = 0, max = 100,
  steps = defaultSteps, percentage = false, stale = false, caption, valueColor,
}: { value: number | null; label: string; formatted?: string; min?: number; max?: number;
  steps?: GaugeStep[]; percentage?: boolean; stale?: boolean; caption?: string; valueColor?: string;
}) {
  const zh = getLocale() === "zh";
  const valid = !stale && value != null && Number.isFinite(value) && max > min;
  const fraction = valid ? Math.max(0, Math.min(1, (value - min) / (max - min))) : 0;
  const bands = gaugeBands(steps, min, max, percentage);
  const color = valid ? (valueColor || bands.find(band => fraction >= band.start && (fraction < band.end || band.end === 1))?.color || "#8c9bad") : "#8c9bad";
  const shown = valid ? (formatted ?? `${value.toFixed(1)}%`) : "—";
  const status = !valid ? (stale ? (zh ? "采集已过期或暂停" : "Stale or paused") : (zh ? "等待有效采样" : "Awaiting a sample")) : caption;
  return <div className={`risk-gauge ${valid ? "" : "risk-gauge-empty"}`}>
    <svg viewBox="0 0 280 238" role={valid ? "meter" : "img"} aria-label={label}
      aria-valuemin={valid ? min : undefined} aria-valuemax={valid ? max : undefined}
      aria-valuenow={valid ? value : undefined} aria-valuetext={valid ? shown : status}>
      <path d={arc(0, 1)} className="risk-gauge-track" />
      {bands.map((band, i) => <path key={i} d={arc(band.start, band.end)} fill="none" stroke={valid ? band.color : "#394654"} strokeWidth="6" />)}
      {Array.from({ length: 41 }, (_, i) => {
        const [x1, y1] = point(i / 40, i % 10 === 0 ? 92 : 98), [x2, y2] = point(i / 40, 103);
        return <line key={i} x1={x1} y1={y1} x2={x2} y2={y2} className={i % 10 === 0 ? "risk-gauge-major" : "risk-gauge-tick"} />;
      })}
      {[0, .25, .5, .75, 1].map(f => { const [x, y] = point(f, 77); return <text key={f} x={x} y={y} dy=".35em" className="risk-gauge-scale">{Number((min + f * (max - min)).toPrecision(3))}</text>; })}
      {valid ? <g className="risk-gauge-needle" style={{ transform: `rotate(${135 + fraction * 270}deg)` }}>
        <path d="M 133 132 L 140 128 L 225 132 L 140 136 Z" fill="#e4edf5" />
      </g> : null}
      <circle cx="140" cy="132" r="4" fill={valid ? "#e4edf5" : "#627487"} />
      <text x="140" y="192" className="risk-gauge-value" fill={color}>{shown}</text>
      {status ? <text x="140" y="220" className="risk-gauge-state" fill={color}>{status}</text> : null}
    </svg>
  </div>;
});
