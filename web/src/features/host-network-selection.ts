export type NetworkChoice = { scope: string; device: string; manual: boolean; pinned: boolean };
type Interface = { device: string; receive_per_second: number | null; transmit_per_second: number | null };
const physical = (name: string) => /^(eth|en|bond)/i.test(name);
const virtual = (name: string) => /^(docker|veth|br-|bridge|virbr|cni|flannel|utun|tun|tap|lo)/i.test(name);
const rate = (value: number | null) => typeof value === "number" && Number.isFinite(value) && value >= 0 ? value : 0;

export function recommendHostInterface(network: Interface[], active: boolean) {
  const devices = [...network].sort((a, b) => a.device < b.device ? -1 : a.device > b.device ? 1 : 0);
  let selected: Interface | undefined, score = 0;
  if (active) for (const item of devices) {
    const current = rate(item.receive_per_second) / 2 + rate(item.transmit_per_second) / 2;
    if (current > 0 && (!selected || physical(item.device) && !physical(selected.device) || physical(item.device) === physical(selected.device) && current > score)) {
      selected = item; score = current;
    }
  }
  const fallback = devices.find(item => physical(item.device)) || devices.find(item => !virtual(item.device)) || devices[0];
  return { device: (selected || fallback)?.device || "", hasTraffic: !!selected };
}

// Preserve the first useful automatic choice and all explicit choices, even
// when a NIC disappears. Data from a different NIC must never replace it silently.
export function resolveNetworkChoice(previous: NetworkChoice | null, scope: string, devices: string[], recommended: string, hasTraffic: boolean, active: boolean): NetworkChoice {
  if (previous?.scope === scope && (previous.manual || !active || !devices.length || previous.pinned && devices.includes(previous.device))) return previous;
  const device = devices.includes(recommended) ? recommended : devices[0] || "";
  const pinned = active && hasTraffic && !!device;
  if (previous?.scope === scope && previous.device === device && previous.pinned === pinned) return previous;
  return { scope, device, manual: false, pinned };
}
