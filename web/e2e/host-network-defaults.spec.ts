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

async function open(page: Page, service: Service, route: string) {
  await page.addInitScript(token => { sessionStorage.setItem("metricspanel-token", token); localStorage.setItem("metricspanel-locale", "en"); }, service.token);
  await page.goto(service.url + "/#" + route);
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
