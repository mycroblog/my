// SPDX-License-Identifier: AGPL-3.0-only

// Serves the built catalog (web/storybook/dist) from disk for the run, on a port of its own, and stops after it. The
// tests read its origin from CATALOG_URL (specs/test.ts). A file never changes during a run, so the browser keeps
// each one it fetched.

import { existsSync } from "node:fs";
import { join, normalize } from "node:path";
import { root } from "../src/server/paths";

const dist = join(root, "web/storybook/dist");

export default function globalSetup() {
  if (!existsSync(join(dist, "iframe.html")))
    throw new Error(
      "catalog: no built catalog in web/storybook/dist; run the tests with scripts/task.sh web:layout-test",
    );
  const server = Bun.serve({
    hostname: "127.0.0.1",
    port: 0,
    async fetch(request) {
      const path = normalize(decodeURIComponent(new URL(request.url).pathname));
      const file = Bun.file(join(dist, path.endsWith("/") ? `${path}index.html` : path));
      if (!path.startsWith("/") || !(await file.exists())) return new Response("not found", { status: 404 });
      return new Response(file, { headers: { "Cache-Control": "max-age=3600, immutable" } });
    },
  });
  process.env["CATALOG_URL"] = `http://127.0.0.1:${String(server.port)}`;
  return () => server.stop(true);
}
