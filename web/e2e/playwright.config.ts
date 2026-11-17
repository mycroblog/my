// SPDX-License-Identifier: AGPL-3.0-only

// The E2E tests of the web client (ADR-0015 §1): one project, `en` on Chromium, against the demo server that
// specs/global-setup.ts starts with the scenario (src/server/demo.ts). Run them with `scripts/task.sh web:e2e`, which
// builds the test build of bin/my first; they run on Bun alone (`bun --bun playwright test`). The specs query by role
// and accessible name, and keep the operations of a screen in @my/e2e (restrict/e2e-queries), out of the patterns that
// made them flaky (restrict/e2e-flaky).

import { availableParallelism } from "node:os";
import { defineConfig, devices } from "@playwright/test";

export default defineConfig({
  testDir: "specs",
  globalSetup: "./specs/global-setup.ts",
  outputDir: "../../.tmp/e2e/test-results",
  forbidOnly: process.env["CI"] !== undefined,
  // No test runs again here: the pull request jobs of CI pass --retries, and the slow suite, which finds the tests
  // that fail only sometimes, --repeat-each.
  retries: 0,
  // The specs share one seeded server; each signs in as a person of the scenario. That one server answers every
  // worker, so more workers than it sustains make every request slower: the suite takes longer with twelve than with
  // six, and its heaviest test (three people signed in) passes its limit.
  workers: process.env["CI"] === undefined ? Math.min(6, Math.max(1, Math.floor(availableParallelism() / 2))) : 2,
  reporter: process.env["CI"] === undefined ? "list" : [["list"], ["github"]],
  use: {
    locale: "en",
    // Without motion: a spec asserts on a screen at rest, never mid-transition (restrict/e2e-flaky).
    reducedMotion: "reduce",
    trace: "retain-on-failure",
  },
  projects: [{ name: "en", use: { ...devices["Desktop Chrome"], locale: "en" } }],
});
