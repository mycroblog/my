// SPDX-License-Identifier: AGPL-3.0-only
// Vitest runs component tests on Node.js in jsdom (ADR-0025, ADR-0027). It extends vite.config.ts, so the
// React Compiler preset transforms the code under test as it does in the build. The preset applies to the
// client environment only, which is why tests never use the `node` environment (web-client/README.md T6).
import { defineConfig, mergeConfig } from "vitest/config";
import viteConfig from "./vite.config.ts";

export default mergeConfig(
  viteConfig,
  defineConfig({
    test: {
      // In CI, only the failing tests, the console output of those alone, and the summary (Vitest's `minimal`
      // reporter), and the failures as annotations on GitHub Actions; elsewhere Vitest's default lists every test.
      ...(process.env["CI"]
        ? { reporters: ["minimal", ...(process.env["GITHUB_ACTIONS"] === "true" ? ["github-actions"] : [])] }
        : {}),
      environment: "jsdom",
      include: ["src/**/*.test.{ts,tsx}", "sw/**/*.test.ts", "vite-plugins/**/*.test.ts"],
      setupFiles: ["./src/test/setup.ts"],
      restoreMocks: true,
      unstubGlobals: true,
      // A test may wait several times for asyncUtilTimeout (src/test/setup.ts); the 5 s default cut off
      // integration tests that a loaded machine slowed down, not tests that were stuck (web-client/README.md T7).
      testTimeout: 30_000,
    },
  }),
);
