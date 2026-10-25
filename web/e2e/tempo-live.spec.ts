import { test, expect } from "@playwright/test";

const endpoint = process.env.METRICSPANEL_TEMPO_E2E_URL || "";
const token = process.env.METRICSPANEL_TEMPO_E2E_TOKEN || "";
const traceID = process.env.METRICSPANEL_TEMPO_E2E_TRACE || "";
const dashboard = process.env.METRICSPANEL_TEMPO_E2E_DASHBOARD || "";

test.use({ extraHTTPHeaders: token ? { Authorization: `Bearer ${token}` } : {} });

test("Real Tempo ingested spans render search, errors, waterfall and durable deep links", async ({ page }) => {
  test.skip(!endpoint, "Requires the owned real Tempo integration fixture");
  expect(traceID).toMatch(/^[a-f0-9]{32}$/);
  expect(dashboard).not.toBe("");
  const errors: string[] = [];
  page.on("pageerror", (error) => errors.push(error.message));
  await page.addInitScript(() => localStorage.setItem("metricspanel-locale", "en"));
  await page.goto(`${endpoint}/d/${dashboard}/live-tempo-verification?from=now-1h&to=now`);
  const panel = page.getByRole("region", { name: "Live Tempo traces", exact: true });
  await panel.getByRole("link", { name: traceID, exact: true }).click();
  await expect(page).toHaveURL(new RegExp(`traceId=${traceID}`));
  const detail = panel.getByRole("region", { name: "Trace details", exact: true });
  await expect(detail).toHaveAttribute("data-span-count", "3");
  const timeline = detail.getByRole("table", { name: "Trace span timeline", exact: true });
  const database = timeline.locator('[data-span-id="0000000000000002"]');
  await expect(database).toHaveAttribute("data-parent-span-id", "0000000000000001");
  await expect(database).toHaveAttribute("data-depth", "1");
  await expect(database).toHaveAttribute("data-status", "error");
  await expect(database).toContainText("25 ms");
  await expect(database.getByRole("img")).toHaveAttribute("aria-label", "INSERT orders: +10 ms, 25 ms");
  await database.getByRole("button", { name: "orders-db INSERT orders", exact: true }).click();
  await expect(detail.getByRole("complementary", { name: "Span attributes", exact: true })).toContainText("database deadline exceeded");
  await expect(timeline.locator('[data-span-id="0000000000000003"]')).toHaveAttribute("data-depth", "2");
  await detail.getByRole("button", { name: "Collapse INSERT orders", exact: true }).click();
  await expect(timeline.locator("tbody tr")).toHaveCount(2);
  await detail.getByRole("button", { name: "Expand INSERT orders", exact: true }).click();
  await expect(timeline.locator("tbody tr")).toHaveCount(3);
  await page.reload();
  await expect(detail).toHaveAttribute("data-span-count", "3");
  for (const width of [390, 768, 1280]) {
    await page.setViewportSize({ width, height: 1000 });
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  }
  await detail.getByRole("button", { name: "Close trace details", exact: true }).click();
  await expect(detail).toHaveCount(0);
  await page.goBack();
  await expect(detail).toHaveAttribute("data-span-count", "3");
  expect(errors).toEqual([]);
});
