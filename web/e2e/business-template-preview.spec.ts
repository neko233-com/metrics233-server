import { test as base, expect, type Page } from "@playwright/test";
import { spawn } from "node:child_process";
import { randomBytes } from "node:crypto";
import { mkdtemp, rm } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "../..");
type Service = { url: string; token: string };
const test = base.extend<{}, { service: Service }>({ service: [async ({}, use) => {
  const temp = await mkdtemp(path.join(os.tmpdir(), "metrics233-template-preview-"));
  const token = randomBytes(32).toString("hex");
  const binary = process.env.METRICSPANEL_TEST_BINARY || path.join(root, "bin", process.platform === "win32" ? "metricspanel.exe" : "metricspanel");
  const child = spawn(binary, ["serve", "--listen", "127.0.0.1:0", "--db", path.join(temp, "test.db")], { cwd: root, windowsHide: true, env: { ...process.env, METRICSPANEL_AUTH: "token", METRICSPANEL_TOKEN: token, METRICSPANEL_INGEST_TOKEN: "", METRICSPANEL_TRUSTED_PROXIES: "", METRICSPANEL_STORAGE: "sqlite" } });
  let url = "", pending = "";
  child.stderr.on("data", chunk => { pending += chunk.toString(); for (;;) { const end = pending.indexOf("\n"); if (end < 0) break; const line = pending.slice(0, end); pending = pending.slice(end + 1); try { const item = JSON.parse(line); if (item.address) url = `http://${item.address}`; } catch { /* Only parse startup metadata. */ } } });
  try { await expect.poll(() => url, { timeout: 15000 }).not.toBe(""); await use({ url, token }); }
  finally {
    if (child.exitCode === null) { const exited = new Promise<void>(resolve => child.once("exit", () => resolve())); child.kill(); await exited; }
    const resolved = path.resolve(temp);
    if (!resolved.startsWith(path.resolve(os.tmpdir()) + path.sep) || !path.basename(resolved).startsWith("metrics233-template-preview-")) throw new Error("Unsafe preview fixture cleanup");
    await rm(resolved, { recursive: true, force: true, maxRetries: 3 });
  }
}, { scope: "worker" }] });

const sentinel = 'service="__unbound_template__"';
const labels = (service: string, role: string, environment: string) => ({ service, role, environment, channel: "fixture", serverId: "7", instance: `${service}-process`, implementation: role === "gateway" ? "rust" : "go" });
const scopeVariable = { name: "business_scope", type: "query", current: sentinel, options: [sentinel], config: { hide: 2, refresh: 0, skipUrlSync: true } };
async function template(page: Page, service: Service, name: string, metrics: string[]) {
  const response = await page.request.post(service.url + "/api/v1/dashboards", { headers: { Authorization: `Bearer ${service.token}` }, data: { name, variables: [scopeVariable], panels: metrics.map((metric, index) => ({ id: `panel-${index}`, title: `Preview value ${index}`, metric, aggregation: "last", visualization: "stat", unit: "count", expr: `sum(${metric}{${"${business_scope:raw}"}})` })) } });
  expect(response.ok()).toBeTruthy(); return response.json();
}
async function bind(page: Page, service: Service, id: string, dashboard: string) {
  const response = await page.request.put(`${service.url}/api/v1/business/instances/${id}/dashboard`, { headers: { Authorization: `Bearer ${service.token}` }, data: { version: 0, mode: "inherit", dashboard_id: dashboard } }); expect(response.ok()).toBeTruthy();
}
async function visit(page: Page, service: Service, dashboard: any, query = "") {
  await page.addInitScript(token => { sessionStorage.setItem("metricspanel-token", token); localStorage.setItem("metricspanel-locale", "en"); }, service.token);
  await page.goto(`${service.url}/d/${encodeURIComponent(dashboard.id)}/${encodeURIComponent(dashboard.name)}${query}`);
}

