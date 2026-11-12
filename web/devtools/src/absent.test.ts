// SPDX-License-Identifier: AGPL-3.0-only

import { afterEach, beforeEach, expect, test } from "bun:test";
import { mkdirSync, mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { findDevtools } from "./absent";

let dir = "";
beforeEach(() => {
  dir = mkdtempSync(join(tmpdir(), "absent-"));
  mkdirSync(join(dir, "assets"));
});
afterEach(() => {
  rmSync(dir, { recursive: true, force: true });
});

test("findDevtools finds the files that contain the overlay", () => {
  writeFileSync(join(dir, "index.html"), "<script src=/assets/index.js></script>");
  writeFileSync(join(dir, "assets", "index.js"), "console.log(1)");
  expect(findDevtools([dir])).toEqual([]);
  writeFileSync(join(dir, "assets", "overlay.js"), 'fetch("/__my-devtools/batches")');
  expect(findDevtools([dir]).map((f) => f.endsWith(join("assets", "overlay.js")))).toEqual([true]);
});

test("findDevtools refuses an empty build", () => {
  expect(() => findDevtools([join(dir, "assets")])).toThrow("holds no build");
});
