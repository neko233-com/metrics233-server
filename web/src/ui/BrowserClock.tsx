import { useEffect, useState } from "react";
import { browserTimezoneLabel, formatTimestamp } from "../display-format";

/** A separate clock keeps one-second updates out of the dashboard render tree. */
export function BrowserClock() {
  const [now, setNow] = useState(Date.now);
  useEffect(() => {
    const timer = window.setInterval(() => { if (!document.hidden) setNow(Date.now()); }, 1000);
    const refresh = () => setNow(Date.now());
    document.addEventListener("visibilitychange", refresh);
    return () => { window.clearInterval(timer); document.removeEventListener("visibilitychange", refresh); };
  }, []);
  return <span className="browser-clock" title={Intl.DateTimeFormat().resolvedOptions().timeZone}>
    <time dateTime={new Date(now).toISOString()}>{formatTimestamp(now)}</time>
    <span className="browser-timezone">{browserTimezoneLabel(new Date(now))}</span>
  </span>;
}
