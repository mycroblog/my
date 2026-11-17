// SPDX-License-Identifier: AGPL-3.0-only

// The axe rules that need a layout engine (ADR-0171 §2): what they read is the computed style and the boxes of the
// page (the contrast of a text on what is painted under it, the size of a target, a region that scrolls), which no
// DOM simulation has. The rules that read the tree alone belong to the component tests. A layout test runs them on
// the design system's stories it opens, in both themes.

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

/**
 * Runs the computed rules with an axe of the test's own, and answers its violations, one line per element. The
 * catalog's accessibility addon runs an axe of its own after a story is drawn, and each axe puts itself on
 * `window.axe` as its code loads: an axe taken from there a moment later may be the addon's, in the middle of its
 * run, which refuses a second one. So one script loads the test's axe, keeps it, puts back what the page had, and
 * runs it: nothing of the page runs between those steps.
 */
const run = `(async () => {
  const had = Object.getOwnPropertyDescriptor(window, "axe");
  ${axe.source};
  const own = window.axe;
  if (had === undefined) delete window.axe;
  else Object.defineProperty(window, "axe", had);
  const result = await own.run(document, {
    runOnly: { type: "rule", values: ${JSON.stringify(computed)} },
    resultTypes: ["violations"],
  });
  return result.violations.flatMap((violation) =>
    violation.nodes.map(
      (node) =>
        violation.id + ": " + node.target.join(" ") + ": " +
        (node.failureSummary ?? "").replaceAll(/\\s+/g, " ").slice(0, 300),
    ),
  );
})()`;

/** The violations of the computed rules on the page as it is, one line per element. */
const violations = (page: Page): Promise<string[]> => page.evaluate<string[]>(run);

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
