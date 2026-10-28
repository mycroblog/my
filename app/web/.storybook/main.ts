// SPDX-License-Identifier: AGPL-3.0-only
// Storybook 10 for the screens of app/web (ADR-0112 §1): each screen story renders the whole app at a route, with MSW
// in the browser (msw/browser) playing the server with the features' default handlers. It composes the @my/ui
// Storybook (`refs`), so one URL shows the components and the screens: in development the kit's own Storybook on
// port 6006 (`scripts/task.sh ui:storybook`), in the static build the kit's build copied into ./ui
// (`scripts/task.sh web:storybook-build`). The Vite config is ../vite.config.ts, without the plugins and the build
// options of the app's own bundle. The stories double as the accessibility tests (../vitest.stories.config.ts).
import { readFileSync } from "node:fs";
import { createRequire } from "node:module";
import type { StorybookConfig } from "@storybook/react-vite";
import type { Plugin, PluginOption } from "vite";

/** The plugins of vite.config.ts that build the app's own bundle (the Service Worker, the license notices). */
const appBundlePlugins = /^(vite-plugin-pwa|my:licenses)/;

function pluginName(plugin: PluginOption): string {
  return plugin !== null && typeof plugin === "object" && "name" in plugin ? plugin.name : "";
}

/**
 * Serves MSW's Service Worker at /mockServiceWorker.js, the root scope it intercepts the page's requests in, and
 * writes it into the build. The file comes from the installed msw, so it always matches the msw/browser of the page.
 */
function mockServiceWorker(): Plugin {
  const fileName = "mockServiceWorker.js";
  const source = () => readFileSync(createRequire(import.meta.url).resolve("msw/mockServiceWorker.js"), "utf8");
  return {
    name: "my:mock-service-worker",
    configureServer(server) {
      server.middlewares.use(`/${fileName}`, (_request, response) => {
        response.setHeader("Content-Type", "text/javascript");
        response.end(source());
      });
    },
    generateBundle() {
      this.emitFile({ type: "asset", fileName, source: source() });
    },
  };
}

const config: StorybookConfig = {
  framework: "@storybook/react-vite",
  stories: ["../src/**/*.stories.tsx"],
  addons: ["@storybook/addon-a11y", "@storybook/addon-vitest"],
  refs: (_config, { configType }) => ({
    ui: { title: "@my/ui", url: configType === "DEVELOPMENT" ? "http://localhost:6006" : "./ui" },
  }),
  core: { disableTelemetry: true },
  // A screen story has no args to document, and the docgen of every module of the app is most of the build's time.
  typescript: { reactDocgen: false },
  viteFinal: (viteConfig) => {
    // The app's inputs (the SPA, the server pages), manifest and license file belong to its own bundle.
    const { rolldownOptions: _input, manifest: _manifest, license: _license, ...build } = viteConfig.build ?? {};
    return {
      ...viteConfig,
      build,
      plugins: [
        ...(viteConfig.plugins ?? []).flat().filter((plugin) => !appBundlePlugins.test(pluginName(plugin))),
        mockServiceWorker(),
      ],
    };
  },
};

export default config;
