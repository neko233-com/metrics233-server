import { useEffect, useState } from "react";

/** Wake at the next observation boundary, not on every browser-clock tick.
 * Visibility/pageshow also catch up after a frozen background tab or clock jump. */
export function useObservationClock(boundaries: readonly (number | undefined)[], active = true): number {
  const [, update] = useState(0);
  const now = Date.now();
  const next = boundaries.reduce<number>((earliest, at) => typeof at === "number" && Number.isFinite(at) && at > now ? Math.min(earliest, at) : earliest, Infinity);
  useEffect(() => {
    if (!active) return;
    const wake = () => update(value => value + 1);
    const visible = () => { if (!document.hidden) wake(); };
    let timer: ReturnType<typeof setTimeout> | undefined;
    const schedule = () => {
      const remaining = next - Date.now();
      // Keep the absolute deadline if the computer clock moves backwards.
      if (remaining > 0) timer = setTimeout(schedule, Math.min(2147483647, remaining));
      else wake();
    };
    if (Number.isFinite(next)) schedule();
    document.addEventListener("visibilitychange", visible);
    window.addEventListener("pageshow", wake);
    return () => { clearTimeout(timer); document.removeEventListener("visibilitychange", visible); window.removeEventListener("pageshow", wake); };
  }, [active, next]);
  return now;
}
