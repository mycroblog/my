// SPDX-License-Identifier: AGPL-3.0-only
// Checks a production build of web/app, as `web:build-check` runs it, for the strings of each locale (#3094, ADR-0009):
// the pages load en's with them and no other locale's, and each other locale is one chunk, `locale-<tag>`, that holds
// the strings of every package in it and loads on demand, with the strings of no third locale.
//
//   bun web/app/scripts/locale-chunks.ts <the build's directory>

import { readdirSync, readFileSync } from "node:fs";
import { join } from "node:path";

type Chunk = { file: string; name?: string; isDynamicEntry?: boolean; imports?: string[] };
type Manifest = Record<string, Chunk>;

/** The pages whose static graph must hold no locale's strings but en's. */
const pages = ["index.html", "auth.html"] as const;

/** The locale whose strings every page reads with the catalog. */
const fallback = "en";

/** The packages whose strings the app ships, by their directory beside web/app. */
const packages = ["app", "design-system", "app-components", "app-features"] as const;

const messagesFile = /^messages\.(.+)\.gen\.ts$/;
const localeChunk = /^locale-(.+)$/;

/** The locale whose strings `chunk` holds, when it is a locale's chunk. */
const localeOf = (chunk: Chunk | undefined): string | undefined => localeChunk.exec(chunk?.name ?? "")?.[1];

/** The keys of the chunks `key` loads with it, itself among them. */
function staticGraph(manifest: Manifest, key: string): Set<string> {
  const seen = new Set<string>();
  const visit = (at: string) => {
    if (seen.has(at)) return;
    seen.add(at);
    for (const next of manifest[at]?.imports ?? []) visit(next);
  };
  visit(key);
  return seen;
}

/**
 * The manifest keys of the messages of each package in a locale other than en (`src/messages.ja.gen.ts`,
 * `../design-system/src/messages.ja.gen.ts`), as they are in `web`.
 */
export function localeModules(web: string): string[] {
  return packages.flatMap((pkg) =>
    readdirSync(join(web, pkg, "src"))
      .filter((file) => ![undefined, fallback].includes(messagesFile.exec(file)?.[1]))
      .toSorted()
      .map((file) => (pkg === "app" ? `src/${file}` : `../${pkg}/src/${file}`)),
  );
}

/** What the build in `dir` does wrong with the strings of the locales of `modules` (localeModules). */
export function localeChunkProblems(dir: string, modules: readonly string[]): string[] {
  const manifest = JSON.parse(readFileSync(join(dir, ".vite/manifest.json"), "utf8")) as Manifest;
  const problems: string[] = [];

  for (const page of pages) {
    if (manifest[page] === undefined) {
      problems.push(`${page} is not in the build: the check cannot read what it loads`);
      continue;
    }
    for (const key of staticGraph(manifest, page)) {
      const locale = localeOf(manifest[key]);
      if (locale !== undefined && locale !== fallback)
        problems.push(
          `${manifest[key]?.file}, which ${page} loads with the page, holds the strings of ${locale}: load them on demand`,
        );
    }
  }

  const locales = new Set<string>();
  for (const key of modules) {
    const locale = messagesFile.exec(key.split("/").at(-1) ?? "")?.[1] ?? "";
    locales.add(locale);
    const chunk = manifest[key];
    if (chunk?.isDynamicEntry !== true) {
      problems.push(`${key} is not loaded on demand: the build has no chunk that imports it lazily`);
      continue;
    }
    const loaded = [...staticGraph(manifest, key)].map((at) => localeOf(manifest[at]));
    if (!loaded.includes(locale))
      problems.push(
        `${key} loads no chunk locale-${locale}: one chunk holds the strings of ${locale} of every package`,
      );
    for (const other of new Set(loaded))
      if (other !== undefined && other !== locale && other !== fallback)
        problems.push(`${key} loads the strings of ${other} with those of ${locale}`);
  }

  for (const locale of locales) {
    const chunks = Object.values(manifest).filter((chunk) => localeOf(chunk) === locale);
    if (chunks.length > 1)
      problems.push(
        `the strings of ${locale} are in ${chunks.length} chunks (${chunks.map((chunk) => chunk.file).join(", ")}): one chunk holds them all`,
      );
  }

  return problems;
}

if (import.meta.main) {
  const dir = process.argv[2];
  if (dir === undefined) {
    console.error("usage: bun web/app/scripts/locale-chunks.ts <the build's directory>");
    process.exit(2);
  }
  const problems = localeChunkProblems(dir, localeModules(join(import.meta.dir, "../..")));
  if (problems.length > 0) {
    for (const problem of problems) console.error(problem);
    process.exit(1);
  }
  console.log(`the pages of the build in ${dir} load the strings of en alone, and each other locale's on demand`);
}
