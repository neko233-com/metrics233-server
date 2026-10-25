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
  const temp = await mkdtemp(path.join(os.tmpdir(), "metrics233-host-network-ui-"));
  const token = randomBytes(32).toString("hex");
  const binary = process.env.METRICSPANEL_TEST_BINARY || path.join(root, "bin", process.platform === "win32" ? "metricspanel.exe" : "metricspanel");
  const server = spawn(binary, ["serve", "--listen", "127.0.0.1:0", "--db", path.join(temp, "test.db")], { cwd: root, windowsHide: true, env: { ...process.env, METRICSPANEL_AUTH: "token", METRICSPANEL_TOKEN: token, METRICSPANEL_INGEST_TOKEN: "", METRICSPANEL_STORAGE: "sqlite", METRICSPANEL_TRUSTED_PROXIES: "" } });
  let url = "", pending = "";
  server.stderr.on("data", chunk => { pending += chunk.toString(); for (;;) { const end = pending.indexOf("\n"); if (end < 0) break; const line = pending.slice(0, end); pending = pending.slice(end + 1); try { const value = JSON.parse(line); if (value.address) url = `http://${value.address}`; } catch { /* Structured startup only. */ } } });
  try { await expect.poll(() => url, { timeout: 15000 }).not.toBe(""); await use({ url, token }); }
  finally {
    if (server.exitCode === null) { const stopped = new Promise<void>(resolve => server.once("exit", () => resolve())); server.kill(); await stopped; }
    const resolved = path.resolve(temp);
    if (!resolved.startsWith(path.resolve(os.tmpdir()) + path.sep) || !path.basename(resolved).startsWith("metrics233-host-network-ui-")) throw new Error("Unsafe host network fixture cleanup");
    await rm(resolved, { recursive: true, force: true, maxRetries: 3 });
  }
}, { scope: "worker" }] });

async function open(page: Page, service: Service, route: string, refreshOff = false) {
  await page.addInitScript(token => { sessionStorage.setItem("metricspanel-token", token); localStorage.setItem("metricspanel-locale", "en"); }, service.token);
  await page.goto(service.url + "/" + (refreshOff ? "?refresh=" : "") + "#" + route);
}
const nic = (device: string, receive: number | null, send = 0) => ({ device, receive_bytes: 1000, transmit_bytes: 1000, receive_per_second: receive, transmit_per_second: send });
const resources = () => ({ observed_at: Date.now(), cpu_percent: 25, cores: 8, memory_percent: 50, memory_total: 16e9, memory_available: 8e9, uptime: 7200, disks: [{ device: "/dev/fixture", mountpoint: "/", fstype: "ext4", total: 150e9, available: 120e9, used_percent: 20 }], network: [nic("anpi0", 0), nic("en0", 10), nic("en1", 20)] });
async function ingest(page: Page, service: Service, samples: unknown[]) {
  expect((await page.request.post(service.url + "/api/v1/ingest", { headers: { Authorization: `Bearer ${service.token}` }, data: { samples } })).ok()).toBeTruthy();
}
async function history(page: Page, service: Service, host: string) {
  const samples = Array.from({ length: 121 }, (_, index) => Math.max(1000, index * 30000)).flatMap(offset => ["anpi0", "en0", "en1"].flatMap(device => ["receive", "transmit"].map(direction => ({ name: `host_network_${direction}_bytes_per_second`, labels: { managed_host: host, device }, value: device === "anpi0" ? 0 : 20, timestamp: Date.now() - offset }))));
  await ingest(page, service, samples);
}

