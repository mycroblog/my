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
import { basename, dirname, join, relative, sep } from "node:path";

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

/**
 * The namespaces of the web packages that hold a catalog of their own (ADR-0009 §1, §9), each mapped to the package's
 * directory: web/<package>/locales/<locale>/<area>.json, named after the package without its hyphens
 * (web/design-system: designsystem). TS formats them with Lingui (@my/i18n compiles them); Go neither formats nor
 * embeds them, and their pseudo-locales are made when they are compiled, never committed (ADR-0081).
 */
export const tsNamespaces: ReadonlyMap<string, string> = new Map(
  [...new Bun.Glob("web/*/locales").scanSync({ cwd: repoRoot, onlyFiles: false })].sort().map((path) => {
    const pkg = join(repoRoot, dirname(path));
    return [basename(pkg).replaceAll("-", ""), pkg] as const;
  }),
);
export const isTsNamespace = (ns: string): boolean => tsNamespaces.has(ns);

/** The directory of the catalog files of `locale` that holds the namespace `ns`; removing files stops at it. */
export const localeDir = (locale: string, ns: string): string => {
  const pkg = tsNamespaces.get(ns);
  return pkg ? join(pkg, "locales", locale) : join(localesDir, locale);
};

/** One area's catalog file: locales/<locale>/<namespace>/<area>.json, or web/<package>/locales/<locale>/<area>.json. */
export const areaFile = (locale: string, ns: string, area: string): string =>
  join(localeDir(locale, ns), isTsNamespace(ns) ? "" : ns, `${area}.json`);

/** The path of the catalog file that holds `key`. */
export const catalogFile = (locale: string, ns: string, key: string): string => areaFile(locale, ns, areaOf(key));

/** One catalog file: locales/<locale>/<namespace>/<area>.json, or web/<package>/locales/<locale>/<area>.json. */
export type CatalogFile = { path: string; locale: string; ns: string; area: string; entries: Namespace };

/** The locale directories under `dir`. */
const localeNames = (dir: string): string[] =>
  readdirSync(dir, { withFileTypes: true })
    .filter((d) => d.isDirectory())
    .map((d) => d.name);

/** The JSON files under `dir`, each as its path segments relative to it, without ".json". */
const jsonFiles = (dir: string): string[][] =>
  readdirSync(dir, { recursive: true, encoding: "utf8" })
    .sort()
    .filter((file) => file.endsWith(".json"))
    .map((file) => file.slice(0, -".json".length).split(sep));

/**
 * Reads every catalog file, those of locales/ and those of the web packages. A JSON file directly in a locale
 * directory of locales/, outside a namespace directory, comes back with an empty namespace, for check.ts to report.
 */
export function loadCatalogFiles(): CatalogFile[] {
  const files: CatalogFile[] = [];
  const read = (path: string): Namespace => JSON.parse(readFileSync(path, "utf8"));
  for (const locale of localeNames(localesDir)) {
    for (const [ns = "", ...area] of jsonFiles(join(localesDir, locale))) {
      const path = `${join(localesDir, locale, ns, ...area)}.json`;
      files.push({ path, locale, ns: area.length ? ns : "", area: area.join("/"), entries: read(path) });
    }
  }
  for (const [ns, pkg] of tsNamespaces) {
    for (const locale of localeNames(join(pkg, "locales"))) {
      for (const area of jsonFiles(join(pkg, "locales", locale))) {
        const path = `${join(pkg, "locales", locale, ...area)}.json`;
        files.push({ path, locale, ns, area: area.join("/"), entries: read(path) });
      }
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

/** Removes a catalog file of the namespace `ns` and the directories it leaves empty, up to the locale's directory. */
export function removeCatalogFile(path: string, locale: string, ns: string): void {
  rmSync(path);
  const stop = localeDir(locale, ns);
  for (let dir = dirname(path); dir !== stop && readdirSync(dir).length === 0; dir = dirname(dir)) {
    rmdirSync(dir);
  }
}
