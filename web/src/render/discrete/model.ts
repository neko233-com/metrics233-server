import type { DataFrame, Field } from "@grafana/data";

export const ROW_LIMIT = 5000;
export const SAMPLE_LIMIT = 200000;
export type Category = {
  id: string;
  label: string;
  value: number | null;
  field: Field;
  frame: DataFrame;
};
export type StateLabel = { text: string; color: string };
export type StatePoint = StateLabel & {
  from: number;
  to: number;
  value: string | number | boolean;
};
export type StateRow = {
  id: string;
  field: Field;
  frame: DataFrame;
  points: StatePoint[];
};
export type Reducer =
  | "lastNotNull"
  | "last"
  | "firstNotNull"
  | "first"
  | "mean"
  | "sum"
  | "min"
  | "max"
  | "count";
export const REDUCERS: readonly string[] = [
  "lastNotNull",
  "last",
  "firstNotNull",
  "first",
  "mean",
  "sum",
  "min",
  "max",
  "count",
];
const finite = (value: unknown): value is number =>
  typeof value === "number" && Number.isFinite(value);

/** Nulls, booleans and numeric-looking strings are never converted to observed zeroes. */
export function reduceNumeric(
  values: ArrayLike<unknown>,
  reducer: Reducer,
): number | null {
  if (reducer === "first" || reducer === "last") {
    const value = values[reducer === "first" ? 0 : values.length - 1];
    return finite(value) ? value : null;
  }
  let count = 0,
    sum = 0,
    extreme: number | null = null;
  for (let index = 0; index < values.length; index++) {
    const value = values[index];
    if (!finite(value)) continue;
    if (reducer === "firstNotNull") return value;
    count++;
    if (reducer === "lastNotNull") extreme = value;
    else if (reducer === "min")
      extreme = extreme === null ? value : Math.min(extreme, value);
    else if (reducer === "max")
      extreme = extreme === null ? value : Math.max(extreme, value);
    else sum += value;
  }
  if (reducer === "count") return count;
  if (!count) return null;
  const result =
    reducer === "mean" ? sum / count : reducer === "sum" ? sum : extreme;
  return finite(result) ? result : null;
}

export function categories(
  frames: readonly DataFrame[],
  reducer: Reducer = "lastNotNull",
  allValues = false,
) {
  const rows: Category[] = [];
  let omitted = 0,
    scanned = 0;
  for (const [frameIndex, frame] of frames.entries()) {
    const text = frame.fields.find((field) => field.type === "string");
    const numeric = frame.fields.filter((field) => field.type === "number");
    for (const [fieldIndex, field] of numeric.entries()) {
      const count = text || allValues ? field.values.length : 1;
      for (let index = 0; index < count; index++) {
        if (rows.length >= ROW_LIMIT || scanned >= SAMPLE_LIMIT) {
          omitted += count - index;
          break;
        }
        const remaining = SAMPLE_LIMIT - scanned;
        const values =
          text || allValues
            ? [field.values[index]]
            : field.values.slice(Math.max(0, field.values.length - remaining));
        if (!text && !allValues && field.values.length > remaining) omitted++;
        scanned += values.length;
        const value = reduceNumeric(
          values,
          text || allValues ? "last" : reducer,
        );
        const category = text ? String(text.values[index] ?? "—") : "";
        rows.push({
          id: `${frameIndex}:${fieldIndex}:${index}`,
          label:
            category + (text && numeric.length > 1 ? ` · ${field.name}` : ""),
          value,
          field,
          frame,
        });
      }
    }
  }
  return { rows, omitted, scanned };
}

