// SPDX-License-Identifier: AGPL-3.0-only

// The UX of the web client in the lab (ADR-0167 §4): the scenarios of ux/specs against the demo server of the E2E
// tests and its scenario (specs/global-setup.ts), each page with its CPU throttled (lab.ts), each scenario run three
// times one after another, so that each measure is a median and no run shares the machine with another. The reporter
// writes the report (reporter.ts). Run them with `scripts/task.sh web:e2e-ux`; the Nightly runs them, never the merge
// queue, so that the noise of a measure never stops a merge.

import { defineConfig, devices } from "@playwright/test";

export default defineConfig({
  testDir: "specs",
  globalSetup: "../specs/global-setup.ts",
  outputDir: "../../../.tmp/ux/test-results",
  forbidOnly: process.env["CI"] !== undefined,
  retries: 0,
  repeatEach: 3,
  workers: 1,
  // A throttled page takes several times as long as the E2E tests allow.
  timeout: 120_000,
  expect: { timeout: 30_000 },
  reporter: [["list"], ["./reporter.ts"]],
  use: {
    ...devices["Desktop Chrome"],
    locale: "en",
    // As the E2E tests: a scenario measures a screen that comes to rest, not its motion (restrict/e2e-flaky).
    reducedMotion: "reduce",
    trace: "retain-on-failure",
  },
});
