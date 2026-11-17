// SPDX-License-Identifier: AGPL-3.0-only
// Lists a small build as `web:build` would, and proves that the list names the files the Go server serves, each with
// its size and the ETag the server computes for the same content: TestListETags of internal/interface/web pins the
// same build to the same values.

import { afterAll, expect, test } from "bun:test";
import { mkdirSync, mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { etagList } from "./etags";

const root = mkdtempSync(join(tmpdir(), "etags-test-"));
afterAll(() => {
  rmSync(root, { recursive: true, force: true });
});

test("the list names each served file with its size and the server's ETag", () => {
  mkdirSync(join(root, ".vite"));
  mkdirSync(join(root, "assets"));
  writeFileSync(join(root, ".vite/manifest.json"), "{}");
  writeFileSync(join(root, ".hidden"), "x");
  writeFileSync(join(root, "index.html"), "vite");
  writeFileSync(join(root, "auth.html"), "vite");
  writeFileSync(join(root, "sw.js"), "hello\n");
  writeFileSync(join(root, "assets/empty.css"), "");
  writeFileSync(join(root, "assets/index.html"), "hello\n");

  expect(etagList(root)).toEqual({
    "assets/empty.css": { size: 0, etag: "47DEQpj8HBSa-_TImW-5JCeu" },
    "assets/index.html": { size: 6, etag: "WJG1tSLV3whtD_CxEPvZ0hu0" },
    "sw.js": { size: 6, etag: "WJG1tSLV3whtD_CxEPvZ0hu0" },
  });
  expect(Object.keys(etagList(root))).toEqual(["assets/empty.css", "assets/index.html", "sw.js"]);
});
