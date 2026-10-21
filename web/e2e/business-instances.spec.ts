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
  const temp = await mkdtemp(path.join(os.tmpdir(), "metrics233-business-instances-"));
  const token = randomBytes(32).toString("hex");
  const binary = process.env.METRICSPANEL_TEST_BINARY || path.join(root, "bin", process.platform === "win32" ? "metricspanel.exe" : "metricspanel");
  const processServer = spawn(binary, ["serve", "--listen", "127.0.0.1:0", "--db", path.join(temp, "test.db")], { cwd: root, windowsHide: true, env: { ...process.env, METRICSPANEL_AUTH: "token", METRICSPANEL_TOKEN: token, METRICSPANEL_INGEST_TOKEN: "", METRICSPANEL_TRUSTED_PROXIES: "", METRICSPANEL_STORAGE: "sqlite" } });
  let url = "", pending = "";
  processServer.stderr.on("data", chunk => { pending += chunk.toString(); for (;;) { const end = pending.indexOf("\n"); if (end < 0) break; const line = pending.slice(0, end); pending = pending.slice(end + 1); try { const item = JSON.parse(line); if (item.address) url = `http://${item.address}`; } catch { /* Structured startup only. */ } } });
  try { await expect.poll(() => url, { timeout: 15000 }).not.toBe(""); await use({ url, token }); }
  finally { if (processServer.exitCode === null) { const exited = new Promise<void>(resolve => processServer.once("exit", () => resolve())); processServer.kill(); await exited; } const resolved = path.resolve(temp); if (!resolved.startsWith(path.resolve(os.tmpdir()) + path.sep) || !path.basename(resolved).startsWith("metrics233-business-instances-")) throw new Error("Unsafe business fixture cleanup"); await rm(resolved, { recursive: true, force: true, maxRetries: 3 }); }
}, { scope: "worker" }] });

async function open(page: Page, service: Service) {
  await page.addInitScript(token => { sessionStorage.setItem("metricspanel-token", token); localStorage.setItem("metricspanel-locale", "en"); }, service.token);
  await page.goto(service.url + "/#dashboards");
  await page.getByRole("tab", { name: "Business virtual dashboards", exact: true }).click();
}
const baseLabels = (service: string, environment = "production") => ({ service, role: "logic", serverId: "0", instance: service + "-$serverId-0", environment, channel: "fixture", implementation: "go", ip: "203.0.113.18" });

