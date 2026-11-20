// SPDX-License-Identifier: AGPL-3.0-only

// The Vite plugin that reloads the theme of styles.css in a running dev server (the app's, the Storybook host's) when
// a design token changes. Tailwind loads the theme (theme.ts, the @plugin of styles.css) into the dev server's own
// process, compiles the stylesheet again only when the plugin or a file it imports by a relative path changes, and
// drops from the module cache only those files. theme.ts imports @my/design-tokens by its name, so Tailwind neither
// sees a change to a token nor reads the tokens anew: on Node its loader hook would read them anew, but on Bun, which
// runs the dev servers, it registers none. So on a change to a file of @my/design-tokens, this plugin drops that
// package's files from the module cache and touches theme.ts, which Tailwind watches: Tailwind then compiles the
// stylesheet again with the tokens read anew, and the page takes the new stylesheet. Written against the part of
// Vite's plugin API it uses, so that this package needs no Vite of its own.

import { utimesSync } from "node:fs";
import { createRequire } from "node:module";
import { dirname, sep } from "node:path";
import { fileURLToPath } from "node:url";

/** The dev server, as far as the plugin uses it. */
interface DevServer {
  watcher: { on(event: "change", listener: (file: string) => void): unknown };
}

/** List it after Tailwind's plugin in the plugins of a Vite configuration. */
export function themeReload() {
  const tokens = dirname(fileURLToPath(import.meta.resolve("@my/design-tokens"))) + sep;
  const theme = fileURLToPath(new URL("theme.ts", import.meta.url));
  const cache = createRequire(import.meta.url).cache;
  return {
    name: "my:theme-reload",
    apply: "serve" as const,
    configureServer(server: DevServer) {
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
