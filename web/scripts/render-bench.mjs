// Production-bundle browser benchmark. Synthetic API responses exclude storage
// and server query execution. Run after building the embedded UI binary:
//   RENDER_BENCH_POINTS=100000 RENDER_BENCH_SERIES=5 node scripts/render-bench.mjs
import { chromium } from "@playwright/test";
import { spawn } from "node:child_process";
import { mkdtemp, rm, writeFile } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";

const repoRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "../..");
const binary = process.env.METRICSPANEL_TEST_BINARY ||
  path.join(repoRoot, "bin", process.platform === "win32" ? "metricspanel.exe" : "metricspanel");
const points = Number(process.env.RENDER_BENCH_POINTS || 100000);
const series = Number(process.env.RENDER_BENCH_SERIES || 5);
const runs = Number(process.env.RENDER_BENCH_RUNS || 3);
if (![points, series, runs].every(Number.isSafeInteger) || points < 2 || points > 1000000 || series < 1 || series > 100 || runs < 1 || runs > 20)
  throw new Error("Benchmark requires 2..1000000 points, 1..100 series and 1..20 runs");
const temp = await mkdtemp(path.join(os.tmpdir(), "mp-render-bench-"));
const server = spawn(binary, ["serve", "--listen", "127.0.0.1:0", "--db", path.join(temp, "control.db")], {
  cwd: repoRoot, windowsHide: true,
  env: { ...process.env, METRICSPANEL_TOKEN: "", METRICSPANEL_STORAGE: "sqlite" },
});
let endpoint = "", pending = "", spawnError;
server.on("error", (error) => { spawnError = error; });
server.stderr.on("data", (chunk) => {
  pending += chunk.toString();
  for (;;) {
    const end = pending.indexOf("\n");
    if (end < 0) break;
    const line = pending.slice(0, end); pending = pending.slice(end + 1);
    try { const item = JSON.parse(line); if (item.address) endpoint = `http://${item.address}`; } catch {}
  }
});
let browser;
try {
  const deadline = Date.now() + 15000;
  while (!endpoint && !spawnError && server.exitCode === null && Date.now() < deadline)
    await new Promise((resolve) => setTimeout(resolve, 100));
  if (!endpoint) throw spawnError || new Error("Benchmark server did not start");
  browser = await chromium.launch();
  const page = await browser.newPage({ viewport: { width: 1536, height: 1024 }, locale: "en-US" });
  const from = 1791590400000, to = from + 3600000;
  const times = Array.from({ length: points }, (_, row) => from + Math.round(row * (to - from) / (points - 1)));
  const fields = Array.from({ length: series }, (_, index) => ({ name: `Series ${index + 1}`, type: "number", config: {} }));
  const values = fields.map((_, column) => Array.from({ length: points }, (_, row) =>
    row === Math.floor(points * 0.54321) ? 9999 + column : row === Math.floor(points * 0.54321) + 1 ? -8888 - column :
      row >= Math.floor(points * 0.7) && row < Math.floor(points * 0.71) ? null : Math.sin(row / 37 + column) * (column + 1)));
  const responseBody = JSON.stringify({ results: { A: { status: 200, frames: [{ schema: { refId: "A", fields: [
    { name: "Time", type: "time", config: {} }, ...fields,
  ] }, data: { values: [times, ...values] } }] } } });
  await page.route("**/api/ds/query**", (route) => route.fulfill({ contentType: "application/json", body: responseBody }));
  await page.addInitScript(() => {
    localStorage.setItem("metricspanel-renderer", "own");
    localStorage.setItem("metricspanel-locale", "en");
    const stats = { jobs: [], longTasks: [], transferredBytes: 0, detachedBuffers: 0 };
    window.__renderBench = stats;
    new PerformanceObserver((list) => {
      for (const entry of list.getEntries()) stats.longTasks.push(entry.duration);
    }).observe({ type: "longtask", buffered: true });
    const NativeWorker = window.Worker;
    window.Worker = class extends NativeWorker {
      constructor(url, options) {
        super(url, options);
        this.addEventListener("message", (event) => {
          if (event.data.result) stats.jobs.push({ workerMs: event.data.durationMs,
            turnaroundMs: performance.now() - this.postedAt,
            inputRows: event.data.result.inputRows, renderedRows: event.data.result.columns[0].length });
        });
      }
      postMessage(message, transfer) {
        this.postedAt = performance.now();
        stats.transferredBytes += transfer.reduce((sum, buffer) => sum + (buffer.byteLength || 0), 0);
        super.postMessage(message, transfer);
        stats.detachedBuffers += transfer.filter((buffer) => buffer instanceof ArrayBuffer && !buffer.byteLength).length;
      }
    };
  });
  const created = await page.request.post(endpoint + "/api/v1/dashboards", { data: {
    name: "Renderer benchmark", panels: [{ id: "bench", title: "Benchmark", metric: "", unit: "", aggregation: "last",
      visualization: "timeseries", config: { id: 7, type: "timeseries", gridPos: { x: 0, y: 0, w: 24, h: 16 },
        targets: [{ refId: "A", expr: "renderer_bench", datasource: { uid: "metricspanel", type: "prometheus" } }] } }],
  } });
  if (!created.ok()) throw new Error(await created.text());
  const saved = await created.json(), observations = [], issues = [];
  page.on("pageerror", (error) => issues.push(error.message));
  const url = `${endpoint}/d/${saved.id}/benchmark?from=${from}&to=${to}&timezone=utc`;
  for (let run = 0; run < runs; run += 1) {
    const started = performance.now();
    await page.goto(url);
    const panel = page.getByRole("region", { name: "Benchmark", exact: true });
    await panel.locator("canvas").waitFor({ state: "visible", timeout: 60000 });
    await page.waitForFunction(() => document.querySelector('.native-timeseries-panel')?.getAttribute("aria-busy") === "false");
    await page.evaluate(() => new Promise((resolve) => requestAnimationFrame(() => requestAnimationFrame(resolve))));
    const renderedMs = performance.now() - started;
    const metrics = await panel.evaluate((element) => ({
      preparation: element.dataset.preparation, inputRows: Number(element.dataset.inputRows),
      renderedRows: Number(element.dataset.renderedRows), memoryBytes: performance.memory?.usedJSHeapSize || null,
      ...window.__renderBench,
    }));
    if (metrics.inputRows !== points) throw new Error("Embedded UI is stale or fixture was not rendered");
    if ((series + 1) * points > 50000 && metrics.preparation !== "worker") throw new Error("Large fixture did not use a Worker");
    observations.push({ run: run + 1, renderedMs, ...metrics });
  }
  if (issues.length) throw new Error(issues.join("\n"));
  const result = { measuredAt: new Date().toISOString(), platform: `${os.platform()}/${os.arch()}`, cpu: os.cpus()[0]?.model,
    node: process.version, chromium: browser.version(), viewport: "1536x1024", points, series, responseBytes: Buffer.byteLength(responseBody),
    scope: "Synthetic API response to production-bundle canvas; excludes storage and server query execution", observations };
  console.log(JSON.stringify(result, null, 2));
  if (process.env.RENDER_BENCH_OUTPUT) await writeFile(path.resolve(process.env.RENDER_BENCH_OUTPUT), JSON.stringify(result, null, 2) + "\n");
} finally {
  await browser?.close();
  if (server.exitCode === null && !spawnError) {
    const exited = new Promise((resolve) => server.once("exit", resolve));
    server.kill(); await exited;
  }
  const resolved = path.resolve(temp);
  if (!resolved.startsWith(path.resolve(os.tmpdir()) + path.sep) || !path.basename(resolved).startsWith("mp-render-bench-"))
    throw new Error("Unsafe benchmark cleanup path");
  await rm(resolved, { recursive: true, force: true, maxRetries: 3 });
}