test("real instance identity, live template and independent copy stay separate", async ({ page, service }, info) => {
  test.setTimeout(90000); const headers = { Authorization: `Bearer ${service.token}` }; const errors: string[] = [];
  page.on("pageerror", cause => errors.push(cause.message));
  await open(page, service);
  await expect(page.getByText(/No reported business instances/)).toBeVisible();
  const initial = await (await page.request.get(service.url + "/api/v1/dashboards", { headers })).json();
  const samples = [
    { name: "game_players_online", labels: baseLabels("ui-identity"), value: 42 },
    { name: "game_player_sessions_started_total", labels: baseLabels("ui-identity"), value: 120 },
    { name: "game_players_online", labels: baseLabels("ui-identity", "staging"), value: 999 },
    { name: "gateway_fixture", labels: { ...baseLabels("ui-gateway"), role: "gateway", serverId: "1" }, value: 1 },
  ];
  expect((await page.request.post(service.url + "/api/v1/ingest", { headers: { ...headers, "X-Forwarded-For": "203.0.113.99" }, data: { samples } })).ok()).toBeTruthy();
  await page.getByRole("button", { name: "Refresh instances", exact: true }).click();
  const row = page.getByRole("row").filter({ hasText: "production / fixture" }).filter({ has: page.getByRole("button", { name: "ui-identity · logic · 0", exact: true }) });
  await expect(row).toContainText("203.0.113.18"); await expect(row).toContainText("127.0.0.1");
  const catalog = await (await page.request.get(service.url + "/api/v1/business/instances", { headers })).json(); expect(catalog.items).toHaveLength(3);
  expect((await (await page.request.get(service.url + "/api/v1/dashboards", { headers })).json()).length).toBe(initial.length);
  await row.getByRole("button").click();
  await expect(page.getByRole("region", { name: "Online players", exact: true })).toContainText("42");
  await expect(page.getByRole("region", { name: "Successful player sessions · cumulative", exact: true })).toContainText("120");
  const libraryResponse = await page.request.post(service.url + "/api/library-elements", { headers, data: { name: "Business library fixture", kind: 1, model: { type: "text", title: "Shared library text", options: { mode: "markdown", content: "Independent snapshot content" } } } }); expect(libraryResponse.ok()).toBeTruthy();
  const library = (await libraryResponse.json()).result;
  const template = await (await page.request.post(service.url + "/api/v1/dashboards", { headers, data: { name: "Shared live fixture", panels: [{ id: "online", title: "Shared online", metric: "game_players_online", aggregation: "last", unit: "count", visualization: "stat", expr: "sum(game_players_online{${business_scope:raw}})" }, { id: "shared-text", title: "Shared library text", metric: "game_players_online", aggregation: "last", visualization: "text", config: { type: "text", libraryPanel: { uid: library.uid, name: library.name } } }] } })).json();
  // Reload the outer workspace to obtain the newly created dashboard catalog.
  await page.reload(); await page.getByRole("tab", { name: "Business virtual dashboards", exact: true }).click();
  await page.getByRole("row").filter({ hasText: "production / fixture" }).getByRole("button", { name: "ui-identity · logic · 0", exact: true }).click();
  await page.getByText("Template binding", { exact: true }).click();
  await page.getByLabel("Native dashboard template", { exact: true }).selectOption(template.id);
  await page.getByRole("button", { name: "Use live template", exact: true }).click();
  await expect(page.getByRole("region", { name: "Shared online", exact: true })).toContainText("42");
  const update = { ...template, panels: template.panels.map((panel: any) => ({ ...panel, title: "Shared updated" })) };
  const updated = await page.request.put(service.url + "/api/v1/dashboards/" + template.id, { headers: { ...headers, "If-Match": String(template.updated_at) }, data: update }); expect(updated.ok()).toBeTruthy();
  await page.getByRole("button", { name: "Refresh instance", exact: true }).click();
  await expect(page.getByRole("region", { name: "Shared updated", exact: true })).toContainText("42");
  await page.getByRole("button", { name: "Save independent copy", exact: true }).click();
  await expect(page.getByRole("heading", { name: "ui-identity · production · logic · 0 · ui-identity-$serverId-0", exact: true })).toBeVisible();
  await expect(page.getByRole("region", { name: "Shared updated", exact: true })).toContainText("42");
  const instance = await (await page.request.get(service.url + "/api/v1/business/instances/" + catalog.items.find((item: any) => item.identity.service === "ui-identity" && item.identity.environment === "production").id, { headers })).json();
  expect(instance.binding.mode).toBe("independent"); expect(instance.binding.dashboard_id).not.toBe(template.id);
  const savedCopy = (await (await page.request.get(service.url + "/api/v1/dashboards", { headers })).json()).find((dashboard: any) => dashboard.id === instance.binding.dashboard_id);
  expect(JSON.stringify(savedCopy)).not.toContain('"libraryPanel"');
  await expect(page.getByRole("region", { name: "Shared library text", exact: true })).toContainText("Independent snapshot content");
  await page.screenshot({ path: info.outputPath("business-independent.png"), fullPage: true });
  expect(errors).toEqual([]);
});

