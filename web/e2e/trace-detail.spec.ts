import { test as base, expect } from "@playwright/test";
import { spawn } from "node:child_process";
import { createServer } from "node:http";
import { mkdtemp, readFile, rm } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "../..");
const traceID = "0123456789abcdef0123456789abcdef";
const missingID = "ffffffffffffffffffffffffffffffff";
const failedID = "eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee";
const test = base.extend<{}, { endpoint: string }>({ endpoint: [async ({}, use) => {
  const temp = await mkdtemp(path.join(os.tmpdir(), "metricspanel233-trace-detail-e2e-"));
  const service = spawn(process.env.METRICSPANEL_TEST_BINARY || path.join(root, "bin", process.platform === "win32" ? "metricspanel.exe" : "metricspanel"),
    ["serve", "--listen", "127.0.0.1:0", "--db", path.join(temp, "control.db")],
    { cwd: root, windowsHide: true, env: { ...process.env, METRICSPANEL_TOKEN: "", METRICSPANEL_STORAGE: "sqlite" } });
  let endpoint = "", pending = "";
  service.stderr!.on("data", (chunk) => {
    pending += chunk.toString();
    for (;;) {
      const end = pending.indexOf("\n");
      if (end < 0) break;
      const line = pending.slice(0, end); pending = pending.slice(end + 1);
      try { const item = JSON.parse(line); if (item.address) endpoint = `http://${item.address}`; } catch {}
    }
  });
  try { await expect.poll(() => endpoint, { timeout: 15000 }).not.toBe(""); await use(endpoint); }
  finally {
    if (service.exitCode === null) { const exited = new Promise<void>((resolve) => service.once("exit", () => resolve())); service.kill(); await exited; }
    const resolved = path.resolve(temp);
    if (!resolved.startsWith(path.resolve(os.tmpdir()) + path.sep) || !path.basename(resolved).startsWith("metricspanel233-trace-detail-e2e-")) throw new Error("Unsafe trace fixture cleanup");
    await rm(resolved, { recursive: true, force: true, maxRetries: 3 });
  }
}, { scope: "worker" }] });