// The bounded heartbeat projection is a fixture; chart queries use the real
// native server/store. Backend recommendation/freshness has separate HTTP tests.
test("paused directory expires current readings without requests, remounts or lost note drafts", async ({ page, service }) => {
  const errors: string[] = []; page.on("pageerror", error => errors.push(error.message));
  let observedAt = Date.now(), requests = 0;
  await ingest(page, service, [{ name: "game_players_online", value: 0, labels: { service: "expiry-fixture", role: "logic", serverId: "83", environment: "test", channel: "fixture", instance: "expiry-process", implementation: "go" } }]);
  await page.route("**/api/v1/business/directory?**", async route => {
    requests++; const response = await route.fetch(), data = await response.json();
    const item = data.items.find((value: any) => value.identity.service === "expiry-fixture");
    if (item) { item.last_seen = observedAt; data.summaries[item.id] = { host_name: "Expiry fixture", host_observed_at: observedAt, host_expires_at: observedAt + 60000, cpu_percent: 25, memory_percent: 50, disk_percent: 20, memory_total: 16e9, memory_available: 8e9, receive_per_second: 1000, transmit_per_second: 2000, traffic_scope: "payload" }; }
    await route.fulfill({ response, json: data });
  });
  await page.clock.install({ time: observedAt });
  await open(page, service, "business", true);
  const card = page.getByRole("article", { name: "Game server 83", exact: true });
  await expect(card.locator(".business-card-meter strong").first()).toHaveText("25.0%");
  const original = await card.elementHandle(), originalRequests = requests;
  const search = page.getByRole("searchbox", { name: "Search all instances", exact: true });
  await search.fill("unfinished search");
  await card.getByRole("button", { name: "Edit note for Game server 83", exact: true }).click();
  const draft = page.getByRole("textbox", { name: "Instance description", exact: true });
  await draft.fill("unsaved maintenance note");
  await page.clock.fastForward(60001);
  await expect(card.locator(".business-card-meter strong")).toHaveText(["—", "—", "—"]);
  await expect(card.locator(".business-card-meter").nth(1)).not.toHaveAttribute("title", /Available/);
  await page.clock.fastForward(30000);
  await expect(card.locator(".business-card-traffic strong")).toHaveText(["—", "—"]);
  await expect(card.locator("footer")).toHaveClass("report-old");
  expect(requests).toBe(originalRequests);
  expect(await original!.evaluate(element => element.isConnected)).toBe(true);
  await expect(search).toHaveValue("unfinished search");
  await expect(draft).toHaveValue("unsaved maintenance note");
  await page.getByRole("button", { name: "Cancel", exact: true }).click();
  observedAt += 90001;
  await page.getByRole("region", { name: "Business instances", exact: true }).getByRole("button", { name: "Refresh", exact: true }).click();
  await expect(card.locator(".business-card-meter strong").first()).toHaveText("25.0%");
  await expect(card.locator(".business-card-traffic strong")).not.toHaveText(["—", "—"]);
  await expect(card.locator("footer")).toHaveClass("report-fresh");
  expect(await original!.evaluate(element => element.isConnected)).toBe(true);
  await expect(search).toHaveValue("unfinished search"); expect(errors).toEqual([]);
});

test("automatic refresh catches up once after a suspended timer without replacing the directory", async ({ page, service }) => {
  const now = Date.now(); let requests = 0;
  await ingest(page, service, [{ name: "game_players_online", value: 0, labels: { service: "resume-fixture", role: "logic", serverId: "84", environment: "test", channel: "fixture", instance: "resume-process", implementation: "go" } }]);
  await page.route("**/api/v1/business/directory?**", async route => { requests++; await route.continue(); });
  await page.clock.install({ time: now });
  await open(page, service, "business");
  await expect.poll(() => requests).toBe(1);
  const first = await page.getByRole("article", { name: "Game server 84", exact: true }).elementHandle();
  const search = page.getByRole("searchbox", { name: "Search all instances", exact: true }); await search.fill("unfinished search");
  await page.clock.pauseAt(now + 1000);
  await page.clock.setSystemTime(now + 6000);
  await page.evaluate(() => document.dispatchEvent(new Event("visibilitychange")));
  await expect.poll(() => requests, { timeout: 2000 }).toBe(2);
  await page.evaluate(() => { document.dispatchEvent(new Event("visibilitychange")); window.dispatchEvent(new Event("pageshow")); });
  await expect(search).toHaveValue("unfinished search");
  expect(await first!.evaluate(element => element.isConnected)).toBe(true); expect(requests).toBe(2);
});

test("host current gauges and NIC rates expire on a resumed page while history and NIC choice survive", async ({ page, service }) => {
  const errors: string[] = []; page.on("pageerror", error => errors.push(error.message));
  let current = resources(); const observedAt = current.observed_at;
  await page.route("**/api/v1/managed-hosts", route => route.fulfill({ json: [{ id: "host-expiry", name: "Host expiry fixture", address: "127.0.0.1", port: 22, user: "fixture", enabled: true, version: 1, target: { name: "fixture", kind: "prometheus", url: "http://127.0.0.1/metrics", interval_seconds: 30, enabled: true }, last_seen: current.observed_at, agent_state: "collecting", agent_os: "darwin", agent_arch: "arm64", collect_host: true, resources: current }] }));
  await history(page, service, "host-expiry"); await page.clock.install({ time: observedAt });
  await open(page, service, "hosts", true);
  const card = page.getByRole("region", { name: "Host expiry fixture", exact: true });
  await expect(card.getByRole("meter", { name: "CPU usage", exact: true })).toHaveAttribute("aria-valuenow", "25");
  await card.getByText("Network interfaces · receive and send", { exact: true }).click();
  const select = card.getByRole("combobox", { name: "History interface", exact: true });
  await select.selectOption("en0");
  const canvas = card.locator(".host-network-resources canvas").first(); await expect(canvas).toBeVisible();
  const original = await canvas.elementHandle();
  // A system-clock jump does not execute pending timers; pageshow must catch up.
  await page.clock.setSystemTime(observedAt + 90001);
  await page.evaluate(() => window.dispatchEvent(new Event("pageshow")));
  await expect(card.getByRole("meter")).toHaveCount(0);
  await expect(card.getByRole("img", { name: "CPU usage", exact: true })).toBeVisible();
  await expect(card.locator(".host-identity + .status")).toHaveText("Heartbeat expired");
  await expect(card.locator(".host-instruments .host-capacity")).not.toContainText(["8 GB"]);
  const rates = card.locator(".host-network-resources tbody tr").first().locator("td");
  await expect(rates).toHaveText(["anpi0", "—", "—"]);
  await expect(select).toHaveValue("en0"); expect(await original!.evaluate(element => element.isConnected)).toBe(true);
  current = { ...resources(), observed_at: observedAt + 90001, network: [nic("anpi0", 0), nic("en0", 30), nic("en1", 900)] };
  await page.getByRole("button", { name: "Refresh", exact: true }).click();
  await expect(card.getByRole("meter", { name: "CPU usage", exact: true })).toHaveAttribute("aria-valuenow", "25");
  await expect(rates).not.toHaveText(["anpi0", "—", "—"]);
  await expect(select).toHaveValue("en0"); expect(await original!.evaluate(element => element.isConnected)).toBe(true);
  expect(errors).toEqual([]);
});

