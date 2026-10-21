import { test as base, expect, type Page } from "@playwright/test";
import { spawn } from "node:child_process";
import { randomBytes } from "node:crypto";
import { mkdtemp, rm } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "../..");
type Service = { url: string; token: string; ingestToken: string };
const test = base.extend<{ service: Service }>({
  service: [async ({}, use) => {
    const temp = await mkdtemp(path.join(os.tmpdir(), "metrics233-reporting-sources-e2e-"));
    const token = randomBytes(24).toString("hex");
    const ingestToken = randomBytes(32).toString("hex");
    const binary = process.env.METRICSPANEL_TEST_BINARY || path.join(root, "bin", process.platform === "win32" ? "metricspanel.exe" : "metricspanel");
    const server = spawn(binary, ["serve", "--listen", "127.0.0.1:0", "--db", path.join(temp, "test.db")], {
      cwd: root, windowsHide: true,
      env: { ...process.env, METRICSPANEL_AUTH: "token", METRICSPANEL_TOKEN: token, METRICSPANEL_INGEST_TOKEN: ingestToken, METRICSPANEL_STORAGE: "sqlite", METRICSPANEL_TRUSTED_PROXIES: "" },
    });
    let endpoint = "", pending = "";
    server.stderr.on("data", (chunk) => {
      pending += chunk.toString();
      for (;;) {
        const end = pending.indexOf("\n");
        if (end < 0) break;
        const line = pending.slice(0, end); pending = pending.slice(end + 1);
        try { const record = JSON.parse(line); if (record.address) endpoint = `http://${record.address}`; } catch { /* A split log is not a credential surface. */ }
      }
    });
    try {
      await expect.poll(() => endpoint, { timeout: 15000 }).not.toBe("");
      await use({ url: endpoint, token, ingestToken });
    } finally {
      if (server.exitCode === null) { const exit = new Promise<void>((resolve) => server.once("exit", () => resolve())); server.kill(); await exit; }
      const resolved = path.resolve(temp);
      if (!resolved.startsWith(path.resolve(os.tmpdir()) + path.sep) || !path.basename(resolved).startsWith("metrics233-reporting-sources-e2e-")) throw new Error("Unsafe reporting-source fixture cleanup");
      await rm(resolved, { recursive: true, force: true, maxRetries: 3 });
    }
  }, { scope: "test" }],
});

async function open(page: Page, service: Service, locale = "en") {
  await page.addInitScript(({ token, locale }) => {
    sessionStorage.setItem("metricspanel-token", token);
    localStorage.setItem("metricspanel-locale", locale);
  }, { token: service.token, locale });
  await page.goto(service.url + "/#reporting-sources");
}

