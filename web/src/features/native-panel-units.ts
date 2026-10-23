import type { Panel } from "../api";

export const preserveCustomUnit = "__preserve_custom_unit__";
const basicUnits = ["", "bytes", "Bps", "seconds", "uptime", "percent", "count", "ops"];
function sdkUnit(unit: string) {
  return unit === "seconds" ? "s" : unit === "count" ? "short" : unit;
}

export function nativePanelUnitSelection(panel?: Panel): string {
  const unit = panel?.unit || "";
  const display = panel?.config?.fieldConfig?.defaults?.unit;
  if (display === "binBps") return "binBps";
  return display != null && display !== sdkUnit(unit) ? preserveCustomUnit : unit;
}

// Send only the narrow display-unit edit; the server owns merging saved config.
export function nativePanelUnitEdit(panel: Panel | undefined, selection: string): { unit: string; display_unit?: "" | "binBps" } {
  if (selection === preserveCustomUnit) return { unit: panel?.unit || "" };
  if (selection === "binBps") return { unit: "bytes", display_unit: "binBps" };
  if (!basicUnits.includes(selection)) throw new Error("Unsupported panel unit");
  const display = panel?.config?.fieldConfig?.defaults?.unit;
  return display != null && display !== sdkUnit(selection)
    ? { unit: selection, display_unit: "" }
    : { unit: selection };
}