test("host instruments adapt to their container and keep automatic/manual NIC identity", async ({ page, service }, info) => {
  test.setTimeout(60000);
  const errors: string[] = []; page.on("pageerror", error => errors.push(error.message));
  let current = resources();
  await page.route("**/api/v1/managed-hosts", route => route.fulfill({ json: [{ id: "host-ui-defaults", name: "Interface fixture", address: "127.0.0.1", port: 22, user: "fixture", enabled: true, version: 1, target: { name: "fixture", kind: "prometheus", url: "http://127.0.0.1/metrics", interval_seconds: 30, enabled: true }, last_seen: Date.now(), agent_state: "collecting", agent_os: "darwin", agent_arch: "arm64", collect_host: true, resources: current }] }));
  await history(page, service, "host-ui-defaults");
  await open(page, service, "hosts");
  await expect(page.getByRole("heading", { name: "Host monitoring", exact: true })).toBeVisible();
  const card = page.getByRole("region", { name: "Interface fixture", exact: true });
  await card.getByText("Network interfaces · receive and send", { exact: true }).click();
  const select = card.getByRole("combobox", { name: "History interface", exact: true });
  await expect(select).toHaveValue("");
  await expect(select.locator("option:checked")).toHaveText("Automatic · en1");
  const canvas = card.locator(".host-network-resources canvas").first(); await expect(canvas).toBeVisible();
  const original = await canvas.elementHandle();
  const refresh = async () => { const response = page.waitForResponse(r => r.url().endsWith("/api/v1/managed-hosts")); await page.getByRole("button", { name: "Refresh", exact: true }).click(); await response; };
  current = { ...resources(), network: [nic("anpi0", 0), nic("en0", 900), nic("en1", 0)] };
  await refresh();
  await expect(select.locator("option:checked")).toHaveText("Automatic · en1");
  expect(await original!.evaluate(element => element.isConnected)).toBeTruthy();
  await select.selectOption("anpi0"); await expect(select).toHaveValue("anpi0");
  current = { ...resources(), network: [nic("en0", 100), nic("en1", 1)] };
  await refresh();
  await expect(select).toHaveValue("anpi0");
  await expect(select.locator("option:checked")).toContainText("no current observation");
  await expect(card.getByRole("status").filter({ hasText: "Its history is retained" })).toBeVisible();
  await select.selectOption("");
  await expect(select.locator("option:checked")).toHaveText("Automatic · en0");
  for (const [width, columns] of [[1512, 3], [1100, 2], [768, 1], [390, 1]]) {
    await page.setViewportSize({ width, height: 1000 });
    await expect.poll(() => card.locator(".host-instruments").evaluate(element => getComputedStyle(element).gridTemplateColumns.split(" ").length)).toBe(columns);
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBeTruthy();
    expect((await card.locator(".risk-gauge").first().boundingBox())!.width).toBeGreaterThanOrEqual(200);
    await page.screenshot({ path: info.outputPath(`host-defaults-${width}.png`), fullPage: false });
  }
  expect(errors).toEqual([]);
});

