// SPDX-License-Identifier: AGPL-3.0-only

// Fails on what sets a physical side in the TypeScript and CSS of the web packages, and names the logical one that
// follows the reading direction, so that a part lays out right-to-left under an I18nProvider of an RTL locale
// (ADR-0009 §1, ADR-0025 §1): a Tailwind utility (ml-, pl-, left-, text-left, rounded-l, border-r…), a CSS
// declaration (margin-left, left, border-right-width, border-top-left-radius, text-align: left, float: right…), and
// the same keys of a React style object (marginLeft, textAlign: "left", and left or right in style={{…}}). It reads
// the code of a file without its comments; tests may name what it refuses.
//
//   bun src/direction.ts <directory or file>...

import { readdirSync, readFileSync, statSync } from "node:fs";
import { extname, join, relative } from "node:path";

const side = (s: string) => (s === "l" || s === "left" ? "s" : "e");
const edge = (s: string) => (s === "left" || s === "l" ? "start" : "end");

// Each physical utility (after its variants and its ! and - prefixes) and its logical replacement.
const rules: [RegExp, (m: RegExpExecArray) => string][] = [
  [/^(scroll-)?([mp])([lr])-(.+)$/, (m) => `${m[1] ?? ""}${m[2] ?? ""}${side(m[3] ?? "")}-${m[4] ?? ""}`],
  [/^(left|right)-(.+)$/, (m) => `inset-${side(m[1] ?? "")}-${m[2] ?? ""}`],
  [/^text-(left|right)$/, (m) => `text-${edge(m[1] ?? "")}`],
  [/^(float|clear)-(left|right)$/, (m) => `${m[1] ?? ""}-${edge(m[2] ?? "")}`],
  [/^rounded-([lr])(-.+)?$/, (m) => `rounded-${side(m[1] ?? "")}${m[2] ?? ""}`],
  [/^rounded-([tb])([lr])(-.+)?$/, (m) => `rounded-${m[1] === "t" ? "s" : "e"}${side(m[2] ?? "")}${m[3] ?? ""}`],
  [/^border-([lr])(-.+)?$/, (m) => `border-${side(m[1] ?? "")}${m[2] ?? ""}`],
];

/** The utility of a class (what follows its last variant, outside brackets), and where it starts. */
function utilityOf(token: string): number {
  let depth = 0;
  let start = 0;
  for (let i = 0; i < token.length; i++) {
    const c = token[i];
    if (c === "[" || c === "(") depth++;
    else if (c === "]" || c === ")") depth--;
    else if (c === ":" && depth === 0) start = i + 1;
  }
  return start;
}

/** The logical class for a class that sets a physical side, or null. */
export function logicalOf(token: string): string | null {
  const start = utilityOf(token);
  const prefix = /^!?-?/.exec(token.slice(start))?.[0] ?? "";
  const utility = token.slice(start + prefix.length).replace(/!$/, "");
  for (const [pattern, logical] of rules) {
    const m = pattern.exec(utility);
    if (m)
      return token.slice(0, start + prefix.length) + logical(m) + token.slice(start + prefix.length + utility.length);
  }
  return null;
}

const transpiler = new Bun.Transpiler({ loader: "tsx" });

const inline = (s: string | undefined) => (s === "left" ? "start" : "end");

// Each physical CSS property, in its kebab-case name, and its logical one.
const properties: [RegExp, (m: RegExpExecArray) => string][] = [
  [/^(margin|padding|scroll-margin|scroll-padding)-(left|right)$/, (m) => `${m[1] ?? ""}-inline-${inline(m[2])}`],
  [/^(left|right)$/, (m) => `inset-inline-${inline(m[1])}`],
  [/^border-(left|right)(-width|-color|-style)?$/, (m) => `border-inline-${inline(m[1])}${m[2] ?? ""}`],
  [
    /^border-(top|bottom)-(left|right)-radius$/,
    (m) => `border-${m[1] === "top" ? "start" : "end"}-${inline(m[2])}-radius`,
  ],
];
// The properties whose physical side is in their value, and the logical value of each side.
const values: Record<string, (side: string) => string> = {
  "text-align": (s) => inline(s),
  float: (s) => `inline-${inline(s)}`,
  clear: (s) => `inline-${inline(s)}`,
};

/** The logical declaration for a CSS declaration that sets a physical side, or null; names are kebab-case. */
export function logicalDeclaration(property: string, value: string): { physical: string; logical: string } | null {
  const name = property.toLowerCase();
  for (const [pattern, logical] of properties) {
    const m = pattern.exec(name);
    if (m) return { physical: name, logical: logical(m) };
  }
  const side = /^(left|right)$/i.exec(value.replace(/!important$/i, "").trim())?.[1]?.toLowerCase();
  const to = values[name];
  return side !== undefined && to ? { physical: `${name}: ${side}`, logical: `${name}: ${to(side)}` } : null;
}

const kebab = (s: string) => s.replace(/[A-Z]/g, (c) => `-${c.toLowerCase()}`);
const camel = (s: string) => s.replace(/-([a-z])/g, (_, c: string) => c.toUpperCase());

export interface Finding {
  file: string;
  line: number;
  physical: string;
  logical: string;
}

const lineAt = (source: string, at: number) => (at < 0 ? 0 : source.slice(0, at).split("\n").length);