test("a direct shared template previews its sole bound gateway, including zero, without rewriting scope", async ({ page, service }, info) => {
  test.setTimeout(90000); const headers = { Authorization: `Bearer ${service.token}` }; const metrics = Array.from({ length: 7 }, (_, i) => `preview_gateway_value_${i}`);
  const samples = ["release", "test"].flatMap(environment => metrics.map((name, index) => ({ name, labels: labels("preview-gateway", "gateway", environment), value: environment === "test" ? 999 : index ? index + 10 : 0 })));
  expect((await page.request.post(service.url + "/api/v1/ingest", { headers, data: { samples } })).ok()).toBeTruthy();
  const dashboard = await template(page, service, "Shared gateway preview", metrics);
  const catalog = await (await page.request.get(service.url + "/api/v1/business/instances?q=preview-gateway", { headers })).json();
  const instance = catalog.items.find((item: any) => item.identity.environment === "release"); await bind(page, service, instance.id, dashboard.id);
  const errors: string[] = []; page.on("pageerror", error => errors.push(error.message));
  await visit(page, service, dashboard);
  await expect(page.getByRole("combobox", { name: "Preview instance", exact: true })).toHaveValue(instance.id);
  await expect(page.getByRole("combobox", { name: "Time range", exact: true })).toHaveValue("1h");
  for (let index = 0; index < 7; index++) await expect(page.getByRole("region", { name: `Preview value ${index}`, exact: true }).locator(".grafana-value strong")).toHaveText(String(index ? index + 10 : 0));
  await page.getByRole("combobox", { name: "Panel size", exact: true }).selectOption("large");
  const box = await page.getByRole("region", { name: "Preview value 0", exact: true }).boundingBox(); expect(box!.width).toBeGreaterThan(900); expect(box!.height).toBeGreaterThan(350);
  let allow!: () => void, intercepted = false; const gate = new Promise<void>(resolve => { allow = resolve; }); const continued: Promise<void>[] = [];
  await page.route(`**/api/v1/business/instances/${instance.id}/dashboard`, async route => { intercepted = true; const done = gate.then(() => route.continue()); continued.push(done); await done; });
  await page.getByRole("button", { name: "Refresh metrics", exact: true }).click();
  await expect.poll(() => intercepted).toBe(true);
  await expect(page.getByRole("region", { name: "Preview value 0", exact: true }).locator(".grafana-value strong")).toHaveText("0");
  const refreshed = page.waitForResponse(response => response.url().endsWith(`/api/v1/business/instances/${instance.id}/dashboard`) && response.status() === 200);
  allow(); await Promise.all(continued); await refreshed; await page.unroute(`**/api/v1/business/instances/${instance.id}/dashboard`);
  const originalPanel = await page.getByRole("region", { name: "Preview value 0", exact: true }).elementHandle();
  const background = page.waitForResponse(response => response.url().endsWith(`/api/v1/business/instances/${instance.id}/dashboard`) && response.status() === 200);
  await page.getByRole("combobox", { name: "Auto refresh", exact: true }).selectOption("5s");
  await background;
  await expect(page.getByRole("region", { name: "Preview value 0", exact: true }).locator(".grafana-value strong")).toHaveText("0");
  expect(await originalPanel!.evaluate(element => element.isConnected)).toBe(true);
  await page.getByRole("combobox", { name: "Auto refresh", exact: true }).selectOption("");
  await page.getByRole("button", { name: "Edit shared template", exact: true }).click();
  await expect(page.getByRole("button", { name: "Panel queries", exact: true })).toBeVisible();
  await page.getByRole("button", { name: "Return to instance preview", exact: true }).click();
  await expect(page.getByRole("region", { name: "Preview value 0", exact: true }).locator(".grafana-value strong")).toHaveText("0");
  const saved = (await (await page.request.get(service.url + "/api/v1/dashboards", { headers })).json()).find((item: any) => item.id === dashboard.id); expect(saved.variables[0].current).toBe(sentinel);
  await page.reload(); await expect(page.getByRole("combobox", { name: "Panel size", exact: true })).toHaveValue("large");
  await expect(page.getByRole("region", { name: "Preview value 0", exact: true }).locator(".grafana-value strong")).toHaveText("0");
  await page.screenshot({ path: info.outputPath("shared-template-preview.png"), fullPage: true }); expect(errors).toEqual([]);
  await page.route("**/api/v1/business/instances?template_id=**", route => route.fulfill({ status: 403, contentType: "application/json", body: JSON.stringify({ error: "Access denied" }) }));
  await page.getByRole("button", { name: "Refresh bound instances", exact: true }).click();
  await expect(page.getByRole("alert")).toContainText("Cannot load bound instances");
  await expect(page.getByRole("region", { name: "Preview value 0", exact: true })).toHaveCount(0);
});

