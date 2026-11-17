// SPDX-License-Identifier: AGPL-3.0-only

// Fails on what sets a physical side in the TypeScript and CSS of the web packages, and names the logical one that
// follows the reading direction, so that a part lays out right-to-left under an I18nProvider of an RTL locale
// (ADR-0009 §1, ADR-0025 §1): a Tailwind utility (ml-, pl-, left-, text-left, rounded-l, border-r…, and an arbitrary
// property such as [margin-left:0]), a CSS declaration (margin-left, left, border-right-width,
// border-top-left-radius, text-align: left, float: right…), and the same keys of a React style object (marginLeft,
// textAlign: "left", and left or right in a style object: one written in place, one the `style` names, and one typed
// CSSProperties; elsewhere left and right may be the coordinates of a box).
//
// A utility that sets a side and has no logical form (a position: bg-left, object-right, origin-top-left; a gradient
// or a mask toward a side: bg-linear-to-r, mask-l-from-50%; touch-pan-left; and a shift along the line, translate-x-)
// fails unless it stands under `ltr:` or `rtl:`, where the other direction is written beside it: a shift that is the
// same in both directions is written twice. translateX() in a value fails unless it turns with the page, by
// `var(--my-direction)` (1, and -1 in a right-to-left page: motion-css.ts). A shift of 0 moves nothing. A branch on the
// direction in TypeScript (`direction === "rtl"`) is how a part mirrors what CSS cannot.
//
// It reads the code of a file without its comments; tests may name what it refuses. It does not see a style object
// that comes from another file, a side assigned to `element.style`, or the `x` of a motion library.
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

const swap = (s: string) => s.replace(/left|right/g, (m) => (m === "left" ? "right" : "left"));
const flip = (s: string | undefined) => (s === "l" ? "r" : "l");

// Each utility that sets a physical side and has no logical form, and the one of the other side.
const sided: [RegExp, (m: RegExpExecArray) => string][] = [
  [
    /^(bg|object|origin)-((?:top-|bottom-)?(?:left|right)|(?:left|right)-(?:top|bottom))$/,
    (m) => `${m[1] ?? ""}-${swap(m[2] ?? "")}`,
  ],
  [/^(bg-(?:linear|gradient)-to)-([tb]?)([lr])$/, (m) => `${m[1] ?? ""}-${m[2] ?? ""}${flip(m[3])}`],
  [/^mask-([lr])(-.+)$/, (m) => `mask-${flip(m[1])}${m[2] ?? ""}`],
  [/^touch-pan-(left|right)$/, (m) => `touch-pan-${swap(m[1] ?? "")}`],
];

/** Whether a shift along the x axis moves nothing, or turns with the page's direction. */
const turns = (shift: string) => /^0(?:px|%|rem)?$/.test(shift.trim()) || shift.includes("--my-direction");

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
  const rest = token.slice(start + prefix.length + utility.length);
  for (const [pattern, logical] of rules) {
    const m = pattern.exec(utility);
    if (m) return token.slice(0, start + prefix.length) + logical(m) + rest;
  }
  // An arbitrary property: [margin-left:0], [text-align:left].
  const [, property, value] = /^\[([a-z-]+):(.+)\]$/.exec(utility) ?? [];
  const d = property === undefined ? null : logicalDeclaration(property, (value ?? "").replaceAll("_", " "));
  if (!d) return null;
  const declared = d.logical.includes(": ") ? d.logical.replace(": ", ":") : `${d.logical}:${value ?? ""}`;
  return `${token.slice(0, start + prefix.length)}[${declared}]${rest}`;
}

/**
 * For a class that sets a physical side and has no logical form, the two classes that say both directions (`bg-left`
 * as `ltr:bg-left rtl:bg-right`), or null: for a class under `ltr:` or `rtl:`, and for a shift that moves nothing or
 * turns with the page.
 */
