// SPDX-License-Identifier: AGPL-3.0-only
// Catalog loading and the repository paths shared by the i18n scripts.
import {
  existsSync,
  mkdirSync,
  readdirSync,
  readFileSync,
  renameSync,
  rmdirSync,
  rmSync,
  writeFileSync,
} from "node:fs";
import { dirname, join, relative, sep } from "node:path";

/** tools/i18n, where the glossary of the tool lives. */
export const toolDir = join(import.meta.dir, "..");
/** The repository the scripts read and write. I18N_TOOL_ROOT points the tests of the tool at a temporary one. */
export const repoRoot = process.env.I18N_TOOL_ROOT || join(toolDir, "..", "..");
export const localesDir = join(repoRoot, "locales");
/** Where the Go package and its parity fixtures live. */
export const goPackageDir = join(repoRoot, "internal", "i18n");
export const goTestdataDir = join(goPackageDir, "testdata");

export const rel = (path: string): string => relative(repoRoot, path);

/** The locale every catalog is written in, and the last fallback. */
export const SOURCE = "en";
/** Pseudo-locales generated from SOURCE (generate.ts); never edited or translated by hand. */
const PSEUDO_LOCALES = ["en-XA", "ar-XB"] as const;
export const isPseudo = (locale: string): boolean => (PSEUDO_LOCALES as readonly string[]).includes(locale);
/** The locales written in the same change as SOURCE: a key they lack is an error. The others catch up later (ADR-0009 §8). */
export const REQUIRED_LOCALES: readonly string[] = ["ja"];

/** One catalog entry, in the FormatJS JSON shape that Weblate reads natively. */
export type Entry = { defaultMessage: string; description?: string };
export type Namespace = Record<string, Entry>;
/** locale -> namespace -> key -> entry */
export type Catalog = Record<string, Record<string, Namespace>>;

/**
 * The file of a key inside its namespace directory, without ".json": the key's first segment, or its first two
 * under "admin" (ADR-0081). `settings.profile.title` goes in settings.json, `admin.report.title` in admin/report.json.
 */
export const areaOf = (key: string): string => {
  const segments = key.split(".");
  return segments.slice(0, segments[0] === "admin" ? 2 : 1).join("/");
};

/** The path of the catalog file that holds `key`: locales/<locale>/<namespace>/<area>.json. */
export const catalogFile = (locale: string, ns: string, key: string): string =>
  join(localesDir, locale, ns, `${areaOf(key)}.json`);

/** One catalog file: locales/<locale>/<namespace>/<area>.json. */
export type CatalogFile = { path: string; locale: string; ns: string; area: string; entries: Namespace };

/**
 * Reads every catalog file. A JSON file directly in a locale directory, outside a namespace directory,
 * comes back with an empty namespace, for check.ts to report.
 */
export function loadCatalogFiles(): CatalogFile[] {
  const files: CatalogFile[] = [];
  for (const locale of readdirSync(localesDir, { withFileTypes: true })) {
    if (!locale.isDirectory()) continue;
    for (const file of readdirSync(join(localesDir, locale.name), { recursive: true, encoding: "utf8" }).sort()) {
      if (!file.endsWith(".json")) continue;
      const path = join(localesDir, locale.name, file);
      const [ns = "", ...area] = file.slice(0, -".json".length).split(sep);
      files.push({
        path,
        locale: locale.name,
        ns: area.length ? ns : "",
        area: area.join("/"),
        entries: JSON.parse(readFileSync(path, "utf8")),
      });
    }
  }
  return files;
}

/** Reads every locales/<locale>/<namespace>/<area>.json file into one catalog. */
export function loadCatalog(files: CatalogFile[] = loadCatalogFiles()): Catalog {
  const catalog: Catalog = {};
  for (const { locale, ns, entries } of files) {
    catalog[locale] ??= {};
    if (ns) Object.assign((catalog[locale][ns] ??= {}), entries);
  }
  return catalog;
}

/**
 * Pseudo-locales keep the source locale's plural categories, so both runtimes format them
 * with its CLDR rules (Go: formattingLocale in internal/i18n).
 */
export const formattingLocale = (locale: string): string => (isPseudo(locale) ? SOURCE : locale);

/** Serializes JSON the way every generated and catalog file is written. */
export const toJSON = (value: unknown): string => `${JSON.stringify(value, null, 2)}\n`;

/** Writes `content` to `path` through a rename, creating its directory; false when the file already holds it. */
export function writeFileAtomic(path: string, content: string): boolean {
  if (existsSync(path) && readFileSync(path, "utf8") === content) return false;
  mkdirSync(dirname(path), { recursive: true });
  writeFileSync(`${path}.tmp`, content);
  renameSync(`${path}.tmp`, path);
  return true;
}

/** Removes a catalog file and the directories it leaves empty, up to the locale's directory. */
export function removeCatalogFile(path: string, locale: string): void {
  rmSync(path);
  for (let dir = dirname(path); dir !== join(localesDir, locale) && readdirSync(dir).length === 0; dir = dirname(dir)) {
    rmdirSync(dir);
  }
}
