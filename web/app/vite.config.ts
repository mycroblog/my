// SPDX-License-Identifier: AGPL-3.0-only

import { fileURLToPath } from "node:url";
import tailwindcss from "@tailwindcss/vite";
import { tanstackRouter } from "@tanstack/router-plugin/vite";
import react from "@vitejs/plugin-react";
import { defaultClientConditions, defineConfig } from "vite";
import { reactCompiler } from "./vite-plugins/react-compiler.ts";

/** Where the Go server embeds the build from (internal/interface/web, //go:embed all:dist). */
const embedDir = fileURLToPath(new URL("../../internal/interface/web/dist", import.meta.url));

/**
 * The server that Vite's dev server passes the client API, the token endpoints and the server's own pages on to, so
 * that the app on http://localhost:5173 talks to it as on its own origin: the server of `docker compose up` unless
 * MY_SERVER names another.
 */
const server = process.env["MY_SERVER"] ?? "https://my.localhost";

// The build of the web client (ADR-0025 §1, §9, ADR-0028 §4): the Go server serves its own HTML shells and builds their
// <script> and <link> tags from the manifest, whose entries are index.html, the app, and auth.html, the auth screens
// that a page load opens, so the build writes the manifest, with a relative base, into the directory Go embeds;
// web:build empties it first and adds web/brand. Every asset is a file of
// its own, since the shell's CSP allows no data: source. React Compiler runs through the plugin of vite-plugins/,
// which fails the build on a function it skips. The routes of src/routes compile into src/routeTree.gen.ts, which
// scripts/routes.ts also writes with the same options, each route's components in a chunk of their own; TanStack
// Router's plugin comes before React's. Tailwind builds the stylesheet of @my/design-system without its own optimizer,
// for the browsers of light-dark() (ADR-0025 §1). The condition @mycroblog/source reads @mycroblog/mykit (under
// @my/app-data) from its sources, which need no build.
export default defineConfig({
  base: "./",
  plugins: [
    tanstackRouter({ target: "react", autoCodeSplitting: true }),
    react(),
    reactCompiler(),
    tailwindcss({ optimize: false }),
  ],
  resolve: { conditions: ["@mycroblog/source", ...defaultClientConditions] },
  build: {
    outDir: embedDir,
    emptyOutDir: false,
    manifest: true,
    assetsInlineLimit: 0,
    cssTarget: ["chrome123", "firefox120", "safari17.5"],
    license: { fileName: "licenses/npm-packages.md" },
    rolldownOptions: {
      input: [
        fileURLToPath(new URL("index.html", import.meta.url)),
        fileURLToPath(new URL("auth.html", import.meta.url)),
      ],
    },
  },
  server: {
    // `my serve -dev-vite-origin http://localhost:5173` loads the client from here (CONTRIBUTING.md).
    port: 5173,
    strictPort: true,
    origin: "http://localhost:5173",
    proxy: Object.fromEntries(
      ["/api", "/oauth", "/auth"].map((path) => [path, { target: server, changeOrigin: true, secure: false }]),
    ),
  },
});
