// SPDX-License-Identifier: AGPL-3.0-only
// The catalogs as cli.ts edits them: every write goes to the file of the key's area, a new key of the source locale
// lands next to its siblings, and the files of the other locales keep the key order of the source file.
import { existsSync, readdirSync } from "node:fs";
import { join } from "node:path";
import {
  areaOf,
  type CatalogFile,
  catalogFile,
  type Entry,
  isPseudo,
  loadCatalogFiles,
  localesDir,
  type Namespace,
  rel,
  removeCatalogFile,
  SOURCE,
  toJSON,
  writeFileAtomic,
} from "./catalog";

export const splitId = (id: string): [ns: string, key: string] => {
  const [ns = "", key = ""] = id.split(/:(.*)/s);
  return [ns, key];
};

/** Where a new key goes in a file: after the last key that shares the longest run of leading segments with it. */
function siblingOf(keys: string[], key: string): string | undefined {
  const segments = key.split(".");
  for (let depth = segments.length - 1; depth > 0; depth--) {
    const prefix = `${segments.slice(0, depth).join(".")}.`;
    const sibling = keys.findLast((k) => k.startsWith(prefix));
    if (sibling) return sibling;
  }
}

export class Store {
  /** The locales written by hand or by a translation run, the source first. */
  readonly locales: string[];
  private readonly files = new Map<string, CatalogFile>();
  private readonly dirty = new Set<string>();

  constructor() {
    this.locales = readdirSync(localesDir, { withFileTypes: true })
      .filter((d) => d.isDirectory() && !isPseudo(d.name))
      .map((d) => d.name)
      .sort((a, b) => Number(b === SOURCE) - Number(a === SOURCE) || a.localeCompare(b));
    for (const file of loadCatalogFiles()) {
      if (!isPseudo(file.locale) && file.ns) this.files.set(file.path, file);
    }
  }

  private file(locale: string, ns: string, key: string): CatalogFile {
    const path = catalogFile(locale, ns, key);
    let file = this.files.get(path);
    if (!file) {
      file = { path, locale, ns, area: areaOf(key), entries: {} };
      this.files.set(path, file);
    }
    return file;
  }

  namespaces(): string[] {
    return readdirSync(join(localesDir, SOURCE), { withFileTypes: true })
      .filter((d) => d.isDirectory())
      .map((d) => d.name);
  }

  get(locale: string, ns: string, key: string): Entry | undefined {
    return this.files.get(catalogFile(locale, ns, key))?.entries[key];
  }

  /** The locales that hold the key, the source first. */
  localesWith(ns: string, key: string): string[] {
    return this.locales.filter((l) => this.get(l, ns, key));
  }

  /** The ids of the source locale. */
  ids(): string[] {
    return [...this.files.values()]
      .filter((f) => f.locale === SOURCE)
      .flatMap((f) => Object.keys(f.entries).map((key) => `${f.ns}:${key}`));
  }

  /**
   * Writes the entry. A new key of the source locale goes after `after`, which must be in the same file, or next to
   * its siblings; throws when `after` is elsewhere.
   */
  set(locale: string, ns: string, key: string, entry: Entry, after?: string): void {
    const file = this.file(locale, ns, key);
    this.dirty.add(file.path);
    if (key in file.entries || locale !== SOURCE) {
      file.entries[key] = entry;
      return;
    }
    const keys = Object.keys(file.entries);
    if (after !== undefined && !keys.includes(after))
      throw new Error(
        `--after ${after}: not a key of ${rel(file.path)}, where ${ns}:${key} belongs (<namespace>/<first segment>.json, <namespace>/admin/<second segment>.json)`,
      );
    const previous = after ?? siblingOf(keys, key) ?? keys.at(-1);
    const next: Namespace = {};
    if (previous === undefined) next[key] = entry;
    for (const [k, e] of Object.entries(file.entries)) {
      next[k] = e;
      if (k === previous) next[key] = entry;
    }
    file.entries = next;
  }

  delete(locale: string, ns: string, key: string): boolean {
    const file = this.files.get(catalogFile(locale, ns, key));
    if (!file || !(key in file.entries)) return false;
    delete file.entries[key];
    this.dirty.add(file.path);
    return true;
  }

  /** Renames the key in every locale; inside one file it keeps its place. */
  rename(ns: string, from: string, to: string): void {
    for (const locale of this.locales) {
      const entry = this.get(locale, ns, from);
      if (!entry) continue;
      const file = this.file(locale, ns, from);
      if (areaOf(from) === areaOf(to)) {
        file.entries = Object.fromEntries(Object.entries(file.entries).map(([k, e]) => [k === from ? to : k, e]));
        this.dirty.add(file.path);
      } else {
        this.delete(locale, ns, from);
        this.set(locale, ns, to, entry);
      }
    }
  }

  /** Writes the changed files, removes the emptied ones, and returns what it did, one line per file. */
  save(): string[] {
    const out: string[] = [];
    for (const path of [...this.dirty].sort()) {
      const file = this.files.get(path);
      if (!file) continue;
      if (Object.keys(file.entries).length === 0) {
        this.files.delete(path);
        if (existsSync(path)) {
          removeCatalogFile(path, file.locale);
          out.push(`removed ${rel(path)}`);
        }
        continue;
      }
      if (file.locale !== SOURCE) {
        // The order of the source file; a key the source lacks stays behind the others.
        const order = Object.keys(
          this.files.get(join(localesDir, SOURCE, file.ns, `${file.area}.json`))?.entries ?? {},
        );
        const ranks = new Map(order.map((key, i) => [key, i]));
        const rank = (key: string) => ranks.get(key) ?? order.length;
        file.entries = Object.fromEntries(Object.entries(file.entries).sort(([a], [b]) => rank(a) - rank(b)));
      }
      if (writeFileAtomic(path, toJSON(file.entries))) out.push(`wrote ${rel(path)}`);
    }
    this.dirty.clear();
    return out;
  }
}
