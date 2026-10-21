import { prepareNumericModel, type NumericRequest } from "./numeric-model";

// Do not ship display processors or source DataFrames through structured clone.
// Every buffer here is owned by this job and can safely be transferred.
self.onmessage = (event: MessageEvent<NumericRequest>) => {
  try {
    const started = performance.now();
    const result = prepareNumericModel(event.data);
    self.postMessage({ result, durationMs: performance.now() - started },
      { transfer: result.columns.map((column) => column.buffer as ArrayBuffer) });
  } catch (error) {
    self.postMessage({ error: error instanceof Error ? error.message : "Time series preparation failed" });
  }
};
