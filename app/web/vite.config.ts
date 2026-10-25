// SPDX-License-Identifier: AGPL-3.0-only
// Vite 8 build of the web client (ADR-0025). React Compiler runs through oxc-transform-react in a plugin of
// our own, which also fails the build on the functions the compiler skips (ADR-0065, web-client/README.md R4).
import { fileURLToPath } from "node:url";
import tailwindcss from "@tailwindcss/vite";
import { tanstackRouter } from "@tanstack/router-plugin/vite";
import react from "@vitejs/plugin-react";
import { defineConfig, searchForWorkspaceRoot } from "vite";
import { VitePWA } from "vite-plugin-pwa";
import { clientCatalogs } from "./vite-plugins/client-catalogs.ts";
import { bundledLicenses } from "./vite-plugins/licenses.ts";
import { reactCompiler } from "./vite-plugins/react-compiler.ts";

/** The source of @mycroblog/mykit (pkg/mykit/ts/src), outside the client workspace. */
const mykitSource = fileURLToPath(new URL("../../pkg/mykit/ts/src", import.meta.url));

/** The shared message catalogs at the repository root (ADR-0009). */
const localesDir = fileURLToPath(new URL("../../locales", import.meta.url));

/**
 * The license notices of bundled data that are not npm dependencies, copied into dist/licenses/: the Unicode emoji data
 * (ADR-0091 §1) and the Noto Color Emoji fonts of @my/ui (ADR-0095 §6).
 */
const licenseFiles = [
  fileURLToPath(new URL("src/features/emoji/unicode/LICENSE-Unicode.txt", import.meta.url)),
  fileURLToPath(new URL("../ui/src/fonts/emoji/OFL.txt", import.meta.url)),
];

export default defineConfig({
  // Go serves the HTML and builds its <script> / <link> tags from the manifest (ADR-0028).
  base: "./",
  build: {
    manifest: true,
    // The license notices of the npm packages in the bundle, next to those of the bundled data (licenseFiles); Go
    // serves both at /licenses/ and the about page links to each (features/about).
    license: { fileName: "licenses/npm-packages.md" },
    // Emit every asset as a file: the CSP of ADR-0019 §9 has no data: source.
    assetsInlineLimit: 0,
    rolldownOptions: {
      // The SPA, the stylesheet of the pages the Go server renders without JavaScript
      // (src/server-pages.css, ADR-0030), and the script of the sign-in page's passkey button, which
      // the page works without (src/server-pages/passkey-sign-in.ts). Go finds them by these manifest keys.
      input: {
        index: fileURLToPath(new URL("index.html", import.meta.url)),
        "server-pages": fileURLToPath(new URL("src/server-pages.css", import.meta.url)),
        "server-pages-passkey": fileURLToPath(new URL("src/server-pages/passkey-sign-in.ts", import.meta.url)),
      },
    },
  },
  resolve: {
    alias: [
      // Zod without eval, for every importer of "zod" in the app and its dependencies (src/lib/zod.ts).
      { find: /^zod$/, replacement: fileURLToPath(new URL("src/lib/zod.ts", import.meta.url)) },
      // The source of the workspace package @mycroblog/mykit (pkg/mykit/ts), the file its `@mycroblog/source` export
      // condition names (ADR-0093 §1), as tsconfig.app.json's customConditions resolve it for tsc. An alias rather
      // than resolve.conditions, which would replace the conditions Vitest resolves Node.js packages with.
      { find: /^@mycroblog\/mykit$/, replacement: `${mykitSource}/index.ts` },
    ],
  },
  server: {
    // `my serve -dev-vite-origin http://localhost:5173` loads the client from here (CONTRIBUTING.md),
    // so the URLs of assets must point at this server, not at the page's origin.
    port: 5173,
    strictPort: true,
    origin: "http://localhost:5173",
    // The client workspace (app/), and the source of @mycroblog/mykit outside it.
    fs: { allow: [searchForWorkspaceRoot(process.cwd()), mykitSource] },
  },
  plugins: [
    // File-based routes from src/routes into src/routeTree.gen.ts, which is not committed (ADR-0081);
    // scripts/routes.ts generates the same tree with these options for tsc and the tests before a build.
    // Each route's components go into their own chunk. TanStack Router's docs require this plugin before
    // the React plugin.
    tanstackRouter({ target: "react", autoCodeSplitting: true }),
    react(),
    reactCompiler(),
    clientCatalogs(localesDir),
    bundledLicenses(licenseFiles),
    tailwindcss(),
    // The Service Worker (ADR-0028): sw/sw.ts with the precache manifest injected, built into dist/sw.js
    // (https://vite-pwa-org.netlify.app/guide/inject-manifest.html). Go generates the Web App Manifest and
    // the HTML, so the plugin writes neither and injects no registration script; src/lib/pwa.ts registers
    // the worker and asks before activating an update.
    VitePWA({
      strategies: "injectManifest",
      srcDir: "sw",
      filename: "sw.ts",
      manifest: false,
      injectRegister: false,
      // Absolute, unlike the build's relative base: the worker lives at /sw.js and controls the whole
      // origin, whichever page registers it.
      base: "/",
      scope: "/",
      injectManifest: {
        // A classic script, which every browser registers.
        rollupFormat: "iife",
        // The worker is a build of its own: its bundled npm packages (Workbox) get their own notices.
        buildPlugins: {
          vite: [{ name: "my:sw-licenses", config: () => ({ build: { license: { fileName: "licenses/npm-packages-sw.md" } } }) }],
        },
        globPatterns: ["**/*.{js,css,svg,png,woff2}"],
        // index.html is the stand-in shell of `vite preview` (the worker precaches the server's shell at /
        // instead), and the stylesheet and the passkey script of the server-rendered pages are never loaded by the app.
        // The Noto Color Emoji splits (@my/ui/emoji.css, about 6 MB in both formats) are cached at run time when a
        // page uses them instead (sw/sw.ts, ADR-0095 §3).
        globIgnores: [
          "index.html",
          "sw.js",
          "assets/server-pages-*.css",
          "assets/server-pages-passkey-*.js",
          "assets/noto-emoji-*.woff2",
        ],
      },
    }),
  ],
});
