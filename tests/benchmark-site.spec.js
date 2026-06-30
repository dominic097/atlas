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
        page.getByRole("heading", { name: /reads a sentence, not the whole file/i })
      ).toBeVisible();
      await expect(page.getByTestId("ratio-tokens")).toBeVisible();
      await expect(page.getByTestId("ratio-latency")).toBeVisible();

      // vs-graphify equation + toggle
      await expect(page.getByTestId("vs-graphify")).toBeVisible();
      await expect(page.getByTestId("vs-graphify-equation")).toContainText("comparable rows");
      await expect(page.getByTestId("graphify-toggle")).toBeVisible();

      // vs-native peer-framed callout + the unified native-parity ladder
      await expect(page.getByTestId("native-callout")).toContainText("Native indexers are the peer bar");
      await expect(page.getByTestId("native-callout")).toContainText("never averaged into");
      await expect(page.getByTestId("parity-ladder")).toBeVisible();
      await expect(page.getByTestId("parity-column")).toContainText(
        String(
          data.liveBenchmarks.filter((r) => r.coverage && r.coverage.ratio <= 1.0).length
        )
      );
      // C# is the strongest standout (×1.84, more defs than native)
      await expect(page.getByTestId("standout-csharp")).toHaveCount(1);

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

      // unified languages explorer + evidence
      await expect(page.getByTestId("matrix")).toBeVisible();
      await expect(page.getByTestId("lx-explorer")).toBeVisible();
      await expect(page.getByTestId("lx-view-toggle")).toBeVisible();
      // 43 languages benchmarked, surfaced as the all-filter count
      await expect(page.getByTestId("lx-chip-all")).toContainText(
        String(data.coreMatrix.length + data.liveBenchmarks.length)
      );
      await expect(page.getByTestId("evidence")).toBeVisible();
      await expect(page.getByTestId("provenance")).toContainText(data.provenance.graphify.version);

      // no console errors
      expect(consoleErrors).toEqual([]);

      // per-language real numbers: switch the explorer to Table view and assert
      // each core language renders its real symbol count + comparable rows
      await page.getByTestId("lx-view-table").click();
      await expect(page.getByTestId("lx-table")).toBeVisible();
      for (const row of data.coreMatrix) {
        await expect(page.getByTestId("lx-table")).toContainText(fmt.format(row.atlas.metrics.symbols));
      }
      // a live exceeds-native language shows its >1.0 coverage ratio
      const csharp = data.liveBenchmarks.find((r) => r.language === "csharp");
      await expect(page.getByTestId("lx-table")).toContainText(
        `${csharp.coverage.ratio.toFixed(2)}×`
      );

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

  test("saturated languages reachable + shown as not comparable via the explorer filter", async ({ page }) => {
    await page.goto(baseURL, { waitUntil: "networkidle" });
    // the unified explorer: filter to not-comparable, then read them in the table
    await page.getByTestId("lx-chip-not-comparable").click();
    await page.getByTestId("lx-view-table").click();
    const table = page.getByTestId("lx-table");
    await expect(table).toContainText("Byond");
    await expect(table).toContainText("ETS");
    // R row label is "R"; assert via its repo too
    await expect(table).toContainText("tidyverse/ggplot2");
    await expect(table).toContainText("not comparable");
    // exactly the 3 saturated rows, none folded into a win
    await expect(page.getByTestId("lx-row")).toHaveCount(
      data.liveBenchmarks.filter((r) => r.querySummary.tokenRatio == null).length
    );
  });

  test("graphify token/latency toggle re-binds the same chart", async ({ page }) => {
    await page.goto(baseURL, { waitUntil: "networkidle" });
    const equation = page.getByTestId("vs-graphify-equation");
    await expect(equation).toContainText("atlasTokens");
    await page.getByTestId("graphify-toggle").getByRole("button", { name: "Latency" }).click();
    await expect(equation).toContainText("atlasMs");
  });
});
