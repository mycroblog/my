// SPDX-License-Identifier: AGPL-3.0-only
// The Service Worker of the build (ADR-0028 §3): sw/sw.ts, bundled into one classic script at the root of the build,
// sw.js, which `my serve` serves at /sw.js without long-term caching. A worker is registered by its fixed address and
// may import nothing at run time, so it is built on its own after the app, with everything it needs inlined.
import { fileURLToPath } from "node:url";
import { build, type Plugin } from "vite";

/** Builds `entry` into `sw.js` in the app's output directory once the app is built. */
export function serviceWorker(entry: string): Plugin {
  let outDir = "";
  let conditions: string[] = [];
  return {
    name: "my:service-worker",
    apply: "build",
    configResolved(config) {
      outDir = config.build.outDir;
      conditions = [...config.resolve.conditions];
    },
    async closeBundle() {
      await build({
        configFile: false,
        logLevel: "warn",
        resolve: { conditions },
        build: {
          outDir,
          emptyOutDir: false,
          copyPublicDir: false,
          minify: true,
          lib: {
            entry: fileURLToPath(new URL(entry, import.meta.url)),
            formats: ["iife"],
            name: "sw",
            fileName: () => "sw.js",
          },
        },
      });
    },
  };
}
