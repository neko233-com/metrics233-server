import type { Panel } from "../api";

export function nativeStateMapping(panel?: Panel): string {
  const mappings = panel?.config?.fieldConfig?.defaults?.mappings;
  if (!mappings?.length) return "";
  if (mappings.length === 1 && mappings[0].type === "value") {
    const options = mappings[0].options;
    if (
      Object.keys(options).length === 2 &&
      options["0"]?.text === "Unavailable" &&
      options["0"]?.color === "red" &&
      options["1"]?.text === "Available" &&
      options["1"]?.color === "green"
    )
      return "availability";
  }
  return "custom";
}
