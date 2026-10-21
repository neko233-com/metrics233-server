import { useEffect, useMemo, useState } from "react";
import type { DataFrame } from "@grafana/data";
import { getLocale } from "../../i18n";
import { prepareNumericModel, type NumericRequest, type NumericResult } from "./numeric-model";
import { completePlot, describePlot, EMPTY_PLOT, packPlotInSlices, preparePlotSync, WORKER_CELL_THRESHOLD, type PlotModel } from "./plot-model";

export function usePlotModel(frames: DataFrame[], width: number, start: number, end: number) {
  const locale = getLocale();
  const spec = useMemo(() => describePlot(frames), [frames, locale]);
  // Quantization avoids redoing the full job for each pixel of a resize drag.
  const targetRows = Math.max(256, Math.ceil(width / 64) * 128);
  const asynchronous = spec.cells > WORKER_CELL_THRESHOLD;
  const synchronous = useMemo(() => asynchronous ? undefined : preparePlotSync(spec, { targetRows, start, end }),
    [spec, asynchronous, targetRows, start, end]);
  const [completed, setCompleted] = useState<{ spec: typeof spec; targetRows: number; start: number; end: number; model: PlotModel }>();
  const [failed, setFailed] = useState<{ spec: typeof spec; targetRows: number; start: number; end: number }>();
  useEffect(() => {
    if (!asynchronous) {
      // Release the previous source-frame reference after a large-to-small
      // query transition, rather than keeping its buffers until unmount.
      setCompleted(undefined);
      setFailed(undefined);
      return;
    }
    if (width <= 0) return;
    const abort = new AbortController();
    let worker: Worker | undefined;
    const prepare = async () => {
      const sources = await packPlotInSlices(spec, abort.signal);
      if (abort.signal.aborted) return;
      const request: NumericRequest = { sources, series: spec.series.map(({ axis, stack, stackGroup }) =>
        ({ axis, stack, stackGroup })), targetRows, start, end };
      let preparation: PlotModel["preparation"] = "fallback";
      let result: NumericResult;
      if (typeof Worker !== "undefined") {
        // Each renderer owns at most one job; effect cleanup terminates obsolete
        // work instead of allowing refreshes to accumulate in a Worker queue.
        worker = new Worker(new URL("./numeric-model.worker.ts", import.meta.url), { type: "module" });
        result = await new Promise<NumericResult>((resolve, reject) => {
          worker!.onmessage = (event: MessageEvent<{ result?: NumericResult; error?: string }>) =>
            event.data.result ? resolve(event.data.result) : reject(new Error(event.data.error));
          worker!.onerror = (event) => { event.preventDefault(); reject(new Error(event.message)); };
          worker!.postMessage(request, sources.flatMap((source) =>
            [source.times.buffer as ArrayBuffer, ...source.columns.map((column) => column.buffer as ArrayBuffer)]));
        });
        preparation = "worker";
      } else result = prepareNumericModel(request);
      if (!abort.signal.aborted) {
        setFailed(undefined);
        setCompleted({ spec, targetRows, start, end, model: completePlot(spec, result, preparation) });
      }
    };
    void prepare().catch(async () => {
      if (abort.signal.aborted) return;
      // Repack after a Worker error because its transferred inputs are detached.
      // This supports restrictive CSPs and unavailable Worker implementations.
      const sources = await packPlotInSlices(spec, abort.signal);
      if (abort.signal.aborted) return;
      const result = prepareNumericModel({ sources, series: spec.series, targetRows, start, end });
      if (!abort.signal.aborted) {
        setFailed(undefined);
        setCompleted({ spec, targetRows, start, end, model: completePlot(spec, result, "fallback") });
      }
    }).catch(() => {
      // Cancellation is expected. A real failure must end the busy state and
      // surface a bounded UI error rather than silently leave stale data busy.
      if (!abort.signal.aborted) setFailed({ spec, targetRows, start, end });
    }).finally(() => worker?.terminate());
    return () => { abort.abort(); worker?.terminate(); };
  }, [asynchronous, spec, targetRows, start, end, width > 0]);
  const preparationFailed = failed?.spec === spec && failed?.targetRows === targetRows && failed?.start === start && failed?.end === end;
  const processing = asynchronous && !preparationFailed && (completed?.spec !== spec || completed?.targetRows !== targetRows ||
    completed?.start !== start || completed?.end !== end);
  return { model: synchronous || completed?.model || EMPTY_PLOT, processing, preparationFailed };
}
