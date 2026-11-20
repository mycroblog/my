// SPDX-License-Identifier: AGPL-3.0-only
// Writes .vite/etags.json.gz, the size and the ETag of every file of the build that the Go server serves: the last step of
// `scripts/task.sh web:build`, once Vite, the Service Worker's build and the copy of web/brand have placed their files
// and compress.ts has compressed them. The server (internal/interface/web) reads the list when it starts instead of the
// files, tens of MB of fonts among them, and refuses a build whose list names other files or sizes. An ETag is the first
// 18 bytes of the content's SHA-256 in unpadded base64url, as the server computes the ETags of what it renders.
// A file that the build holds only compressed (`<file>.br` and `<file>.gz`) is listed under its own path, with the size
// and the ETag of its content, which the server sends gunzipped to a request that accepts neither encoding, and with
// those of each compressed form under `encodings`, by the Content-Encoding it is sent with.
// Usage: bun web/app/scripts/etags.ts <build directory>
import { createHash } from "node:crypto";
import { readdirSync, readFileSync, writeFileSync } from "node:fs";
import { join, relative, sep } from "node:path";
import { gunzipSync, gzipSync } from "node:zlib";

/** Where the list goes in the build, gzipped like Vite's manifest: hidden, so the server never serves it. */
const etagsPath = ".vite/etags.json.gz";

/** The HTML that Vite writes at the build's root, which the server replaces with its own shells. */
const viteHTML = new Set(["index.html", "auth.html", "pay.html"]);

/** The Content-Encoding of each extension of a compressed form. */
const encodings = { ".br": "br", ".gz": "gzip" } as const;

/** The size and the ETag of some content. */
type Tagged = { size: number; etag: string };

/** A served file of the list: its content's, and its compressed forms' by their Content-Encoding. */
type Listed = Tagged & { encodings?: Partial<Record<(typeof encodings)[keyof typeof encodings], Tagged>> };

function tagged(content: Buffer): Tagged {
  return {
    size: content.length,
    etag: createHash("sha256").update(content).digest().subarray(0, 18).toString("base64url"),
  };
}

/** The size and the ETag of each served file of the build in `dir`, by its path with forward slashes, in order. */
export function etagList(dir: string): Record<string, Listed> {
  const list = new Map<string, Listed>();
  for (const entry of readdirSync(dir, { recursive: true, withFileTypes: true })) {
    const file = join(entry.parentPath, entry.name);
    let path = relative(dir, file).split(sep).join("/");
    const ext = path.slice(path.lastIndexOf("."));
    const encoding = Object.hasOwn(encodings, ext) ? encodings[ext as keyof typeof encodings] : undefined;
    if (encoding !== undefined) path = path.slice(0, -ext.length);
    // A hidden file, or a file of a hidden directory, is not served.
    if (!entry.isFile() || path.split("/").some((part) => part.startsWith(".")) || viteHTML.has(path)) continue;
    const content = readFileSync(file);
    if (encoding === undefined) {
      list.set(path, { ...list.get(path), ...tagged(content) });
      continue;
    }
    const listed: Listed = list.get(path) ?? { size: 0, etag: "" };
    if (encoding === "gzip") Object.assign(listed, tagged(gunzipSync(content)));
    listed.encodings = { ...listed.encodings, [encoding]: tagged(content) };
    list.set(path, listed);
  }
  for (const [path, listed] of list) {
    if (listed.etag === "") throw new Error(`${path}.br has no ${path}.gz, from which the server sends ${path}`);
  }
  return Object.fromEntries([...list].sort(([a], [b]) => (a < b ? -1 : 1)));
}

if (import.meta.main) {
  const dir = process.argv[2];
  if (dir === undefined) throw new Error("usage: etags.ts <build directory>");
  writeFileSync(join(dir, etagsPath), gzipSync(`${JSON.stringify(etagList(dir))}\n`, { level: 9 }));
}