export function pieParts(rows: readonly Category[], limit = 200) {
  const positive = rows.filter((row) => row.value !== null && row.value > 0);
  const max = positive.reduce((value, row) => Math.max(value, row.value!), 0);
  const denominator = positive.reduce(
    (value, row) => value + row.value! / max,
    0,
  );
  const sorted = [...positive].sort(
    (a, b) => b.value! - a.value! || a.id.localeCompare(b.id),
  );
  const visible = sorted.length > limit ? sorted.slice(0, limit - 1) : sorted;
  const shares = new Map(
    positive.map((row) => [row.id, row.value! / max / denominator]),
  );
  const parts = visible.map((row) => ({
    ...row,
    share: shares.get(row.id)!,
    members: 1,
  }));
  if (sorted.length > limit) {
    const rest = sorted.slice(limit - 1);
    const share = rest.reduce(
      (sum, row) => sum + row.value! / max / denominator,
      0,
    );
    const value = rest.reduce((sum, row) => sum + row.value!, 0);
    parts.push({
      ...rest[0],
      id: "other",
      label: "Other",
      value: Number.isFinite(value) ? value : null,
      share,
      members: rest.length,
    });
  }
  return {
    parts,
    shares,
    negative: rows.some((row) => row.value !== null && row.value < 0),
  };
}

export function colorForKey(key: string): string {
  let hash = 2166136261;
  for (let index = 0; index < key.length; index++)
    hash = Math.imul(hash ^ key.charCodeAt(index), 16777619);
  const palette = [
    "#70a7ff",
    "#4ec9a1",
    "#d895ed",
    "#ebc65c",
    "#75cfde",
    "#ef8e82",
    "#a3b8dd",
    "#bdc875",
  ];
  return palette[(hash >>> 0) % palette.length];
}

/** Discrete observations stop at nulls and long gaps; missing time is never marked healthy. */
export function stateRows(
  frames: readonly DataFrame[],
  from: number,
  to: number,
  describe: (field: Field, value: string | number | boolean) => StateLabel,
) {
  const rows: StateRow[] = [];
  let scanned = 0,
    omitted = false;
  for (const [frameIndex, frame] of frames.entries()) {
    const time = frame.fields.find((field) => field.type === "time");
    if (!time) continue;
    for (const [fieldIndex, field] of frame.fields.entries()) {
      if (!["number", "string", "boolean"].includes(field.type)) continue;
      if (rows.length >= ROW_LIMIT || scanned >= SAMPLE_LIMIT) {
        omitted = true;
        break;
      }
      const observations: {
        time: number;
        value: string | number | boolean | null;
      }[] = [];
      const count = Math.min(
        time.values.length,
        field.values.length,
        SAMPLE_LIMIT - scanned,
      );
      if (count < Math.min(time.values.length, field.values.length))
        omitted = true;
      for (let index = 0; index < count; index++) {
        const rawTime = time.values[index],
          value = field.values[index];
        const timestamp =
          rawTime instanceof Date
            ? rawTime.getTime()
            : typeof rawTime === "number"
              ? rawTime
              : NaN;
        scanned++;
        if (!Number.isFinite(timestamp)) continue;
        observations.push({
          time: timestamp,
          value:
            finite(value) ||
            typeof value === "string" ||
            typeof value === "boolean"
              ? value
              : null,
        });
      }
      observations.sort((a, b) => a.time - b.time);
      // A repeated timestamp represents replacement, not simultaneous states.
      const unique = observations.filter(
        (point, index) => observations[index + 1]?.time !== point.time,
      );
      const intervals = unique
        .slice(1, 102)
        .map((point, index) => point.time - unique[index].time)
        .filter((value) => value > 0)
        .sort((a, b) => a - b);
      const cadence = intervals.length
        ? intervals[Math.floor(intervals.length / 2)]
        : 1;
      const points: StatePoint[] = [];
      unique.forEach((point, index) => {
        if (point.value === null) return;
        const end = Math.min(
          to,
          unique[index + 1]?.time ?? point.time + cadence,
          point.time + cadence * 2.5,
        );
        if (end <= from || point.time >= to) return;
        points.push({
          from: Math.max(from, point.time),
          to: end,
          value: point.value,
          ...describe(field, point.value),
        });
      });
      rows.push({ id: `${frameIndex}:${fieldIndex}`, field, frame, points });
    }
  }
  return { rows, scanned, omitted };
}

export function timelineSegments(points: readonly StatePoint[]): StatePoint[] {
  const output: StatePoint[] = [];
  for (const point of points) {
    const previous = output[output.length - 1];
    if (
      previous &&
      previous.to === point.from &&
      previous.text === point.text &&
      previous.color === point.color
    )
      previous.to = point.to;
    else output.push({ ...point });
  }
  return output;
}