test("business NIC changes hide the previous scope, retain range, and preserve missing manual history", async ({ page, service }) => {
  test.setTimeout(60000);
  const errors: string[] = []; page.on("pageerror", error => errors.push(error.message));
  const headers = { Authorization: `Bearer ${service.token}` };
  await ingest(page, service, [{ name: "game_players_online", value: 0, timestamp: Date.now() - 1000, labels: { service: "network-ui-fixture", role: "logic", serverId: "7", environment: "release", channel: "fixture", instance: "network-ui-process", implementation: "go" } }]);
  const directory = await (await page.request.get(service.url + "/api/v1/business/directory", { headers })).json();
  const id = directory.items.find((item: any) => item.identity.service === "network-ui-fixture").id;
  const baseView = await (await page.request.get(service.url + `/api/v1/business/instances/${id}/network?language=en`, { headers })).json();
  await history(page, service, "host-business-network");
  let devices = ["anpi0", "en0", "en1"], recommended = "en1", delayed = false, release: (() => void) | undefined;
  const requests: string[] = [];
  const windows: number[][] = [];
  page.on("request", request => {
    if (request.method() !== "POST" || !new URL(request.url()).pathname.endsWith("/api/ds/query")) return;
    const body = request.postDataJSON();
    if (body.queries?.some((query: any) => query.expr?.includes('managed_host="host-business-network"')) && body.queries.some((query: any) => query.range)) windows.push([Number(body.from), Number(body.to)]);
  });
  await page.route(`**/api/v1/business/instances/${id}/network?**`, async route => {
    const requested = new URL(route.request().url()).searchParams.get("device") || "";
    requests.push(requested);
    if (delayed && requested === "anpi0") { delayed = false; await new Promise<void>(resolve => { release = resolve; }); }
    const device = requested || recommended;
    const panel = (direction: string, kind: string) => ({ id: `host-${kind}-${direction}`, title: kind === "stat" ? `Interface ${direction}` : "Interface traffic · last hour", visualization: kind, unit: "Bps", expr: `host_network_${direction}_bytes_per_second{managed_host="host-business-network",device=${JSON.stringify(device)}}`, aggregation: "last" });
    const panels = devices.includes(device) ? [panel("receive", "stat"), panel("transmit", "stat"), panel("receive", "timeseries")] : [panel("receive", "timeseries")];
    await route.fulfill({ json: { ...baseView, devices, device, recommended_device: recommended, recommendation_has_traffic: true, host_stale: false, host_name: "Fixture machine", host: { id: "host-business-network", name: "Interface traffic", panels } } });
  });
  await open(page, service, "business");
  await page.getByRole("button", { name: "Open Game server 7 Production fixture", exact: true }).click();
  await page.getByRole("button", { name: "Network traffic", exact: true }).click();
  const host = page.getByRole("region", { name: "Host interface traffic", exact: true });
  const select = host.getByRole("combobox", { name: "Network interface", exact: true });
  await expect(select.locator("option:checked")).toHaveText("Automatic · en1");
  const plot = host.locator(".u-over"); await expect(plot).toBeVisible(); await plot.scrollIntoViewIfNeeded();
  const area = (await plot.boundingBox())!;
  await page.mouse.move(area.x + area.width * .2, area.y + area.height * .45); await page.mouse.down();
  await page.mouse.move(area.x + area.width * .6, area.y + area.height * .45, { steps: 8 }); await page.mouse.up();
  await expect.poll(() => { const range = windows.at(-1); return range ? range[1] - range[0] : Infinity; }).toBeLessThan(30 * 60000);
  const selectedWindow = windows.at(-1)!;
  const original = await host.locator("canvas").first().elementHandle();
  const refresh = async () => { const response = page.waitForResponse(r => r.url().includes(`/instances/${id}/network?`)); await page.getByRole("button", { name: "Refresh", exact: true }).click(); await response; };
  recommended = "en0"; await refresh();
  expect(requests.at(-1)).toBe("en1");
  await expect(select.locator("option:checked")).toHaveText("Automatic · en1");
  expect(windows.at(-1)).toEqual(selectedWindow); expect(await original!.evaluate(element => element.isConnected)).toBeTruthy();
  delayed = true; await select.selectOption("anpi0");
  await expect(host.getByRole("status", { name: "", exact: true }).filter({ hasText: "Loading selected interface" })).toBeVisible();
  await expect(host.locator("canvas")).toHaveCount(0);
  await expect.poll(() => !!release).toBeTruthy(); release!();
  await expect(host.getByRole("status").filter({ hasText: "Loading selected interface" })).toHaveCount(0);
  expect(windows.at(-1)).toEqual(selectedWindow);
  devices = ["en0", "en1"]; await refresh();
  await expect(select).toHaveValue("anpi0");
  await expect(host.getByRole("status").filter({ hasText: "Its history is retained" })).toBeVisible();
  await expect(host.getByText("Interface receive", { exact: true })).toHaveCount(0);
  await expect(host.locator("canvas").first()).toBeVisible();
  await select.selectOption("");
  await expect(select.locator("option:checked")).toHaveText("Automatic · en0");
  expect(windows.at(-1)).toEqual(selectedWindow);
  expect(errors).toEqual([]);
});
