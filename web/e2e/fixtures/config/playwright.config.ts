// SPDX-License-Identifier: AGPL-3.0-only
// A Playwright configuration of @my/e2e runs the specs with the motion reduced, in its own `use`: a project's does not
// count.

import { defineConfig } from "@playwright/test";

const config = {
  testDir: "specs",
  use: { locale: "en" },
  projects: [{ name: "en", use: { reducedMotion: "reduce" as const } }],
};

export default defineConfig(config); // error: e2e-flaky use.reducedMotion
