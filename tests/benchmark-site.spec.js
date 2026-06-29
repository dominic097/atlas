const fs = require("node:fs");
const path = require("node:path");
const { test, expect } = require("@playwright/test");

const baseURL = process.env.ATLAS_BENCHMARK_URL || "http://127.0.0.1:4179/";
const data = JSON.parse(
  fs.readFileSync(path.join(__dirname, "..", "data", "benchmark-data.json"), "utf8")
);
const fmt = new Intl.NumberFormat("en-US");

// The 5 install commands that must appear VERBATIM under #install.
const INSTALL_COMMANDS = [
  "brew install --cask dominic097/atlas/atlas",
  "npm install -g @dominic097/atlas",
  "atlas index . --reindex",
  "atlas context --paths path/to/changed-file.go",
  "atlas mcp --transport http --http 127.0.0.1:8765",
];

test.describe("Atlas — The Benchmark Instrument", () => {
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
      page.on("pageerror", (err) => consoleErrors.push(String(err)));

      const response = await page.goto(baseURL, { waitUntil: "networkidle" });
      expect(response.status()).toBe(200);

      // hero headline + ratios
      await expect(page.getByTestId("hero")).toBeVisible();
      await expect(
        page.getByRole("heading", { name: /smallest useful picture of a code change/i })
      ).toBeVisible();
      await expect(page.getByTestId("ratio-tokens")).toBeVisible();
      await expect(page.getByTestId("ratio-latency")).toBeVisible();

      // vs-graphify equation + toggle
      await expect(page.getByTestId("vs-graphify")).toBeVisible();
      await expect(page.getByTestId("vs-graphify-equation")).toContainText("comparable rows");
      await expect(page.getByTestId("graphify-toggle")).toBeVisible();

      // vs-native verbatim callout
      await expect(page.getByTestId("native-callout")).toContainText("Different graph model");
      await expect(page.getByTestId("native-callout")).toContainText("never averaged in");

      // not-comparable honesty section
      await expect(page.getByTestId("not-comparable")).toBeVisible();
      await expect(page.getByTestId("fault-lane")).toBeVisible();

      // graph exhibit present (two instances exist; at least one canvas)
      expect(await page.getByTestId("graph-canvas").count()).toBeGreaterThanOrEqual(1);

      // install commands verbatim under #install
      const install = page.getByTestId("install");
      await expect(install).toBeVisible();
      for (const cmd of INSTALL_COMMANDS) {
        await expect(install).toContainText(cmd);
      }

      // matrix tabs + evidence
      await expect(page.getByTestId("matrix")).toBeVisible();
      await expect(page.getByTestId("evidence")).toBeVisible();
      await expect(page.getByTestId("provenance")).toContainText(data.provenance.graphify.version);

      // no console errors
      expect(consoleErrors).toEqual([]);

      // core matrix language rows show real symbol/node counts + equiv/rows
      for (const row of data.coreMatrix) {
        const locator = page.getByTestId(`matrix-row-${row.language}`);
        await expect(locator).toBeVisible();
        await expect(locator).toContainText(fmt.format(row.atlas.metrics.symbols));
        await expect(locator).toContainText(fmt.format(row.graphify.metrics.nodes));
        await expect(locator).toContainText(
          `${row.querySummary.equivalentRows}/${row.querySummary.rows}`
        );
      }

      // no horizontal overflow
      const sizes = await page.evaluate(() => ({
        body: document.body.scrollWidth,
        viewport: window.innerWidth,
      }));
      expect(sizes.body).toBeLessThanOrEqual(sizes.viewport + 2);

      // evidence link guarantees
      expect(await page.locator("a[data-source-artifact]").count()).toBeGreaterThanOrEqual(10);
      expect(await page.locator("a[download]").count()).toBeGreaterThanOrEqual(4);

      await page.screenshot({
        path: `output/playwright/atlas-benchmark-${viewport.name}.png`,
        fullPage: true,
      });
    });
  }

  test("saturated languages shown as not comparable in the live matrix", async ({ page }) => {
    await page.goto(baseURL, { waitUntil: "networkidle" });
    // switch the matrix to the Live tab, then filter to no-comparable rows
    await page.getByTestId("matrix-tab-live").click();
    await page.selectOption("#live-filter", "partial");
    const body = page.locator("#live-body");
    await expect(body).toContainText("Byond");
    await expect(body).toContainText("ETS");
    // R row is keyed by its language code; assert via its repo + status.
    await expect(body).toContainText("tidyverse/ggplot2");
    await expect(body).toContainText("not comparable");
  });

  test("graphify token/latency toggle re-binds the same chart", async ({ page }) => {
    await page.goto(baseURL, { waitUntil: "networkidle" });
    const equation = page.getByTestId("vs-graphify-equation");
    await expect(equation).toContainText("atlasTokens");
    await page.getByTestId("graphify-toggle").getByRole("button", { name: "Latency" }).click();
    await expect(equation).toContainText("atlasMs");
  });
});
