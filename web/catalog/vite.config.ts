// SPDX-License-Identifier: AGPL-3.0-only

import { createRequire } from "node:module";
import { fileURLToPath } from "node:url";
import { devtools } from "@my/devtools/vite";
import tailwindcss from "@tailwindcss/vite";
import react from "@vitejs/plugin-react";
import { defaultClientConditions, defineConfig, type Plugin } from "vite";

// Tailwind loads the theme of @my/design-system (theme.ts) in this process. Under Bun its cache-busting loader hook is
// off, so Bun's module cache keeps what theme.ts imports (@my/design-tokens), and Tailwind rebuilds only when theme.ts
// or the stylesheet changes. So when a workspace source file in that cache changes, drop the workspace's files from it
// and restart the server, whose new Tailwind loads the theme afresh; the page reloads on the restart.
function reloadTheme(): Plugin {
  const { cache } = createRequire(import.meta.url);
  const web = fileURLToPath(new URL("..", import.meta.url));
  return {
    name: "my:reload-theme",
    apply: "serve",
    configureServer(server) {
      server.watcher.on("change", (file) => {
        const loaded = Object.keys(cache).filter((path) => path.startsWith(web) && !path.includes("/node_modules/"));
        if (!loaded.includes(file)) return;
        for (const path of loaded) Reflect.deleteProperty(cache, path);
        void server.restart();
      });
    },
  };
}

// The catalog page runs on the dev server only (`scripts/task.sh web:catalog`). Its production build exists for
// web:devtools-absent, which proves that the build leaves @my/devtools out. Tailwind builds the stylesheet of
// @my/design-system without its own optimizer, and the build targets the browsers of light-dark() (ADR-0110 §14):
// older targets would lower it to the system's scheme alone and lose an element's color-scheme. The condition
// @mycroblog/source reads @mycroblog/mykit (under /app/timeline's @my/app-data) from its sources, which need no build.
export default defineConfig({
  plugins: [devtools(), react(), tailwindcss({ optimize: false }), reloadTheme()],
  resolve: { conditions: ["@mycroblog/source", ...defaultClientConditions] },
  build: { cssTarget: ["chrome123", "firefox120", "safari17.5"] },
});
