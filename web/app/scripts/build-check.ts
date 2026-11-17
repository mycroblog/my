// SPDX-License-Identifier: AGPL-3.0-only
// Fails when a production build of the app breaks what its files alone show (ADR-0171 §2):
//   - each font it ships has its license notice in licenses/ (ADR-0091 §13, ADR-0009 §5), which the Go server serves
//     at /licenses/;
//   - no chunk that an entry loads with the page holds Lexical, the composer's editor (ADR-0092 §4): it is a chunk of
//     its own, which the composer fetches when it opens (@my/app-features, compose/chunk.tsx).
// Usage: bun web/app/scripts/build-check.ts <build directory>
import { existsSync, readdirSync, readFileSync } from "node:fs";
import { join } from "node:path";

/** The license notice of each font the build ships, by the start of its files' names: the first that matches. */
const fontLicenses: readonly (readonly [prefix: string, license: string])[] = [
  ["noto-emoji-", "OFL.txt"],
  ["noto-sans-cjk-", "OFL-NotoSansCJK.txt"],
  ["noto-sans-", "OFL-NotoSans.txt"],
];

/** The first line of the SIL Open Font License, which every notice of `fontLicenses` is. */
const oflTitle = "SIL OPEN FONT LICENSE Version 1.1";

const fontFile = /\.(?:woff2?|ttf|otf)$/;

/** The attribute Lexical sets on an editor's root: its code names it, and no other code of the app does. */
const lexicalMarker = "data-lexical-editor";

/** A chunk of Vite's manifest (https://vite.dev/guide/backend-integration). */
type Chunk = { file: string; isEntry?: boolean; imports?: string[] };

/** The fonts of the build in `dir` with no license notice, and the notices that are missing or not the license. */
function fontLicenseProblems(dir: string): string[] {
  const problems: string[] = [];
  const needed = new Set<string>();
  for (const name of readdirSync(join(dir, "assets"))) {
    if (!fontFile.test(name)) continue;
    const license = fontLicenses.find(([prefix]) => name.startsWith(prefix))?.[1];
    if (license === undefined) {
      problems.push(`assets/${name} is a font with no license notice: name its notice in fontLicenses`);
    } else {
      needed.add(license);
    }
  }
  for (const license of needed) {
    const path = join(dir, "licenses", license);
    if (!existsSync(path)) {
      problems.push(`licenses/${license} is missing: the build ships its font, so vite.config.ts must list it`);
    } else if (!readFileSync(path, "utf8").includes(oflTitle)) {
      problems.push(`licenses/${license} is not the "${oflTitle}"`);
    }
  }
  return problems;
}

/** The chunks that an entry of the build in `dir` loads with the page and that hold Lexical. */
function eagerLexicalProblems(dir: string): string[] {
  const manifest = JSON.parse(readFileSync(join(dir, ".vite/manifest.json"), "utf8")) as Record<string, Chunk>;
  const holdsLexical = (chunk: Chunk) => readFileSync(join(dir, chunk.file), "latin1").includes(lexicalMarker);
  const entries = Object.entries(manifest).filter(([, chunk]) => chunk.isEntry === true);
  if (entries.length === 0) return [".vite/manifest.json names no entry"];

  const problems: string[] = [];
  for (const [key, entry] of entries) {
    // The entry and its static imports, which the server's shell preloads (internal/interface/web, entryOf).
    const eager = new Map<string, Chunk>([[key, entry]]);
    const visit = (keys: readonly string[] = []) => {
      for (const k of keys) {
        const chunk = manifest[k];
        if (chunk === undefined || eager.has(k)) continue;
        eager.set(k, chunk);
        visit(chunk.imports);
      }
    };
    visit(entry.imports);
    for (const chunk of eager.values()) {
      if (holdsLexical(chunk)) {
        problems.push(`${chunk.file}, which ${key} loads with the page, holds Lexical: import the editor lazily`);
      }
    }
  }
  if (!Object.values(manifest).some((chunk) => chunk.file.endsWith(".js") && holdsLexical(chunk))) {
    problems.push(
      `no chunk holds "${lexicalMarker}": the check of the editor's chunk reads a marker Lexical no longer has`,
    );
  }
  return problems;
}

/** What the build in `dir` breaks, one line each; none when it is sound. */
export function buildProblems(dir: string): string[] {
  return [...fontLicenseProblems(dir), ...eagerLexicalProblems(dir)];
}

if (import.meta.main) {
  const dir = process.argv[2];
  if (dir === undefined) throw new Error("usage: build-check.ts <build directory>");
  const problems = buildProblems(dir);
  if (problems.length > 0) {
    console.error(`the build in ${dir} is not sound:`);
    for (const problem of problems) console.error(`  ${problem}`);
    process.exit(1);
  }
  console.log(`the build in ${dir} carries the license of each font and keeps Lexical out of the entries`);
}
