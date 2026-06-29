const fs = require("node:fs");
const path = require("node:path");
const { test, expect } = require("@playwright/test");

const baseURL = process.env.ATLAS_BENCHMARK_URL || "http://127.0.0.1:4179/";
const data = JSON.parse(fs.readFileSync(path.join(__dirname, "..", "data", "benchmark-data.json"), "utf8"));
const fmt = new Intl.NumberFormat("en-US");

test.describe("Atlas benchmark site", () => {
  for (const viewport of [
    { width: 1440, height: 900, name: "desktop" },
    { width: 768, height: 1024, name: "tablet" },
    { width: 390, height: 844, name: "mobile" },
  ]) {
    test(`renders verified benchmark data on ${viewport.name}`, async ({ page }) => {
      await page.setViewportSize(viewport);
      const consoleErrors = [];
      page.on("console", (message) => {
        if (message.type() === "error") consoleErrors.push(message.text());
      });

      const response = await page.goto(baseURL, { waitUntil: "networkidle" });
      expect(response.status()).toBe(200);
      await expect(page.getByRole("heading", { name: /benchmark evidence/i })).toBeVisible();
      await expect(page.getByTestId("benchmark-source-root")).toContainText("benchmark JSON artifacts");
      await expect(page.getByTestId("core-matrix")).toBeVisible();
      await expect(page.getByTestId("coverage-audit")).toBeVisible();
      await expect(page.getByTestId("saturation-evidence")).toBeVisible();
      expect(consoleErrors).toEqual([]);

      for (const row of data.coreMatrix) {
        const locator = page.getByTestId(`matrix-row-${row.language}`);
        await expect(locator).toBeVisible();
        await expect(locator).toContainText(row.atlas.status);
        await expect(locator).toContainText(row.graphify.status);
        await expect(locator).toContainText(fmt.format(row.atlas.metrics.symbols));
        await expect(locator).toContainText(fmt.format(row.graphify.metrics.nodes));
        await expect(locator).toContainText(`${row.querySummary.equivalentRows}/${row.querySummary.rows}`);
      }

      const sizes = await page.evaluate(() => ({
        body: document.body.scrollWidth,
        viewport: window.innerWidth,
      }));
      expect(sizes.body).toBeLessThanOrEqual(sizes.viewport + 2);
      expect(await page.locator("a[data-source-artifact]").count()).toBeGreaterThanOrEqual(10);

      await page.screenshot({
        path: `output/playwright/atlas-benchmark-${viewport.name}.png`,
        fullPage: true,
      });
    });
  }

  test("keeps saturation rows non-comparable", async ({ page }) => {
    await page.goto(baseURL, { waitUntil: "networkidle" });
    await page.selectOption("#live-filter", "saturated");
    await expect(page.locator("#live-body")).toContainText("Byond");
    await expect(page.locator("#live-body")).toContainText("Ets");
    await expect(page.locator("#live-body")).toContainText("R");
    await expect(page.locator("#live-body")).toContainText("not comparable");
  });
});