export function bothDirections(token: string): string | null {
  const start = utilityOf(token);
  const variants = token.slice(0, start);
  if (/(?:^|:)(?:ltr|rtl):/.test(variants)) return null;
  const prefix = /^!?-?/.exec(token.slice(start))?.[0] ?? "";
  const utility = token.slice(start + prefix.length).replace(/!$/, "");
  const rest = token.slice(start + prefix.length + utility.length);
  const both = (ltr: string, rtl: string) => `${variants}ltr:${ltr}${rest} ${variants}rtl:${rtl}${rest}`;
  const shift = /^translate-x-(.+)$/.exec(utility);
  if (shift) {
    if (turns(shift[1] ?? "")) return null;
    const sign = prefix.replace("!", "");
    return both(prefix + utility, prefix.replace("-", "") + (sign === "-" ? "" : "-") + utility);
  }
  for (const [pattern, other] of sided) {
    const m = pattern.exec(utility);
    if (m) return both(prefix + utility, prefix + other(m));
  }
  return null;
}

/** The class or classes to write for a class that sets a physical side, or null. */
const directed = (token: string) => logicalOf(token) ?? bothDirections(token);

// The function of a transform that shifts along the x axis, named apart from its parenthesis, so that this file names
// no shift of its own.
const shiftX = "translateX";

/** Each translateX() of `code` that does not turn with the page, where it is, and the one that does. */
function shifts(code: string): { at: number; physical: string; logical: string }[] {
  return Array.from(code.matchAll(new RegExp(`\\b${shiftX}\\(`, "g")), (m) => {
    let depth = 0;
    let end = m.index + m[0].length - 1;
    for (; end < code.length; end++) {
      if (code[end] === "(") depth++;
      else if (code[end] === ")" && --depth === 0) break;
    }
    const shift = code.slice(m.index + m[0].length, end);
    return turns(shift)
      ? []
      : [
          {
            at: m.index,
            physical: `${shiftX}(${shift})`,
            logical: `${shiftX}(calc(${shift} * var(--my-direction)))`,
          },
        ];
  }).flat();
}

// A .ts file is read as TypeScript, not TSX, where a generic arrow function (`<T>(`) would read as a tag.
const transpilers = { ts: new Bun.Transpiler({ loader: "ts" }), tsx: new Bun.Transpiler({ loader: "tsx" }) };

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
    const logical = directed(token);
    if (logical !== null) found.push({ file, line: lineAt(code, m.index), physical: token, logical });
  }
  for (const { at, ...shift } of shifts(code)) found.push({ file, line: lineAt(code, at), ...shift });
  for (const m of code.matchAll(/([{;]\s*)(-?[a-zA-Z-]+)\s*:\s*([^;{}]*)(?=[;}])/g)) {
    const d = logicalDeclaration(m[2] ?? "", m[3] ?? "");
    if (d) found.push({ file, line: lineAt(code, m.index + (m[1] ?? "").length), ...d });
  }
  return found.sort((a, b) => a.line - b.line);
}

/** The end of the object that opens at `open` in `code`: its closing brace. */
function objectEnd(code: string, open: number): number {
  let depth = 0;
  for (let i = open; i < code.length; i++) {
    if (code[i] === "{") depth++;
    else if (code[i] === "}" && --depth === 0) return i;
  }
  return code.length;
}

/** The end of the value of a property that starts at `start` in `code`: the comma or the brace that closes it. */
function valueEnd(code: string, start: number): number {
  let depth = 0;
  for (let i = start; i < code.length; i++) {
    const c = code[i];
    if (c === "{" || c === "(" || c === "[") depth++;
    else if (c === "}" || c === ")" || c === "]") {
      if (depth === 0) return i;
      depth--;
    } else if (c === "," && depth === 0) return i;
  }
  return code.length;
}

