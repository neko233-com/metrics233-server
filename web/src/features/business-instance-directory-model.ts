export type BusinessIdentity = { service: string; role: string; serverId: string; environment: string; channel: string; instance: string; implementation: string };
export type BusinessNote = { version: number; description: string; updated_at: number };
export type BusinessInstance = { id: string; identity: BusinessIdentity; reported_ip: string; observed_ip: string; first_seen: number; last_seen: number; note?: BusinessNote; binding: { version: number; mode: "default" | "inherit" | "independent"; dashboard_id: string } };
export type InstanceSummary = { host_name?: string; host_observed_at?: number; host_expires_at?: number; cpu_percent: number | null; memory_percent: number | null; disk_percent: number | null; memory_total?: number; memory_available?: number; receive_per_second: number | null; transmit_per_second: number | null; traffic_scope: "payload" | "http" };
export type InstancePage = { items: BusinessInstance[]; next_cursor: string; capacity: number; at_capacity: boolean; total: number; matched: number; counts: Record<string, number>; environments: string[]; summaries: Record<string, InstanceSummary> };
export type DirectorySort = "server" | "recent" | "host";

export function instanceTitle(identity: BusinessIdentity, chinese: boolean): string {
  const role = identity.role === "gateway" ? (chinese ? "网关" : "Gateway")
    : ["logic", "game"].includes(identity.role) ? (chinese ? "游戏服" : "Game server") : identity.role === "sdk" ? "SDK" : identity.service;
  return `${role} ${identity.serverId}`.trim();
}

export function sortDirectory(items: BusinessInstance[], sort: DirectorySort): BusinessInstance[] {
  const natural = (a: string, b: string) => a.localeCompare(b, "en", { numeric: true });
  return [...items].sort((a, b) => {
    const primary = sort === "recent" ? b.last_seen - a.last_seen : sort === "host"
      ? natural(a.reported_ip || a.observed_ip, b.reported_ip || b.observed_ip)
      : natural(a.identity.serverId, b.identity.serverId);
    return primary || natural(a.identity.service, b.identity.service) || natural(a.identity.instance, b.identity.instance) || a.id.localeCompare(b.id);
  });
}

export function reportFresh(timestamp: number, now = Date.now()): boolean {
  return timestamp > 0 && timestamp <= now + 30000 && now - timestamp <= 90000;
}
