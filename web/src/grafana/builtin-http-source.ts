import type { DataQuery } from "@grafana/data";

export function isBuiltinHTTPSource(type: string) {
  return type === "prometheus" || type === "loki" || type === "tempo";
}

export function interpolateHTTPQuery<T extends DataQuery>(query: T, replace: (value: string) => string): T {
  const output = { ...query } as Record<string, unknown>;
  for (const key of ["expr", "query", "traceql", "traceId", "legendFormat"] as const) {
    const value = (query as Record<string, unknown>)[key];
    if (typeof value === "string") output[key] = replace(value);
  }
  return output as T;
}
