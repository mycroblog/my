// SPDX-License-Identifier: AGPL-3.0-only
// The story tests of app/web (ADR-0165 §1): every screen story runs in Chromium through Vitest browser mode and the
// Storybook Vitest addon, with its play function, and fails on any axe violation (parameters.a11y.test = 'error' in
// .storybook/preview.tsx) and on a console.error or console.warn (@my/ui's test/consoleGuard.ts). The component tests
// are vitest.config.ts (jsdom).
import { storybookTest } from "@storybook/addon-vitest/vitest-plugin";
import { playwright } from "@vitest/browser-playwright";
import { defineConfig, mergeConfig } from "vitest/config";
import { screenBudget } from "./l4/budgetRun.ts";
import viteConfig from "./vite.config.ts";

export default mergeConfig(
  viteConfig,
  defineConfig({
    plugins: [storybookTest({ configDir: ".storybook" })],
    // Found by Vite only once a story renders, which would reload the running test.
    optimizeDeps: { include: ["react/compiler-runtime", "react-aria/private/live-announcer/LiveAnnouncer"] },
    test: {
      // In CI, only the failing tests and the summary, and the failures as annotations on GitHub Actions.
      ...(process.env["CI"]
        ? { reporters: ["minimal", ...(process.env["GITHUB_ACTIONS"] === "true" ? ["github-actions"] : [])] }
        : {}),
      name: "storybook",
      // A screen loads the app and its data through the worker; room for a loaded machine (web-client/README.md T7).
      testTimeout: 30_000,
      // What each story shows against the budget of its screen's L1 file (l4/budget.ts).
      setupFiles: ["./src/test/storyBudget.ts"],
      globalSetup: ["./l4/budgetRun.ts"],
      browser: {
        commands: { screenBudget },
        enabled: true,
        headless: true,
        // The fixtures' time zone, so that a screen counts the same on every machine: the time zone settings show
        // more when the device's zone differs from the saved one (screen budgets, l4/budget.ts).
        provider: playwright({ contextOptions: { timezoneId: "UTC" } }),
        instances: [{ browser: "chromium" as const }],
      },
    },
  }),
);
