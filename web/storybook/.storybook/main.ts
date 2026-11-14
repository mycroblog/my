// SPDX-License-Identifier: AGPL-3.0-only

// The one Storybook host of the web client (ADR-0165 §1, ADR-0110 §9): each package keeps its stories next to its
// components, and the host composes them by layer, a title prefix per layer: the tokens and the components of
// @my/design-system, the shared components of @my/app-components, and the screens of @my/app, which render the whole
// app on the fake server of @my/e2e. The host depends, directly or through @my/app, on each package whose stories it
// composes, so that the workspace's dependency graph names every package a story can change
// (scripts/changed-contexts.sh, the stories context). Tailwind builds the stylesheet of @my/design-system without
// its own optimizer, for the browsers of light-dark() (ADR-0110 §14), and the condition @mycroblog/source reads
// @mycroblog/mykit (under @my/app-data) from its sources, as the app's Vite configuration does.

import tailwindcss from "@tailwindcss/vite";
import type { StorybookConfig } from "@storybook/react-vite";
import react from "@vitejs/plugin-react";
import { defaultClientConditions, mergeConfig } from "vite";

const web = (dir: string) => `../../${dir}/src`;

const config: StorybookConfig = {
  framework: "@storybook/react-vite",
  stories: [
    { directory: web("design-system"), files: "tokens.stories.tsx", titlePrefix: "Tokens" },
    { directory: web("design-system"), files: "!(tokens).stories.tsx", titlePrefix: "Design system" },
    { directory: web("app-components"), files: "**/*.stories.tsx", titlePrefix: "App components" },
    { directory: web("app"), files: "**/*.stories.tsx", titlePrefix: "Screens" },
  ],
  // Axe on each story, and the stories as tests (ADR-0025 §1).
  addons: ["@storybook/addon-a11y", "@storybook/addon-vitest"],
  core: { disableTelemetry: true, disableWhatsNewNotifications: true },
  typescript: { reactDocgen: false },
  viteFinal: (vite) =>
    mergeConfig(vite, {
      plugins: [react(), tailwindcss({ optimize: false })],
      resolve: { conditions: ["@mycroblog/source", ...defaultClientConditions] },
      build: { cssTarget: ["chrome123", "firefox120", "safari17.5"] },
    }),
};

export default config;
