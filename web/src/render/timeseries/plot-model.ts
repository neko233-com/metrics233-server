import { getFieldDisplayName, getFieldSeriesColor, type DataFrame, type Field } from "@grafana/data";
import type uPlot from "uplot";
import { getLocale } from "../../i18n";
import { nextFrame } from "../frame-budget";
import { seriesPresentations, type SeriesInput, type PresentationLocale } from "../series-presentation";
import { prepareNumericModel, type NumericRequest, type NumericResult, type NumericSource } from "./numeric-model";
import { riskColor, riskTheme as theme } from "./risk-thresholds";

export { riskTheme as theme } from "./risk-thresholds";
const COLORS = ["#5794f2", "#73bf69", "#ff9830", "#f2495c", "#b877d9", "#56a3a6", "#fade2a"];
export const WORKER_CELL_THRESHOLD = 50_000;
export type PlotSeries = {
  id: string; name: string; rawName: string; description: string; field: Field; values: (number | null)[];
  lastValue: number | null; color: string; dashed: boolean;
  axis: "y" | "y2"; axisLabel?: string; log: boolean;
  stack: "none" | "normal" | "percent"; stackGroup: string; defaultVisible: boolean;
  riskKey?:string;
};
type Source = { time: Field; values: Field[]; shift: number };
export type PlotSpec = { sources: Source[]; series: Omit<PlotSeries, "values" | "lastValue">[]; signature: string; cells: number };
export type PlotModel = {
  data: uPlot.AlignedData; series: PlotSeries[]; signature: string;
  inputRows: number; alignedRows: number; preparation: "sync" | "worker" | "fallback";
};
export const EMPTY_PLOT: PlotModel = { data: [[]], series: [], signature: "", inputRows: 0, alignedRows: 0, preparation: "sync" };

/** Display metadata is small and stays on the UI thread. Source buffers and
 * cached display state are never mutated by numeric normalization. */
export function describePlot(frames: DataFrame[], locale: PresentationLocale = getLocale(), fallbackTitle?: string): PlotSpec {
  const sources: Source[] = [], series: PlotSpec["series"] = [], presentations: SeriesInput[] = [];
  const occurrences = new Map<string, number>();
  // The SDK checks field identity within its frame when disambiguating names.
  // Clone the field containers together, while sharing read-only sample arrays.
  const metadataFrames = frames.map((frame) => ({ ...frame, fields: frame.fields.map((field) =>
    ({ ...field, state: field.state ? { ...field.state,
      calcs: field.state.calcs ? { ...field.state.calcs } : undefined } : undefined })) }));
  let cells = 0;
  metadataFrames.forEach((frame, frameIndex) => {
    const time = frame.fields.find((field) => field.type === "time");
    if (!time) return;
    const values: Field[] = [];
    const comparison = frame.meta?.timeCompare;
    frame.fields.forEach((field, fieldIndex) => {
      if (field.type !== "number" && field.type !== "enum") return;
      values.push(field);
      const customName = field.config.displayName || field.config.displayNameFromDS;
      let displayName = customName ? getFieldDisplayName(field, frame, metadataFrames) : undefined;
      if (displayName && comparison?.isTimeShiftQuery) {
        // Grafana adds one English suffix even to an explicit SQL/field alias.
        // Remove only that SDK addition; an author's own suffix is part of the
        // alias body. Cached unadorned aliases may equal their configured text.
        const alias = displayName !== customName ? displayName.replace(/ \(comparison\)$/, "") : displayName;
        displayName = alias + (locale === "zh" ? " (比较)" : " (Comparison)");
      }
      const fieldName = comparison?.isTimeShiftQuery
        ? field.name.replace(/\s*\((?:comparison|比较)\)$/i, "") : field.name;
      presentations.push({
        metric: field.labels?.__name__ || (/^[a-zA-Z_:][a-zA-Z0-9_:]*$/.test(fieldName) && fieldName !== "Value" ? fieldName : undefined),
        labels: field.labels, fallbackName: fieldName === "Value" && fallbackTitle?.trim() ? fallbackTitle : fieldName,
        displayName,
        reference: frame.refId || String(frameIndex + 1), comparison: comparison?.isTimeShiftQuery === true,
      });
      // Visibility, colors and point identity must survive a locale/legend change.
      const identity = JSON.stringify([frame.refId || String(frameIndex + 1), fieldIndex, field.name,
        Object.keys(field.labels || {}).sort().map(key => [key, field.labels![key]]), comparison?.isTimeShiftQuery === true]);
      const occurrence = occurrences.get(identity) || 0;
      occurrences.set(identity, occurrence + 1);
      let color = COLORS[series.length % COLORS.length];
      try { color = getFieldSeriesColor(field, theme).color || color; } catch { /* Plugin metadata is optional. */ }
      const custom = (field.config.custom || {}) as {
        axisPlacement?: string; axisLabel?: string; scaleDistribution?: { type?: string };
        stacking?: { mode?: string; group?: string }; hideFrom?: { viz?: boolean };
      };
      const stack = custom.stacking?.mode;
      series.push({
        id: identity + ":" + occurrence, name: "", rawName: "", description: "", field, color,
        dashed: comparison?.isTimeShiftQuery === true,
        axis: custom.axisPlacement === "right" ? "y2" : "y", axisLabel: custom.axisLabel,
        log: custom.scaleDistribution?.type === "log",
        stack: stack === "percent" ? "percent" : stack === "normal" ? "normal" : "none",
        stackGroup: custom.stacking?.group || "default", defaultVisible: !custom.hideFrom?.viz,
        riskKey:JSON.stringify([field.config.thresholds,field.config.color?.mode,field.config.min,field.config.max]),
      });
    });
    if (!values.length) return;
    const shift = comparison?.isTimeShiftQuery && Number.isFinite(comparison.diffMs) ? Math.abs(comparison.diffMs!) : 0;
    sources.push({ time, values, shift });
    cells += time.values.length * (values.length + 1);
  });
  seriesPresentations(presentations, locale).forEach((item, index) => {
    series[index].name = item.name;
    series[index].rawName = item.rawName;
    series[index].description = item.description;
  });
  const signature = JSON.stringify(series.map(({ field: _field, ...item }) => item));
  return { sources, series, signature, cells };
}

