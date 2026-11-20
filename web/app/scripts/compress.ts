// SPDX-License-Identifier: AGPL-3.0-only
// Compresses the build in place, so that the Go server embeds only the compressed forms of its text files: a step of
// `scripts/task.sh web:build` after the inventory of the payment entry's scripts and before the list of the ETags.
// - A file the server serves (a script, a stylesheet, an SVG, a license notice) becomes `<file>.br` (Brotli at its
//   highest quality) and `<file>.gz` (gzip -9), and the file itself goes. The server answers with Brotli when the
//   request accepts it, with gzip otherwise, and gunzips the `.gz` for a request that accepts neither.
// - A file the server only reads (Vite's manifest, the HTML that Vite writes) becomes `<file>.gz` alone.
// - Fonts and images are compressed already and stay as they are, and so does the inventory of the payment entry's
//   scripts, which each release copies from the image.
// Usage: bun web/app/scripts/compress.ts <build directory>
import { readdirSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { join, relative, sep } from "node:path";
import { brotliCompressSync, constants, gzipSync } from "node:zlib";

/** The extensions of the text files that are compressed. */
const compressible = /\.(?:js|mjs|css|html|json|svg|txt|md|xml|webmanifest)$/;

/** The files that stay as they are: the inventory that the Dockerfile copies out of the build. */
const kept = new Set([".vite/payment-scripts.json"]);

/** The HTML that Vite writes at the build's root, which the server never serves. */
const viteHTML = new Set(["index.html", "auth.html", "pay.html"]);

/** Whether the server serves the file at `path` (with forward slashes) of a build, rather than only reading it. */
function served(path: string): boolean {
  return !path.split("/").some((part) => part.startsWith(".")) && !viteHTML.has(path);
}

/** Compresses each text file of the build in `dir` and removes it, returning the paths it compressed, in order. */
export function compressBuild(dir: string): string[] {
  const done: string[] = [];
  const paths = readdirSync(dir, { recursive: true, withFileTypes: true })
    .filter((entry) => entry.isFile())
    .map((entry) => relative(dir, join(entry.parentPath, entry.name)).split(sep).join("/"))
    .sort();
  for (const path of paths) {
    // The server reads a .br or a .gz as a compressed form of the file without its extension.
    if (/\.(?:br|gz)$/.test(path)) throw new Error(`${path}: the build already holds a compressed file`);
  }
  for (const path of paths) {
    if (!compressible.test(path) || kept.has(path)) continue;
    const file = join(dir, path);
    const content = readFileSync(file);
    // gzip's header holds no name or time, so the same content compresses to the same bytes.
    writeFileSync(`${file}.gz`, gzipSync(content, { level: 9 }));
    if (served(path)) {
      writeFileSync(
        `${file}.br`,
        brotliCompressSync(content, {
          params: {
            [constants.BROTLI_PARAM_QUALITY]: constants.BROTLI_MAX_QUALITY,
            [constants.BROTLI_PARAM_MODE]: constants.BROTLI_MODE_TEXT,
            [constants.BROTLI_PARAM_SIZE_HINT]: content.length,
          },
        }),
      );
    }
    rmSync(file);
    done.push(path);
  }
  return done;
}

if (import.meta.main) {
  const dir = process.argv[2];
  if (dir === undefined) throw new Error("usage: compress.ts <build directory>");
  compressBuild(dir);
}
