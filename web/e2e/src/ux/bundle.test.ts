// SPDX-License-Identifier: AGPL-3.0-only

// The bundle of each HTML entry (bundle.ts): the entry, its static imports and their stylesheets, each file once; a
// dynamic import is left out.

import { gzipSync } from "node:zlib";
import { expect, test } from "bun:test";
import { bundles, type Manifest } from "./bundle";

test("an entry's bundle is the entry, its static imports and their stylesheets, each once", () => {
  const manifest: Manifest = {
    "index.html": {
      file: "assets/index.js",
      isEntry: true,
      imports: ["_shared.js", "_ui.js"],
      css: ["assets/index.css"],
    },
    "auth.html": { file: "assets/auth.js", isEntry: true, imports: ["_shared.js"] },
    "_shared.js": { file: "assets/shared.js" },
    "_ui.js": { file: "assets/ui.js", imports: ["_shared.js"], css: ["assets/ui.css"] },
    "src/compose.tsx": { file: "assets/compose.js", isEntry: false },
    "sw.ts": { file: "sw.js", isEntry: true },
  };
  const sizes: Record<string, number> = {
    "assets/index.js": 100,
    "assets/auth.js": 40,
    "assets/shared.js": 30,
    "assets/ui.js": 20,
    "assets/index.css": 10,
    "assets/ui.css": 5,
    "assets/compose.js": 1000,
  };
  const content = (file: string) => new Uint8Array(sizes[file] ?? 0).fill(97);
  const read: string[] = [];
  const result = bundles(manifest, (file) => {
    read.push(file);
    return content(file);
  });
  const gz = (...files: string[]) => files.reduce((sum, f) => sum + gzipSync(content(f)).byteLength, 0);
  expect(result).toEqual([
    { entry: "auth.html", files: 2, bytes: 70, gzip: gz("assets/auth.js", "assets/shared.js") },
    {
      entry: "index.html",
      files: 5,
      bytes: 165,
      gzip: gz("assets/index.js", "assets/index.css", "assets/shared.js", "assets/ui.js", "assets/ui.css"),
    },
  ]);
  expect(read).not.toContain("assets/compose.js");
});
