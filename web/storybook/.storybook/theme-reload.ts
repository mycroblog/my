// SPDX-License-Identifier: AGPL-3.0-only

// Reloads the theme of @my/design-system in the running dev server when a design token changes. Tailwind loads the
// theme (theme.ts, the @plugin of styles.css) into the dev server's own process, compiles the stylesheet again only
// when the plugin or a file it imports by a relative path changes, and drops from the module cache only those files.
// theme.ts imports @my/design-tokens by its name, so Tailwind neither sees a change to a token nor reads the tokens
// anew: on Node its loader hook would read them anew, but on Bun, which runs the dev server, it registers none. So on
// a change to a file of @my/design-tokens, this plugin drops that package's files from the module cache and touches
// theme.ts, which Tailwind watches: Tailwind then compiles the stylesheet again with the tokens read anew, and the page
// takes the new stylesheet.

import { utimesSync } from "node:fs";
import { createRequire } from "node:module";
import { join, sep } from "node:path";
import { searchForWorkspaceRoot, type Plugin } from "vite";

export function themeReload(): Plugin {
  let tokens = "";
  let theme = "";
  let cache: NodeJS.Require["cache"] = {};
  return {
    name: "my:theme-reload",
    apply: "serve",
    configResolved(config) {
      const root = searchForWorkspaceRoot(config.root);
      tokens = join(root, "web", "design-tokens", "src") + sep;
      theme = join(root, "web", "design-system", "src", "theme.ts");
      cache = createRequire(join(root, "package.json")).cache;
    },
    configureServer(server) {
      server.watcher.on("change", (file) => {
        if (!file.startsWith(tokens)) return;
        // The module cache is the runtime's own object, not a Map of ours.
        for (const id of Object.keys(cache)) if (id.startsWith(tokens)) Reflect.deleteProperty(cache, id);
        const now = new Date();
        utimesSync(theme, now, now);
      });
    },
  };
}