test("Tempo detail queries the backend and renders hierarchy, waterfall, errors and persisted deep links", async ({ page, endpoint }) => {
  test.setTimeout(90000);
  const errors: string[] = [];
  page.on("pageerror", (error) => errors.push(error.message));
  await page.addInitScript(() => localStorage.setItem("metricspanel-locale", "en"));
  const fixture = await readFile(path.join(root, "internal/server/testdata/tempo-trace.json"));
  const upstreamRequests: string[] = [];
  const upstream = createServer((request, response) => {
    upstreamRequests.push(request.url || "");
    response.setHeader("Content-Type", "application/json");
    if (request.url?.startsWith("/api/search?")) response.end(JSON.stringify({ traces: [{ traceID, rootServiceName: "checkout", rootTraceName: "POST /checkout", startTimeUnixNano: "1791633600000000000", durationMs: 120.5 }] }));
    else if (request.url?.startsWith("/loki/api/v1/query_range?")) response.end(JSON.stringify({ status: "success", data: { resultType: "streams", result: [{ stream: { service: "checkout" }, values: [["1791633600000000000", "checkout deadline exceeded"]] }] } }));
    else if (request.url === `/api/traces/${traceID}`) response.end(fixture);
    else if (request.url === `/api/traces/${failedID}`) { response.writeHead(503); response.end("upstream unavailable"); }
    else { response.writeHead(404); response.end("missing"); }
  });
  await new Promise<void>((resolve) => upstream.listen(0, "127.0.0.1", resolve));
  const address = upstream.address();
  if (!address || typeof address === "string") throw new Error("Fixture did not listen");
  const uid = `tempo-detail-${address.port}`;
  try {
    const savedSource = await page.request.post(endpoint + "/api/datasources", { data: { uid, name: uid, type: "tempo", url: `http://127.0.0.1:${address.port}`, access: "proxy" } });
    expect(savedSource.ok(), await savedSource.text()).toBeTruthy();
    const savedDashboard = await page.request.post(endpoint + "/api/v1/dashboards", { data: { name: "Trace detail contract", panels: [{ id: "trace-detail", title: "Checkout traces", metric: "", unit: "", aggregation: "last", visualization: "traces", config: { id: 1, type: "traces", gridPos: { x: 0, y: 0, w: 24, h: 24 }, targets: [{ refId: "A", datasource: { uid, type: "tempo" }, query: "{}" }] } }] } });
    expect(savedDashboard.ok(), await savedDashboard.text()).toBeTruthy();
    const dashboard = await savedDashboard.json();
    const dashboardURL = `${endpoint}/d/${dashboard.id}/trace-detail-contract?from=1791633600000&to=1791634200000&timezone=utc`;
    await page.goto(dashboardURL);
    const panel = page.getByRole("region", { name: "Checkout traces", exact: true });
    const detail = panel.getByRole("region", { name: "Trace details", exact: true });
    await panel.getByRole("link", { name: traceID, exact: true }).click();
    await expect(page).toHaveURL(new RegExp(`traceId=${traceID}`));
    await expect(detail).toHaveAttribute("data-span-count", "4");
    const timeline = detail.getByRole("table", { name: "Trace span timeline", exact: true });
    await expect(timeline.locator("tbody tr")).toHaveCount(4);
    const db = timeline.locator('[data-span-id="0000000000000002"]');
    await expect(db).toHaveAttribute("data-parent-span-id", "0000000000000001");
    await expect(db).toHaveAttribute("data-depth", "1");
    await expect(timeline.locator('[data-span-id="0000000000000003"]')).toHaveAttribute("data-depth", "2");
    await expect(db).toHaveAttribute("data-status", "error");
    await expect(db).toContainText("25 ms");
    await expect(db.getByRole("img")).toHaveAttribute("aria-label", "INSERT orders: +10 ms, 25 ms");
    await db.getByRole("button", { name: "orders-db INSERT orders", exact: true }).click();
    const attributes = detail.getByRole("complementary", { name: "Span attributes", exact: true });
    await expect(attributes).toContainText("database deadline exceeded");
    await expect(attributes).toContainText('"db.system.name": "mysql"');
    await detail.getByRole("button", { name: "Collapse INSERT orders", exact: true }).click();
    await expect(timeline.locator("tbody tr")).toHaveCount(3);
    await detail.getByRole("button", { name: "Expand INSERT orders", exact: true }).click();
    await expect(timeline.locator("tbody tr")).toHaveCount(4);
    for (const width of [390, 768, 1280]) {
      await page.setViewportSize({ width, height: 1000 });
      expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
    }
    await page.reload();
    await expect(detail).toHaveAttribute("data-span-count", "4");
    expect(upstreamRequests).toContain(`/api/traces/${traceID}`);
    expect(upstreamRequests.some((url) => url.startsWith(`/api/traces/${traceID}?`))).toBe(false);
    await detail.getByRole("button", { name: "Close trace details", exact: true }).click();
    await expect(detail).toHaveCount(0);
    await expect(page).not.toHaveURL(/traceId=/);
    await page.goBack();
    await expect(detail).toHaveAttribute("data-span-count", "4");
    await page.goto(`${dashboardURL}&traceId=${missingID}`);
    await expect(detail).toContainText("No spans found for this trace");
    await page.goto(`${dashboardURL}&traceId=${failedID}`);
    await expect(detail.getByRole("alert")).toContainText("HTTP 503");
    const beforeRetry = upstreamRequests.filter((url) => url === `/api/traces/${failedID}`).length;
    await detail.getByRole("button", { name: "Retry", exact: true }).click();
    await expect.poll(() => upstreamRequests.filter((url) => url === `/api/traces/${failedID}`).length).toBeGreaterThan(beforeRetry);
    await expect(detail.getByRole("alert")).toContainText("HTTP 503");
    const lokiSource = await page.request.post(endpoint + "/api/datasources", { data: { uid: uid + "-loki", name: "Builtin Loki HTTP", type: "loki", url: `http://127.0.0.1:${address.port}`, access: "proxy" } });
    expect(lokiSource.ok(), await lokiSource.text()).toBeTruthy();
    const lokiDashboard = await page.request.post(endpoint + "/api/v1/dashboards", { data: { name: "Builtin Loki contract", panels: [{ id: "loki", title: "Loki HTTP logs", metric: "", unit: "", aggregation: "last", visualization: "logs", config: { id: 1, type: "logs", targets: [{ refId: "A", datasource: { uid: uid + "-loki", type: "loki" }, expr: '{service="checkout"}' }] } }] } });
    expect(lokiDashboard.ok(), await lokiDashboard.text()).toBeTruthy();
    await page.goto(`${endpoint}/d/${(await lokiDashboard.json()).id}/builtin-loki-contract?from=1791633600000&to=1791634200000`);
    await expect(page.getByRole("region", { name: "Loki HTTP logs", exact: true })).toContainText("checkout deadline exceeded");
    expect(upstreamRequests.some((url) => url.startsWith("/loki/api/v1/query_range?"))).toBe(true);
    expect(errors).toEqual([]);
  } finally {
    const closed = new Promise<void>((resolve, reject) => upstream.close((error) => error ? reject(error) : resolve()));
    upstream.closeAllConnections();
    await closed;
  }
});