test("Chinese onboarding, numeric uptime and editable historical risk lines render without creating alerts", async ({ page, service }, info) => {
  const headers = { Authorization: `Bearer ${service.token}` }, now = Date.now();
  const errors: string[] = []; page.on("pageerror", error => errors.push(error.message));
  const samples = Array.from({ length: 60 }, (_, i) => ({ name: "ui_risk_cpu", timestamp: now - (60-i)*5000, value: [20,90,99,40][i%4] }));
  expect((await page.request.post(service.url+"/api/v1/ingest", { headers, data:{ samples } })).ok()).toBeTruthy();
  const created = await page.request.post(service.url+"/api/v1/dashboards", { headers, data:{ id:"ui-risk-fixture", name:"Risk fixture", panels:[
    { id:"cpu", title:"CPU risk", metric:"ui_risk_cpu", expr:"ui_risk_cpu", aggregation:"last", visualization:"timeseries", unit:"percent", config:{ fieldConfig:{defaults:{color:{mode:"thresholds"},thresholds:{mode:"absolute",steps:[{value:null,color:"green"},{value:85,color:"yellow"},{value:95,color:"red"}]}},overrides:[]} } },
    { id:"uptime", title:"Uptime", metric:"metricspanel_uptime_seconds", aggregation:"last", visualization:"stat", unit:"uptime" },
  ] } });
  expect(created.ok()).toBeTruthy();
  const saved = await created.json();
  await open(page,service,"zh");
  await page.goto(service.url+"/d/"+encodeURIComponent(saved.id)+"/Risk%20fixture");
  const chart=page.getByRole("region",{name:"CPU risk",exact:true});
  await expect(chart.locator("canvas")).toBeVisible();
  await expect(chart.locator(".native-risk-legend")).toContainText("预警 ≥ 85");
  await expect(chart.locator(".native-risk-legend")).toContainText("严重 ≥ 95");
  const colors=await chart.locator("canvas").evaluate((canvas:HTMLCanvasElement)=>{
    const pixels=canvas.getContext("2d")!.getImageData(0,0,canvas.width,canvas.height).data; let green=0,yellow=0,red=0;
    for(let i=0;i<pixels.length;i+=4){const[r,g,b]=[pixels[i],pixels[i+1],pixels[i+2]];if(g>130&&r<130&&b<200)green++;if(r>170&&g>130&&b<140)yellow++;if(r>170&&g<130&&b<170)red++;}
    return{green,yellow,red};
  });
  expect(colors.green).toBeGreaterThan(5);expect(colors.yellow).toBeGreaterThan(5);expect(colors.red).toBeGreaterThan(5);
  const uptime=page.getByRole("region",{name:"Uptime",exact:true});
  await expect(uptime.locator("canvas,.native-timeseries-plot-host")).toHaveCount(0);
  await expect(uptime).toContainText(/秒|分钟/);
  await chart.getByRole("button",{name:"编辑 CPU risk",exact:true}).click();
  const dialog=page.getByRole("dialog");
  await dialog.getByLabel("预警阈值",{exact:true}).fill("75");
  await dialog.getByLabel("严重阈值",{exact:true}).fill("90");
  await dialog.getByRole("button",{name:"保存面板",exact:true}).click();
  await expect(dialog).not.toBeVisible();
  await expect(chart.locator(".native-risk-legend")).toContainText("预警 ≥ 75");
  await expect(chart.locator(".native-risk-legend")).toContainText("严重 ≥ 90");
  let dashboard=(await(await page.request.get(service.url+"/api/v1/dashboards",{headers})).json()).find((item:any)=>item.id===saved.id);
  expect(dashboard.panels[0].expr).toBe("ui_risk_cpu");
  for(const [color,css] of [["dark-red","rgb(196, 22, 42)"],["invalid-fixture-color","rgb(133, 170, 255)"]]) {
    dashboard.panels[0].config.fieldConfig.defaults.thresholds.steps[2].color=color;
    const update=await page.request.put(service.url+"/api/v1/dashboards/"+saved.id,{headers:{...headers,"If-Match":String(dashboard.updated_at)},data:dashboard});
    expect(update.ok()).toBeTruthy();dashboard=await update.json();await page.reload();
    await expect(chart.locator("canvas")).toBeVisible();
    await expect(chart.locator(".native-risk-legend > span > span").last()).toHaveCSS("color",css);
    await chart.getByRole("button",{name:"编辑 CPU risk",exact:true}).click();
    await page.getByRole("dialog").getByRole("button",{name:"保存面板",exact:true}).click();
    await expect(page.getByRole("dialog")).not.toBeVisible();
    dashboard=(await(await page.request.get(service.url+"/api/v1/dashboards",{headers})).json()).find((item:any)=>item.id===saved.id);
    expect(dashboard.panels[0].config.fieldConfig.defaults.thresholds.steps[2].color).toBe(color);
  }
  expect((await(await page.request.get(service.url+"/api/v1/alerts/rules",{headers})).json())).toEqual([]);
  await page.screenshot({path:info.outputPath("chinese-risk-uptime.png"),fullPage:true});
  await page.goto(service.url+"/d/system/System%20overview");
  await expect(page.getByRole("heading",{name:"快速开始",exact:true})).toBeVisible();
  await expect(page.getByText("接入采集器",{exact:true})).toBeVisible();
  await expect(page.getByText("查看实时指标",{exact:true})).toBeVisible();
  expect(errors).toEqual([]);
});

