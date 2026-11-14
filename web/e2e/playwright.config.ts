// SPDX-License-Identifier: AGPL-3.0-only

// The E2E tests of the web client (ADR-0015 §1): one project, `en` on Chromium, against the demo server that
// specs/global-setup.ts starts with the scenario (src/server/demo.ts). Run them with `scripts/task.sh web:e2e`, which
// builds the test build of bin/my first; they run on Bun alone (`bun --bun playwright test`). The specs query by role
// and accessible name, and keep the operations of a screen in @my/e2e (restrict/e2e-queries).

import { defineConfig, devices } from "@playwright/test";

export default defineConfig({
  testDir: "specs",
  globalSetup: "./specs/global-setup.ts",
  outputDir: "../../.tmp/e2e/test-results",
  forbidOnly: process.env["CI"] !== undefined,
  retries: 0,
  // The specs share one seeded server; each signs in as a person of the scenario.
  ...(process.env["CI"] === undefined ? {} : { workers: 2 }),
  reporter: process.env["CI"] === undefined ? "list" : [["list"], ["github"]],
  use: {
    locale: "en",
    trace: "retain-on-failure",
  },
  projects: [{ name: "en", use: { ...devices["Desktop Chrome"], locale: "en" } }],
});
