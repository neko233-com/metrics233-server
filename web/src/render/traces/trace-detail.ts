import type { DataFrame } from "@grafana/data";

export type TraceSpan = {
  id: string;
  parent: string;
  traceID: string;
  service: string;
  name: string;
  start: number;
  duration: number;
  status: string;
  message: string;
  kind: string;
  tags: string;
};

export type SpanTreeRow = { span: TraceSpan; depth: number; hasChildren: boolean };

export function traceSpans(frames: DataFrame[]): TraceSpan[] {
  const spans: TraceSpan[] = [];
  const seen = new Set<string>();
  for (const frame of frames) {
    const fields = new Map(frame.fields.map((field) => [field.name.toLowerCase(), field]));
    const ids = fields.get("spanid");
    if (!ids) continue;
    const value = (name: string, index: number) => fields.get(name)?.values[index];
    for (let index = 0; index < ids.values.length; index += 1) {
      const id = String(ids.values[index] ?? "");
      const start = Number(value("starttime", index));
      const duration = Number(value("durationms", index));
      if (!id || seen.has(id) || !Number.isFinite(start) || !Number.isFinite(duration) || duration < 0) continue;
      seen.add(id);
      spans.push({ id, parent: String(value("parentspanid", index) ?? ""),
        traceID: String(value("traceid", index) ?? ""), service: String(value("servicename", index) ?? ""),
        name: String(value("operationname", index) ?? ""), start, duration,
        status: String(value("status", index) ?? "unset"), message: String(value("statusmessage", index) ?? ""),
        kind: String(value("kind", index) ?? ""), tags: String(value("tags", index) ?? "{}") });
    }
  }
  return spans.sort((left, right) => left.start - right.start || left.id.localeCompare(right.id));
}

// Iterative traversal bounds stack usage and preserves otherwise disconnected
// spans when the upstream trace is partial or contains a parent cycle.
export function spanTree(spans: TraceSpan[], collapsed: ReadonlySet<string>): SpanTreeRow[] {
  const ids = new Set(spans.map((span) => span.id));
  const children = new Map<string, TraceSpan[]>();
  const roots: TraceSpan[] = [];
  for (const span of spans) {
    if (!span.parent || span.parent === span.id || !ids.has(span.parent)) roots.push(span);
    else {
      const siblings = children.get(span.parent) || [];
      siblings.push(span);
      children.set(span.parent, siblings);
    }
  }
  const rows: SpanTreeRow[] = [];
  const seen = new Set<string>();
  const visit = (root: TraceSpan) => {
    const stack = [{ span: root, depth: 0, hidden: false }];
    while (stack.length) {
      const row = stack.pop()!;
      if (seen.has(row.span.id)) continue;
      seen.add(row.span.id);
      const descendants = children.get(row.span.id) || [];
      if (!row.hidden) rows.push({ span: row.span, depth: row.depth, hasChildren: descendants.length > 0 });
      for (let index = descendants.length - 1; index >= 0; index -= 1)
        stack.push({ span: descendants[index], depth: row.depth + 1, hidden: row.hidden || collapsed.has(row.span.id) });
    }
  };
  for (const span of roots) visit(span);
  for (const span of spans) if (!seen.has(span.id)) visit(span);
  return rows;
}

export function traceDuration(value: number) {
  if (!Number.isFinite(value)) return "—";
  if (value >= 1000) return `${(value / 1000).toFixed(2)} s`;
  if (value > 0 && value < 1) return `${(value * 1000).toFixed(1)} µs`;
  return `${value.toFixed(value < 10 ? 2 : 0)} ms`;
}
