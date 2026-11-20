// SPDX-License-Identifier: AGPL-3.0-only

// The documentation site's screenshots (ADR-0015 §5): shots/specs against the demo server of the E2E tests and its
// scenario, on a port of its own (global-setup.ts), one shot after another, at one desktop viewport, in English, in
// UTC, without motion. Run them with `scripts/task.sh web:docs-screenshots`, which writes
// web/docs/src/assets/screenshots/; nothing in CI runs them.

import { defineConfig, devices } from "@playwright/test";

export default defineConfig({
  testDir: "specs",
  globalSetup: "./global-setup.ts",
  outputDir: "../../../.tmp/shots/test-results",
  forbidOnly: process.env["CI"] !== undefined,
  retries: 0,
  workers: 1,
  timeout: 120_000,
  reporter: "list",
  use: {
    ...devices["Desktop Chrome"],
    viewport: { width: 1280, height: 800 },
    deviceScaleFactor: 1,
    locale: "en",
    timezoneId: "UTC",
    reducedMotion: "reduce",
    // The Service Worker would answer the API before the times are moved (time.ts).
    serviceWorkers: "block",
    // Drawn on the CPU, one raster thread, in sRGB, text without hinting: so the text and the icons come out the same
    // on every run. A picture scaled down (an avatar) may still differ in a few pixels from one run to the next.
    launchOptions: {
      args: ["--disable-gpu", "--num-raster-threads=1", "--force-color-profile=srgb", "--font-render-hinting=none"],
    },
    trace: "retain-on-failure",
  },
});