/** A CSS file's physical utilities (in @apply and the like) and its physical declarations, each where it is. */
function findInCSS(file: string, source: string): Finding[] {
  // The comments become blanks of their length, so that an offset in the code is one in the source.
  const code = source.replace(/\/\*[\s\S]*?\*\//g, (c) => c.replace(/[^\n]/g, " "));
  const found: Finding[] = [];
  for (const m of code.matchAll(/(\S+)/g)) {
    const token = (m[1] ?? "").replace(/^["'`{(]+|["'`,;{})]+$/g, "");
    const logical = logicalOf(token);
    if (logical !== null) found.push({ file, line: lineAt(code, m.index), physical: token, logical });
  }
  for (const m of code.matchAll(/([{;]\s*)(-?[a-zA-Z-]+)\s*:\s*([^;{}]*)(?=[;}])/g)) {
    const d = logicalDeclaration(m[2] ?? "", m[3] ?? "");
    if (d) found.push({ file, line: lineAt(code, m.index + (m[1] ?? "").length), ...d });
  }
  return found.sort((a, b) => a.line - b.line);
}

// The spans of the style objects written in place (style={{…}}) in transpiled code, where `left` and `right` are
// sides; elsewhere they may be the coordinates of a box.
function styleSpans(code: string): [number, number][] {
  return Array.from(code.matchAll(/\bstyle:\s*\{/g), (m) => {
    let depth = 0;
    for (let i = m.index + m[0].length - 1; i < code.length; i++) {
      if (code[i] === "{") depth++;
      else if (code[i] === "}" && --depth === 0) return [m.index, i] as [number, number];
    }
    return [m.index, code.length] as [number, number];
  });
}

/**
 * A TypeScript file's physical utilities, and the physical keys of its React style objects: a camel-case key that
 * names a side (marginLeft, borderTopRightRadius) anywhere, textAlign, float or clear set to "left" or "right", and
 * left or right in a style object written in place. Each is named once, at the line of its first use.
 */
function findInTS(file: string, source: string): Finding[] {
  const code = transpiler.transformSync(source);
  const seen = new Set<string>();
  const found: Finding[] = [];
  const add = (physical: string, logical: string, at: number) => {
    if (seen.has(physical)) return;
    seen.add(physical);
    found.push({ file, line: lineAt(source, at), physical, logical });
  };
  for (const token of code.split(/[\s"'`,;{}]+/)) {
    const logical = token === "" ? null : logicalOf(token);
    if (logical !== null) add(token, logical, source.indexOf(token));
  }
  const spans = styleSpans(code);
  for (const m of code.matchAll(/(?<![\w$.])([a-zA-Z]+):\s*(?:"([^"]*)"|'([^']*)')?/g)) {
    const key = m[1] ?? "";
    if ((key === "left" || key === "right") && !spans.some(([a, b]) => m.index > a && m.index < b)) continue;
    if (key !== "left" && key !== "right" && !/[A-Z]/.test(key) && !(kebab(key) in values)) continue;
    const d = logicalDeclaration(kebab(key), m[2] ?? m[3] ?? "");
    if (!d) continue;
    const physical = d.physical.includes(":") ? `${key}: "${m[2] ?? m[3] ?? ""}"` : key;
    const logical = d.logical.includes(":") ? `${key}: "${d.logical.split(": ")[1] ?? ""}"` : camel(d.logical);
    const inStyle = new RegExp(`style=\\{\\{[^}]*?\\b${key}\\s*:`).exec(source);
    const side = key === "left" || key === "right";
    add(
      physical,
      logical,
      side && inStyle ? inStyle.index + inStyle[0].lastIndexOf(key) : source.search(new RegExp(`\\b${key}\\s*:`)),
    );
  }
  return found;
}

/** The physical utilities, declarations and style keys of one file's source. */
export const findIn = (file: string, source: string): Finding[] =>
  extname(file) === ".css" ? findInCSS(file, source) : findInTS(file, source);

const checked = new Set([".ts", ".tsx", ".css"]);
const skipped = new Set(["node_modules", "dist", ".astro"]);

function files(path: string): string[] {
  if (!statSync(path).isDirectory()) return [path];
  return readdirSync(path, { withFileTypes: true }).flatMap((e) => {
    if (skipped.has(e.name)) return [];
    const p = join(path, e.name);
    if (e.isDirectory()) return files(p);
    return checked.has(extname(e.name)) && !/\.test\.tsx?$/.test(e.name) ? [p] : [];
  });
}

/** What sets a physical side in the TypeScript and CSS under `paths`, tests and builds aside. */
export const findPhysical = (paths: string[]) =>
  paths.flatMap(files).flatMap((f) => findIn(relative(process.cwd(), f), readFileSync(f, "utf8")));

if (import.meta.main) {
  const paths = process.argv.slice(2);
  if (paths.length === 0) {
    console.error("usage: bun src/direction.ts <directory or file>...");
    process.exit(2);
  }
  const found = findPhysical(paths);
  for (const f of found)
    console.error(
      `${f.file}:${f.line}: ${f.physical} sets a physical side; use ${f.logical}, which follows the reading direction`,
    );
  if (found.length > 0) process.exit(1);
  console.log(`no physical side in ${paths.join(", ")}`);
}
