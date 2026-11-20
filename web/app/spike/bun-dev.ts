// SPDX-License-Identifier: AGPL-3.0-only
// Spike #2906, not for main: what Bun's dev server (an HTML import with HMR) serves for index.html, to see whether
// a page of another server (the Go shell) could load the app and its HMR client from it, as from Vite's.
// Usage: bun spike/bun-dev.ts
import index from "./dev.html";

const server = Bun.serve({ port: 5174, development: { hmr: true }, routes: { "/": index } });
const html = await (await fetch(`${server.url}`)).text();
console.log(html);
for (const src of html.matchAll(/(?:src|href)="([^"]+)"/g)) {
  const res = await fetch(new URL(src[1]!, server.url));
  console.log(src[1], res.status, res.headers.get("content-type"), res.headers.get("access-control-allow-origin"));
}
server.stop(true);
