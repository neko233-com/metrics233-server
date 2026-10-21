// This module has no React or Grafana dependency: the same code runs in the
// Worker and in the small-frame fallback. NaN represents a missing sample on
// the wire; only the bounded drawing result becomes uPlot's null arrays.
export type NumericSource = { times: Float64Array; columns: Float64Array[] };
export type NumericSeries = { axis: string; stack: string; stackGroup: string };
export type NumericRequest = {
  sources: NumericSource[];
  series: NumericSeries[];
  targetRows: number;
  start: number;
  end: number;
};
export type NumericResult = {
  columns: Float64Array[];
  lastValues: number[];
  inputRows: number;
  alignedRows: number;
};

/** Keep every bucket's extrema for every series, endpoints and gap edges.
 * The pixel budget is soft: preserving many gaps takes priority over size. */
export function minMaxIndices(columns: Float64Array[], targetRows: number, start: number, end: number): Uint32Array {
  const x = columns[0];
  const length = x.length;
  if (!length) return new Uint32Array();
  let first = lowerBound(x, start), last = lowerBound(x, end);
  // One neighbour at each edge preserves the clipped line's interpolation.
  first = Math.max(0, first - 1);
  last = Math.min(length - 1, last);
  const count = last - first + 1;
  const budget = Math.max(4, Math.floor(targetRows));
  if (count <= budget) return Uint32Array.from({ length: count }, (_, index) => first + index);
  const selected = new Uint8Array(length);
  selected[first] = selected[last] = 1;
  const seriesCount = Math.max(1, columns.length - 1);
  const buckets = Math.max(1, Math.floor((budget - 2) / (2 * seriesCount)));
  for (let column = 1; column < columns.length; column += 1) {
    const values = columns[column];
    for (let bucket = 0; bucket < buckets; bucket += 1) {
      const from = first + Math.floor(bucket * count / buckets);
      const to = first + Math.floor((bucket + 1) * count / buckets);
      let min = -1, max = -1;
      for (let row = from; row < to; row += 1) {
        const value = values[row];
        const present = Number.isFinite(value);
        if (row > first && present !== Number.isFinite(values[row - 1])) {
          selected[row - 1] = selected[row] = 1;
        }
        if (!present) continue;
        if (min < 0 || value < values[min]) min = row;
        if (max < 0 || value > values[max]) max = row;
      }
      if (min >= 0) selected[min] = 1;
      if (max >= 0) selected[max] = 1;
    }
  }
  let size = 0;
  for (let row = first; row <= last; row += 1) size += selected[row];
  const result = new Uint32Array(size);
  for (let row = first, index = 0; row <= last; row += 1) if (selected[row]) result[index++] = row;
  return result;
}

function lowerBound(values: Float64Array, value: number): number {
  let low = 0, high = values.length;
  while (low < high) {
    const mid = (low + high) >>> 1;
    if (values[mid] < value) low = mid + 1;
    else high = mid;
  }
  return low;
}

export function prepareNumericModel(request: NumericRequest): NumericResult {
  const { sources, series } = request;
  const reference = sources[0]?.times || new Float64Array();
  const aligned = sources.every(({ times }) => times.length === reference.length && times.every((time, row) =>
    Number.isFinite(time) && time === reference[row] && (row === 0 || time > times[row - 1])));
  let columns: Float64Array[];
  if (aligned) {
    columns = [reference, ...sources.flatMap((source) => source.columns)];
  } else {
    const timestamps = new Set<number>();
    for (const source of sources) for (const time of source.times) if (Number.isFinite(time)) timestamps.add(time);
    const x = Float64Array.from(timestamps).sort();
    const positions = new Map<number, number>();
    for (let row = 0; row < x.length; row += 1) positions.set(x[row], row);
    columns = [x];
    for (const source of sources) for (const values of source.columns) {
      const target = new Float64Array(x.length).fill(NaN);
      for (let row = 0; row < source.times.length; row += 1) {
        const position = positions.get(source.times[row]);
        if (position !== undefined && Number.isFinite(values[row])) target[position] = values[row];
      }
      columns.push(target);
    }
  }
  const groups = new Map<string, number[]>();
  for (let index = 0; index < series.length; index += 1) {
    const item = series[index];
    if (item.stack === "none") continue;
    const key = `${item.axis}:${item.stackGroup}`;
    const members = groups.get(key);
    if (members) members.push(index);
    else groups.set(key, [index]);
  }
  for (const members of groups.values()) {
    const percent = series[members[0]].stack === "percent";
    for (let row = 0; row < columns[0].length; row += 1) {
      let total = 0, cumulative = 0;
      if (percent) for (const index of members) {
        const value = columns[index + 1][row];
        if (Number.isFinite(value)) total += Math.max(0, value);
      }
      for (const index of members) {
        const values = columns[index + 1], value = values[row];
        if (!Number.isFinite(value)) continue;
        cumulative += percent ? total > 0 ? Math.max(0, value) / total * 100 : 0 : value;
        values[row] = cumulative;
      }
    }
  }
  const lastValues = columns.slice(1).map((values) => {
    for (let row = values.length - 1; row >= 0; row -= 1) if (Number.isFinite(values[row])) return values[row];
    return NaN;
  });
  const indices = minMaxIndices(columns, request.targetRows, request.start, request.end);
  const sampled = indices.length === columns[0].length ? columns : columns.map((values) =>
    Float64Array.from(indices, (index) => values[index]));
  return {
    columns: sampled, lastValues,
    inputRows: sources.reduce((sum, source) => sum + source.times.length, 0),
    alignedRows: columns[0].length,
  };
}
