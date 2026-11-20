// SPDX-License-Identifier: AGPL-3.0-only

// The font utilities the design system writes (the only package that writes classes, ADR-0025 §4) each generate a
// rule from the theme of theme.ts: a weight or a family the theme does not have generates nothing, and the text keeps
// the one it inherits. Tailwind compiles the utilities of styles.css with the theme, and every `font-*` class of the
// sources is built on its own. A unit test, without a DOM.

import { expect, test } from "bun:test";
import { readFileSync } from "node:fs";
import { Glob } from "bun";
import { compile } from "tailwindcss";
import theme from "./theme";

const src = new URL(".", import.meta.url).pathname;

/** The compiler of the utilities of styles.css, with the theme of theme.ts as its plugin. */
async function compiler() {
  const css = `@import "tailwindcss/utilities.css" source(none);\n@plugin "./theme.ts";`;
  return compile(css, {
    base: src,
    loadStylesheet: (id: string, base: string) => {
      const path = Bun.resolveSync(id, base);
      return Promise.resolve({ path, base: src, content: readFileSync(path, "utf8") });
    },
    // The one module styles.css loads is its plugin, theme.ts.
    loadModule: (id: string, base: string) =>
      Promise.resolve({ path: Bun.resolveSync(id, base), base: src, module: theme }),
  });
}

/**
 * The `font-*` classes written in the sources (the files Tailwind scans, `@source "./**\/*.tsx"`), without their
 * variants: a class stands between a quote, a backtick or a space and the next one.
 */
function fontClasses(): string[] {
  const found = new Set<string>();
  for (const file of new Glob("**/*.tsx").scanSync({ cwd: src })) {
    const text = readFileSync(`${src}${file}`, "utf8");
    for (const [, name = ""] of text.matchAll(/(?<=["'`\s:])(font-[a-z][a-z-]*)(?=["'`\s])/g)) found.add(name);
  }
  return [...found].toSorted();
}

test("the sources write the font utilities of the type scale", () => {
  const classes = fontClasses();
  expect(["font-label", "font-normal", "font-strong"].filter((name) => !classes.includes(name))).toEqual([]);
});

test("every font utility the sources write generates a rule", async () => {
  const classes = fontClasses();
  const missing: string[] = [];
  for (const name of classes) {
    const css = (await compiler()).build([name]);
    if (!css.includes(`.${name} {`)) missing.push(name);
  }
  expect(missing).toEqual([]);
});