test("multi-environment logic preview requires a choice, isolates switches and retains explicit time selection", async ({ page, service }) => {
  test.setTimeout(90000); const headers = { Authorization: `Bearer ${service.token}` }; const environments = ["release", "test", "shared", ""];
  const samples = environments.map((environment, index) => ({ name: "game_players_online", labels: labels("preview-logic", "logic", environment), value: 31 + index }));
  expect((await page.request.post(service.url + "/api/v1/ingest", { headers, data: { samples } })).ok()).toBeTruthy();
  const dashboard = await template(page, service, "Shared logic preview", ["game_players_online"]);
  const catalog = await (await page.request.get(service.url + "/api/v1/business/instances?q=preview-logic", { headers })).json();
  for (const item of catalog.items) await bind(page, service, item.id, dashboard.id);
  await visit(page, service, dashboard, "?from=now-6h&to=now");
  const selector = page.getByRole("combobox", { name: "Preview instance", exact: true });
  await expect(selector).toHaveValue(""); await expect(page.getByRole("region", { name: "Preview value 0", exact: true })).toHaveCount(0);
  await expect(page.getByRole("combobox", { name: "Time range", exact: true })).toHaveValue("6h");
  const release = catalog.items.find((item: any) => item.identity.environment === "release"), testing = catalog.items.find((item: any) => item.identity.environment === "test");
  await selector.selectOption(release.id); await expect(page.getByRole("region", { name: "Preview value 0", exact: true }).locator(".grafana-value strong")).toHaveText("31");
  let allow!: () => void, intercepted = false; const gate = new Promise<void>(resolve => { allow = resolve; }); const continued: Promise<void>[] = [];
  await page.route(`**/api/v1/business/instances/${testing.id}/dashboard`, async route => { intercepted = true; const done = gate.then(() => route.continue()); continued.push(done); await done; });
  await selector.selectOption(testing.id); await expect.poll(() => intercepted).toBe(true);
  await expect(page.getByRole("region", { name: "Preview value 0", exact: true })).toHaveCount(0);
  allow(); await Promise.all(continued); await page.unroute(`**/api/v1/business/instances/${testing.id}/dashboard`);
  await expect(page.getByRole("region", { name: "Preview value 0", exact: true }).locator(".grafana-value strong")).toHaveText("32");
  let sqlRequests = 0; const sqlPattern = `**/api/v1/business/instances/${testing.id}/data?kind=sql**`;
  await page.route(sqlPattern, async route => { sqlRequests++; await new Promise(resolve => setTimeout(resolve, 6200)); await route.continue(); });
  await page.getByRole("combobox", { name: "Auto refresh", exact: true }).selectOption("5s");
  const slowResponse = page.waitForResponse(response => response.url().includes(`/business/instances/${testing.id}/data?kind=sql`) && response.status() === 200, { timeout: 14000 });
  await page.getByRole("button", { name: "Slow SQL Top10", exact: true }).click(); await expect(page.getByRole("heading", { name: "Slow SQL Top10", exact: true })).toBeVisible();
  await slowResponse; expect(sqlRequests).toBe(1);
  await page.getByRole("combobox", { name: "Auto refresh", exact: true }).selectOption(""); await page.unroute(sqlPattern);
  await page.getByRole("button", { name: "Dashboards", exact: true }).first().click();
  await expect(page.getByRole("heading", { name: "Dashboards", exact: true })).toBeVisible();
  const environment = page.getByRole("combobox", { name: "Environment", exact: true });
  const cards = page.getByRole("article", { name: "Game server 7", exact: true });
  for (const name of ["Production", "Test", "Shared", "Unspecified"]) await expect(cards.getByRole("button", { name: `Open Game server 7 ${name} fixture`, exact: true })).toBeVisible();
  await environment.selectOption("test");
  await page.getByRole("article", { name: "Game server 7", exact: true }).getByRole("button", { name: "Open Game server 7 Test fixture", exact: true }).click();
  await expect(page.getByRole("combobox", { name: "Time range", exact: true })).toHaveValue("6h");
});