test("all regions and SQL entities paginate with a bounded virtual DOM and honest missing values", async ({ page, service }, info) => {
  test.setTimeout(90000); const headers = { Authorization: `Bearer ${service.token}` }; const labels = baseLabels("ui-tables"); const now = Date.now(); const samples: any[] = [];
  for (let i = 0; i < 235; i++) samples.push({ name: "game_players_online_by_region", labels: { ...labels, country: "Fixture", province: "Province", city: `City-${String(i).padStart(3, "0")}` }, value: i === 234 ? 0 : 1, timestamp: now - 1000 });
  samples.push({ name: "game_players_online", labels, value: 234, timestamp: now - 1000 });
  for (let i = 0; i < 160; i++) for (const offset of i === 159 ? [1000] : [120000, 60000, 1000]) {
    const factor = (121000 - offset) / 60000, count = 10 * factor, duration = (160 - i) * .01 * count;
    for (const [name, value] of [["game_sql_calls_total", count], ["game_sql_failures_total", 0], ["game_sql_rows_affected_total", count * 2], ["game_sql_duration_seconds_sum", duration], ["game_sql_duration_seconds_count", count]]) samples.push({ name, value, timestamp: now - offset, labels: { ...labels, table: `entity_${String(i).padStart(3, "0")}`, operation: "update" } });
  }
  expect((await page.request.post(service.url + "/api/v1/ingest", { headers, data: { samples } })).ok()).toBeTruthy();
  await open(page, service); await page.getByRole("button", { name: "ui-tables · logic · 0", exact: true }).click();
  await page.getByRole("button", { name: "Player regions · all", exact: true }).click();
  await expect(page.getByRole("status").filter({ hasText: "100 / 235" })).toBeVisible();
  const table = page.getByRole("table", { name: "Player regions", exact: true });
  expect(await table.locator("tbody tr:not([aria-hidden])").count()).toBeLessThanOrEqual(20);
  expect((await table.locator("tbody tr:not([aria-hidden])").first().boundingBox())!.height).toBe(44);
  await page.getByLabel("Scrollable results", { exact: true }).evaluate(element => { element.scrollTop = element.scrollHeight; });
  await expect(page.getByRole("status").filter({ hasText: "200 / 235" })).toBeVisible();
  await page.getByRole("button", { name: "Load more", exact: true }).click();
  await expect(page.getByRole("status").filter({ hasText: "235 / 235" })).toBeVisible();
  await page.getByLabel("Scrollable results", { exact: true }).evaluate(element => { element.scrollTop = element.scrollHeight; });
  await expect(table.getByText("City-234", { exact: true })).toBeVisible();
  await expect(table.getByRole("row").filter({ hasText: "City-234" })).toContainText("0");
  expect(await table.locator("tbody tr:not([aria-hidden])").count()).toBeLessThanOrEqual(20);
  await expect(page.getByRole("link", { name: "IP location data: DB-IP Lite", exact: true })).toHaveAttribute("href", "https://db-ip.com");
  await page.screenshot({ path: info.outputPath("business-regions-all.png"), fullPage: true });
  await page.getByRole("button", { name: "Entity SQL", exact: true }).click();
  await expect(page.getByRole("status").filter({ hasText: "100 / 160" })).toBeVisible();
  const sql = page.getByRole("table", { name: "Entity SQL statistics", exact: true });
  await expect(sql.locator("tbody tr:not([aria-hidden])").first()).toContainText("entity_000");
  await expect(sql.locator("tbody tr:not([aria-hidden])").first()).toContainText("1,600");
  await page.getByRole("button", { name: "Load more", exact: true }).click();
  await expect(page.getByRole("status").filter({ hasText: "160 / 160" })).toBeVisible();
  await page.getByLabel("Scrollable results", { exact: true }).evaluate(element => { element.scrollTop = element.scrollHeight; });
  const missing = sql.getByRole("row").filter({ hasText: "entity_159" }); await expect(missing).toBeVisible(); await expect(missing).toContainText("—");
  expect(await sql.locator("tbody tr:not([aria-hidden])").count()).toBeLessThanOrEqual(20);
  await page.screenshot({ path: info.outputPath("business-sql-all.png"), fullPage: true });
  await page.setViewportSize({ width: 390, height: 900 });
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBeTruthy();
  await page.getByRole("button", { name: "Switch language", exact: true }).click();
  await expect(page.getByRole("heading", { name: "Entity SQL · 数据库实际执行", exact: true })).toBeVisible();
  await page.screenshot({ path: info.outputPath("business-sql-zh-mobile.png"), fullPage: true, animations: "disabled" });
});

