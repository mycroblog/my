// SPDX-License-Identifier: AGPL-3.0-only

// The tests that run in a browser without a server (ADR-0171 §1, §4): they open the stories of the built catalog
// (web/storybook/dist, which `scripts/task.sh web:stories` builds), which global-setup.ts serves from disk, with the
// E2E tests' runner. A test asks for a story by its URL and sets the viewport, the direction and the theme itself; no
// story pins them. Two projects, each a kind of test with a task of its own:
//
// - `layout` (specs/layout, `scripts/task.sh web:layout-test`): what only a layout engine decides of the design
//   system's layout parts and overlay surfaces, and the few things above them that only a browser draws (which font
//   draws a script, the axe rules that need computed style).
// - `browser` (specs/browser, `scripts/task.sh web:browser-test`): what a part does with a browser's own input, which
//   no DOM simulation has (an input method's composition, the caret's keys, the clipboard). They are no layout tests.

import { defineConfig, devices } from "@playwright/test";

const chrome = { ...devices["Desktop Chrome"], locale: "en" };

export default defineConfig({
  testDir: "specs",
  globalSetup: "./global-setup.ts",
  outputDir: "../../../.tmp/catalog/test-results",
  forbidOnly: process.env["CI"] !== undefined,
  retries: 0,
  // The limit of a run (ADR-0171 §7): the tests of a project, which a task runs alone, take 60 seconds at most. Over
  // it, a test moves to a lower layer or goes; the limit does not grow.
  globalTimeout: 60_000,
  // A story is a page of its own with no state behind it, so every test runs beside the others, one per core: what a
  // test waits for is the browser drawing the page.
  fullyParallel: true,
  ...(process.env["CI"] === undefined ? {} : { workers: "100%" }),
  reporter: process.env["CI"] === undefined ? "list" : [["list"], ["github"]],
  use: {
    locale: "en",
    // Without motion: a test asserts on a page at rest, never mid-transition (restrict/e2e-flaky).
    reducedMotion: "reduce",
    trace: "retain-on-failure",
  },
  projects: [
    { name: "layout", testDir: "specs/layout", use: chrome },
    { name: "browser", testDir: "specs/browser", use: chrome },
  ],
});
