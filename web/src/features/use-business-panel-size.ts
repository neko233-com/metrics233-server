import { useState } from "react";

const key = "metricspanel-business-panel-size";
export function useBusinessPanelSize() {
  const [large, update] = useState(() => {
    try { return localStorage.getItem(key) === "large"; } catch { return false; }
  });
  return [large, (value: boolean) => {
    update(value);
    try { localStorage.setItem(key, value ? "large" : "normal"); } catch { /* The control still works without persistent storage. */ }
  }] as const;
}
