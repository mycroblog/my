// SPDX-License-Identifier: AGPL-3.0-only
// Compresses a small build as `web:build` does, and proves that the build then holds each text file only in its
// compressed forms, which decompress to the file, that the files the server only reads keep a gzip form alone, and
// that fonts, images and the inventory of the payment entry's scripts stay as they are; then that etags.ts lists
// each served file once, under its own path, with its content's size and ETag and those of its compressed forms.

import { afterAll, expect, test } from "bun:test";
import { createHash } from "node:crypto";
import { existsSync, mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { brotliDecompressSync, gunzipSync } from "node:zlib";
import { compressBuild } from "./compress";
import { etagList } from "./etags";

const root = mkdtempSync(join(tmpdir(), "compress-test-"));
afterAll(() => {
  rmSync(root, { recursive: true, force: true });
});

/** The size and the server's ETag of a file of the build in `dir`. */
function tagged(dir: string, path: string) {
  const content = readFileSync(join(dir, path));
  return {
    size: content.length,
    etag: createHash("sha256").update(content).digest().subarray(0, 18).toString("base64url"),
  };
}

function build(name: string): string {
  const dir = join(root, name);
  mkdirSync(join(dir, ".vite"), { recursive: true });
  mkdirSync(join(dir, "assets"));
  writeFileSync(join(dir, ".vite/manifest.json"), "{}");
  writeFileSync(join(dir, ".vite/payment-scripts.json"), '{"entry":"pay.html"}');
  writeFileSync(join(dir, "index.html"), "vite");
  writeFileSync(join(dir, "sw.js"), "hello\n");
  writeFileSync(join(dir, "assets/app.css"), "body{}".repeat(100));
  writeFileSync(join(dir, "assets/font.woff2"), "wOF2");
  writeFileSync(join(dir, "icon-192.png"), "PNG");
  return dir;
}

test("each text file is replaced by its Brotli and gzip forms, and each file only read by its gzip form", () => {
  const dir = build("forms");
  expect(compressBuild(dir)).toEqual([".vite/manifest.json", "assets/app.css", "index.html", "sw.js"]);

  for (const path of ["sw.js", "assets/app.css"]) {
    expect(existsSync(join(dir, path))).toBe(false);
  }
  expect(brotliDecompressSync(readFileSync(join(dir, "sw.js.br"))).toString()).toBe("hello\n");
  expect(gunzipSync(readFileSync(join(dir, "sw.js.gz"))).toString()).toBe("hello\n");
  expect(gunzipSync(readFileSync(join(dir, "assets/app.css.gz"))).toString()).toBe("body{}".repeat(100));

  expect(existsSync(join(dir, ".vite/manifest.json"))).toBe(false);
  expect(gunzipSync(readFileSync(join(dir, ".vite/manifest.json.gz"))).toString()).toBe("{}");
  expect(existsSync(join(dir, ".vite/manifest.json.br"))).toBe(false);
  expect(existsSync(join(dir, "index.html.gz"))).toBe(true);
  expect(existsSync(join(dir, "index.html.br"))).toBe(false);

  expect(readFileSync(join(dir, ".vite/payment-scripts.json"), "utf8")).toBe('{"entry":"pay.html"}');
  expect(readFileSync(join(dir, "assets/font.woff2"), "utf8")).toBe("wOF2");
  expect(readFileSync(join(dir, "icon-192.png"), "utf8")).toBe("PNG");
});

test("the same build compresses to the same bytes", () => {
  const a = build("same-a");
  const b = build("same-b");
  compressBuild(a);
  compressBuild(b);
  for (const path of ["sw.js.br", "sw.js.gz", "assets/app.css.br", "assets/app.css.gz"]) {
    expect(readFileSync(join(a, path))).toEqual(readFileSync(join(b, path)));
  }
});

test("a build that already holds a compressed file is refused", () => {
  const dir = build("refused");
  writeFileSync(join(dir, "assets/data.gz"), "x");
  expect(() => compressBuild(dir)).toThrow("assets/data.gz");
});

test("the list names a compressed file under its own path, with its content and its forms", () => {
  const dir = build("list");
  compressBuild(dir);
  const list = etagList(dir);
  expect(Object.keys(list)).toEqual(["assets/app.css", "assets/font.woff2", "icon-192.png", "sw.js"]);
  expect(list["sw.js"]).toEqual({
    size: 6,
    etag: "WJG1tSLV3whtD_CxEPvZ0hu0",
    encodings: {
      br: tagged(dir, "sw.js.br"),
      gzip: tagged(dir, "sw.js.gz"),
    },
  });
  expect(list["assets/font.woff2"]).toEqual(tagged(dir, "assets/font.woff2"));
});
