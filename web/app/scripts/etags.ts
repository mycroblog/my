// SPDX-License-Identifier: AGPL-3.0-only
// Writes .vite/etags.json, the size and the ETag of every file of the build that the Go server serves as it is: the last
// step of `scripts/task.sh web:build`, once Vite, the Service Worker's build and the copy of web/brand have placed
// their files. The server (internal/interface/web) reads the list when it starts instead of the files, tens of MB of
// fonts among them, and refuses a build whose list names other files or sizes. An ETag is the first 18 bytes of the
// file's SHA-256 in unpadded base64url, as the server computes the ETags of what it renders.
// Usage: bun web/app/scripts/etags.ts <build directory>
import { createHash } from "node:crypto";
import { readdirSync, readFileSync, writeFileSync } from "node:fs";
import { join, relative, sep } from "node:path";

/** Where the list goes in the build: hidden, like Vite's manifest, so the server never serves it. */
const etagsPath = ".vite/etags.json";

/** The HTML that Vite writes at the build's root, which the server replaces with its own shells. */
const viteHTML = new Set(["index.html", "auth.html", "pay.html"]);

/** The size and the ETag of each served file of the build in `dir`, by its path with forward slashes, in order. */
export function etagList(dir: string): Record<string, { size: number; etag: string }> {
  const list: [string, { size: number; etag: string }][] = [];
  for (const entry of readdirSync(dir, { recursive: true, withFileTypes: true })) {
    const file = join(entry.parentPath, entry.name);
    const path = relative(dir, file).split(sep).join("/");
    // A hidden file, or a file of a hidden directory, is not served.
    if (!entry.isFile() || path.split("/").some((part) => part.startsWith(".")) || viteHTML.has(path)) continue;
    const content = readFileSync(file);
    const etag = createHash("sha256").update(content).digest().subarray(0, 18).toString("base64url");
    list.push([path, { size: content.length, etag }]);
  }
  return Object.fromEntries(list.sort(([a], [b]) => (a < b ? -1 : 1)));
}

if (import.meta.main) {
  const dir = process.argv[2];
  if (dir === undefined) throw new Error("usage: etags.ts <build directory>");
  writeFileSync(join(dir, etagsPath), `${JSON.stringify(etagList(dir))}\n`);
}