test("retention defaults to fifteen days, persists and rejects a stale administrative edit", async ({ page, service }) => {
  const errors: string[] = [];
  page.on("pageerror", error => errors.push(error.message));
  await open(page, service);
  await page.getByRole("button", { name: "Collectors", exact: true }).click();
  const section = page.getByRole("region", { name: "Data retention", exact: true });
  const input = section.getByLabel("Retention days", { exact: true });
  await expect(input).toHaveValue("15");
  await input.fill("7");
  await section.getByRole("button", { name: "Save retention", exact: true }).click();
  await expect(page.getByText("Data retention saved", { exact: true })).toBeVisible();
  await page.reload();
  await expect(input).toHaveValue("7");
  const headers = { Authorization: `Bearer ${service.token}` };
  const endpoint = service.url + "/api/v1/settings/retention";
  const current = await (await page.request.get(endpoint, { headers })).json();
  expect((await page.request.put(endpoint, { headers, data: { days: 14, version: current.version } })).ok()).toBeTruthy();
  await input.fill("6");
  await section.getByRole("button", { name: "Save retention", exact: true }).click();
  await expect(input).toHaveValue("14");
  await expect(page.getByText(/Retention changed elsewhere/)).toBeVisible();
  await input.fill("15");
  await section.getByRole("button", { name: "Save retention", exact: true }).click();
  await expect(section.getByRole("button", { name: "Save retention", exact: true })).toBeDisabled();
  for (const width of [390, 1280]) {
    await page.setViewportSize({ width, height: 900 });
    await expect(section).toBeVisible();
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBeTruthy();
  }
  await page.screenshot({ path: path.join(os.tmpdir(), "metrics233-data-retention-verified.png") });
  expect(errors).toEqual([]);
});

test("observed canonical IP can be blocked, unblocked and allowlisted without trusting visitor labels", async ({ page, service }) => {
  test.setTimeout(60000);
  const headers = { Authorization: `Bearer ${service.token}` };
  const read = async () => {
    const response = await page.request.get(service.url + "/api/v1/reporting-sources", { headers });
    expect(response.ok()).toBeTruthy(); return response.json();
  };
  const push = (authorized = true) => page.request.post(service.url + "/api/v1/ingest", {
    headers: { ...(authorized ? { Authorization: `Bearer ${service.ingestToken}` } : {}), "X-Forwarded-For": "203.0.113.90" },
    data: { samples: [{ name: "reporting_sources_ui_fixture", labels: { service: "source-ui-fixture", client_ip: "203.0.113.91" }, value: 1 }] },
  });
  expect((await push(false)).status()).toBe(401);
  expect((await read()).sources).toEqual([]);
  expect((await push()).ok()).toBeTruthy();
  await open(page, service);
  await expect(page.getByRole("heading", { name: "Reporting sources", exact: true })).toBeVisible();
  const list = page.getByRole("region", { name: "Source IPs", exact: true });
  const local = list.getByRole("row").filter({ has: page.getByText("127.0.0.1", { exact: true }) });
  await expect(local).toContainText("source-ui-fixture");
  await expect(local).toContainText("Allowed by default");
  expect((await read()).sources.map((source: any) => source.address)).toEqual(["127.0.0.1"]);
  await local.getByRole("button", { name: "Block", exact: true }).click();
  await expect(local).toContainText("Blocked");
  expect((await push()).status()).toBe(403);
  const blocked = await read();
  const ingestHeaders = { Authorization: `Bearer ${service.ingestToken}` };
  expect((await page.request.get(service.url + "/api/v1/reporting-sources", { headers: ingestHeaders })).status()).toBe(403);
  expect((await page.request.put(service.url + "/api/v1/reporting-sources/policy", {
    headers: ingestHeaders, data: { version: blocked.version, allowlist_only: false, address: "127.0.0.1", action: "allow" },
  })).status()).toBe(403);
  expect((await read()).version).toBe(blocked.version);
  await local.getByRole("button", { name: "Unblock and allow", exact: true }).click();
  await expect(local).toContainText("Allowlisted");
  expect((await push()).ok()).toBeTruthy();
  const mode = page.getByRole("checkbox", { name: "Only allow listed IPs to report", exact: true });
  await mode.check();
  await expect(mode).toBeChecked();
  await expect.poll(async () => (await read()).allowlist_only).toBe(true);
  await local.getByRole("button", { name: "Remove from allowlist", exact: true }).click();
  await expect(local).toContainText("Not allowlisted");
  expect((await push()).status()).toBe(403);
  const address = page.getByLabel("IP address", { exact: true });
  await address.fill("2001:0db8:0000:0000:0000:0000:0000:0009");
  await page.getByRole("button", { name: "Add to allowlist", exact: true }).click();
  const manual = list.getByRole("row").filter({ has: page.getByText("2001:db8::9", { exact: true }) });
  await expect(manual).toContainText("No reports yet");
  await expect(manual).toContainText("Allowlisted");

  // Advance the real catalog version out of band; the UI must not retry its stale mutation.
  const before = await read();
  const external = await page.request.put(service.url + "/api/v1/reporting-sources/policy", {
    headers, data: { version: before.version, allowlist_only: true, address: "203.0.113.77", action: "allow" },
  });
  expect(external.ok()).toBeTruthy();
  await local.getByRole("button", { name: "Allow", exact: true }).click();
  await expect(page.getByRole("alert")).toContainText("policy changed elsewhere");
  await expect(local.getByRole("button", { name: "Allow", exact: true })).toBeDisabled();
  expect((await read()).sources.find((source: any) => source.address === "127.0.0.1").disposition).toBe("");
  await page.getByRole("button", { name: "Refresh sources", exact: true }).click();
  await expect(local.getByRole("button", { name: "Allow", exact: true })).toBeEnabled();
  await local.getByRole("button", { name: "Allow", exact: true }).click();
  await expect(local).toContainText("Allowlisted");
  await mode.uncheck();
  await expect(mode).not.toBeChecked();
  await expect.poll(async () => (await read()).allowlist_only).toBe(false);
  expect((await push()).ok()).toBeTruthy();
});

