/** Presentation uses the computer's local timezone; transport timestamps stay unchanged. */
export function formatTimestamp(value: number | string | Date | null | undefined): string {
  if (value == null || value === "") return "—";
  const date = value instanceof Date ? value : new Date(value);
  if (!Number.isFinite(date.getTime())) return "—";
  const pad = (part: number) => String(part).padStart(2, "0");
  return `${String(date.getFullYear()).padStart(4, "0")}-${pad(date.getMonth() + 1)}-${pad(date.getDate())} ${pad(date.getHours())}:${pad(date.getMinutes())}:${pad(date.getSeconds())}`;
}

export function browserTimezoneLabel(at = new Date()): string {
  const minutes = -at.getTimezoneOffset(), absolute = Math.abs(minutes);
  return `GMT${minutes < 0 ? "-" : "+"}${Math.floor(absolute / 60)}${absolute % 60 ? `:${String(absolute % 60).padStart(2, "0")}` : ""}`;
}

export function formatChartTimestamp(value: number): string {
  return formatTimestamp(value).replace(" ", "\n");
}

export function formatBytes(value: number): string {
  if (!Number.isFinite(value)) return "—";
  const units = ["B", "KB", "MB", "GB", "TB", "PB"];
  const index = Math.min(units.length - 1, Math.max(0, Math.floor(Math.log10(Math.max(Math.abs(value), 1)) / 3)));
  return `${(value / 1000 ** index).toLocaleString("en", { useGrouping: false, maximumFractionDigits: 1 })} ${units[index]}`;
}
