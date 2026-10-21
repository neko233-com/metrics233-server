import { test as base, expect } from "@playwright/test";
import { spawn } from "node:child_process";
import { randomBytes } from "node:crypto";
import { mkdtemp, rm } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "../..");
const test = base.extend<{ service: { url: string; token: string } }>({
  service: async ({}, use) => {
    const temp = await mkdtemp(path.join(os.tmpdir(), "metrics233-simple-chart-e2e-"));
    const token = randomBytes(24).toString("hex");
    const cleanEnv = Object.fromEntries(Object.entries(process.env).filter(([key]) => !key.startsWith("METRICSPANEL_")));
    const server = spawn(process.env.METRICSPANEL_TEST_BINARY || path.join(root, "bin", process.platform === "win32" ? "metricspanel.exe" : "metricspanel"),
      ["serve", "--listen", "127.0.0.1:0", "--db", path.join(temp, "test.db")], {
        cwd: root, windowsHide: true,
        env: { ...cleanEnv, METRICSPANEL_AUTH: "token", METRICSPANEL_TOKEN: token, METRICSPANEL_STORAGE: "sqlite" },
      });
    let url = "", pending = "";
    server.stderr!.on("data", chunk => {
      pending += chunk.toString();
      for (;;) {
        const end = pending.indexOf("\n");
        if (end < 0) break;
        const line = pending.slice(0, end); pending = pending.slice(end + 1);
        try { const record = JSON.parse(line); if (record.address) url = `http://${record.address}`; } catch { /* Ignore split startup records. */ }
      }
    });
    try {
      await expect.poll(() => url, { timeout: 15000 }).not.toBe("");
      await use({ url, token });
    } finally {
      if (server.exitCode === null) {
        const exited = new Promise<void>(resolve => server.once("exit", () => resolve()));
        server.kill(); await exited;
      }
      const resolved = path.resolve(temp);
      if (!resolved.startsWith(path.resolve(os.tmpdir()) + path.sep) || !path.basename(resolved).startsWith("metrics233-simple-chart-e2e-"))
        throw new Error("Unsafe simple-chart fixture cleanup");
      await rm(resolved, { recursive: true, force: true, maxRetries: 3 });
    }
  },
});

