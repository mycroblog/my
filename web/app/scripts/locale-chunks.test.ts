// SPDX-License-Identifier: AGPL-3.0-only
// Checks small manifests as `web:build-check` checks the app's for the strings of each locale (#3094), and proves that
// the check passes a sound build and names what an unsound one breaks: a page that loads another locale's strings with
// it, a locale with no chunk loaded on demand, and a locale's chunk that brings a third locale's.

import { afterEach, beforeEach, expect, test } from "bun:test";
import { mkdirSync, mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { localeChunkProblems, localeModules } from "./locale-chunks";

type Chunk = { file: string; name?: string; src?: string; isEntry?: boolean; isDynamicEntry?: boolean };
type Manifest = Record<string, Chunk & { imports?: string[]; dynamicImports?: string[] }>;

const modules = ["src/messages.ja.gen.ts", "../design-system/src/messages.ja.gen.ts", "src/messages.ar-XB.gen.ts"];

/** A locale's messages in a package, a chunk of its own loaded on demand that imports the locale's one chunk. */
const messages = (key: string, locale: string, hash: string): Manifest[string] => ({
  file: `assets/messages.${locale}.gen-${hash}.js`,
  name: `messages.${locale}.gen`,
  src: key,
  isDynamicEntry: true,
  imports: [`_locale-${locale}.js`],
});

/**
 * A sound build: index.html and auth.html load en's strings with them through a shared chunk, which imports each other
 * locale's messages lazily, and each of those imports its locale's one chunk.
 */
const sound = (): Manifest => ({
  "index.html": { file: "assets/index.js", src: "index.html", isEntry: true, imports: ["_strings.js"] },
  "auth.html": { file: "assets/auth.js", src: "auth.html", isEntry: true, imports: ["_strings.js"] },
  "_strings.js": {
    file: "assets/strings.js",
    name: "strings",
    imports: ["_locale-en.js"],
    dynamicImports: modules,
  },
  "_locale-en.js": { file: "assets/locale-en-AAAA.js", name: "locale-en" },
  "_locale-ja.js": { file: "assets/locale-ja-BBBB.js", name: "locale-ja" },
  "_locale-ar-XB.js": { file: "assets/locale-ar-XB-CCCC.js", name: "locale-ar-XB" },
  "src/messages.ja.gen.ts": messages("src/messages.ja.gen.ts", "ja", "1"),
  "../design-system/src/messages.ja.gen.ts": messages("../design-system/src/messages.ja.gen.ts", "ja", "2"),
  "src/messages.ar-XB.gen.ts": messages("src/messages.ar-XB.gen.ts", "ar-XB", "3"),
});

/** Changes the chunk `key` of `manifest` with `fields`. */
const change = (manifest: Manifest, key: string, fields: Partial<Manifest[string]>) => {
  const chunk = manifest[key];
  if (chunk === undefined) throw new Error(`test: the manifest has no ${key}`);
  manifest[key] = { ...chunk, ...fields };
};

let dir = "";
const build = (manifest: Manifest) => {
  writeFileSync(join(dir, ".vite/manifest.json"), JSON.stringify(manifest));
  return localeChunkProblems(dir, modules);
};

beforeEach(() => {
  dir = mkdtempSync(join(tmpdir(), "locale-chunks-"));
  mkdirSync(join(dir, ".vite"));
});
afterEach(() => {
  rmSync(dir, { recursive: true, force: true });
});

test("a build whose pages load en's strings alone, and each other locale's on demand, is sound", () => {
  expect(build(sound())).toEqual([]);
});

test("a page that loads another locale's strings with it is a problem, through any chunk of its static graph", () => {
  const manifest = sound();
  change(manifest, "_strings.js", { imports: ["_locale-en.js", "_locale-ja.js"] });
  change(manifest, "auth.html", { imports: ["_strings.js", "_locale-ar-XB.js"] });

  expect(build(manifest)).toEqual([
    "assets/locale-ja-BBBB.js, which index.html loads with the page, holds the strings of ja: load them on demand",
    "assets/locale-ja-BBBB.js, which auth.html loads with the page, holds the strings of ja: load them on demand",
    "assets/locale-ar-XB-CCCC.js, which auth.html loads with the page, holds the strings of ar-XB: load them on demand",
  ]);
});

test("a locale whose messages are not loaded on demand, or load no chunk of the locale, is a problem", () => {
  const manifest = sound();
  // The app's ja messages are bundled with a page; the design system's ar-XB load into a chunk of no locale.
  delete manifest["src/messages.ja.gen.ts"];
  change(manifest, "src/messages.ar-XB.gen.ts", { imports: [] });

  expect(build(manifest)).toEqual([
    "src/messages.ja.gen.ts is not loaded on demand: the build has no chunk that imports it lazily",
    "src/messages.ar-XB.gen.ts loads no chunk locale-ar-XB: one chunk holds the strings of ar-XB of every package",
  ]);
});

test("a locale's messages that bring a third locale's strings, or a locale in two chunks, are a problem", () => {
  const manifest = sound();
  change(manifest, "src/messages.ja.gen.ts", { imports: ["_locale-ja.js", "_locale-ar-XB.js", "_locale-en.js"] });
  change(manifest, "../design-system/src/messages.ja.gen.ts", { imports: ["_locale-ja-2.js"] });
  manifest["_locale-ja-2.js"] = { file: "assets/locale-ja-DDDD.js", name: "locale-ja" };

  expect(build(manifest)).toEqual([
    "src/messages.ja.gen.ts loads the strings of ar-XB with those of ja",
    "the strings of ja are in 2 chunks (assets/locale-ja-BBBB.js, assets/locale-ja-DDDD.js): one chunk holds them all",
  ]);
});

test("a build without one of the pages is a problem: the check would pass whatever the page loads", () => {
  const manifest = sound();
  delete manifest["auth.html"];

  expect(build(manifest)).toEqual(["auth.html is not in the build: the check cannot read what it loads"]);
});

test("the locales checked are each package's messages but en's, as the app's build names them", () => {
  const web = join(import.meta.dir, "../..");
  const found = localeModules(web);

  // The pseudo-locales and ja are in every package that has strings; en is read with the pages.
  for (const pkg of ["src", "../design-system/src", "../app-components/src", "../app-features/src"])
    for (const locale of ["ja", "en-XA", "ar-XB"]) expect(found).toContain(`${pkg}/messages.${locale}.gen.ts`);
  expect(found.filter((key) => key.endsWith("/messages.en.gen.ts"))).toEqual([]);
  expect(found.filter((key) => !/\/messages\.[\w-]+\.gen\.ts$/.test(key))).toEqual([]);
});
