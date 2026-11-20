// SPDX-License-Identifier: AGPL-3.0-only
// Spike #2906, not for main: the Service Worker without vite-plugin-pwa. Bun.build bundles sw/sw.ts into one classic
// script, and workbox-build's injectManifest writes the precache manifest of the build into it, with the globs of
// vite.config.ts.
// Usage: bun spike/bun-sw.ts <build dir>
import { resolve } from "node:path";
import { injectManifest } from "workbox-build";

const out = resolve(process.argv[2] ?? "../../.tmp/bun-build");
const started = performance.now();
const built = await Bun.build({
  entrypoints: [resolve(import.meta.dir, "../sw/sw.ts")],
  target: "browser",
  format: "iife",
  minify: true,
  conditions: ["@mycroblog/source"],
  define: { "process.env.NODE_ENV": '"production"' },
});
if (!built.success) {
  for (const log of built.logs) console.error(log);
  process.exit(1);
}
const swSrc = resolve(out, ".vite/sw.src.js");
await Bun.write(swSrc, built.outputs[0]!);
const { count, size, warnings } = await injectManifest({
  swSrc,
  swDest: resolve(out, "sw.js"),
  globDirectory: out,
  globPatterns: ["**/*.{js,css,svg,png}"],
  globIgnores: ["sw.js", ".vite/**", "assets/auth-*.js", "assets/auth-*.css", "assets/pay-*.js", "assets/pay-*.css"],
  // Bun's names carry their own hash: as vite-plugin-pwa's dontCacheBustURLsMatching for assets/.
  dontCacheBustURLsMatching: /^assets\//,
});
console.log(JSON.stringify({ ms: Math.round(performance.now() - started), precached: count, bytes: size, warnings }));
