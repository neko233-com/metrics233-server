import { test as base, expect } from "@playwright/test";
import { spawn } from "node:child_process";
import { mkdtemp, rm } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "../..");
const test = base.extend<{}, { endpoint: string }>({
  endpoint: [async ({}, use) => {
    const temp = await mkdtemp(path.join(os.tmpdir(), "metricspanel233-business-e2e-"));
    const service = spawn(process.env.METRICSPANEL_TEST_BINARY || path.join(root, "bin", process.platform === "win32" ? "metricspanel.exe" : "metricspanel"), ["serve", "--listen", "127.0.0.1:0", "--db", path.join(temp, "control.db")], {
      cwd: root, windowsHide: true, env: { ...process.env, METRICSPANEL_AUTH: "token", METRICSPANEL_TOKEN: "", METRICSPANEL_STORAGE: "sqlite" },
    });
    let endpoint = "", pending = "";
    service.stderr!.on("data", (chunk) => {
      pending += chunk.toString();
      for (;;) {
        const end = pending.indexOf("\n");
        if (end < 0) break;
        const line = pending.slice(0, end); pending = pending.slice(end + 1);
        try { const record = JSON.parse(line); if (record.address) endpoint = `http://${record.address}`; } catch {}
      }
    });
    try {
      await expect.poll(() => endpoint, { timeout: 15000 }).not.toBe("");
      await use(endpoint);
    } finally {
      if (service.exitCode === null) { const exit = new Promise<void>((resolve) => service.once("exit", () => resolve())); service.kill(); await exit; }
      const resolved = path.resolve(temp);
      if (!resolved.startsWith(path.resolve(os.tmpdir()) + path.sep) || !path.basename(resolved).startsWith("metricspanel233-business-e2e-")) throw new Error("Unsafe business fixture cleanup");
      await rm(resolved, { recursive: true, force: true, maxRetries: 3 });
    }
  }, { scope: "worker" }],
});

test("Business JSON push creates a scoped dashboard with real gauge and counter values", async ({ page, endpoint }) => {
  test.setTimeout(90000);
  const errors: string[] = [];
  page.on("pageerror", (error) => errors.push(error.message));
  page.on("console", (entry) => { if (entry.type() === "error") errors.push(entry.text()); });
  await page.addInitScript(() => localStorage.setItem("metricspanel-locale", "en"));
  const now = Date.now();
  const samples = ["production", "test"].flatMap((environment) => [0, 1, 2].flatMap((i) => [
    { name: "business_active_users", labels: { service: "shop", environment }, value: environment === "production" ? 42 : 999, timestamp: now - (2 - i) * 60000 },
    { name: "business_orders_total", labels: { service: "shop", environment }, value: 100 + i * 60, timestamp: now - (2 - i) * 60000 },
  ]));
  const ingested = await page.request.post(endpoint + "/api/v1/ingest", { data: { samples } });
  expect(ingested.ok()).toBeTruthy();
  expect((await ingested.json()).accepted).toBe(12);
  await page.goto(endpoint + "/#business");
  await expect(page.getByRole("heading", { name: "Business monitoring", exact: true })).toBeVisible();
  await expect(page.getByRole("button", { name: "Create business dashboard", exact: true })).toBeDisabled();
  await page.getByLabel("Metric", { exact: true }).selectOption("business_active_users");
  await page.getByLabel("Dashboard name", { exact: true }).fill("Production business users");
  await page.getByText("Label filters", { exact: true }).click();
  await page.getByLabel("Labels (JSON)", { exact: true }).fill('{"environment":"production"}');
  await page.getByRole("button", { name: "Create business dashboard", exact: true }).click();
  await expect(page.getByRole("heading", { name: "Production business users", exact: true })).toBeVisible();
  await expect(page.getByRole("region", { name: "Current total", exact: true })).toContainText("42");
  await expect(page.getByRole("region", { name: "Value over time", exact: true }).locator("canvas").first()).toBeVisible();
  await expect(page.getByRole("region", { name: "By label set", exact: true })).toContainText("42");
  if (process.env.METRICSPANEL_QA_ARTIFACT_DIR) await page.screenshot({ path: path.join(process.env.METRICSPANEL_QA_ARTIFACT_DIR, "business-dashboard.png") });
  await page.reload();
  await expect(page.getByRole("region", { name: "Current total", exact: true })).toContainText("42");
  await page.goto(endpoint + "/#business");
  await page.getByLabel("Metric", { exact: true }).selectOption("business_orders_total");
  await expect(page.getByLabel("Metric type", { exact: true })).toHaveValue("counter");
  await page.getByLabel("Dashboard name", { exact: true }).fill("Production orders");
  await page.getByText("Label filters", { exact: true }).click();
  await page.getByLabel("Labels (JSON)", { exact: true }).fill('{"environment":"production"}');
  await page.getByRole("button", { name: "Create business dashboard", exact: true }).click();
  await expect(page.getByRole("region", { name: "Current total", exact: true })).toContainText("220");
  await expect(page.getByRole("region", { name: "Events per second · 5m rate", exact: true }).locator("canvas").first()).toBeVisible();
  const saved = await (await page.request.get(endpoint + "/api/v1/dashboards")).json();
  expect(saved.find((d: any) => d.name === "Production orders").panels[1].expr).toBe('sum(rate(business_orders_total{environment="production"}[5m]))');
  expect(errors).toEqual([]);
});

test("Business onboarding is contained at 390/768/1280 and rejects malformed label filters", async ({ page, endpoint }) => {
  await page.addInitScript(() => localStorage.setItem("metricspanel-locale", "en"));
  expect((await page.request.post(endpoint + "/api/v1/ingest", { data: { samples: [{ name: "business_active_users", value: 42 }] } })).ok()).toBeTruthy();
  await page.goto(endpoint + "/#business");
  for (const width of [390, 768, 1280]) {
    await page.setViewportSize({ width, height: 900 });
    await expect(page.getByRole("heading", { name: "Business monitoring", exact: true })).toBeVisible();
    await expect.poll(() => page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
    if (process.env.METRICSPANEL_QA_ARTIFACT_DIR && width !== 768) await page.screenshot({ path: path.join(process.env.METRICSPANEL_QA_ARTIFACT_DIR, `business-${width}.png`) });
  }
  await page.getByLabel("Metric", { exact: true }).selectOption("business_active_users");
  await page.getByText("Label filters", { exact: true }).click();
  await page.getByLabel("Labels (JSON)", { exact: true }).fill('{"bad": 1}');
  await page.getByRole("button", { name: "Create business dashboard", exact: true }).click();
  await expect(page.getByRole("alert")).toContainText("Labels must be a JSON object with string values");
  await expect(page.getByRole("heading", { name: "Business monitoring", exact: true })).toBeVisible();
});
