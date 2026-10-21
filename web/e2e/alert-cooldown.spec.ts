import { test as base, expect } from "@playwright/test";
import { spawn } from "node:child_process";
import { mkdtemp, rm } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "../..");
const test = base.extend<{}, { endpoint: string }>({
  endpoint: [async ({}, use) => {
    const temp = await mkdtemp(path.join(os.tmpdir(), "metrics233-alert-cooldown-e2e-"));
    const binary = process.env.METRICSPANEL_TEST_BINARY || path.join(root, "bin", process.platform === "win32" ? "metricspanel.exe" : "metricspanel");
    const server = spawn(binary, ["serve", "--listen", "127.0.0.1:0", "--db", path.join(temp, "test.db")], {
      cwd: root, windowsHide: true,
      env: { ...process.env, METRICSPANEL_AUTH: "token", METRICSPANEL_TOKEN: "", METRICSPANEL_INGEST_TOKEN: "", METRICSPANEL_STORAGE: "sqlite" },
    });
    let endpoint = "";
    let logs = "";
    server.stderr.on("data", (chunk) => {
      logs += chunk.toString();
      for (const line of logs.split("\n")) {
        try { const record = JSON.parse(line); if (record.address) endpoint = `http://${record.address}`; } catch { /* Split log record. */ }
      }
    });
    try {
      await expect.poll(() => endpoint, { timeout: 15000, message: "isolated alert-cooldown server must start" }).not.toBe("");
      await use(endpoint);
    } finally {
      if (server.exitCode === null) {
        const exit = new Promise<void>((resolve) => server.once("exit", () => resolve()));
        server.kill();
        await exit;
      }
      const resolved = path.resolve(temp);
      if (!resolved.startsWith(path.resolve(os.tmpdir()) + path.sep) || !path.basename(resolved).startsWith("metrics233-alert-cooldown-e2e-"))
        throw new Error("Refusing cleanup outside owned alert-cooldown fixture");
      await rm(resolved, { recursive: true, force: true, maxRetries: 3 });
    }
  }, { scope: "worker" }],
});

test("alert cooldown defaults to one hour, persists zero and bounds without changing state timers", async ({ page, endpoint }) => {
  await page.addInitScript(() => localStorage.setItem("metricspanel-locale", "en"));
  await page.goto(endpoint + "/#alerts");
  await page.getByRole("button", { name: "Create alert", exact: true }).first().click();
  let dialog = page.getByRole("dialog", { name: "Create alert", exact: true });
  await expect(dialog.getByLabel("Notification cooldown (s)", { exact: true })).toHaveValue("3600");
  await expect(dialog.getByLabel("Pending period (s)", { exact: true })).toHaveValue("0");
  await expect(dialog.getByLabel("Keep firing for (s)", { exact: true })).toHaveValue("0");
  await expect(dialog.getByText(/300 seconds can filter brief CPU or disk spikes/)).toBeVisible();
  await expect(dialog.getByText(/including alerts after brief recoveries and periodic reminders/)).toBeVisible();
  await dialog.getByLabel("Rule name", { exact: true }).fill("Cooldown browser fixture");
  await dialog.getByLabel("PromQL condition", { exact: true }).fill("vector(0)");
  await dialog.getByRole("button", { name: "Save rule", exact: true }).click();
  await expect(dialog).not.toBeVisible();
  const rules = async () => {
    const response = await page.request.get(endpoint + "/api/v1/alerts/rules");
    expect(response.ok()).toBeTruthy();
    return response.json();
  };
  const saved = (await rules()).find((rule: any) => rule.title === "Cooldown browser fixture");
  expect(saved.notification_cooldown_seconds).toBe(3600);
  const read = async () => (await rules()).find((rule: any) => rule.uid === saved.uid);
  const edit = async () => {
    await page.getByRole("button", { name: "Edit Cooldown browser fixture", exact: true }).click();
    return page.getByRole("dialog", { name: "Edit alert rule", exact: true });
  };
  dialog = await edit();
  const input = dialog.getByLabel("Notification cooldown (s)", { exact: true });
  await expect(input).toHaveValue("3600");
  for (const invalid of ["-1", "604801", "1.5", ""]) {
    await input.fill(invalid);
    expect(await input.evaluate((element: HTMLInputElement) => element.checkValidity())).toBe(false);
    await dialog.getByRole("button", { name: "Save rule", exact: true }).click();
    await expect(dialog).toBeVisible();
    expect((await read()).notification_cooldown_seconds).toBe(3600);
  }
  await input.fill("0");
  await dialog.getByLabel("Pending period (s)", { exact: true }).fill("300");
  await dialog.getByLabel("Keep firing for (s)", { exact: true }).fill("300");
  await dialog.getByRole("button", { name: "Save rule", exact: true }).click();
  await expect(dialog).not.toBeVisible();
  expect((await read()).notification_cooldown_seconds).toBe(0);
  dialog = await edit();
  await expect(dialog.getByLabel("Notification cooldown (s)", { exact: true })).toHaveValue("0");
  await expect(dialog.getByLabel("Pending period (s)", { exact: true })).toHaveValue("300");
  await expect(dialog.getByLabel("Keep firing for (s)", { exact: true })).toHaveValue("300");
  await dialog.getByLabel("Notification cooldown (s)", { exact: true }).fill("604800");
  await dialog.getByRole("button", { name: "Save rule", exact: true }).click();
  await expect(dialog).not.toBeVisible();
  expect((await read()).notification_cooldown_seconds).toBe(604800);
  expect((await read()).for_seconds).toBe(300);
  expect((await read()).keep_firing_for_seconds).toBe(300);
});

test("legacy rules keep cooldown disabled and Chinese mobile help explains recovery and repeats", async ({ page, endpoint }) => {
  const response = await page.request.post(endpoint + "/api/v1/alerts/rules", {
    data: { uid: "cooldown-legacy", title: "Legacy cooldown fixture", expr: "vector(0)", condition: "nonzero", interval_seconds: 30, for_seconds: 120, keep_firing_for_seconds: 180 },
  });
  expect(response.ok()).toBeTruthy();
  await page.addInitScript(() => localStorage.setItem("metricspanel-locale", "zh"));
  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto(endpoint + "/#alerts");
  await page.getByRole("button", { name: "编辑 Legacy cooldown fixture", exact: true }).click();
  const dialog = page.getByRole("dialog", { name: "编辑告警规则", exact: true });
  await expect(dialog.getByLabel("通知冷却（秒）", { exact: true })).toHaveValue("0");
  await expect(dialog.getByText(/首次、新实例和严重级升级不受此冷却限制/)).toBeVisible();
  await dialog.getByText("通知冷却如何生效", { exact: true }).click();
  await expect(dialog.getByText(/发送失败仍会重试/)).toBeVisible();
  await expect(dialog.getByText(/重复提醒同时受冷却下限约束/)).toBeVisible();
  await expect(dialog.getByText(/重复间隔设为 0s/)).toBeVisible();
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true);
  await dialog.getByRole("button", { name: "保存规则", exact: true }).click();
  await expect(dialog).not.toBeVisible();
  const saved = await (await page.request.get(endpoint + "/api/v1/alerts/rules/cooldown-legacy")).json();
  expect(saved.notification_cooldown_seconds).toBe(0);
  expect(saved.for_seconds).toBe(120);
  expect(saved.keep_firing_for_seconds).toBe(180);
});
