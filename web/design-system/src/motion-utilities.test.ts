// SPDX-License-Identifier: AGPL-3.0-only

// Every duration and easing the design system writes is a token of @my/design-tokens (ADR-0110 §18): a class that
// moves something, and a rule or keyframe theme.ts adds (the animations, the motion of a change of screen), names its
// time and its easing as a token's custom property, never as a value of its own or as Tailwind's default. Tailwind
// compiles every class of the sources with the theme, as styles.css does, and each declaration of an animation or a
// transition is read. A unit test, without a DOM.

import { expect, test } from "bun:test";
import { readFileSync } from "node:fs";
import { properties } from "@my/design-tokens";
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
    loadModule: (id: string, base: string) =>
      Promise.resolve({ path: Bun.resolveSync(id, base), base: src, module: theme }),
  });
}

/**
 * Every word of the strings of the sources Tailwind scans (`@source "./**\/*.tsx"`), where a class is written,
 * variants and all; the comments are prose ("a view transition") and the code names (`transition` of the tokens).
 */
function candidates(): string[] {
  const found = new Set<string>();
  for (const file of new Glob("**/*.tsx").scanSync({ cwd: src })) {
    if (file.includes(".test.")) continue;
    const code = readFileSync(`${src}${file}`, "utf8")
      .replace(/\/\*[\s\S]*?\*\//g, " ")
      .replace(/(^|\s)\/\/.*$/gm, " ");
    for (const [, ...strings] of code.matchAll(/"([^"\n]*)"|'([^'\n]*)'|`([^`]*)`/g)) {
      for (const [word] of strings.join(" ").matchAll(/[^\s{}$]+/g)) found.add(word);
    }
  }
  return [...found];
}

/** The classes of the sources that move something, without their variants. */
const moving = /^(?:(?:animate|duration|ease|delay)-|transition(?:-|$))/;

const timing = /(?:^|[;{\s])((?:animation|transition)(?:-duration|-timing-function|-delay)?)\s*:\s*([^;}]+)/g;
const keywords = new Set(["linear", "ease", "ease-in", "ease-out", "ease-in-out", "step-start", "step-end"]);

/**
 * What a declaration of an animation or a transition names that is no token: a custom property that is not one of
 * the tokens', a time, or an easing written as a value (a keyword, a function), also as the fallback of a `var()`.
 */
function strays(value: string): string[] {
  const found: string[] = [];
  let rest = value.replace(/\s*!important\s*$/, "");
  const innermost = /var\(\s*(--[\w-]+)\s*(?:,([^()]*))?\)/;
  for (let m = innermost.exec(rest); m !== null; m = innermost.exec(rest)) {
    const [whole, name = "", fallback] = m;
    if (!(name in properties)) found.push(name);
    if (fallback !== undefined) found.push(...strays(fallback));
    rest = rest.replace(whole, " ");
  }
  for (const [fn] of rest.matchAll(/\b(?:cubic-bezier|steps|linear)\([^)]*\)/g)) found.push(fn);
  for (const word of rest.replace(/\b(?:cubic-bezier|steps|linear)\([^)]*\)/g, " ").split(/[\s,]+/)) {
    if (keywords.has(word) || /^-?[\d.]+m?s$/.test(word)) found.push(word);
  }
  return found;
}

test("every class of the sources that moves something generates a rule", async () => {
  // A compiler keeps what it built, so that each class is built by one of its own, against the stylesheet of none.
  const none = (await compiler()).build([]).length;
  const missing: string[] = [];
  for (const word of candidates().filter((w) => moving.test(w.slice(w.lastIndexOf(":") + 1)))) {
    if ((await compiler()).build([word]).length === none) missing.push(word);
  }
  expect(missing).toEqual([]);
});

test("every duration and easing the design system writes is a token", async () => {
  const css = (await compiler()).build(candidates());
  const declarations = [...css.matchAll(timing)].map(([, property = "", value = ""]) => ({ property, value }));
  // The animations of the theme and the motion of a change of screen are among what is read.
  expect(declarations.map((d) => d.value)).toContain(
    "my-reaction var(--voice-reaction-duration) var(--voice-reaction-ease) both",
  );
  expect(declarations.some((d) => d.value.includes("var(--transition-ease-enter)"))).toBe(true);
  const found = declarations
    .map(({ property, value }) => ({ property, value, strays: strays(value) }))
    .filter((d) => d.strays.length > 0);
  expect(found).toEqual([]);
});

test("a value that is no token is found", () => {
  expect(strays("transform var(--tw-duration, var(--default-transition-duration))")).toEqual([
    "--default-transition-duration",
    "--tw-duration",
  ]);
  expect(strays("my-reaction var(--voice-reaction-duration) ease-out both")).toEqual(["ease-out"]);
  expect(strays("150ms cubic-bezier(0.4, 0, 0.2, 1)")).toEqual(["cubic-bezier(0.4, 0, 0.2, 1)", "150ms"]);
  expect(strays("var(--transition-replace) var(--transition-ease-crossfade) both my-fade-in")).toEqual([]);
});