test("population snapshots exclude old IPs and regions after restart and expose partial or stale data", async ({ page, service }, info) => {
  test.setTimeout(60000);
  const headers = { Authorization: `Bearer ${service.token}` }, now = Date.now(), labels = baseLabels("ui-snapshot");
  const push = async (samples: any[]) => { const result = await page.request.post(service.url + "/api/v1/ingest", { headers, data: { samples } }); expect(result.ok()).toBeTruthy(); };
  await push([
    { name: "game_players_online", labels: { ...labels, ip: "203.0.113.1" }, value: 9, timestamp: now - 120000 },
    { name: "game_players_online_by_region", labels: { ...labels, ip: "203.0.113.1", country: "Fixture", city: "Old" }, value: 9, timestamp: now - 120000 },
    { name: "game_players_online", labels: { ...labels, ip: "203.0.113.2" }, value: 0, timestamp: now - 2000 },
  ]);
  await open(page, service); await page.getByRole("button", { name: "ui-snapshot · logic · 0", exact: true }).click();
  await expect(page.getByRole("region", { name: "Online players", exact: true }).getByText("0", { exact: true })).toBeVisible();
  await page.getByRole("button", { name: "Player regions · all", exact: true }).click();
  await expect(page.getByText("No players in this snapshot.", { exact: true })).toBeVisible();
  const table = page.getByRole("table", { name: "Player regions", exact: true }); await expect(table.getByText("Old", { exact: true })).toHaveCount(0);
  await push([
    { name: "game_players_online", labels: { ...labels, ip: "203.0.113.2" }, value: 3, timestamp: now - 1000 },
    { name: "game_players_online_by_region", labels: { ...labels, ip: "203.0.113.2", country: "Fixture", city: "New" }, value: 2, timestamp: now - 1000 },
    { name: "game_players_online", labels: { ...labels, environment: "staging" }, value: 999, timestamp: now - 1000 },
    { name: "game_players_online_by_region", labels: { ...labels, environment: "staging", country: "Fixture", city: "Other environment" }, value: 999, timestamp: now - 1000 },
  ]);
  await page.getByRole("button", { name: "Refresh instance", exact: true }).click();
  await expect(page.getByText("Region snapshot is incomplete", { exact: true })).toBeVisible();
  await expect(table.getByText("New", { exact: true })).toBeVisible(); await expect(table.getByText("Old", { exact: true })).toHaveCount(0); await expect(table.getByText("Other environment", { exact: true })).toHaveCount(0);
  await push([{ name: "game_players_online_by_region", labels: { ...labels, ip: "203.0.113.2", country: "Fixture", city: "Rest" }, value: 1, timestamp: now - 1000 }]);
  await page.getByRole("button", { name: "Refresh instance", exact: true }).click();
  await expect(table.getByText("Rest", { exact: true })).toBeVisible(); await expect(page.getByText("Region snapshot is incomplete", { exact: true })).toHaveCount(0);
  await page.getByRole("button", { name: "Overview", exact: true }).last().click();
  await expect(page.getByRole("region", { name: "Online players", exact: true }).getByText("3", { exact: true })).toBeVisible();
  const staleLabels = baseLabels("ui-stale");
  await push([{ name: "game_players_online", labels: staleLabels, value: 2, timestamp: now - 120000 }, { name: "game_players_online_by_region", labels: { ...staleLabels, country: "Fixture", city: "Stale" }, value: 2, timestamp: now - 120000 }]);
  await page.getByRole("button", { name: "All business instances", exact: true }).click(); await page.getByRole("button", { name: "ui-stale · logic · 0", exact: true }).click();
  await expect(page.getByText("Player data is stale (over 90 seconds)", { exact: true })).toBeVisible();
  await page.screenshot({ path: info.outputPath("business-population-stale.png"), fullPage: true, animations: "disabled" });
});
