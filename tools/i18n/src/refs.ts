// SPDX-License-Identifier: AGPL-3.0-only
// Where the source refers to catalog keys: a quoted key literal ("server:home.title"), or the Go accessor of a key
// (`.ServerHomeTitle(`, a method expression, a field chain in an html/template file, and the `ServerHomeTitleArgs`
// struct). check.ts reports the keys nothing refers to; cli.ts lists and rewrites the references.
import { existsSync, readFileSync } from "node:fs";
import { join } from "node:path";
import { repoRoot, writeFileAtomic } from "./catalog";
import { goName } from "./generate";

export type Ref = { file: string; line: number; text: string };

type Source = {
  /** Path relative to the repository. */
  path: string;
  text: string;
  /** Go and template files refer to keys by accessor name as well as by literal. */
  accessors: boolean;
  /** Inside package i18n an accessor needs no selector before it. */
  bare: boolean;
  /** False for the files that keep a key alive only for themselves (Go tests, the tool's own tests and fixtures). */
  counts: boolean;
};
type Hit = { source: Source; index: number };

const QUOTED = /(["'`])([a-z][a-z0-9]*:[a-z][a-z0-9_]*(?:\.[a-z][a-z0-9_]*)*)\1/g;
const escapeRegExp = (s: string) => s.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");

/**
 * Whether the identifier `name` at `index` (after `dot`, "." or nothing) is a use of a generated accessor rather than
 * a field that happens to share its name: in a template a field chain (`.L.ServerX`); in Go a call (`.ServerX(`), a
 * method expression (`(*i18n.Localizer).ServerX`), or the argument struct (`i18n.ServerXArgs`, bare inside the package).
 */
function isAccessor(source: Source, index: number, dot: string, name: string): boolean {
  if (!source.path.endsWith(".go")) return dot === ".";
  if (name.endsWith("Args") && (dot ? source.text.slice(0, index).endsWith("i18n") : source.bare)) return true;
  if (!dot) return false;
  return source.text[index + dot.length + name.length] === "(" || source.text[index - 1] === ")";
}

function loadSources(): Source[] {
  const scan = (pattern: string) =>
    [...new Bun.Glob(pattern).scanSync({ cwd: repoRoot })].filter((p) => !p.includes("node_modules/")).sort();
  const source = (path: string, accessors: boolean, counts: boolean): Source => ({
    path,
    text: readFileSync(join(repoRoot, path), "utf8"),
    accessors,
    bare: path.startsWith("internal/i18n/"),
    counts,
  });
  const out: Source[] = [];
  for (const p of scan("{cmd,internal,pkg}/**/*.go")) {
    if (!p.endsWith("_gen.go")) out.push(source(p, true, !p.endsWith("_test.go") && !p.startsWith("internal/i18n/")));
  }
  for (const p of scan("internal/**/*.html")) out.push(source(p, true, !p.startsWith("internal/i18n/")));
  for (const p of scan("tools/i18n/src/**/*.ts")) out.push(source(p, false, false));
  const cases = "internal/i18n/testdata/cases.json";
  if (existsSync(join(repoRoot, cases))) out.push(source(cases, false, false));
  return out;
}

/** Every reference to a catalog key in the source, indexed once. */
export class References {
  private readonly sources = loadSources();
  private readonly literals = new Map<string, Hit[]>();
  private readonly accessors = new Map<string, Hit[]>();
  private readonly accessor: RegExp | undefined;

  /** `namespaces` are the catalog's; they give the accessor names their first word. */
  constructor(namespaces: string[]) {
    const prefixes = namespaces.map((ns) => goName(ns));
    this.accessor = prefixes.length
      ? new RegExp(`(\\.|(?<![A-Za-z0-9_.]))((?:${prefixes.map(escapeRegExp).join("|")})[A-Z][A-Za-z0-9_]*)`, "g")
      : undefined;
    const add = (map: Map<string, Hit[]>, name: string, hit: Hit) => {
      const hits = map.get(name);
      if (hits) hits.push(hit);
      else map.set(name, [hit]);
    };
    for (const source of this.sources) {
      for (const { 2: id = "", index } of source.text.matchAll(QUOTED)) add(this.literals, id, { source, index });
      if (!source.accessors || !this.accessor) continue;
      for (const { 1: dot = "", 2: name = "", index } of source.text.matchAll(this.accessor)) {
        if (isAccessor(source, index, dot, name)) add(this.accessors, name, { source, index });
      }
    }
  }

  private hits(id: string): Hit[] {
    const name = goName(id);
    return [
      ...(this.literals.get(id) ?? []),
      ...(this.accessors.get(name) ?? []),
      ...(this.accessors.get(`${name}Args`) ?? []),
    ];
  }

  /** Whether the code that ships refers to the key; what check.ts calls used. */
  used(id: string): boolean {
    return this.hits(id).some((h) => h.source.counts);
  }

  /** Every reference to the key, tests and fixtures included: what breaks when the key goes away. */
  of(id: string): Ref[] {
    return this.hits(id)
      .map(({ source, index }) => {
        const start = source.text.lastIndexOf("\n", index) + 1;
        const end = source.text.indexOf("\n", index);
        return {
          file: source.path,
          line: source.text.slice(0, index).split("\n").length,
          text: source.text.slice(start, end < 0 ? undefined : end).trim(),
        };
      })
      .sort((a, b) => a.file.localeCompare(b.file) || a.line - b.line);
  }

  /** The key literals of the shipping code that `defined` does not hold, for the namespaces of the catalog. */
  undefinedLiterals(namespaces: string[], defined: (id: string) => boolean): Map<string, Ref[]> {
    const out = new Map<string, Ref[]>();
    for (const [id, hits] of this.literals) {
      if (!namespaces.includes(id.split(":")[0] ?? "") || defined(id) || !hits.some((h) => h.source.counts)) continue;
      out.set(
        id,
        hits
          .filter((h) => h.source.counts)
          .map((h) => ({
            file: h.source.path,
            line: h.source.text.slice(0, h.index).split("\n").length,
            text: "",
          })),
      );
    }
    return out;
  }

  /** Whether rewrite() renames the accessors in the file. */
  rewritesAccessors(path: string): boolean {
    return this.sources.some((s) => s.accessors && s.path === path);
  }

  /** Rewrites every reference of the keys of `renames` (old id -> new id) and returns the files it changed. */
  rewrite(renames: Map<string, string>): string[] {
    const names = new Map<string, string>();
    for (const [from, to] of renames) {
      names.set(goName(from), goName(to));
      names.set(`${goName(from)}Args`, `${goName(to)}Args`);
    }
    const literal = (all: string, quote: string, id: string) => {
      const to = renames.get(id);
      return to ? `${quote}${to}${quote}` : all;
    };
    const changed: string[] = [];
    for (const source of this.sources) {
      let text = source.text.replace(QUOTED, literal);
      if (source.accessors && this.accessor) {
        text = source.text.replace(this.accessor, (all, dot: string, name: string, index: number) => {
          const to = names.get(name);
          return to && isAccessor(source, index, dot, name) ? `${dot}${to}` : all;
        });
        text = text.replace(QUOTED, literal);
      }
      if (text !== source.text && writeFileAtomic(join(repoRoot, source.path), text)) changed.push(source.path);
    }
    return changed;
  }
}
