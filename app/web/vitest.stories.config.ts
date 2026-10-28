// SPDX-License-Identifier: AGPL-3.0-only
// The story tests of app/web (ADR-0112 §1): every screen story runs in Chromium through Vitest browser mode and the
// Storybook Vitest addon, with its play function, and fails on any axe violation (parameters.a11y.test = 'error' in
// .storybook/preview.tsx) and on a console.error or console.warn (@my/ui's test/consoleGuard.ts). Screens have no
// screenshot baselines: the E2E screenshots cover them. The component tests are vitest.config.ts (jsdom).
import { storybookTest } from "@storybook/addon-vitest/vitest-plugin";
import { playwright } from "@vitest/browser-playwright";
import { defineConfig, mergeConfig } from "vitest/config";
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
      browser: {
        enabled: true,
        headless: true,
        provider: playwright(),
        instances: [{ browser: "chromium" as const }],
      },
    },
  }),
);