// The spans of the style objects of transpiled code, where `left` and `right` are sides; elsewhere they may be the
// coordinates of a box. A style object is the value of a `style` (style={…}), with the objects of the names it reads
// whole (not a member of one, such as `box.left`), and the object of a name the source types CSSProperties.
function styleSpans(code: string, source: string): { spans: [number, number][]; names: string[] } {
  const spans: [number, number][] = [];
  const named = new Set<string>();
  /** Adds the span from `start` to `end`, and the objects of the names read whole in it. */
  const add = (start: number, end: number) => {
    spans.push([start, end]);
    // A name read whole: not a key (`left:`), unless it follows a `?`; no member, call or index of it.
    for (const m of code
      .slice(start, end)
      .matchAll(/(\?\s*)?(?<![\w$.])([A-Za-z_$][\w$]*)(?![\w$]*\s*[.([])(\s*:)?/g)) {
      const name = m[2] ?? "";
      if ((m[3] !== undefined && m[1] === undefined) || named.has(name)) continue;
      named.add(name);
      declared(name);
    }
  };
  /** Adds the object that the name is declared as. */
  const declared = (name: string) => {
    const at = new RegExp(`\\b(?:const|let|var)\\s+${name.replaceAll("$", "\\$")}\\s*=\\s*\\{`).exec(code);
    if (at) add(at.index + at[0].length - 1, objectEnd(code, at.index + at[0].length - 1));
  };
  for (const m of code.matchAll(/\bstyle:\s*/g)) add(m.index + m[0].length, valueEnd(code, m.index + m[0].length));
  for (const m of source.matchAll(/\b(?:const|let|var)\s+([A-Za-z_$][\w$]*)\s*:\s*(?:React\.)?CSSProperties\b/g))
    if (!named.has(m[1] ?? "")) {
      named.add(m[1] ?? "");
      declared(m[1] ?? "");
    }
  return { spans, names: [...named] };
}

/**
 * A TypeScript file's physical utilities and shifts, and the physical keys of its React style objects: a camel-case
 * key that names a side (marginLeft, borderTopRightRadius) anywhere, textAlign, float or clear set to "left" or
 * "right", and left or right in a style object (styleSpans). Each is named once, at the line of its first use.
 */
function findInTS(file: string, source: string): Finding[] {
  const code = transpilers[file.endsWith(".tsx") ? "tsx" : "ts"].transformSync(source);
  const seen = new Set<string>();
  const found: Finding[] = [];
  const add = (physical: string, logical: string, at: number) => {
    if (seen.has(physical)) return;
    seen.add(physical);
    found.push({ file, line: lineAt(source, at), physical, logical });
  };
  for (const token of code.split(/[\s"'`,;{}]+/)) {
    const logical = token === "" ? null : directed(token);
    if (logical !== null) add(token, logical, source.indexOf(token));
  }
  for (const shift of shifts(code)) add(shift.physical, shift.logical, source.indexOf(`${shiftX}(`));
  const { spans, names } = styleSpans(code, source);
  /** Where a style object of the source sets the side `key`: one written in place, then one a `style` names. */
  const sideAt = (key: string) => {
    const inPlace = new RegExp(`style=\\{\\{[^}]*?\\b${key}\\s*:`).exec(source);
    if (inPlace) return inPlace.index + inPlace[0].lastIndexOf(key);
    for (const name of names) {
      const at = new RegExp(
        `\\b(?:const|let|var)\\s+${name.replaceAll("$", "\\$")}\\b[^={]*=\\s*\\{[^}]*?\\b${key}\\s*:`,
      ).exec(source);
      if (at) return at.index + at[0].lastIndexOf(key);
    }
    return source.search(new RegExp(`\\b${key}\\s*:`));
  };
  for (const m of code.matchAll(/(?<![\w$.])([a-zA-Z]+):\s*(?:"([^"]*)"|'([^']*)')?/g)) {
    const key = m[1] ?? "";
    if ((key === "left" || key === "right") && !spans.some(([a, b]) => m.index >= a && m.index < b)) continue;
    if (key !== "left" && key !== "right" && !/[A-Z]/.test(key) && !(kebab(key) in values)) continue;
    const d = logicalDeclaration(kebab(key), m[2] ?? m[3] ?? "");
    if (!d) continue;
    const physical = d.physical.includes(":") ? `${key}: "${m[2] ?? m[3] ?? ""}"` : key;
    const logical = d.logical.includes(":") ? `${key}: "${d.logical.split(": ")[1] ?? ""}"` : camel(d.logical);
    const side = key === "left" || key === "right";
    add(physical, logical, side ? sideAt(key) : source.search(new RegExp(`\\b${key}\\s*:`)));
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
