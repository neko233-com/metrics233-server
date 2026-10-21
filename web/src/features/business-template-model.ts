import type { Dashboard } from "../api";

export type TemplateInstance = {
  id: string;
  identity: { service: string; role: string; serverId: string; environment: string; channel: string; instance: string; implementation: string };
  last_seen: number;
  binding: { version: number; mode: "default" | "inherit" | "independent"; dashboard_id: string };
};
export type TemplateInstancePage = { items: TemplateInstance[]; next_cursor: string; capacity: number; at_capacity: boolean };
export type ResolvedTemplateInstance = { instance: TemplateInstance; dashboard: Dashboard };

export function hasBusinessScope(dashboard: Dashboard): boolean {
  return !dashboard.grafana && !!dashboard.variables?.some(variable => variable.name === "business_scope");
}

export function isUnboundBusinessTemplate(dashboard: Dashboard): boolean {
  return hasBusinessScope(dashboard) && dashboard.variables!.some(variable =>
    variable.name === "business_scope" &&
    String(variable.config?.current?.value ?? variable.current ?? variable.options?.[0] ?? "").trim() === 'service="__unbound_template__"');
}

export function isBusinessTemplateCandidate(dashboard: Dashboard): boolean {
  return isUnboundBusinessTemplate(dashboard) || (hasBusinessScope(dashboard) && dashboard.variables!.some(variable =>
    variable.name === "business_scope" &&
    !String(variable.config?.current?.value ?? variable.current ?? variable.options?.[0] ?? "").trim()));
}

// The server binding, rather than a dashboard name or caller-supplied owner marker,
// determines whether an instance can preview this shared template.
export function boundTemplateInstances(items: TemplateInstance[], templateID: string): TemplateInstance[] {
  return items.filter(item => item.binding.mode === "inherit" && item.binding.dashboard_id === templateID);
}

export function templateSelection(items: TemplateInstance[], nextCursor: string, cursor: string, previous: string): string {
  if (items.some(item => item.id === previous)) return previous;
  return !cursor && !nextCursor && items.length === 1 ? items[0].id : "";
}

export function isResolvedTemplateInstance(value: ResolvedTemplateInstance, instanceID: string, templateID: string): boolean {
  return value.instance.id === instanceID && value.dashboard.id === templateID &&
    value.instance.binding.mode === "inherit" && value.instance.binding.dashboard_id === templateID;
}
