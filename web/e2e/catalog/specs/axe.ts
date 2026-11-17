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

/** The violations of the computed rules on the page as it is, one line per element. */
async function violations(page: Page): Promise<string[]> {
  await page.evaluate(`${axe.source}; undefined`);
  return page.evaluate(async (rules) => {
    const run = (
      globalThis as unknown as {
        axe: {
          run: (
            context: Document,
            options: unknown,
          ) => Promise<{ violations: { id: string; nodes: { target: unknown[]; failureSummary?: string }[] }[] }>;
        };
      }
    ).axe.run;
    const result = await run(document, { runOnly: { type: "rule", values: rules } });
    return result.violations.flatMap((violation) =>
      violation.nodes.map(
        (node) =>
          `${violation.id}: ${node.target.join(" ")}: ${(node.failureSummary ?? "").replaceAll(/\s+/g, " ").slice(0, 300)}`,
      ),
    );
  }, computed);
}

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
