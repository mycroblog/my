// SPDX-License-Identifier: AGPL-3.0-only
// Checks small builds as `web:build-check` checks the app's, and proves that the check passes a sound build and names
// what an unsound one breaks: a font without its license notice, and Lexical in a chunk an entry loads with the page.

import { afterEach, beforeEach, expect, test } from "bun:test";
import { mkdirSync, mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { buildProblems } from "./build-check";

const ofl = "Copyright 2022 The Noto Project Authors\n\nSIL OPEN FONT LICENSE Version 1.1 - 26 February 2007\n";

let dir = "";
const write = (path: string, content: string) => {
  writeFileSync(join(dir, path), content);
};

// A sound build: a font of each family with its notice, and an entry whose static imports hold no Lexical, which a
// chunk the entry imports dynamically holds.
beforeEach(() => {
  dir = mkdtempSync(join(tmpdir(), "build-check-"));
  for (const sub of [".vite", "assets", "licenses"]) mkdirSync(join(dir, sub));
  for (const font of [
    "noto-emoji-colrv1-flags-AAAA.woff2",
    "noto-sans-cjk-jp-kana-BBBB.woff2",
    "noto-sans-latin-CCCC.woff2",
  ]) {
    write(`assets/${font}`, "");
  }
  for (const license of ["OFL.txt", "OFL-NotoSansCJK.txt", "OFL-NotoSans.txt"]) write(`licenses/${license}`, ofl);
  write("assets/index.js", 'import "./shared.js"; import("./editor.js");');
  write("assets/shared.js", "export const answer = 42;");
  write("assets/editor.js", 'root.setAttribute("data-lexical-editor", "true");');
  write(
    ".vite/manifest.json",
    JSON.stringify({
      "index.html": { file: "assets/index.js", isEntry: true, imports: ["_shared.js"], dynamicImports: ["_editor.js"] },
      "_shared.js": { file: "assets/shared.js" },
      "_editor.js": { file: "assets/editor.js", isDynamicEntry: true, imports: ["_shared.js"] },
    }),
  );
});
afterEach(() => {
  rmSync(dir, { recursive: true, force: true });
});

test("a build with the license of each font and Lexical in a chunk of its own is sound", () => {
  expect(buildProblems(dir)).toEqual([]);
});

test("a font whose license notice is missing, is another text, or is not named is a problem", () => {
  rmSync(join(dir, "licenses/OFL-NotoSansCJK.txt"));
  write("licenses/OFL.txt", "UNICODE LICENSE V3\n");
  write("assets/inter-latin-DDDD.woff2", "");
  expect(buildProblems(dir)).toEqual([
    "assets/inter-latin-DDDD.woff2 is a font with no license notice: name its notice in fontLicenses",
    'licenses/OFL.txt is not the "SIL OPEN FONT LICENSE Version 1.1"',
    "licenses/OFL-NotoSansCJK.txt is missing: the build ships its font, so vite.config.ts must list it",
  ]);
});

test("a build without a font needs no font license", () => {
  rmSync(join(dir, "assets/noto-sans-cjk-jp-kana-BBBB.woff2"));
  rmSync(join(dir, "licenses/OFL-NotoSansCJK.txt"));
  expect(buildProblems(dir)).toEqual([]);
});

test("Lexical in a chunk an entry imports statically is a problem", () => {
  write("assets/shared.js", 'root.setAttribute("data-lexical-editor", "true");');
  expect(buildProblems(dir)).toEqual([
    "assets/shared.js, which index.html loads with the page, holds Lexical: import the editor lazily",
  ]);
});

test("a build in which no chunk holds Lexical's marker is a problem: the check would pass whatever the chunks hold", () => {
  write("assets/editor.js", "export const editor = 1;");
  expect(buildProblems(dir)).toEqual([
    'no chunk holds "data-lexical-editor": the check of the editor\'s chunk reads a marker Lexical no longer has',
  ]);
});
