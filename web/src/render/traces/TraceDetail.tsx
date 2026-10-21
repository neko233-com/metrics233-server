import { useEffect, useMemo, useState } from "react";
import { toDataFrame, type DataFrameJSON } from "@grafana/data";
import { ChevronDown, ChevronRight, X } from "lucide-react";
import { api } from "../../api";
import { t } from "../../i18n";
import type { RendererProps } from "../types";
import { spanTree, traceDuration, traceSpans } from "./trace-detail";

type DetailState = { id: string; loading: boolean; error: string; frames: RendererProps["frames"] };
type Props = { traceID: string; datasourceUID: string; frames: RendererProps["frames"]; from: number; to: number; onClose?: () => void };
const pageSize = 100;

export default function TraceDetail({ traceID, datasourceUID, frames, from, to, onClose }: Props) {
  const embedded = useMemo(() => traceSpans(frames), [frames]);
  const [state, setState] = useState<DetailState>({ id: "", loading: false, error: "", frames: [] });
  const [retry, setRetry] = useState(0);
  const [collapsed, setCollapsed] = useState<Set<string>>(() => new Set());
  const [page, setPage] = useState(0);
  const [selectedID, setSelectedID] = useState("");
  const hasEmbedded = embedded.some((span) => span.traceID === traceID || !span.traceID);
  useEffect(() => {
    setCollapsed(new Set());
    setPage(0);
    setSelectedID("");
  }, [traceID]);
  useEffect(() => {
    if (hasEmbedded || !datasourceUID) return;
    const controller = new AbortController();
    setState({ id: traceID, loading: true, error: "", frames: [] });
    api<{ results: Record<string, { frames?: DataFrameJSON[]; error?: string }> }>("/api/ds/query", {
      method: "POST", signal: controller.signal,
      body: JSON.stringify({ from: String(from), to: String(to), queries: [{ refId: "TraceDetail", datasource: { uid: datasourceUID, type: "tempo" }, queryType: "traceId", traceId: traceID }] }),
    }).then((response) => {
      if (controller.signal.aborted) return;
      const result = response.results.TraceDetail;
      if (!result) throw new Error("Tempo trace response is missing");
      if (result.error) throw new Error(result.error);
      setState({ id: traceID, loading: false, error: "", frames: (result.frames || []).map(toDataFrame) });
    }).catch((error: unknown) => {
      if (!controller.signal.aborted)
        setState({ id: traceID, loading: false, frames: [], error: error instanceof Error ? error.message : String(error) });
    });
    return () => controller.abort();
  }, [datasourceUID, from, hasEmbedded, retry, to, traceID]);

  const detailSpans = useMemo(() => hasEmbedded ? embedded.filter((span) => !span.traceID || span.traceID === traceID) : state.id === traceID ? traceSpans(state.frames) : [], [embedded, hasEmbedded, state, traceID]);
  const rows = useMemo(() => spanTree(detailSpans, collapsed), [collapsed, detailSpans]);
  const bounds = useMemo(() => {
    let start = Infinity, end = -Infinity, errors = 0;
    for (const span of detailSpans) {
      start = Math.min(start, span.start);
      end = Math.max(end, span.start + span.duration);
      if (span.status === "error") errors += 1;
    }
    return { start, duration: end - start, errors };
  }, [detailSpans]);
  const selected = detailSpans.find((span) => span.id === selectedID);
  const currentPage = Math.min(page, Math.max(0, Math.ceil(rows.length / pageSize) - 1));
  const visible = rows.slice(currentPage * pageSize, (currentPage + 1) * pageSize);
  const error = !hasEmbedded && !datasourceUID ? t("Select a Tempo data source to load trace details") : state.id === traceID ? state.error : "";
  const busy = !hasEmbedded && !!datasourceUID && (state.id !== traceID || state.loading);
  const toggle = (id: string) => setCollapsed((previous) => {
    const next = new Set(previous);
    if (next.has(id)) next.delete(id); else next.add(id);
    return next;
  });

  return <section className="native-trace-detail" aria-label={t("Trace details")} data-span-count={detailSpans.length}>
    <div className="native-trace-detail-heading">
      <strong>{t("Trace details")}</strong><code title={traceID}>{traceID}</code>
      {onClose ? <button type="button" className="icon-button" aria-label={t("Close trace details")} onClick={onClose}><X size={15} /></button> : null}
    </div>
    {error ? <div className="form-error" role="alert">{error}{datasourceUID ? <button type="button" onClick={() => setRetry((value) => value + 1)}>{t("Retry")}</button> : null}</div> : null}
    {busy ? <p role="status" className="native-traces-empty">{t("Loading trace details…")}</p> : null}
    {!busy && !error && !detailSpans.length ? <p role="status" className="native-traces-empty">{t("No spans found for this trace")}</p> : null}
    {detailSpans.length ? <>
      <p className="native-trace-summary" role="status">{detailSpans.length} {t("spans")} · {traceDuration(bounds.duration)} · {bounds.errors} {t("errors")}</p>
      <div className="native-trace-waterfall">
        <table aria-label={t("Trace span timeline")}>
          <thead><tr><th scope="col">{t("Service / operation")}</th><th scope="col">{t("Timeline")}</th><th scope="col">{t("Duration")}</th><th scope="col">{t("Status")}</th></tr></thead>
          <tbody>{visible.map(({ span, depth, hasChildren }) => {
            const left = bounds.duration > 0 ? (span.start - bounds.start) / bounds.duration * 100 : 0;
            const width = bounds.duration > 0 ? span.duration / bounds.duration * 100 : 0;
            return <tr key={span.id} data-span-id={span.id} data-parent-span-id={span.parent} data-depth={depth} data-status={span.status}>
              <td><div className="native-trace-operation" style={{ paddingLeft: Math.min(depth, 12) * 14 }}>
                {hasChildren ? <button className="icon-button" type="button" aria-label={`${collapsed.has(span.id) ? t("Expand") : t("Collapse")} ${span.name}`} aria-expanded={!collapsed.has(span.id)} onClick={() => toggle(span.id)}>{collapsed.has(span.id) ? <ChevronRight size={13} /> : <ChevronDown size={13} />}</button> : <span className="native-trace-leaf" />}
                <button className="native-trace-span-select" onClick={() => setSelectedID(span.id)} title={`${span.service} / ${span.name}`}><span>{span.service || "—"}</span><strong>{span.name || span.id}</strong></button>
              </div></td>
              <td><div className="native-trace-track" role="img" aria-label={`${span.name}: +${traceDuration(span.start - bounds.start)}, ${traceDuration(span.duration)}`}><span style={{ left: `${Math.max(0, left)}%`, width: `${Math.max(0, Math.min(100 - left, width))}%` }} /></div></td>
              <td className="mono">{traceDuration(span.duration)}</td><td><span className={span.status === "error" ? "native-traces-status" : ""}>{span.status}</span></td>
            </tr>;
          })}</tbody>
        </table>
      </div>
      {rows.length > pageSize ? <div className="native-trace-pagination"><button type="button" disabled={currentPage === 0} onClick={() => setPage(currentPage - 1)}>{t("Previous")}</button><span>{currentPage + 1} / {Math.ceil(rows.length / pageSize)}</span><button type="button" disabled={(currentPage + 1) * pageSize >= rows.length} onClick={() => setPage(currentPage + 1)}>{t("Next")}</button></div> : null}
      {selected ? <aside className="native-trace-span-details" aria-label={t("Span attributes")}><strong>{selected.name}</strong><code>{selected.id}</code>{selected.message ? <p role={selected.status === "error" ? "alert" : undefined}>{selected.message}</p> : null}<details open><summary>{t("Attributes")}{selected.kind ? ` · ${selected.kind}` : ""}</summary><pre>{formatTags(selected.tags)}</pre></details></aside> : null}
    </> : null}
  </section>;
}

function formatTags(tags: string) {
  try { return JSON.stringify(JSON.parse(tags), null, 2); } catch { return tags; }
}
