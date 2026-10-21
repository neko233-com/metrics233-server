import {
  useCallback,
  useDeferredValue,
  useEffect,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import {
  formattedValueToString,
  getDisplayProcessor,
  type Field,
} from "@grafana/data";
import uPlot from "uplot";
import "uplot/dist/uPlot.min.css";
import { Pencil, Trash2 } from "lucide-react";
import { AnnotationEditor } from "../../components/AnnotationEditor";
import { getLocale, t } from "../../i18n";
import { annotationStateLabel, type Annotation } from "../../grafana/annotations";
import { resolveTimeRange } from "../../grafana/time-range";
import {
  annotationGeometry,
  changeSeriesVisibility,
  timeSeriesLegend,
  timeSeriesTooltip,
  timeSeriesZoomSelection,
} from "../../grafana/time-series";
import type { RendererProps } from "../types";
import { theme } from "./plot-model";
import { usePlotModel } from "./use-plot-model";
import { drawRiskBands, riskSteps, riskStroke } from "./risk-thresholds";
import { dataPointTime, MetricDetailsButton } from "../MetricDetails";
import "./TimeSeriesRenderer.css";

// uPlot and browser tests read pixels repeatedly. Mark 2D contexts so Chrome
// uses a readback-friendly backing store instead of emitting a warning.
if (typeof HTMLCanvasElement !== "undefined") {
  const prototype = HTMLCanvasElement.prototype as typeof HTMLCanvasElement.prototype & {
    __metricspanelReadFrequently?: boolean;
  };
  if (!prototype.__metricspanelReadFrequently) {
    const original = prototype.getContext;
    prototype.getContext = function (this: HTMLCanvasElement, ...args: any[]) {
      if (args[0] === "2d") {
        return original.call(this, "2d", {
          ...(args[1] || {}),
          willReadFrequently: true,
        }) as any;
      }
      return original.apply(this, args as any) as any;
    } as typeof prototype.getContext;
    prototype.__metricspanelReadFrequently = true;
  }
}

type CursorState = { index: number; left: number; top: number };
type PlotBounds = {
  left: number;
  top: number;
  width: number;
  height: number;
  start: number;
  end: number;
};

type Props = RendererProps;

function formatValue(field: Field, value: unknown, timezone: string): string {
  if (value == null) return "—";
  try {
    const display =
      field.display ||
      getDisplayProcessor({ field, timeZone: timezone, theme });
    return formattedValueToString(display(value));
  } catch {
    return String(value);
  }
}

function AnnotationOverlay({
  bounds,
  events,
  timezone,
}: {
  bounds?: PlotBounds;
  events: Annotation[];
  timezone: string;
}) {
  if (!bounds || !events.length) return null;
  return (
    <svg
      className="grafana-timeseries-annotations native-timeseries-annotations"
      width={bounds.width}
      height={bounds.height}
      style={{ left: bounds.left, top: bounds.top }}
      aria-label={t("Annotations")}
    >
      {events.map((event, index) => {
        const geometry = annotationGeometry(event, bounds.start, bounds.end, bounds.width,
          (time) => (time - bounds.start) / (bounds.end - bounds.start) * bounds.width);
        if (!geometry) return null;
        const color = event.color || "#5794f2";
        const date = new Date(event.time).toLocaleString(undefined, {
          timeZone: timezone === "browser" ? undefined : timezone === "utc" ? "UTC" : timezone,
        });
        const title = event.text +
          (event.newState
            ? "\n" + t("Alert state") + ": " +
              annotationStateLabel(event.prevState || "Normal", t) + " → " +
              annotationStateLabel(event.newState, t)
            : "") +
          "\n" + event.tags.join(", ") + "\n" + date;
        return (
          <g
            key={event.key || event.id || index}
            className="annotation-marker"
            data-annotation-key={event.key}
            data-annotation-id={event.id}
            data-alert-state={event.newState}
            role="img"
            aria-label={t("Annotation") + ": " + event.text}
          >
            <title>{title}</title>
            {geometry.region ? (
              <rect
                x={geometry.left}
                y={0}
                width={geometry.right - geometry.left}
                height={bounds.height}
                fill={color}
                opacity={0.12}
              />
            ) : null}
            <line x1={geometry.left} x2={geometry.left} y1={0} y2={bounds.height}
              stroke={color} strokeDasharray="3 3" opacity={0.8} />
            {geometry.region ? (
              <line x1={geometry.right} x2={geometry.right} y1={0} y2={bounds.height}
                stroke={color} strokeDasharray="3 3" opacity={0.6} />
            ) : null}
            <circle cx={geometry.left} cy={7} r={4} fill={color} className="native-timeseries-annotation-marker" />
          </g>
        );
      })}
    </svg>
  );
}

export default function TimeSeriesRenderer({
  panel,
  frames,
  range,
  timeInfo,
  controls,
  annotations = [],
  annotationDashboard,
  annotationError,
  queryError,
  loading = false,
  streaming = false,
  onRange = () => {},
  onEdit,
  onRemove,
}: Props) {
  const host = useRef<HTMLDivElement>(null);
  const plotRef = useRef<uPlot | undefined>(undefined);
  const lastData = useRef<uPlot.AlignedData | undefined>(undefined);
  const onRangeRef = useRef(onRange);
  const rangeRef = useRef<ReturnType<typeof resolveTimeRange> | undefined>(undefined);
  const ready = useRef(false);
  const [bounds, setBounds] = useState<PlotBounds | undefined>(undefined);
  const [size, setSize] = useState({ width: 0, height: 0 });
  const [plot, setPlot] = useState<uPlot | undefined>(undefined);
  const [cursor, setCursor] = useState<CursorState | undefined>(undefined);
  const [visible, setVisible] = useState<string[] | null>(null);
  const [legendSort, setLegendSort] = useState<"name" | "value">("name");
  // Cursor and legend state must not advance a relative "now" range and start
  // a new normalization job. Resolve again when query frames or range change.
  const resolved = useMemo(() => resolveTimeRange(range), [range, frames]);
  onRangeRef.current = onRange;
  rangeRef.current = resolved;
  const options = panel.config?.options || {};
  // Metadata is cheap; large numeric jobs run off-thread after a sliced copy.
  const deferredFrames = useDeferredValue(frames);
  const { model, processing, preparationFailed } = usePlotModel(deferredFrames, size.width, resolved.start, resolved.end, t(panel.title));
  const ids = useMemo(() => model.series.map((item) => item.id), [model]);
  const defaultVisible = useMemo(
    () => model.series.filter((item) => item.defaultVisible).map((item) => item.id),
    [model],
  );
  const activeIds = visible ?? defaultVisible;
  const activeSet = useMemo(() => new Set(activeIds), [activeIds]);
  const legend = timeSeriesLegend(options);
  const tooltip = timeSeriesTooltip(options);
  const leftSeries = model.series.filter((item) => item.axis === "y");
  const rightSeries = model.series.filter((item) => item.axis === "y2");
  const leftPercentStacked = leftSeries.some((item) => item.stack === "percent");
  const rightPercentStacked = rightSeries.some((item) => item.stack === "percent");
  const leftLabel = leftSeries.find((item) => item.axisLabel)?.axisLabel;
  const rightLabel = rightSeries.find((item) => item.axisLabel)?.axisLabel;
  const leftLog = leftSeries.some((item) => item.log);
  const rightLog = rightSeries.some((item) => item.log);
  const hasSamples = model.data[0].length > 0;
  const canRender = size.width > 0 && size.height > 0 && model.series.length > 0 && model.data[0].length > 0;
  const onZoom = useCallback((from: number, to: number) => {
    const next = timeSeriesZoomSelection({ from, to }, rangeRef.current?.timezone || "browser");
    if (!next) return;
    const current = rangeRef.current;
    if (current && Math.round(current.start) === next.from && Math.round(current.end) === next.to) return;
    onRangeRef.current?.(next);
  }, []);

  useEffect(() => {
    setVisible(null);
  }, [model.signature]);

  useLayoutEffect(() => {
    const element = host.current;
    if (!element || !canRender) return;
    const locale = getLocale() === "zh" ? "zh-CN" : "en-US";
    const axisTime = new Intl.DateTimeFormat(locale, {
      hour: "2-digit",
      minute: "2-digit",
      second: "2-digit",
      timeZone: resolved.timezone === "browser" ? undefined :
        resolved.timezone === "utc" ? "UTC" : resolved.timezone,
    });
    const profiles = (["y","y2"] as const).flatMap(axis=>{
      const items=model.series.filter(item=>item.axis===axis);
      const profile=items[0];
      return profile&&items.every(item=>JSON.stringify(riskSteps(item.field))===JSON.stringify(riskSteps(profile.field))) ? [profile] : [];
    });
    const percentRange = (axis:string, log:boolean): uPlot.Scale["range"] => {
      const items=model.series.filter(item=>item.axis===axis),unit=items[0]?.field.config.unit;
      if(log || !["percent","percentunit"].includes(unit||"") || !items.every(item=>item.field.config.unit===unit)) return undefined;
      // Keep warning lines in view without clipping valid out-of-range samples.
      return (_chart,min,max) => [Math.min(0,min),Math.max(unit==="percentunit"?1:100,max)];
    };
    const axisValues = (axis: "y" | "y2", ticks: number[]) => {
      if (axis === "y" ? leftPercentStacked : rightPercentStacked) return ticks.map(value => `${Math.round(value)}%`);
      const items = model.series.filter(item => item.axis === axis);
      const field = items[0]?.field;
      return field && items.every(item => item.field.config.unit === field.config.unit)
        ? ticks.map(value => formatValue(field, value, resolved.timezone))
        : ticks.map(value => value.toLocaleString(locale, { maximumSignificantDigits: 4 }));
    };
    const chartOptions: uPlot.Options = {
      width: size.width,
      height: size.height,
      ms: 1,
      class: "native-timeseries-plot",
      scales: {
        x: { time: true, min: resolved.start, max: resolved.end },
        y: {
          auto: !leftPercentStacked,
          range: leftPercentStacked ? [0, 100] : percentRange("y",leftLog),
          distr: leftLog ? 3 : 1,
          log: 10,
        },
        ...(rightSeries.length
          ? {
              y2: {
                auto: !rightPercentStacked,
                range: rightPercentStacked ? [0, 100] : percentRange("y2",rightLog),
                distr: rightLog ? 3 : 1,
                log: 10,
              },
            }
          : {}),
      },
      axes: [
        {
          scale: "x",
          stroke: "rgba(180, 190, 205, 0.65)",
          grid: { stroke: "rgba(180, 190, 205, 0.10)", width: 1 },
          ticks: { stroke: "rgba(180, 190, 205, 0.16)", width: 1 },
          values: (_plot, ticks) => ticks.map((value) => axisTime.format(new Date(value))),
        },
        {
          scale: "y",
          label: leftLabel,
          labelSize: 18,
          size: 64,
          stroke: "rgba(180, 190, 205, 0.65)",
          grid: { stroke: "rgba(180, 190, 205, 0.10)", width: 1 },
          ticks: { stroke: "rgba(180, 190, 205, 0.16)", width: 1 },
          values: (_plot: uPlot, ticks: number[]) => axisValues("y", ticks),
        },
        ...(rightSeries.length
          ? [{
              scale: "y2",
              side: 1 as const,
              label: rightLabel,
              labelSize: 18,
              size: 64,
              values: (_plot: uPlot, ticks: number[]) => axisValues("y2", ticks),
              stroke: "rgba(180, 190, 205, 0.65)",
              grid: { show: false },
              ticks: { stroke: "rgba(180, 190, 205, 0.16)", width: 1 },
            }]
          : []),
      ],
      series: [
        { label: t("Time") },
        ...model.series.map((item) => ({
          label: item.name,
          scale: item.axis,
          stroke: (chart:uPlot)=>riskStroke(chart,item.field,item.axis,item.color),
          width: 1.5,
          dash: item.dashed ? [1, 5, 4, 5] : undefined,
          show: activeSet.has(item.id),
          points: { show: false },
          spanGaps: false,
        })),
      ],
      legend: { show: false },
      cursor: {
        drag: { x: true, y: false, setScale: true },
        focus: { prox: 24 },
      },
      hooks: {
        drawClear:[chart=>profiles.forEach(item=>drawRiskBands(chart,item.field,item.axis))],
        setScale: [(chart, scale) => {
          if (!ready.current || scale !== "x") return;
          const min = chart.scales.x.min;
          const max = chart.scales.x.max;
          if (min != null && max != null) onZoom(min, max);
        }],
        setCursor: [(chart) => {
          const index = chart.cursor.idx;
          const left = chart.cursor.left;
          const top = chart.cursor.top;
          if (index == null || left == null || top == null) {
            setCursor(undefined);
            return;
          }
          setCursor((previous) =>
            previous && previous.index === index && previous.left === left && previous.top === top
              ? previous
              : { index, left: Math.round(left), top: Math.round(top) },
          );
        }],
        draw: [(chart) => {
          profiles.forEach(item=>drawRiskBands(chart,item.field,item.axis,true));
          const ratio = window.devicePixelRatio || 1;
          const next = {
            left: chart.bbox.left / ratio,
            top: chart.bbox.top / ratio,
            width: chart.bbox.width / ratio,
            height: chart.bbox.height / ratio,
            start: Number(chart.scales.x.min),
            end: Number(chart.scales.x.max),
          };
          if (![next.left, next.top, next.width, next.height, next.start, next.end].every(Number.isFinite) ||
            next.width <= 0 || next.height <= 0 || next.end <= next.start) return;
          setBounds((previous) =>
            previous && Object.keys(next).every((key) =>
              previous[key as keyof typeof previous] === next[key as keyof typeof next])
              ? previous
              : next,
          );
        }],
      },
    };
    ready.current = false;
    const next = new uPlot(chartOptions, model.data, element);
    plotRef.current = next;
    lastData.current = model.data;
    ready.current = true;
    setPlot(next);
    return () => {
      ready.current = false;
      plotRef.current = undefined;
      lastData.current = undefined;
      setPlot(undefined);
      next.destroy();
    };
  }, [canRender, model.signature, resolved.timezone, getLocale()]);

  useLayoutEffect(() => {
    const chart = plotRef.current;
    if (chart && lastData.current !== model.data) {
      chart.setData(model.data, false);
      lastData.current = model.data;
    }
  }, [model.data]);

  useLayoutEffect(() => {
    plotRef.current?.setSize({
      width: Math.max(1, size.width),
      height: Math.max(1, size.height),
    });
  }, [size.width, size.height]);

  useLayoutEffect(() => {
    const chart = plotRef.current;
    if (!chart) return;
    model.series.forEach((item, index) => {
      chart.setSeries(index + 1, { show: activeSet.has(item.id) });
    });
  }, [activeSet, model.signature]);

  useLayoutEffect(() => {
    const chart = plotRef.current;
    if (!chart) return;
    const min = chart.scales.x.min;
    const max = chart.scales.x.max;
    if (min !== resolved.start || max !== resolved.end)
      chart.setScale("x", { min: resolved.start, max: resolved.end });
  }, [resolved.start, resolved.end]);

  useEffect(() => {
    const element = host.current;
    if (!element) return;
    const measure = () => {
      const rect = element.getBoundingClientRect();
      setSize({ width: Math.floor(rect.width), height: Math.floor(rect.height) });
    };
    measure();
    const observer = new ResizeObserver(measure);
    observer.observe(element);
    return () => observer.disconnect();
  }, []);

  const tooltipItems = useMemo(() => {
    if (!cursor || cursor.index < 0 || tooltip.mode === "none") return [];
    const candidates = model.series
      .map((item, index) => ({
        item,
        index,
        value: item.values[cursor.index],
        y: item.values[cursor.index] == null
          ? Infinity
          : Math.abs((plot?.valToPos(item.values[cursor.index]!, item.axis) || 0) - cursor.top),
      }))
      .filter(({ item, value }) => activeSet.has(item.id) && value != null &&
        (!tooltip.hideZeros || value !== 0));
    const sorted = tooltip.sort === "none"
      ? candidates
      : [...candidates].sort((left, right) =>
          tooltip.sort === "desc"
            ? Number(right.value) - Number(left.value)
            : Number(left.value) - Number(right.value),
        );
    return tooltip.mode === "single"
      ? [...sorted].sort((left, right) => left.y - right.y).slice(0, 1)
      : sorted;
  }, [activeSet, cursor, model, plot, tooltip.hideZeros, tooltip.mode, tooltip.sort]);

  const sortedLegend = useMemo(() => {
    const result = [...model.series];
    return result.sort((left, right) => {
      if (legendSort === "name") return left.name.localeCompare(right.name);
      return (right.lastValue ?? Number.NEGATIVE_INFINITY) - (left.lastValue ?? Number.NEGATIVE_INFINITY);
    });
  }, [legendSort, model]);
  const metricDetails = useMemo(() => model.series.map(item => ({
    id: item.id, name: item.name, rawName: item.rawName, description: item.description,
    metric: item.field.labels?.__name__, labels: item.field.labels, unit: item.field.config.unit,
    timestamp: dataPointTime(item.values, model.data[0]),
  })), [model]);

  return (
    <section
      className="chart-panel native-timeseries-panel"
      data-panel-id={panel.id}
      data-series-count={model.series.length}
      data-input-rows={model.inputRows}
      data-aligned-rows={model.alignedRows}
      data-rendered-rows={model.data[0].length}
      data-preparation={model.preparation}
      aria-busy={loading || processing}
      aria-label={t(panel.title)}
    >
      <div className="panel-heading">
        <h2>{t(panel.title)}</h2>
        <div className="panel-actions">
          <MetricDetailsButton panel={panel} timezone={resolved.timezone} items={metricDetails} />
          {controls}
          {annotationDashboard ? (
            <AnnotationEditor
              dashboard={annotationDashboard}
              panelId={Number(panel.config?.id || 0)}
              time={Math.round((resolved.start + resolved.end) / 2)}
              events={annotations}
            />
          ) : null}
          {streaming ? <span className="status healthy" role="status"><i />{t("Live")}</span> : null}
          {onEdit ? (
            <button className="icon-button" aria-label={t("Edit") + " " + t(panel.title)} onClick={onEdit}>
              <Pencil size={15} />
            </button>
          ) : null}
          {onRemove ? (
            <button className="icon-button danger-hover" aria-label={t("Remove") + " " + t(panel.title)} onClick={onRemove}>
              <Trash2 size={15} />
            </button>
          ) : null}
        </div>
      </div>
      {timeInfo ? <p className="panel-time-info">{timeInfo}</p> : null}
      {model.series.some(item=>riskSteps(item.field).length>1) ? <div className="native-risk-legend" aria-label={t("Risk thresholds")}>
        {model.series.filter(item=>riskSteps(item.field).length>1).slice(0,3).map(item=><span key={item.id}>
          {model.series.length>1?item.name+": ":""}{riskSteps(item.field).filter(step=>Number.isFinite(step.value)).map((step,index)=><span key={step.value} style={{color:step.color}}>{index===0?t("Warning"):t("Critical")} ≥ {formatValue(item.field,step.value,resolved.timezone)} </span>)}
        </span>)}
      </div> : null}
      {annotationError || queryError || preparationFailed ? (
        <p role="alert" className="form-error">{t(annotationError || queryError || "Chart error")}
          {queryError && hasSamples ? " · " + t("Any displayed values belong to the previous query result.") : ""}
        </p>
      ) : null}
      <div className="native-timeseries-viewport">
        <div className="native-timeseries-plot-host" ref={host} onDoubleClick={() => onZoom(resolved.start, resolved.end)} />
        {plot ? <AnnotationOverlay bounds={bounds} events={annotations} timezone={resolved.timezone} /> : null}
        {cursor && tooltipItems.length ? (
          <div
            className="grafana-timeseries-tooltip native-timeseries-tooltip"
            role="group"
            aria-label={t("Data at cursor")}
            style={{
              left: Math.max(8, Math.min(size.width - 260, cursor.left + 14)),
              top: Math.max(8, Math.min(size.height - 120, cursor.top + 14)),
              maxHeight: tooltip.maxHeight,
              maxWidth: tooltip.maxWidth,
            }}
          >
            <time>
              {new Date(Number(model.data[0][cursor.index])).toLocaleString(undefined, {
                timeZone: resolved.timezone === "browser"
                  ? undefined
                  : resolved.timezone === "utc"
                    ? "UTC"
                    : resolved.timezone,
              })}
            </time>
            {tooltipItems.map(({ item, value }) => (
              <div className="native-timeseries-tooltip-row" key={item.id}>
                <i style={{ background: item.color }} />
                <span>{item.name}</span>
                <strong>{formatValue(item.field, value, resolved.timezone)}</strong>
              </div>
            ))}
          </div>
        ) : null}
        {!hasSamples ? (
          <div className="native-timeseries-empty" role="status">
            {loading || processing ? t("Loading metrics…") : t("No samples in this time range")}
          </div>
        ) : null}
      </div>
      {legend.showLegend && model.series.length ? (
        <div className={"native-timeseries-legend " + (legend.placement === "right" ? "right" : "bottom")}>
          <div className="native-timeseries-legend-heading">
            <span>{t("Legend")}</span>
            <button
              type="button"
              aria-label={t("Sort by name or value")}
              onClick={() => setLegendSort((current) => current === "name" ? "value" : "name")}
            >
              {legendSort === "name" ? t("Name") : t("Value")}
            </button>
          </div>
          <div className="native-timeseries-legend-list">
            {sortedLegend.map((item) => (
                <div key={item.id} className="native-timeseries-legend-item">
                  <button
                    type="button"
                    aria-pressed={activeSet.has(item.id)}
                    onClick={(event) => setVisible((current) =>
                      changeSeriesVisibility(
                        ids,
                        current ?? defaultVisible,
                        item.id,
                        event.ctrlKey || event.metaKey ? "append" : "select",
                      ),
                    )}
                  >
                    <i style={{ background: item.color }} data-testid="series-icon" />
                    <span>{item.name}</span>
                  </button>
                  <strong>{formatValue(item.field, item.lastValue, resolved.timezone)}</strong>
                </div>
              ))}
          </div>
        </div>
      ) : null}
    </section>
  );
}