test("source filtering and 50-row pagination remain local and Chinese mobile layout stays contained", async ({ page, service }) => {
  let reads = 0;
  const sources = Array.from({ length: 51 }, (_, index) => ({
    address: `198.51.100.${index + 1}`, disposition: "", allowed: true,
    first_seen: Date.now(), last_seen: Date.now(), accepted_batches: 1, accepted_samples: 1,
    protocols: ["json"], last_service: `source-service-${index + 1}`,
  }));
  await page.route("**/api/v1/reporting-sources", async (route) => {
    reads++;
    await route.fulfill({ json: { version: 1, allowlist_only: false, observation_limit: 1024, policy_limit: 4096, sources } });
  });
  await open(page, service, "zh");
  await expect(page.getByRole("heading", { name: "上报来源", exact: true })).toBeVisible();
  const list = page.getByRole("region", { name: "来源 IP 列表", exact: true });
  await expect(list.locator("tbody tr")).toHaveCount(50);
  const loadedReads = reads;
  await page.getByRole("button", { name: "下一页来源", exact: true }).click();
  await expect(list.locator("tbody tr")).toHaveCount(1);
  await expect(list).toContainText("198.51.100.51");
  await page.getByLabel("按 IP 或服务筛选", { exact: true }).fill("source-service-51");
  await expect(list.locator("tbody tr")).toHaveCount(1);
  expect(reads).toBe(loadedReads);
  await page.getByLabel("按 IP 或服务筛选", { exact: true }).fill("absent-fixture");
  await expect(page.getByRole("heading", { name: "没有匹配来源", exact: true })).toBeVisible();
  for (const width of [390, 768, 1280]) {
    await page.setViewportSize({ width, height: 900 });
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true);
  }
  await expect(page.getByText(/同一 NAT 出口的服务共用 IP 策略/)).toBeVisible();
  await expect(page.getByText(/并非在线状态/)).toBeVisible();
});
