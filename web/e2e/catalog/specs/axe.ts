// SPDX-License-Identifier: AGPL-3.0-only

// The axe rules that need a layout engine (ADR-0171 §2): what they read is the computed style and the boxes of the
// page (the contrast of a text on what is painted under it, the size of a target, a region that scrolls), which no
// DOM simulation has. The rules that read the tree alone belong to the component tests. A layout test runs them on
// the design system's stories it opens, in both themes. The check of every story (specs/stories) runs every rule of
// the tags instead, on the page as it is.

import { expect, type Page } from "@playwright/test";
import axe from "axe-core";

/** The rules of the tags of ADR-0110 §9 that read the computed style or the boxes. */
const computed = [
  "color-contrast",
  "link-in-text-block",
  "target-size",
  "scrollable-region-focusable",
  "avoid-inline-spacing",
];

/** The tags of the axe rules a page must pass (ADR-0110 §9). */
const tags = ["wcag2a", "wcag2aa", "wcag21a", "wcag21aa", "wcag22aa"];

/**
 * Where a page keeps the test's own axe once it loaded it: a key no code of the page knows. Each axe puts itself on
 * `window.axe` as its code loads, and so would the catalog's accessibility addon if a person started its check: an
 * axe taken from there may be another's, in the middle of its run, which refuses a second one.
 */
const own = `window[Symbol.for("my e2e: the test's axe")]`;

/**
 * A script that loads the test's axe on the page and keeps it, and puts back what the page had: nothing of the page
 * runs between those steps. A page loads it once, the first time a test asks it for violations: its source is half a
 * megabyte to send and to parse.
 */
const load = `(() => {
  const had = Object.getOwnPropertyDescriptor(window, "axe");
  ${axe.source};
  Object.defineProperty(window, Symbol.for("my e2e: the test's axe"), { value: window.axe });
  if (had === undefined) delete window.axe;
  else Object.defineProperty(window, "axe", had);
})()`;

/** Loads the test's axe on the page, unless the page holds it already. */
async function loaded(page: Page): Promise<void> {
  if (await page.evaluate<boolean>(`${own} !== undefined`)) return;
  await page.evaluate(load);
}

/**
 * A script that runs the test's axe, which `loaded` put on the page, and answers its violations, one line per
 * element. `context` and `options` are axe's own, as the source of an expression.
 */
const script = (context: string, options: string) => `(async () => {
  const result = await ${own}.run(${context}, { ...${options}, resultTypes: ["violations"] });
  return result.violations.flatMap((violation) =>
    violation.nodes.map(
      (node) =>
        violation.id + ": " + node.target.join(" ") + ": " +
        (node.failureSummary ?? "").replaceAll(/\\s+/g, " ").slice(0, 300),
    ),
  );
})()`;

const computedRules = script("document", JSON.stringify({ runOnly: { type: "rule", values: computed } }));

/**
 * Every rule of the tags, as the catalog's accessibility addon runs them for a person (web/storybook's preview): on
 * the page without what Storybook itself puts on it, and without `region`, since a part's story has no landmarks.
 */
const taggedRules = script(
  `{ include: document.body, exclude: [".sb-wrapper", "#storybook-docs", "#storybook-highlights-root"] }`,
  JSON.stringify({ runOnly: { type: "tag", values: tags }, rules: { region: { enabled: false } } }),
);

/** The violations of the computed rules on the page as it is, one line per element. */
const violations = async (page: Page): Promise<string[]> => {
  await loaded(page);
  return page.evaluate<string[]>(computedRules);
};

/**
 * The violations of every rule of the tags of ADR-0110 §9 on the page as it is, one line per element. Axe waits on
 * timers of its own, so a page whose clock a test holds answers only while the test moves it.
 */
export const violationsOfTags = async (page: Page): Promise<string[]> => {
  await loaded(page);
  return page.evaluate<string[]>(taggedRules);
};

/** Sets the page's color scheme as the server's shell does, on <html>. */
export async function setTheme(page: Page, theme: "light" | "dark"): Promise<void> {
  await page.evaluate((name) => {
    document.documentElement.setAttribute("data-theme", name);
  }, theme);
}

/** The page passes the computed rules of axe in the light theme and in the dark one; it is left in the light one. */
export async function expectNoComputedViolations(page: Page): Promise<void> {
  for (const theme of ["dark", "light"] as const) {
    await setTheme(page, theme);
    await expect.poll(async () => ({ theme, violations: await violations(page) })).toEqual({ theme, violations: [] });
  }
}