test("stored Explore keeps all twelve series readable and gives PromQL explicit units", async ({ page, service }) => {
  const errors: string[] = [];
  page.on("pageerror", error => errors.push(error.message));
  const now = Date.now(), headers = { Authorization: `Bearer ${service.token}` };
  const samples = Array.from({ length: 12 }, (_, instance) => [50, 30, 10].map(age => ({
    name: "readability_latency_seconds", timestamp: now - age * 1000, value: 0.003 + instance / 1000,
    labels: { instance: `fixture-${String(instance).padStart(2, "0")}`, service: "fixture-api", diagnostic_marker: "diagnostic-only-233" },
  }))).flat();
  expect((await page.request.post(service.url + "/api/v1/ingest", { headers, data: { samples } })).ok()).toBeTruthy();
  await page.addInitScript(({ token }) => { sessionStorage.setItem("metricspanel-token", token); localStorage.setItem("metricspanel-locale", "en"); }, { token: service.token });
  await page.goto(service.url + "/#explore");
  await page.getByLabel("PromQL expression", { exact: true }).fill("readability_latency_seconds");
  await page.getByRole("button", { name: "Run query", exact: true }).click();
  const chart = page.locator(".explore-chart .chart-panel");
  await expect(chart).toHaveAttribute("data-series-count", "12");
  await expect(chart.locator("polyline")).toHaveCount(12);
  await expect(chart.locator(".chart-legend")).toContainText("12 series");
  await expect(chart.locator(".series-legend > span")).toHaveCount(12);
  await expect(chart).not.toContainText("Showing 8");
  await expect(chart).not.toContainText("diagnostic-only-233");
  await expect(chart.locator(".latest-value")).not.toContainText("B");
  const colors = await chart.locator("polyline").evaluateAll(lines => lines.map(line => line.getAttribute("stroke")));
  expect(colors.every(color => color !== null && /^#[0-9a-f]{6}$/i.test(color))).toBeTruthy();
  await page.getByRole("button", { name: "Run query", exact: true }).click();
  await expect(chart).toHaveAttribute("aria-busy", "false");
  expect(await chart.locator("polyline").evaluateAll(lines => lines.map(line => line.getAttribute("stroke")))).toEqual(colors);
  const details = chart.getByRole("button", { name: "Metric details · Query result", exact: true });
  await details.focus(); await page.keyboard.press("Enter");
  const dialog = page.getByRole("dialog");
  await expect(dialog).toContainText("diagnostic-only-233");
  await expect(dialog).toContainText("readability_latency_seconds");
  await dialog.getByRole("button", { name: "Close", exact: true }).click();
  await page.getByLabel("Unit", { exact: true }).selectOption("seconds");
  await page.getByLabel("PromQL expression", { exact: true }).fill("vector(0.003)");
  await page.getByRole("button", { name: "Run query", exact: true }).click();
  await expect(chart.locator(".latest-value")).toHaveText("3 ms");
  await page.getByLabel("Unit", { exact: true }).selectOption("percentunit");
  await page.getByLabel("PromQL expression", { exact: true }).fill("vector(0.75)");
  await page.getByRole("button", { name: "Run query", exact: true }).click();
  await expect(chart.locator(".latest-value")).toHaveText("75.0%");
  for (const width of [390, 768, 1280]) {
    await page.setViewportSize({ width, height: 900 });
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth + 1)).toBeTruthy();
  }
  expect(errors).toEqual([]);
});

test("simple dashboard aliases preserve labels in details and empty results never show a sample", async ({ page, service }) => {
  const headers = { Authorization: `Bearer ${service.token}` }, now = Date.now();
  expect((await page.request.post(service.url + "/api/v1/ingest", { headers, data: { samples: [
    { name: "readability_custom", value: 0, timestamp: now - 1000, labels: { instance: "fixture-zero", diagnostic_marker: "details-only-zero" } },
  ] } })).ok()).toBeTruthy();
  const created = await page.request.post(service.url + "/api/v1/dashboards", { headers, data: {
    id: "simple-readability", name: "Simple readability", panels: [
      { id: "alias", title: "Explicit alias", metric: "readability_custom", aggregation: "last", unit: "count", visualization: "timeseries", config: { fieldConfig: { defaults: { displayName: "My readable alias" }, overrides: [] } } },
      { id: "empty", title: "Empty simple table", metric: "readability_custom", labels: { instance: "missing-fixture" }, aggregation: "last", unit: "", visualization: "table" },
    ],
  } });
  expect(created.ok()).toBeTruthy();
  const saved = await created.json();
  await page.addInitScript(({ token }) => { sessionStorage.setItem("metricspanel-token", token); localStorage.setItem("metricspanel-locale", "en"); }, { token: service.token });
  await page.goto(service.url + `/d/${encodeURIComponent(saved.id)}/Simple%20readability`);
  const alias = page.getByRole("region", { name: "Explicit alias", exact: true });
  await expect(alias.locator(".chart-legend")).toContainText("My readable alias");
  await expect(alias.locator(".latest-value")).toHaveText("0");
  await expect(alias).not.toContainText("details-only-zero");
  await alias.getByRole("button", { name: "Metric details · Explicit alias", exact: true }).click();
  await expect(page.getByRole("dialog")).toContainText("details-only-zero");
  await page.getByRole("dialog").getByRole("button", { name: "Close", exact: true }).click();
  const empty = page.getByRole("region", { name: "Empty simple table", exact: true });
  await expect(empty).toContainText("No samples in this time range");
  await expect(empty.locator("tbody tr")).toHaveCount(0);
});