function numericValue(value: unknown, shift = 0): number {
  if (value == null) return NaN;
  const number = value instanceof Date ? value.getTime() : Number(value);
  return Number.isFinite(number) ? number + shift : NaN;
}

function allocateSources(spec: PlotSpec) {
  return spec.sources.map((source) => ({
    times: new Float64Array(source.time.values.length),
    columns: source.values.map(() => new Float64Array(source.time.values.length)),
  }));
}

function packChunk(source: Source, target: NumericSource, column: number, from: number, to: number) {
  const values = column === 0 ? source.time.values : source.values[column - 1].values;
  const output = column === 0 ? target.times : target.columns[column - 1];
  const shift = column === 0 ? source.shift : 0;
  for (let row = from; row < to; row += 1) output[row] = numericValue(values[row], shift);
}

export function packPlot(spec: PlotSpec): NumericSource[] {
  const packed = allocateSources(spec);
  spec.sources.forEach((source, index) => {
    for (let column = 0; column <= source.values.length; column += 1)
      packChunk(source, packed[index], column, 0, source.time.values.length);
  });
  return packed;
}

/** Copy in 8 ms slices before transferring. A source DataFrame remains usable
 * by other panels and cannot be detached or mutated by a stale Worker. */
export async function packPlotInSlices(spec: PlotSpec, signal: AbortSignal): Promise<NumericSource[]> {
  const packed = allocateSources(spec);
  let started = performance.now();
  for (let index = 0; index < spec.sources.length; index += 1) {
    const source = spec.sources[index];
    for (let column = 0; column <= source.values.length; column += 1) {
      for (let row = 0; row < source.time.values.length; row += 8192) {
        if (signal.aborted) throw new DOMException("aborted", "AbortError");
        packChunk(source, packed[index], column, row, Math.min(row + 8192, source.time.values.length));
        if (performance.now() - started >= 8) {
          await nextFrame();
          started = performance.now();
        }
      }
    }
  }
  return packed;
}

export function completePlot(spec: PlotSpec, result: NumericResult, preparation: PlotModel["preparation"]): PlotModel {
  const x = Array.from(result.columns[0]);
  const series = spec.series.map((item, index) => ({
    ...item,
    values: Array.from(result.columns[index + 1], (value) => Number.isFinite(value) ? value : null),
    lastValue: Number.isFinite(result.lastValues[index]) ? result.lastValues[index] : null,
    color:riskColor(item.field,result.lastValues[index])||item.color,
  }));
  return { data: [x, ...series.map((item) => item.values)], series, signature: spec.signature,
    inputRows: result.inputRows, alignedRows: result.alignedRows, preparation };
}

export function preparePlotSync(spec: PlotSpec, options: Omit<NumericRequest, "sources" | "series">): PlotModel {
  return completePlot(spec, prepareNumericModel({ sources: packPlot(spec), series: spec.series, ...options }), "sync");
}
