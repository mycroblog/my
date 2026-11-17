// SPDX-License-Identifier: AGPL-3.0-only

// What a colour outside the tokens looks like in a text (ADR-0171 §2), for restrict/token-colors of styles.js, which
// reads the strings of the TypeScript, and for the CSS files, which Oxlint does not read and this checks itself: a
// hex colour, a colour function with channels of its own, and a named colour as the value of a declaration.
// `transparent`, `currentColor`, the CSS-wide keywords and a `color-mix()` of tokens are not colours of their own.
// The tokens are defined in web/design-tokens and in the design system's theme (theme.ts), which no one passes here.
//
//   bun src/colors.js <directory or CSS file>...

import { readdirSync, readFileSync, statSync } from "node:fs";
import { extname, join, relative } from "node:path";

// A fragment of a URL (`/page#add`) and a character reference (`&#8203;`) are not colours;
// what a `_` joins is a value of an arbitrary Tailwind class (`shadow-[0_0_0_1px_#ccc]`).
const HEX = /(?<![a-z0-9&#/.-])#(?:[0-9a-f]{3,4}|[0-9a-f]{6}|[0-9a-f]{8})(?![a-z0-9-])/gi;
const FUNCTION = /(?<![a-z0-9-])(rgba?|hsla?|hwb|lab|lch|oklab|oklch|color)\(/gi;

/** The named colours of CSS Color 4. */
const NAMED = new Set(
  (
    "aliceblue antiquewhite aqua aquamarine azure beige bisque black blanchedalmond blue blueviolet brown burlywood " +
    "cadetblue chartreuse chocolate coral cornflowerblue cornsilk crimson cyan darkblue darkcyan darkgoldenrod darkgray " +
    "darkgreen darkgrey darkkhaki darkmagenta darkolivegreen darkorange darkorchid darkred darksalmon darkseagreen " +
    "darkslateblue darkslategray darkslategrey darkturquoise darkviolet deeppink deepskyblue dimgray dimgrey dodgerblue " +
    "firebrick floralwhite forestgreen fuchsia gainsboro ghostwhite gold goldenrod gray green greenyellow grey honeydew " +
    "hotpink indianred indigo ivory khaki lavender lavenderblush lawngreen lemonchiffon lightblue lightcoral lightcyan " +
    "lightgoldenrodyellow lightgray lightgreen lightgrey lightpink lightsalmon lightseagreen lightskyblue lightslategray " +
    "lightslategrey lightsteelblue lightyellow lime limegreen linen magenta maroon mediumaquamarine mediumblue " +
    "mediumorchid mediumpurple mediumseagreen mediumslateblue mediumspringgreen mediumturquoise mediumvioletred " +
    "midnightblue mintcream mistyrose moccasin navajowhite navy oldlace olive olivedrab orange orangered orchid " +
    "palegoldenrod palegreen paleturquoise palevioletred papayawhip peachpuff peru pink plum powderblue purple " +
    "rebeccapurple red rosybrown royalblue saddlebrown salmon sandybrown seagreen seashell sienna silver skyblue " +
    "slateblue slategray slategrey snow springgreen steelblue tan teal thistle tomato turquoise violet wheat white " +
    "whitesmoke yellow yellowgreen"
  ).split(" "),
);

/** The colour utilities of Tailwind, whose arbitrary value is a colour, and an arbitrary property of a colour. */
const ARBITRARY =
  /(?:(?:bg|text|border(?:-[xytbse])?|outline|ring|ring-offset|inset-ring|fill|stroke|shadow|inset-shadow|drop-shadow|text-shadow|decoration|accent|caret|divide|from|via|to|placeholder)-\[(?:color:)?|\[(?:[a-z-]*color|background|fill|stroke):)([a-z]+)\]/gi;

/** The hex colours, the colour functions and the named colours of arbitrary Tailwind values in a text. */
export function rawColors(/** @type {string} */ text) {
  const found = [...(text.match(HEX) ?? []), ...Array.from(text.matchAll(FUNCTION), (m) => `${m[1] ?? ""}()`)];
  for (const m of text.matchAll(ARBITRARY)) if (NAMED.has((m[1] ?? "").toLowerCase())) found.push(m[1] ?? "");
  return found;
}

/** The named colours among the words of a value that is a colour's (`1px solid red`, `white`). */
export function namedColor(/** @type {string} */ value) {
  // The words of a var() or a url() are names, not colours.
  const words = value.replace(/(?:var|url|env)\([^()]*\)/gi, " ").match(/(?<![\w-])[a-z]+(?![\w(-])/gi) ?? [];
  return words.filter((word) => NAMED.has(word.toLowerCase()));
}

/** The CSS properties whose value holds a colour, a custom property among them. */
const COLOR_PROPERTY =
  /^(?:--.*|(?:[a-z-]+-)?color|background(?:-image)?|border(?:-(?:top|bottom|left|right|inline|block|inline-start|inline-end|block-start|block-end))?|outline|fill|stroke|box-shadow|text-shadow|text-decoration|column-rule|filter|scrollbar-color)$/;

/**
 * The colours outside the tokens of a CSS file: of each declaration, the hex colours and colour functions of its
 * value, and its named colours when the property takes a colour. Its comments are not read.
 * @returns {{ file: string, line: number, color: string }[]}
 */
export function colorsInCSS(/** @type {string} */ file, /** @type {string} */ source) {
  // The comments become blanks of their length, so that an offset in the code is one in the source.
  const code = source.replace(/\/\*[\s\S]*?\*\//g, (c) => c.replace(/[^\n]/g, " "));
  const found = [];
  for (const m of code.matchAll(/(?<=^|[{;]\s*)(-{0,2}[a-zA-Z][\w-]*)\s*:\s*([^;{}]*)(?=[;}])/gm)) {
    const [, property = "", value = ""] = m;
    const line = code.slice(0, m.index).split("\n").length;
    const colors = [...rawColors(value), ...(COLOR_PROPERTY.test(property.toLowerCase()) ? namedColor(value) : [])];
    for (const color of colors) found.push({ file, line, color });
  }
  return found;
}

const skipped = new Set(["node_modules", "dist", ".astro"]);

/** The CSS files under a path. */
function files(/** @type {string} */ path) {
  if (!statSync(path).isDirectory()) return [path];
  return readdirSync(path, { withFileTypes: true }).flatMap((/** @type {import("node:fs").Dirent} */ e) => {
    if (skipped.has(e.name)) return [];
    const p = join(path, e.name);
    if (e.isDirectory()) return files(p);
    return extname(e.name) === ".css" ? [p] : [];
  });
}

if (import.meta.main) {
  const paths = process.argv.slice(2);
  if (paths.length === 0) {
    console.error("usage: bun src/colors.js <directory or CSS file>...");
    process.exit(2);
  }
  const found = paths.flatMap(files).flatMap((f) => colorsInCSS(relative(process.cwd(), f), readFileSync(f, "utf8")));
  for (const f of found)
    console.error(
      `${f.file}:${f.line}: ${f.color} is a colour outside the tokens; use a token of @my/design-tokens ` +
        "(var(--color-surface), var(--color-text)…), currentColor or transparent, or add the token it lacks",
    );
  if (found.length > 0) process.exit(1);
  console.log(`no colour outside the tokens in the CSS of ${paths.join(", ")}`);
}