/** Classic Prometheus buckets are cumulative; ordinary numeric frames contain raw observations. */
export function histogramBins(frames: readonly DataFrame[]) {
  const bins: { label: string; value: number }[] = [];
  const groups = new Map<string, { upper: number; value: number }[]>();
  let classic = false,
    invalid = false,
    scanned = 0,
    truncated = false,
    fieldCount = 0;
  const raw: number[] = [];
  framesLoop: for (const frame of frames) {
    const labels = frame.fields.find((field) => field.type === "string");
    for (const field of frame.fields.filter(
      (field) => field.type === "number",
    )) {
      if (scanned >= SAMPLE_LIMIT || fieldCount++ >= ROW_LIMIT) {
        truncated = true;
        break framesLoop;
      }
      if (field.labels?.le !== undefined) {
        classic = true;
        const upper =
          field.labels.le === "+Inf" || field.labels.le === "Inf"
            ? Infinity
            : Number(field.labels.le);
        const value = reduceNumeric(
          field.values.slice(-Math.max(0, SAMPLE_LIMIT - scanned)),
          "lastNotNull",
        );
        scanned += Math.min(
          field.values.length,
          Math.max(0, SAMPLE_LIMIT - scanned),
        );
        if (scanned >= SAMPLE_LIMIT) truncated = true;
        if (
          value === null ||
          value < 0 ||
          Number.isNaN(upper) ||
          field.labels.le.trim() === ""
        ) {
          invalid = true;
          continue;
        }
        const identity = Object.entries(field.labels)
          .filter(([key]) => key !== "le" && key !== "__name__")
          .sort(([a], [b]) => a.localeCompare(b));
        const key = JSON.stringify(identity);
        const group = groups.get(key) || [];
        group.push({ upper, value });
        groups.set(key, group);
      } else if (labels) {
        for (let index = 0; index < field.values.length; index++) {
          if (bins.length >= ROW_LIMIT || scanned >= SAMPLE_LIMIT) {
            truncated = true;
            break;
          }
          scanned++;
          const value = field.values[index];
          if (finite(value) && value < 0) invalid = true;
          if (finite(value) && value >= 0)
            bins.push({ label: String(labels.values[index] ?? index), value });
        }
      } else {
        for (const value of field.values) {
          if (scanned >= SAMPLE_LIMIT) {
            truncated = true;
            break;
          }
          scanned++;
          if (finite(value)) raw.push(value);
        }
      }
    }
  }
  if (classic) {
    if (raw.length || bins.length) invalid = true;
    for (const [key, group] of groups) {
      group.sort((a, b) => a.upper - b.upper);
      let previous = 0,
        lower = -Infinity;
      for (const bucket of group) {
        if (bucket.upper <= lower || bucket.value < previous) {
          invalid = true;
          break;
        }
        const groupLabel =
          groups.size > 1
            ? JSON.parse(key)
                .map(([name, value]: [string, string]) => `${name}=${value}`)
                .join(" · ") + " · "
            : "";
        bins.push({
          label: `${groupLabel}(${lower}, ${bucket.upper}]`,
          value: bucket.value - previous,
        });
        lower = bucket.upper;
        previous = bucket.value;
      }
    }
  } else if (raw.length) {
    if (bins.length) invalid = true;
    let min = Infinity,
      max = -Infinity;
    for (const value of raw) {
      min = Math.min(min, value);
      max = Math.max(max, value);
    }
    const count =
      min === max
        ? 1
        : Math.min(100, Math.max(1, Math.ceil(Math.sqrt(raw.length))));
    const scale = Math.max(Math.abs(min), Math.abs(max), Number.MIN_VALUE);
    const start = min / scale,
      step = (max / scale - start) / count;
    const counts = Array<number>(count).fill(0);
    for (const value of raw)
      counts[
        count === 1
          ? 0
          : Math.min(count - 1, Math.floor((value / scale - start) / step))
      ]++;
    for (let index = 0; index < count; index++)
      bins.push({
        label:
          count === 1
            ? String(min)
            : `${((start + step * index) * scale).toPrecision(3)} – ${((start + step * (index + 1)) * scale).toPrecision(3)}`,
        value: counts[index],
      });
  }
  return { bins: invalid ? [] : bins, invalid, truncated };
}
