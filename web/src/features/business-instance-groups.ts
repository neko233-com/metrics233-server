// Only explicit environment values identify these groups. A missing or unknown
// value never inherits the production label from a service name or server role.
export function groupBusinessInstances<T extends { identity: { environment: string } }>(items: T[]) {
  const groups = new Map<string, T[]>();
  for (const item of items) {
    const environment = item.identity.environment || "";
    const entries = groups.get(environment) || [];
    entries.push(item);
    groups.set(environment, entries);
  }
  const order = ["release", "test", "intranet", "shared"];
  return [...groups].sort(([a], [b]) => {
    const aIndex = order.indexOf(a), bIndex = order.indexOf(b);
    return (aIndex < 0 ? order.length : aIndex) - (bIndex < 0 ? order.length : bIndex) || a.localeCompare(b);
  }).map(([environment, instances]) => ({ environment, instances }));
}
