// SPDX-License-Identifier: AGPL-3.0-only

import { afterEach, beforeEach, expect, test } from "bun:test";
import { mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { deal } from "./dom-tests";

const files = (count: number) => Array.from({ length: count }, (_, i) => `f${i}`);

test("deal gives every file to one list, in turn, four or more to a list", () => {
  expect(deal(files(9), 2)).toEqual([
    ["f0", "f2", "f4", "f6", "f8"],
    ["f1", "f3", "f5", "f7"],
  ]);
  expect(deal(files(9), 24).map((list) => list.length)).toEqual([5, 4]);
});

test("deal makes one list of a few files, and none of none", () => {
  expect(deal(files(3), 4)).toEqual([["f0", "f1", "f2"]]);
  expect(deal([], 4)).toEqual([]);
});

let dir = "";
beforeEach(() => {
  dir = mkdtempSync(join(tmpdir(), "dom-tests-"));
});
afterEach(() => {
  rmSync(dir, { recursive: true, force: true });
});

const run = (dom: string) =>
  Bun.spawnSync([process.execPath, join(import.meta.dir, "dom-tests.ts"), dom], {
    cwd: dir,
    stdout: "pipe",
    stderr: "pipe",
  });
const passing =
  'import { expect, test } from "bun:test";\ntest("a document", () => expect(document.body).toBeDefined());\n';
const failing = 'import { expect, test } from "bun:test";\ntest("fails", () => expect(1).toBe(2));\n';

test("it runs every file of the DOM on its preload, and no other test file", () => {
  writeFileSync(join(dir, "a.dom.test.tsx"), passing);
  writeFileSync(join(dir, "b.dom.test.tsx"), passing);
  writeFileSync(join(dir, "c.jsdom.test.tsx"), failing);
  writeFileSync(join(dir, "d.test.tsx"), failing);
  const result = run("happy-dom");
  const passed = [...result.stderr.toString().matchAll(/(\d+) pass/g)].map((m) => Number(m[1]));
  expect(passed.reduce((sum, n) => sum + n, 0)).toBe(2);
  expect(result.exitCode).toBe(0);
});

test("it fails when a file fails", () => {
  writeFileSync(join(dir, "a.dom.test.tsx"), passing);
  writeFileSync(join(dir, "b.dom.test.tsx"), failing);
  expect(run("happy-dom").exitCode).toBe(1);
});

test("it passes in a package with no test of the DOM, and refuses a DOM it does not know", () => {
  expect(run("jsdom").exitCode).toBe(0);
  expect(run("browser").exitCode).toBe(2);
});
