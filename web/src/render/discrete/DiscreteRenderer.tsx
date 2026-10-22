import {
  useEffect,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from "react";
import { getFieldDisplayName, type Field } from "@grafana/data";
import { Pencil, Trash2 } from "lucide-react";
import { getLocale, t } from "../../i18n";
import { MetricDetailsButton, pointTimeText } from "../MetricDetails";
import { seriesPresentations } from "../series-presentation";
import { resolveTimeRange } from "../../grafana/time-range";
import type { RendererProps } from "../types";
import {
  categories,
  colorForKey,
  pieParts,
  REDUCERS,
  stateRows,
  timelineSegments,
  type Category,
  type Reducer,
  type StatePoint,
} from "./model";
import "./DiscreteRenderer.css";

const ROW_HEIGHT = 38;
function shareText(share: number) {
  const percent = share * 100;
  return `${percent > 0 && percent < 0.1 ? percent.toPrecision(2) : percent.toFixed(1)}%`;
}
function formatted(field: Field, value: unknown) {
  if (value === null) return "—";
  const display = field.display?.(value);
  return display
    ? `${display.prefix || ""}${display.text}${display.suffix || ""}`
    : String(value);
}
function Frame({
  props,
  children,
  details,
}: {
  props: RendererProps;
  children: ReactNode;
  details?: {
    id: string;
    name: string;
    rawName: string;
    labels?: Record<string, string>;
  }[];
}) {
  return (
    <section
      className={`chart-panel native-discrete native-${props.panel.visualization || props.panel.config?.type}`}
      aria-label={t(props.panel.title)}
      aria-busy={props.loading}
    >
      <div className="panel-heading">
        <h2 title={props.panel.config?.description}>{t(props.panel.title)}</h2>
        <div className="panel-actions">
          {details && (
            <MetricDetailsButton
              panel={props.panel}
              items={details}
              timezone={resolveTimeRange(props.range).timezone}
              reducer={
                props.panel.config?.options?.reduceOptions?.calcs?.[0] ||
                "lastNotNull"
              }
            />
          )}
          {props.onEdit && (
            <button
              className="icon-button"
              aria-label={`${t("Edit")} ${t(props.panel.title)}`}
              onClick={props.onEdit}
            >
              <Pencil size={15} />
            </button>
          )}
          {props.onRemove && (
            <button
              className="icon-button danger-hover"
              aria-label={`${t("Remove")} ${t(props.panel.title)}`}
              onClick={props.onRemove}
            >
              <Trash2 size={15} />
            </button>
          )}
        </div>
      </div>
      {props.timeInfo && <p className="panel-time-info">{props.timeInfo}</p>}
      {props.queryError && (
        <p className="form-error" role="alert">
          {t(props.queryError)}
          {props.frames.length
            ? ` · ${t("Any displayed values belong to the previous query result.")}`
            : ""}
        </p>
      )}
      {children}
    </section>
  );
}

function VirtualRows({
  rows,
  render,
}: {
  rows: readonly Category[];
  render: (row: Category) => ReactNode;
}) {
  const [scroll, setScroll] = useState(0);
  const viewport = useRef<HTMLDivElement>(null);
  const [height, setHeight] = useState(300);
  useEffect(() => {
    const element = viewport.current;
    if (!element) return;
    const observer = new ResizeObserver(() => setHeight(element.clientHeight));
    observer.observe(element);
    return () => observer.disconnect();
  }, []);
  const start = Math.min(
    Math.max(0, rows.length - 1),
    Math.max(0, Math.floor(scroll / ROW_HEIGHT) - 4),
  );
  const visible = rows.slice(start, start + Math.ceil(height / ROW_HEIGHT) + 8);
  return (
    <div
      ref={viewport}
      className="native-discrete-list"
      onScroll={(event) => setScroll(event.currentTarget.scrollTop)}
      role="list"
      aria-label={t("All categories")}
      data-total-rows={rows.length}
      tabIndex={0}
    >
      <div style={{ height: rows.length * ROW_HEIGHT, position: "relative" }}>
        {visible.map((row, index) => (
          <div
            key={row.id}
            className="native-discrete-row"
            role="listitem"
            aria-setsize={rows.length}
            aria-posinset={start + index + 1}
            style={{ top: (start + index) * ROW_HEIGHT }}
          >
            {render(row)}
          </div>
        ))}
      </div>
    </div>
  );
}

export function CategoryRenderer(props: RendererProps) {
  const options = props.panel.config?.options?.reduceOptions;
  const reducer = options?.calcs?.[0] || "lastNotNull";
  const model = useMemo(
    () => categories(props.frames, reducer as Reducer, !!options?.values),
    [props.frames, reducer, options?.values],
  );
  const presentations = useMemo(
    () =>
      seriesPresentations(
        model.rows.map((row) => ({
          metric: row.field.labels?.__name__,
          labels: row.field.labels,
          displayName:
            row.field.config.displayName || row.field.config.displayNameFromDS
              ? getFieldDisplayName(row.field, row.frame, props.frames)
              : undefined,
          fallbackName:
            row.field.name === "Value" ? t(props.panel.title) : row.field.name,
        })),
        getLocale(),
      ),
    [model.rows, props.frames, props.panel.title],
  );
  const rows = useMemo(
    () =>
      model.rows.map((row, index) => ({
        ...row,
        label: row.label || presentations[index].name,
      })),
    [model.rows, presentations],
  );
  const parts = useMemo(() => pieParts(rows, 16), [rows]);
  const isPie = ["pie", "piechart"].includes(
    props.panel.visualization || props.panel.config?.type || "",
  );
  const min = rows.reduce((value, row) => Math.min(value, row.value ?? 0), 0);
  const max = rows.reduce((value, row) => Math.max(value, row.value ?? 0), 0);
  const scale = Math.max(Math.abs(min), max, Number.MIN_VALUE);
  const domain = max / scale - min / scale || 1;
  const zero = (-min / scale / domain) * 100;
  const details = rows.map((row) => ({
    id: row.id,
    name: row.label,
    rawName: row.field.name,
    labels: row.field.labels,
  }));
  let angle = -Math.PI / 2;
  return (
    <Frame props={props} details={details}>
      {model.omitted > 0 && (
        <p className="subtle" role="status">
          {t(
            "Panel display budget reached. Narrow the query to see remaining categories.",
          )}
        </p>
      )}
      {!REDUCERS.includes(reducer) ? (
        <p className="form-error" role="alert">
          {t("Unsupported reducer")}: {reducer}
        </p>
      ) : !rows.length ? (
        <p className="native-discrete-empty" role="status">
          {t(
            props.loading
              ? "Loading metrics…"
              : "No samples in this time range",
          )}
        </p>
      ) : (
        <>
          {isPie &&
            (parts.negative ? (
              <p className="form-error" role="alert">
                {t(
                  "Pie charts require non-negative values. Use a bar chart for signed values.",
                )}
              </p>
            ) : parts.parts.length ? (
              <div className="native-pie-plot">
                <svg
                  viewBox="0 0 240 200"
                  role="img"
                  aria-label={t(props.panel.title)}
                >
                  {parts.parts.map((part) => {
                    const start = angle;
                    angle += part.share * Math.PI * 2;
                    const title = `${part.label === "Other" ? `${t("Other")} (${part.members})` : part.label} · ${part.value === null ? t("Out of range") : formatted(part.field, part.value)} · ${shareText(part.share)}`;
                    const path =
                      part.share >= 1 - Number.EPSILON
                        ? undefined
                        : `M120 100 L${120 + 88 * Math.cos(start)} ${100 + 88 * Math.sin(start)} A88 88 0 ${part.share > 0.5 ? 1 : 0} 1 ${120 + 88 * Math.cos(angle)} ${100 + 88 * Math.sin(angle)} Z`;
                    return path ? (
                      <path
                        key={part.id}
                        d={path}
                        fill={colorForKey(part.label)}
                        stroke="var(--panel, #191f29)"
                        strokeWidth="1"
                        tabIndex={0}
                      >
                        <title>{title}</title>
                      </path>
                    ) : (
                      <circle
                        key={part.id}
                        cx="120"
                        cy="100"
                        r="88"
                        fill={colorForKey(part.label)}
                        tabIndex={0}
                      >
                        <title>{title}</title>
                      </circle>
                    );
                  })}
                  <circle
                    cx="120"
                    cy="100"
                    r="53"
                    fill="var(--surface, #171c24)"
                  />
                  <text x="120" y="97" textAnchor="middle">
                    {rows.length}
                  </text>
                  <text
                    x="120"
                    y="118"
                    textAnchor="middle"
                    className="native-pie-caption"
                  >
                    {t("Categories")}
                  </text>
                </svg>
              </div>
            ) : (
              <p className="native-discrete-empty" role="status">
                {t("No positive values to show proportions")}
              </p>
            ))}
          {isPie && parts.parts.some((part) => part.members > 1) && (
            <p className="subtle">
              {t(
                "Small slices are combined visually; all categories remain listed.",
              )}
            </p>
          )}
          <VirtualRows
            rows={rows}
            render={(row) => {
              const percentage =
                !parts.negative && row.value !== null && row.value > 0
                  ? parts.shares.get(row.id)
                  : undefined;
              const left =
                row.value === null
                  ? zero
                  : Math.min(
                      zero,
                      ((row.value / scale - min / scale) / domain) * 100,
                    );
              const width =
                row.value === null
                  ? 0
                  : Math.abs((row.value / scale / domain) * 100);
              return (
                <>
                  <span className="native-discrete-label" title={row.label}>
                    {isPie && (
                      <i style={{ background: colorForKey(row.label) }} />
                    )}
                    {row.label}
                  </span>
                  {!isPie && (
                    <span className="native-bar-track">
                      <i
                        className="native-bar-zero"
                        style={{ left: `${zero}%` }}
                      />
                      <i
                        className="native-bar-fill"
                        style={{
                          left: `${left}%`,
                          width: `${width}%`,
                          background:
                            (row.field.config.color?.mode === "thresholds" ||
                            row.field.config.color?.mode === "fixed" ||
                            row.field.config.mappings?.length
                              ? row.field.display?.(row.value)?.color
                              : undefined) || colorForKey(row.label),
                        }}
                      />
                    </span>
                  )}
                  <strong className="native-discrete-value">
                    {formatted(row.field, row.value)}
                    {isPie && percentage !== undefined
                      ? ` · ${shareText(percentage)}`
                      : ""}
                  </strong>
                </>
              );
            }}
          />
        </>
      )}
    </Frame>
  );
}

function StateStrip({
  points,
  history,
  from,
  to,
  label,
  width,
  timezone,
}: {
  points: StatePoint[];
  history: boolean;
  from: number;
  to: number;
  label: string;
  width: number;
  timezone: string;
}) {
  const canvas = useRef<HTMLCanvasElement>(null);
  const [hover, setHover] = useState<StatePoint>();
  const segments = useMemo(
    () => (history ? points : timelineSegments(points)),
    [points, history],
  );
  useLayoutEffect(() => {
    const element = canvas.current;
    if (!element || width < 1 || to <= from) return;
    const ratio = Math.min(2, window.devicePixelRatio || 1);
    element.width = Math.ceil(width * ratio);
    element.height = 28 * ratio;
    const context = element.getContext("2d");
    if (!context) return;
    context.setTransform(ratio, 0, 0, ratio, 0, 0);
    context.clearRect(0, 0, width, 28);
    for (const point of segments) {
      const left = ((point.from - from) / (to - from)) * width;
      const size = ((point.to - point.from) / (to - from)) * width;
      context.fillStyle = point.color;
      context.fillRect(
        left,
        2,
        Math.max(1, size - (history && size > 3 ? 2 : 0)),
        24,
      );
    }
  }, [segments, history, from, to, width]);
  return (
    <div className="native-state-strip">
      <canvas
        ref={canvas}
        style={{ width, height: 28 }}
        role="img"
        aria-label={`${label} · ${points.at(-1)?.text || t("No data")}`}
        onMouseLeave={() => setHover(undefined)}
        onMouseMove={(event) => {
          const timestamp =
            from +
            ((event.clientX -
              event.currentTarget.getBoundingClientRect().left) /
              width) *
              (to - from);
          let low = 0,
            high = segments.length - 1;
          while (low <= high) {
            const mid = (low + high) >>> 1,
              point = segments[mid];
            if (timestamp < point.from) high = mid - 1;
            else if (timestamp >= point.to) low = mid + 1;
            else {
              setHover(point);
              return;
            }
          }
          setHover(undefined);
        }}
      />
      {hover && (
        <div className="native-state-tooltip" role="status">
          {label} · {hover.text}
          <br />
          {pointTimeText(hover.from, timezone)} →{" "}
          {pointTimeText(hover.to, timezone)}
        </div>
      )}
    </div>
  );
}

export function StateRenderer(props: RendererProps) {
  const range = resolveTimeRange(props.range);
  const { start: from, end: to } = range;
  const model = useMemo(
    () =>
      stateRows(props.frames, from, to, (field, value) => ({
        text: t(formatted(field, value)),
        color:
          (field.config.mappings?.length ||
          field.config.color?.mode === "thresholds"
            ? field.display?.(value)?.color
            : undefined) || colorForKey(String(value)),
      })),
    [props.frames, from, to],
  );
  const viewport = useRef<HTMLDivElement>(null);
  const [size, setSize] = useState({ height: 280, width: 450 });
  const [scroll, setScroll] = useState(0);
  const hasRows = model.rows.length > 0;
  useEffect(() => {
    const element = viewport.current;
    if (!element) return;
    const measure = () =>
      setSize({ height: element.clientHeight, width: element.clientWidth });
    measure();
    const observer = new ResizeObserver(measure);
    observer.observe(element);
    return () => observer.disconnect();
  }, [hasRows]);
  const names = useMemo(
    () =>
      seriesPresentations(
        model.rows.map((row) => ({
          metric: row.field.labels?.__name__,
          labels: row.field.labels,
          displayName:
            row.field.config.displayName || row.field.config.displayNameFromDS
              ? getFieldDisplayName(row.field, row.frame, props.frames)
              : undefined,
          fallbackName:
            row.field.name === "Value" ? t(props.panel.title) : row.field.name,
        })),
        getLocale(),
      ),
    [model.rows, props.frames, props.panel.title],
  );
  const start = Math.min(
    Math.max(0, model.rows.length - 1),
    Math.max(0, Math.floor(scroll / ROW_HEIGHT) - 3),
  );
  const visible = model.rows.slice(
    start,
    start + Math.ceil(size.height / ROW_HEIGHT) + 6,
  );
  const width = Math.max(40, size.width - 178);
  return (
    <Frame
      props={props}
      details={model.rows.map((row, index) => ({
        id: row.id,
        name: names[index].name,
        rawName: row.field.name,
        labels: row.field.labels,
      }))}
    >
      {model.omitted && (
        <p className="subtle" role="status">
          {t(
            "Panel display budget reached. Narrow the query to see remaining categories.",
          )}
        </p>
      )}
      {!model.rows.length ? (
        <p role="status" className="native-discrete-empty">
          {t(
            props.loading
              ? "Loading metrics…"
              : "State panels need a time field and state values",
          )}
        </p>
      ) : (
        <>
          <div
            className="native-state-list"
            ref={viewport}
            onScroll={(event) => setScroll(event.currentTarget.scrollTop)}
            tabIndex={0}
            role="list"
            aria-label={t("All states")}
            data-total-rows={model.rows.length}
          >
            <div
              style={{
                height: model.rows.length * ROW_HEIGHT,
                position: "relative",
              }}
            >
              {visible.map((row, index) => (
                <div
                  className="native-state-row"
                  key={row.id}
                  role="listitem"
                  aria-setsize={model.rows.length}
                  aria-posinset={start + index + 1}
                  style={{ top: (start + index) * ROW_HEIGHT }}
                >
                  <span
                    className="native-state-label"
                    title={names[start + index].name}
                  >
                    {names[start + index].name}
                    <small>
                      {row.points.length
                        ? `${t("Last observed")}: ${row.points.at(-1)!.text}`
                        : t("No data")}
                    </small>
                  </span>
                  <StateStrip
                    points={row.points}
                    from={from}
                    to={to}
                    width={width}
                    timezone={range.timezone}
                    label={names[start + index].name}
                    history={
                      props.panel.visualization === "status-history" ||
                      props.panel.config?.type === "status-history"
                    }
                  />
                </div>
              ))}
            </div>
          </div>
          <div className="native-state-axis">
            <span>
              {new Date(from).toLocaleTimeString(
                getLocale() === "zh" ? "zh-CN" : "en-GB",
                {
                  hour: "2-digit",
                  minute: "2-digit",
                  timeZone:
                    range.timezone === "browser"
                      ? undefined
                      : range.timezone === "utc"
                        ? "UTC"
                        : range.timezone,
                },
              )}
            </span>
            <span>
              {new Date(to).toLocaleTimeString(
                getLocale() === "zh" ? "zh-CN" : "en-GB",
                {
                  hour: "2-digit",
                  minute: "2-digit",
                  timeZone:
                    range.timezone === "browser"
                      ? undefined
                      : range.timezone === "utc"
                        ? "UTC"
                        : range.timezone,
                },
              )}
            </span>
          </div>
          <p className="subtle native-state-note">
            {t(
              "Blank intervals have no observations; values are not filled across gaps.",
            )}
          </p>
        </>
      )}
    </Frame>
  );
}
