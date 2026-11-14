// SPDX-License-Identifier: AGPL-3.0-only

// The story tests (ADR-0110 §9, ADR-0025 §1): Storybook's Vitest addon runs every story of the host as a test in
// Chromium (browser mode, headless) at the minimum width, 320 px (ADR-0110 §10, the viewport global "minimum"; a story
// that names another viewport keeps its own), in the light theme once left-to-right in en and once right-to-left in
// ar-XB, a project each; and the stories tagged `themes` once more in the dark theme: every screen, and the tokens and
// the components whose look the theme changes. The contrast of every color pair is checked without rendering
// (web/design-tokens), but not a component that sets a token on the wrong surface, which only a rendered page shows.
// The a11y addon fails a story on an axe violation with the tags of ADR-0110 §9, and the
// preview's afterEach on a story that scrolls sideways (.storybook/preview.tsx). The addon's setup file imports React
// before Vite has bundled it, so React's entries are bundled ahead.

import { storybookTest } from "@storybook/addon-vitest/vitest-plugin";
import { playwright } from "@vitest/browser-playwright";
import { defineConfig, type TestProjectInlineConfiguration } from "vitest/config";

const project = (name: string, globals: Record<string, string>, tags?: string[]): TestProjectInlineConfiguration => ({
  extends: true,
  // The addon sizes the page by the story's viewport global: the host's minimum (.storybook/preview.tsx).
  plugins: [
    storybookTest({
      configDir: ".storybook",
      initialGlobals: { ...globals, viewport: { value: "minimum" } },
      ...(tags ? { tags: { include: tags } } : {}),
    }),
  ],
  test: {
    name,
    browser: { enabled: true, headless: true, provider: playwright(), instances: [{ browser: "chromium" }] },
  },
});

export default defineConfig({
  optimizeDeps: {
    include: ["react", "react-dom", "react-dom/client", "react/jsx-runtime", "react/jsx-dev-runtime"],
  },
  test: {
    projects: [
      project("en", { locale: "en", theme: "light" }),
      project("ar-XB", { locale: "ar-XB", theme: "light" }),
      project("dark", { locale: "en", theme: "dark" }, ["themes"]),
    ],
  },
});
