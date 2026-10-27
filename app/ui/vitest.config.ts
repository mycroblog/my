// SPDX-License-Identifier: AGPL-3.0-only
// Story tests of @my/ui (ADR-0027): every story runs in Chromium through Vitest browser mode and the
// Storybook Vitest addon. A story fails on any axe violation (parameters.a11y.test = 'error' in
// .storybook/preview.tsx) and on a visual difference from its screenshots (.storybook/vitest.setup.ts).
// There is one project per direction × theme, so contrast is checked in both themes and layout in RTL.
// Screenshot baselines are generated only in the pinned Playwright image (`scripts/task.sh ui:snapshots-update`).
// A test of either project also fails on a console.error or console.warn (test/consoleGuard.ts).
import { storybookTest } from "@storybook/addon-vitest/vitest-plugin";
import { playwright } from "@vitest/browser-playwright";
import { defineConfig, mergeConfig } from "vitest/config";
import viteConfig from "./vite.config.ts";

/** en runs left to right and ar-XB right to left; ja and en-XA add nothing a snapshot would show. */
const locales = ["en", "ar-XB"] as const;
const themes = ["light", "dark"] as const;

/**
 * In CI, only the failing tests, the console output of those alone, and the summary (Vitest's `minimal` reporter), and
 * the failures as annotations on GitHub Actions; a green run prints nothing per test. Elsewhere Vitest's default lists
 * every test.
 */
const reporters = process.env["CI"]
  ? { reporters: ["minimal", ...(process.env["GITHUB_ACTIONS"] === "true" ? ["github-actions"] : [])] }
  : {};

export default mergeConfig(
  viteConfig,
  defineConfig({
    test: {
      ...reporters,
      projects: [
        // Component tests (src/**/*.test.tsx): Testing Library in jsdom, run by `scripts/task.sh ui:unit-test`
        // (`bun run test:unit`). Like app/web, they never use the `node` environment (web-client/README.md T6).
        {
          extends: true,
          test: {
            name: "unit",
            environment: "jsdom",
            include: ["src/**/*.test.{ts,tsx}"],
            setupFiles: ["./test/consoleGuard.ts", "./test/setup.ts"],
            restoreMocks: true,
            // Room for several waits of asyncUtilTimeout (test/setup.ts) on a loaded machine (web-client/README.md T7).
            testTimeout: 30_000,
          },
        },
        ...locales.flatMap((locale) =>
        themes.map((theme) => ({
          extends: true,
          plugins: [storybookTest({ configDir: ".storybook", initialGlobals: { locale, theme } })],
          test: {
            name: `storybook:${locale}:${theme}`,
            setupFiles: ["./test/consoleGuard.ts", "./.storybook/vitest.setup.ts"],
            browser: {
              enabled: true,
              headless: true,
              provider: playwright(),
              instances: [{ browser: "chromium" as const }],
            },
          },
        })),
        ),
      ],
    },
  }),
);
